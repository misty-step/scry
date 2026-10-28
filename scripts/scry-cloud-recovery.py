#!/usr/bin/env python3
"""Bounded Cloudflare-only recovery driver; CLI itself prints an offline plan.

Call admit() *before* a billable list/request, schedule change, temporary secret,
resource or preview is created. A bounded bridge alone is not billing admission.
run() requires explicit reviewed approval, exact prepared source and the native
production lock.

Pricing checked 2026-10-27:
https://developers.cloudflare.com/r2/pricing/#r2-pricing
https://developers.cloudflare.com/durable-objects/platform/pricing/#compute-billing
Both meters round billable usage up. A single new R2 Class A billing unit can
cost $4.50; a single new DO duration unit can cost $12.50. We cannot assume free
allocation or already-paid headroom, or promise a $1 ceiling from prorated cost.
The $17 combined exposure below is NOT a full upper bound: requests, CPU, SQL,
storage, retries, cleanup and retained resources must also be bounded/accounted.
"""
from datetime import datetime, timezone
import email.parser
import email.policy
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import sys
import tarfile
import time
import urllib.error
import urllib.parse
import urllib.request


class RecoveryBlocked(RuntimeError):
    """Fixed, content-free admission refusal, safe for native workflow output."""


FIXED_REASONS = frozenset({
    'INCOMPLETE_RECOVERY_METER_BOUND', 'INVALID_PRIVATE_PREVIEW_HOST', 'INVALID_RECOVERY_CLAIM_BUCKET',
    'INVALID_RECOVERY_METER_BOUND', 'INVALID_RECOVERY_SOURCE_PIN', 'INVALID_UPLOAD_MODULE',
    'PRIVATE_PROOF_RESPONSE_OVERSIZE', 'PRODUCTION_HEALTH_SERVICE_APPROVAL_REQUIRED', 'RECOVERY_APPROVAL_ALREADY_CLAIMED',
    'RECOVERY_APPROVAL_REQUIRED', 'RECOVERY_ARTIFACT_CHECKSUM_MISMATCH', 'RECOVERY_ARTIFACT_NOT_COMMITTED',
    'RECOVERY_BILLING_PERIOD_AMBIGUOUS', 'RECOVERY_BILLING_PERIOD_BOUND_UNPROVEN', 'RECOVERY_BILLING_PERIOD_INCOMPLETE',
    'RECOVERY_BILLING_PERIOD_REQUIRED', 'RECOVERY_BILLING_PERIOD_UNAVAILABLE', 'RECOVERY_BILLING_PERIOD_UNVERIFIED',
    'RECOVERY_BINDINGS_UNVERIFIED', 'RECOVERY_BINDING_VALUE_UNVERIFIED', 'RECOVERY_BUCKET_ALREADY_EXISTS',
    'RECOVERY_BUCKET_CLEANUP_UNVERIFIED', 'RECOVERY_BUCKET_CREATION_UNVERIFIED', 'RECOVERY_BUCKET_NOT_PRIVATE',
    'RECOVERY_BUCKET_READBACK_MISMATCH', 'RECOVERY_CLAIM_CLEANUP_IDENTITY_CHANGED', 'RECOVERY_CLAIM_CLEANUP_RECEIPT_INVALID',
    'RECOVERY_CLAIM_CLEANUP_UNCERTAIN', 'RECOVERY_CLAIM_CLEANUP_UNVERIFIED', 'RECOVERY_CLAIM_OUTCOME_UNCERTAIN',
    'RECOVERY_CLAIM_OVERSIZE', 'RECOVERY_CLAIM_READBACK_MISMATCH', 'RECOVERY_CLAIM_TRANSPORT_UNAVAILABLE',
    'RECOVERY_CLEANUP_BUCKET_IDENTITY_CHANGED', 'RECOVERY_CLEANUP_FAILED', 'RECOVERY_CLEANUP_FOREIGN_OBJECT',
    'RECOVERY_CLEANUP_IDENTITY_CHANGED', 'RECOVERY_CLEANUP_LIST_UNBOUNDED', 'RECOVERY_CONTROL_PLANE_REFUSED',
    'RECOVERY_CONTROL_PLANE_UNAVAILABLE', 'RECOVERY_CONTROL_PLANE_UNVERIFIED', 'RECOVERY_COST_BOUND_UNPROVEN',
    'RECOVERY_CREATION_OUTCOME_UNCERTAIN', 'RECOVERY_DEADLINE_EXCEEDED', 'RECOVERY_DEPLOYMENT_CALLBACK_FAILED',
    'RECOVERY_DEPLOYMENT_CALLBACK_REQUIRED', 'RECOVERY_EXISTING_SUBDOMAIN_REQUIRED', 'RECOVERY_GATE_IDENTITY_CHANGED',
    'RECOVERY_LIMITS_UNVERIFIED', 'RECOVERY_LOGGING_UNVERIFIED', 'RECOVERY_MIGRATION_OUTCOME_UNCERTAIN',
    'RECOVERY_NAMESPACE_CLEANUP_UNVERIFIED', 'RECOVERY_NAMESPACE_INVENTORY_CHANGED', 'RECOVERY_NAMESPACE_INVENTORY_INCOMPLETE',
    'RECOVERY_NAMESPACE_INVENTORY_UNBOUNDED', 'RECOVERY_NAMESPACE_INVENTORY_UNVERIFIED', 'RECOVERY_NAMESPACE_UNVERIFIED',
    'RECOVERY_NOT_VERIFIED', 'RECOVERY_OPERATION_UNVERIFIED', 'RECOVERY_PREPARED_SOURCE_MISMATCH',
    'RECOVERY_PREVIEW_AUTH_REFUSAL_UNVERIFIED', 'RECOVERY_PREVIEW_BUDGET_EXHAUSTED', 'RECOVERY_PREVIEW_EXCHANGE_FAILED',
    'RECOVERY_PREVIEW_SESSION_INVALID', 'RECOVERY_PREVIEW_TOKEN_INVALID', 'RECOVERY_PROOF_INVALID',
    'RECOVERY_PUBLIC_DOMAIN_PRESENT', 'RECOVERY_PUBLIC_ENDPOINT_PRESENT', 'RECOVERY_PUBLIC_ROUTE_PRESENT',
    'RECOVERY_RECURRENCE_NOT_VERIFIED', 'RECOVERY_REQUEST_BUDGET_EXHAUSTED', 'RECOVERY_SCHEDULE_PRESENT',
    'RECOVERY_SERVICE_ENTRYPOINT_UNVERIFIED', 'RECOVERY_TOOL_CHECKSUM_MISMATCH', 'RECOVERY_TOOL_SOURCE_INVALID',
    'RECOVERY_TRANSFER_NOT_VERIFIED', 'RECOVERY_UPLOAD_READBACK_MISMATCH', 'RECOVERY_USAGE_MODEL_UNVERIFIED',
    'RECOVERY_WORKER_ALREADY_EXISTS', 'RECOVERY_WORKER_CLEANUP_UNVERIFIED',
})


def fixed_reason(error):
    # Our own fixed assertion codes are safe; never persist provider messages,
    # exception reprs, payloads, URLs containing keys, or arbitrary callback text.
    if isinstance(error, RecoveryBlocked) and str(error) in FIXED_REASONS:
        return str(error)
    if type(error).__name__ == 'GateError':
        return {'cloud control-plane request unavailable': 'RECOVERY_CONTROL_PLANE_UNAVAILABLE',
                'cloud control-plane request refused': 'RECOVERY_CONTROL_PLANE_REFUSED',
                'cloud control-plane request budget exhausted': 'RECOVERY_REQUEST_BUDGET_EXHAUSTED'}.get(
                    str(error), 'RECOVERY_CONTROL_PLANE_UNVERIFIED')
    return 'RECOVERY_OPERATION_UNVERIFIED'


# These are reviewed bridge admission limits, not claims about a hosted run.
BRIDGE_LIMITS = {
    'archive_bytes': 16 * 1024 * 1024,
    'application_bytes': 1024 * 1024,
    'photos': 32,
    'application_entries': 1_000,
    'capability_ms': 15 * 60 * 1000,
    'restore_attempts': 1,
    'backup_attempts': 1,
}
# Exact integer micro-USD, deliberately rounding each meter independently and
# assuming no included allowance. Resource-counter bounds are implemented below;
# the SQLite allocated-storage envelope still requires independent review before
# runtime admission. Offline tests are not a hosted cost or recovery receipt.
METERS = {
    'r2_class_a': (1_000_000, 4_500_000, 1_000_000),
    'r2_class_b': (1_000_000, 360_000, 1_000_000),
    'do_requests': (1_000_000, 150_000, 1_000_000),
    'do_duration_gb_seconds': (1_000_000, 12_500_000, 1_000_000),
    'sql_rows_written': (1_000_000, 1_000_000, 1_000_000),
    'sql_rows_read': (1_000_000, 1_000, 1_000_000),
    'worker_requests': (1_000_000, 300_000, 1_000_000),
    'worker_cpu_ms': (1_000_000, 20_000, 2_000_000),
    'sql_gb_month': (1, 200_000, 1),
    'r2_gb_month': (1, 15_000, 1),
}


def rounded_quote(usage):
    """Worst-case rounded incremental units for explicitly bounded dimensions.

    Quote only: does not enforce usage, prove approval, or include subscriptions,
    non-Standard storage, taxes, provider calls or future storage periods.
    """
    if set(usage) != set(METERS):
        raise RecoveryBlocked('INCOMPLETE_RECOVERY_METER_BOUND')
    total = 0
    for name, (unit, price, _) in METERS.items():
        count = usage[name]
        if type(count) is not int or count < 0:
            raise RecoveryBlocked('INVALID_RECOVERY_METER_BOUND')
        total += ((count + unit - 1) // unit) * price
    return total


def plan():
    """Expose only content-free constraints and truthful unresolved admission."""
    return {
        'status': 'blocked',
        'reason': 'RECOVERY_COST_BOUND_UNPROVEN',
        'bridge_limits': dict(BRIDGE_LIMITS),
        'conditional_first_period_quote_micro_usd': rounded_quote({n: v[2] for n, v in METERS.items()}),
        'meter_ceilings_requiring_exact_source_review': {n: v[2] for n, v in METERS.items()},
        'complete_cost_upper_bound_cents': None,
        'remaining': [
            'independent review and exact-source CI of enforced resource bounds',
            'exact-source-bound, unexpired approval and single-use private claim',
            'reviewed temporary storage envelope within the failed-cleanup allowance',
            'hosted execution and cleanup proof (not established by offline tests)',
        ],
        'cloud_requests_made': False,
        'credentials_generated': False,
        'learner_bodies_requested': False,
    }


def admit(receipt=None, *, approval=None, starting=True, now_ms=None):
    """Read an existing content-free approval; never create or infer authority.

    The caller must verify reviewed_source -> exact prepared release through the
    existing generated-metadata-only source gate before making cloud requests.
    A private claim consumes approval_id until its fixed expiry under the existing
    production concurrency lock. Cleanup may delete that exact claim only after
    expiry independently prevents re-admission. This is not account-wide CAS.
    """
    now_ms = int(time.time() * 1000) if now_ms is None else now_ms
    try:
        if approval is None:
            path = Path(__file__).resolve().parents[1] / 'deploy/cloud-recovery-approval.json'
            approval = json.loads(path.read_text()) if path.exists() else None
        valid = (isinstance(approval, dict) and isinstance(receipt, dict)
            and approval.get('approved') is True
            and approval.get('scope') == 'one-cloud-only-recovery-drill'
            and isinstance(approval.get('approval_id'), str)
            and re.fullmatch('[a-f0-9]{32}', approval['approval_id'])
            and re.fullmatch('[a-f0-9]{40}', approval.get('reviewed_source', ''))
            and re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+', approval.get('release_version', ''))
            and os.environ.get('GITHUB_REPOSITORY') == 'misty-step/scry'
            and os.environ.get('GITHUB_REF') == 'refs/heads/master'
            and os.environ.get('GITHUB_EVENT_NAME') == 'workflow_dispatch'
            and os.environ.get('GITHUB_ACTOR') == 'github-actions[bot]'
            and os.environ.get('GITHUB_RUN_ATTEMPT') == '1'
            and os.environ.get('GITHUB_RUN_ID', '').isdigit()
            and re.fullmatch('[a-f0-9]{40}', receipt.get('source', ''))
            and type(receipt.get('artifact_id')) is int and receipt['artifact_id'] > 0
            and type(approval.get('expires_ms')) is int
            and now_ms < approval['expires_ms'] <= now_ms + 3 * 60 * 60 * 1000
            and (not starting or approval['expires_ms'] - now_ms >= 90 * 60 * 1000)
            and type(approval.get('one_time_cents')) is int
            and type(approval.get('failed_cleanup_storage_cents_per_month')) is int)
    except (OSError, TypeError, ValueError):
        valid = False
    if not valid:
        raise RecoveryBlocked('RECOVERY_APPROVAL_REQUIRED')
    if approval['one_time_cents'] < 2000 or approval['failed_cleanup_storage_cents_per_month'] < 25:
        raise RecoveryBlocked('RECOVERY_COST_BOUND_UNPROVEN')
    return dict(approval)


# Pending I/O can outlive capability expiry; retain the reviewed 72 h drain
# envelope, but never substitute a UTC calendar month for the account cycle.
DRAIN_MS = 72 * 60 * 60 * 1000


def verify_billing_period(request, *, now_ms=None):
    """Read-only account-cycle proof, never a subscription/permission change.

    Source: https://developers.cloudflare.com/api/resources/accounts/subresources/subscriptions/methods/get/
    Source: https://developers.cloudflare.com/billing/understand/how-billing-works/#billing-cycles
    Cloudflare documents one shared date for all monthly subscriptions. The
    caller separately verifies existing Workers Standard and R2 context. No
    documented Workers/R2 rate-plan ID vocabulary exists here, so do not guess
    product labels. Validate the complete list and unique IDs, then exclude only
    known zone-scoped subscriptions: they do not meter this account-level drill.
    Every account-scoped row must be Paid, monthly, non-contract USD with one
    shared actual period. Unknown scopes or invalid account rows remain a HOLD.
    """
    now_ms = int(time.time() * 1000) if now_ms is None else now_ms
    try:
        response = request('/subscriptions', envelope=True)
    except Exception:
        raise RecoveryBlocked('RECOVERY_BILLING_PERIOD_UNAVAILABLE') from None
    try:
        need(isinstance(response, dict) and response.get('success') is True
             and response.get('errors') == [], 'RECOVERY_BILLING_PERIOD_UNVERIFIED')
        rows, info = response['result'], response['result_info']
        need(isinstance(rows, list) and 0 < len(rows) <= 1000 and isinstance(info, dict)
             and all(type(info.get(k)) is int for k in ('count', 'page', 'per_page', 'total_count'))
             and info['page'] == 1 and info['count'] == info['total_count'] == len(rows)
             and info['per_page'] >= len(rows), 'RECOVERY_BILLING_PERIOD_INCOMPLETE')
        periods, identities = set(), set()
        for row in rows:
            plan = row['rate_plan']
            need(isinstance(row['id'], str) and 0 < len(row['id']) <= 32 and row['id'] not in identities
                 and plan['scope'] in ('account', 'zone'), 'RECOVERY_BILLING_PERIOD_UNVERIFIED')
            identities.add(row['id'])
            if plan['scope'] == 'zone':
                continue
            need(row['state'] == 'Paid' and row['frequency'] == 'monthly' and row['currency'] == 'USD'
                 and plan['currency'] == 'USD'
                 and plan['externally_managed'] is False and plan['is_contract'] is False,
                 'RECOVERY_BILLING_PERIOD_UNVERIFIED')
            bounds = []
            for field in ('current_period_start', 'current_period_end'):
                value = row[field]
                need(isinstance(value, str) and re.fullmatch(
                    r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|\+00:00)', value),
                    'RECOVERY_BILLING_PERIOD_UNVERIFIED')
                bounds.append(int(datetime.fromisoformat(value.replace('Z', '+00:00')).timestamp() * 1000))
            start, end = bounds
            need(start <= now_ms < end and 0 < end - start <= 32 * 24 * 60 * 60 * 1000,
                 'RECOVERY_BILLING_PERIOD_UNVERIFIED')
            periods.add((start, end))
        need(len(periods) == 1, 'RECOVERY_BILLING_PERIOD_AMBIGUOUS')
        start, end = periods.pop()
        need(now_ms + DRAIN_MS < end, 'RECOVERY_BILLING_PERIOD_BOUND_UNPROVEN')
    except RecoveryBlocked:
        raise
    except (KeyError, TypeError, ValueError, OverflowError):
        raise RecoveryBlocked('RECOVERY_BILLING_PERIOD_UNVERIFIED') from None
    # Keep identifiers, rates and all other private billing metadata in memory
    # only. This short window is passed directly to claim/run, not approval JSON.
    return {'format': 'scry-account-billing-period-v1', 'checked_ms': now_ms,
            'period_start_ms': start, 'period_end_ms': end}


def require_billing_period(window, approval, *, now_ms=None, fresh=True):
    now_ms = int(time.time() * 1000) if now_ms is None else now_ms
    need(isinstance(window, dict) and window.get('format') == 'scry-account-billing-period-v1'
         and all(type(window.get(k)) is int for k in ('checked_ms', 'period_start_ms', 'period_end_ms')),
         'RECOVERY_BILLING_PERIOD_REQUIRED')
    # Cover the entire remaining approval window plus bounded admitted-I/O
    # drain, rather than relying on an observation made shortly before renewal.
    # A reviewed continuation must also include its prior partial usage in this
    # same billing period. Missing accounting start is first-run compatibility.
    accounting = approval.get('accounting_started_ms', window['checked_ms'])
    need(window['period_start_ms'] <= window['checked_ms'] <= now_ms < approval['expires_ms']
         and type(accounting) is int and 0 < accounting
         and window['period_start_ms'] <= accounting <= window['checked_ms']
         and window['period_end_ms'] > approval['expires_ms'] + DRAIN_MS
         and (not fresh or now_ms - window['checked_ms'] <= 5 * 60 * 1000),
         'RECOVERY_BILLING_PERIOD_BOUND_UNPROVEN')


def claim(approval, receipt, request, bucket, *, receipt_path=None, billing_period=None):
    """Consume one approval under the sole native production-writer lock.

    REST GET + PUT is intentionally NOT called atomic CAS. All authorized
    callers must hold scry-rust-production concurrency. Existing, conflicting or
    uncertain claims never retry; they require a new owner decision. The claim
    is outside snapshot retention. It is deleted only AFTER approval expiry,
    when the same approval can no longer admit a new run independently.
    """
    approval = admit(receipt, approval=approval)
    require_billing_period(billing_period, approval)
    need(re.fullmatch('[a-z0-9-]{3,63}', bucket), 'INVALID_RECOVERY_CLAIM_BUCKET')
    path = '/r2/buckets/' + bucket + '/objects/operations/recovery-claims/' + approval['approval_id'] + '.json'
    owner = getattr(request, '__self__', None)
    need(owner is not None and callable(getattr(owner, 'allow_claim', None)), 'RECOVERY_CLAIM_TRANSPORT_UNAVAILABLE')
    owner.allow_claim(path)
    need(request(path, raw=True, absent=True) is None, 'RECOVERY_APPROVAL_ALREADY_CLAIMED')
    value = {'format': 'scry-cloud-recovery-claim-v1', 'approval_id': approval['approval_id'],
             'reviewed_source': approval['reviewed_source'], 'source': receipt['source'],
             'artifact_id': receipt['artifact_id'], 'release_version': approval['release_version'],
             'github_run_id': os.environ['GITHUB_RUN_ID']}
    encoded = json.dumps(value, sort_keys=True, separators=(',', ':')).encode()
    need(len(encoded) < 1024, 'RECOVERY_CLAIM_OVERSIZE')
    saved = Path(receipt_path) if receipt_path else Path(__file__).resolve().parents[1] / 'target/recovery-claim.json'
    saved.parent.mkdir(parents=True, exist_ok=True)
    with saved.open('x') as output:
        json.dump({'format': 'scry-cloud-recovery-claim-cleanup-v1', 'path': path, 'body': encoded.decode(),
                   'sha256': hashlib.sha256(encoded).hexdigest(), 'expires_ms': approval['expires_ms']}, output)
    try:
        request(path, method='PUT', body=encoded, headers={'Content-Type': 'application/json'})
        actual, _ = request(path, raw=True)
        need(actual == encoded, 'RECOVERY_CLAIM_READBACK_MISMATCH')
    except Exception:
        raise RecoveryBlocked('RECOVERY_CLAIM_OUTCOME_UNCERTAIN') from None
    return value


def cleanup_claim(receipt_path, request, *, clock=time.time, sleep=time.sleep):
    """Delete only our exact claim after fixed expiry; no permanent paid ledger."""
    saved = Path(receipt_path)
    if not saved.exists():
        return {'claim_cleanup': 'not_created'}
    value = json.loads(saved.read_text())
    path, body, until = value.get('path'), value.get('body'), value.get('expires_ms')
    need(value.get('format') == 'scry-cloud-recovery-claim-cleanup-v1' and isinstance(body, str)
         and len(body.encode()) < 1024 and hashlib.sha256(body.encode()).hexdigest() == value.get('sha256')
         and isinstance(path, str) and re.fullmatch(r'/r2/buckets/[a-z0-9-]+/objects/operations/recovery-claims/[a-f0-9]{32}\.json', path)
         and type(until) is int and 0 < until <= int(clock() * 1000) + 3 * 60 * 60 * 1000,
         'RECOVERY_CLAIM_CLEANUP_RECEIPT_INVALID')
    claim_body = json.loads(body)
    need(claim_body.get('format') == 'scry-cloud-recovery-claim-v1'
         and claim_body.get('github_run_id') == os.environ.get('GITHUB_RUN_ID')
         and path.endswith('/' + claim_body.get('approval_id', '') + '.json'), 'RECOVERY_CLAIM_CLEANUP_IDENTITY_CHANGED')
    owner = getattr(request, '__self__', None)
    need(owner is not None and callable(getattr(owner, 'allow_claim', None)), 'RECOVERY_CLAIM_TRANSPORT_UNAVAILABLE')
    owner.allow_claim(path)
    while clock() * 1000 <= until:
        sleep(min(60, (until - clock() * 1000) / 1000 + 0.001))
    actual = request(path, raw=True, absent=True)
    if actual is None:
        return {'claim_cleanup': 'verified_absent'}
    need(actual[0] == body.encode(), 'RECOVERY_CLAIM_CLEANUP_IDENTITY_CHANGED')
    try:
        request(path, method='DELETE')
    except Exception:
        # An uncertain DELETE is inspected, never repeated blindly.
        need(request(path, raw=True, absent=True) is None, 'RECOVERY_CLAIM_CLEANUP_UNCERTAIN')
    need(request(path, raw=True, absent=True) is None, 'RECOVERY_CLAIM_CLEANUP_UNVERIFIED')
    return {'claim_cleanup': 'verified_absent'}


def need(condition, reason):
    if not condition:
        raise RecoveryBlocked(reason)


def multipart(metadata, modules, session=False):
    boundary = 'scry-recovery-multipart-boundary'
    parts = [('metadata', 'application/json', json.dumps(metadata).encode())]
    parts += [(name, 'application/wasm' if name.endswith('.wasm') else 'application/javascript+module', data)
              for name, data in modules.items()]
    if session:
        parts.append(('wrangler-session-config', 'application/json', b'{"workers_dev":true,"minimal_mode":true}'))
    body = b''
    for name, kind, data in parts:
        need(boundary.encode() not in data and re.fullmatch('[a-zA-Z0-9_.-]+', name), 'INVALID_UPLOAD_MODULE')
        body += ('--' + boundary + '\r\nContent-Disposition: form-data; name="' + name + '"; filename="' + name +
                 '"\r\nContent-Type: ' + kind + '\r\n\r\n').encode() + data + b'\r\n'
    return body + ('--' + boundary + '--\r\n').encode(), {'Content-Type': 'multipart/form-data; boundary=' + boundary}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def private_request(url, *, method='GET', headers=None, body=None):
    # Only provider-returned private preview hosts, never the Cloudflare control
    # plane bearer. Callers validate the exact session host before using tokens.
    parsed = urllib.parse.urlsplit(url)
    need(parsed.scheme == 'https' and parsed.hostname and not parsed.username and not parsed.password
         and parsed.port in (None, 443) and not parsed.fragment
         and parsed.hostname.endswith(('.workers.dev', '.cloudflareworkers.com')), 'INVALID_PRIVATE_PREVIEW_HOST')
    request = urllib.request.Request(url, method=method, data=body, headers=headers or {})
    try:
        response = urllib.request.build_opener(NoRedirect()).open(request, timeout=60)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        data = response.read(65537)
        need(len(data) <= 65536, 'PRIVATE_PROOF_RESPONSE_OVERSIZE')
        return response.status, data


def frozen_modules(artifact):
    artifact = Path(artifact)
    proof = json.loads((artifact / 'proof.json').read_text())
    need(proof.get('source_state') == 'committed', 'RECOVERY_ARTIFACT_NOT_COMMITTED')
    modules = {name: (artifact / 'worker' / name).read_bytes() for name in ('index.js', 'index_bg.wasm')}
    need(all(hashlib.sha256(data).hexdigest() == proof['worker'].get(name) for name, data in modules.items()),
         'RECOVERY_ARTIFACT_CHECKSUM_MISMATCH')
    source = json.loads((artifact / 'source.json').read_text())
    tooling = {}
    with tarfile.open(artifact / 'source.tar.gz', 'r:gz') as archive:
        for path in ('recovery/cloud-restore.mjs', 'recovery/target-adapter.mjs'):
            member = archive.getmember(path)
            need(member.isfile() and member.size < 128 * 1024, 'RECOVERY_TOOL_SOURCE_INVALID')
            data = archive.extractfile(member).read()
            matches = [v for v in source['files'] if v['path'] == path]
            need(len(matches) == 1 and hashlib.sha256(data).hexdigest() == matches[0]['sha256'], 'RECOVERY_TOOL_CHECKSUM_MISMATCH')
            tooling[Path(path).name] = data
    # The adapter is a separately frozen recovery tool; native application bytes
    # remain exactly those already exercised by the prepared artifact.
    modules['target-adapter.mjs'] = tooling['target-adapter.mjs']
    return proof, modules, {'cloud-restore.mjs': tooling['cloud-restore.mjs']}



class Drill:
    """One call, one new namespace pair, no retries or production mutation."""
    def __init__(self, request, external=private_request, *, clock=time.time):
        self.request, self.external, self.clock = request, external, clock
        self.calls = self.external_calls = 0
        self.start = clock()
        self.cleaning = False
        self.created = {}
        self.bucket = None
        self.namespace_ids = {}
        self.uncertain = set()
        self.cleanup_outcomes = []

    def call(self, path, method='GET', **kwargs):
        self.calls += 1
        need(self.calls <= (128 if self.cleaning else 75), 'RECOVERY_REQUEST_BUDGET_EXHAUSTED')
        need(self.cleaning or self.clock() - self.start < 900, 'RECOVERY_DEADLINE_EXCEEDED')
        return self.request(path, method=method, **kwargs)

    def preview(self, url, **kwargs):
        self.external_calls += 1
        need(self.external_calls <= 5 and self.clock() - self.start < 900, 'RECOVERY_PREVIEW_BUDGET_EXHAUSTED')
        return self.external(url, **kwargs)

    def private(self, name):
        base = '/workers/scripts/' + name
        need(self.call(base + '/subdomain') == {'enabled': False, 'previews_enabled': False}, 'RECOVERY_PUBLIC_ENDPOINT_PRESENT')
        need(self.call(base + '/routes') == [], 'RECOVERY_PUBLIC_ROUTE_PRESENT')
        need(self.call(base + '/schedules').get('schedules') == [], 'RECOVERY_SCHEDULE_PRESENT')
        domains = self.call('/workers/domains?service=' + name)
        need(not any(v.get('service') == name for v in domains), 'RECOVERY_PUBLIC_DOMAIN_PRESENT')

    def upload(self, name, metadata, modules, *, create=False):
        base = '/workers/scripts/' + name
        body, headers = multipart(metadata, modules)
        if create:
            self.uncertain.add(name)
        self.call(base, method='PUT', body=body, headers=headers)
        if create:
            self.uncertain.discard(name)
            # Only a successfully acknowledged creation grants cleanup authority.
            self.created[name] = None
        raw, kind = self.call(base + '/content/v2', raw=True)
        form = email.parser.BytesParser(policy=email.policy.default).parsebytes(('Content-Type: ' + kind + '\r\n\r\n').encode() + raw)
        observed = {}
        for part in form.iter_parts():
            key = part.get_filename() or part.get_param('name', header='Content-Disposition')
            need(key in modules and key not in observed, 'RECOVERY_UPLOAD_READBACK_MISMATCH')
            observed[key] = part.get_payload(decode=True)
        need(observed == modules, 'RECOVERY_UPLOAD_READBACK_MISMATCH')
        return self.call(base + '/settings')

    def shell(self, name):
        need(self.call('/workers/scripts/' + name + '/settings', absent=True) is None, 'RECOVERY_WORKER_ALREADY_EXISTS')
        self.upload(name, {'main_module': 'empty.mjs', 'compatibility_date': '2026-09-10', 'bindings': [],
            'observability': {'enabled': False}, 'logpush': False, 'usage_model': 'standard', 'limits': {'cpu_ms': 30000, 'subrequests': 100}},
            {'empty.mjs': b'export default {fetch(){return new Response(null,{status:404})}}'}, create=True)
        self.private(name)

    def bind(self, name, metadata, modules, class_name):
        # Record the intended class before the operation: an ambiguous response
        # cannot safely be retried or cleaned as if no migration happened.
        self.created[name] = 'unverified-migration'
        settings = self.upload(name, metadata, modules)
        need(settings.get('limits') == metadata['limits'], 'RECOVERY_LIMITS_UNVERIFIED')
        need(settings.get('usage_model') == 'standard', 'RECOVERY_USAGE_MODEL_UNVERIFIED')
        # Script-level logging is read from its dedicated API, not the version
        # settings response. Missing fields are not proof that logging is off.
        logging = self.call('/workers/scripts/' + name + '/script-settings')
        need(isinstance(logging, dict) and isinstance(logging.get('observability'), dict)
             and logging['observability'].get('enabled') is False and logging.get('logpush') is False,
             'RECOVERY_LOGGING_UNVERIFIED')
        need({(v['name'], v['type']) for v in settings['bindings']} == {(v['name'], v['type']) for v in metadata['bindings']}, 'RECOVERY_BINDINGS_UNVERIFIED')
        by_name = {v['name']: v for v in settings['bindings']}
        for expected in metadata['bindings']:
            actual = by_name[expected['name']]
            for field in ('text', 'bucket_name', 'class_name', 'script_name', 'service', 'environment'):
                if field in expected and not (field == 'text' and expected['type'] == 'secret_text'):
                    need(actual.get(field) == expected[field], 'RECOVERY_BINDING_VALUE_UNVERIFIED')
            if expected['type'] == 'service':
                need(actual.get('entrypoint') in (None, ''), 'RECOVERY_SERVICE_ENTRYPOINT_UNVERIFIED')
        own = [v for v in settings['bindings'] if v.get('class_name') == class_name and v.get('type') == 'durable_object_namespace']
        need(len(own) == 1 and re.fullmatch('[a-f0-9]{32}', own[0].get('namespace_id', '')), 'RECOVERY_NAMESPACE_UNVERIFIED')
        self.created[name] = class_name
        self.namespace_ids[name] = own[0]['namespace_id']
        self.private(name)
        return settings

    def namespace_absent(self, identity, script):
        # Cloudflare's namespace.list uses SyncV4PagePaginationArray: advance
        # until an EMPTY result, never merely a short page or metadata count.
        # https://github.com/cloudflare/cloudflare-python/blob/main/src/cloudflare/pagination.py
        # Object lists are not namespace-existence proof; limit=1 is invalid
        # there. No destructive retry; at most8 inventory reads under128 calls.
        identities, owned, known = set(), False, {}
        for page in range(1, 9):
            value = self.call('/workers/durable_objects/namespaces?page=' + str(page) + '&per_page=1000', envelope=True)
            need(isinstance(value, dict) and value.get('success') is True, 'RECOVERY_NAMESPACE_INVENTORY_UNVERIFIED')
            rows, info = value.get('result'), value.get('result_info')
            if info is None:
                info = {}
            need(isinstance(rows, list) and len(rows) <= 1000 and isinstance(info, dict),
                 'RECOVERY_NAMESPACE_INVENTORY_UNVERIFIED')
            for field in ('count', 'page', 'per_page', 'total_count', 'total_pages'):
                number = info.get(field)
                if number is None:
                    continue
                need(type(number) is int and number >= 0, 'RECOVERY_NAMESPACE_INVENTORY_UNVERIFIED')
                if field == 'page':
                    need(number == page, 'RECOVERY_NAMESPACE_INVENTORY_CHANGED')
                elif field == 'count':
                    need(number == len(rows), 'RECOVERY_NAMESPACE_INVENTORY_INCOMPLETE')
                else:
                    need(field not in known or known[field] == number, 'RECOVERY_NAMESPACE_INVENTORY_CHANGED')
                    known[field] = number
            if 'per_page' in known:
                need(1 <= known['per_page'] <= 1000 and len(rows) <= known['per_page'], 'RECOVERY_NAMESPACE_INVENTORY_INCOMPLETE')
            if 'total_pages' in known:
                need(known['total_pages'] <= 8 and (not rows or page <= known['total_pages']),
                     'RECOVERY_NAMESPACE_INVENTORY_INCOMPLETE')
            for row in rows:
                need(isinstance(row, dict) and all(isinstance(row.get(k), str) and row[k] for k in ('id', 'script', 'class'))
                     and row['id'] not in identities, 'RECOVERY_NAMESPACE_INVENTORY_UNVERIFIED')
                identities.add(row['id'])
                owned |= row['id'] == identity or row['script'] == script
            if 'total_count' in known:
                need(len(identities) <= known['total_count'] <= 8000, 'RECOVERY_NAMESPACE_INVENTORY_INCOMPLETE')
            if not rows:
                need('total_count' not in known or len(identities) == known['total_count'],
                     'RECOVERY_NAMESPACE_INVENTORY_INCOMPLETE')
                need('total_pages' not in known or page > known['total_pages']
                     or (page == 1 and not identities and known['total_pages'] == 1),
                     'RECOVERY_NAMESPACE_INVENTORY_INCOMPLETE')
                return not owned
        raise RecoveryBlocked('RECOVERY_NAMESPACE_INVENTORY_UNBOUNDED')

    def cleanup(self):
        self.cleaning = True
        failed = []
        for name, class_name in reversed(list(self.created.items())):
            step = 'worker_identity'
            try:
                base = '/workers/scripts/' + name
                settings = self.call(base + '/settings')
                if class_name:
                    step = 'namespace_identity'
                    need(name in self.namespace_ids, 'RECOVERY_MIGRATION_OUTCOME_UNCERTAIN')
                    need(any(v.get('namespace_id') == self.namespace_ids[name] and v.get('class_name') == class_name
                             for v in settings['bindings']), 'RECOVERY_CLEANUP_IDENTITY_CHANGED')
                    metadata = {'main_module': 'empty.mjs', 'compatibility_date': '2026-09-10', 'bindings': [], 'usage_model': 'standard',
                        'migrations': {'old_tag': 'recovery-v1', 'new_tag': 'recovery-deleted-v1', 'deleted_classes': [class_name]}}
                    body, headers = multipart(metadata, {'empty.mjs': b'export default {fetch(){return new Response(null,{status:404})}}'})
                    step = 'namespace_delete'
                    self.call(base, method='PUT', body=body, headers=headers)
                    step = 'namespace_verify'
                    need(self.namespace_absent(self.namespace_ids[name], name), 'RECOVERY_NAMESPACE_CLEANUP_UNVERIFIED')
                step = 'worker_delete'
                self.call(base, method='DELETE')
                step = 'worker_verify'
                need(self.call(base + '/settings', absent=True) is None, 'RECOVERY_WORKER_CLEANUP_UNVERIFIED')
                self.cleanup_outcomes.append({'resource': name, 'step': step, 'status': 'verified_absent', 'reason': None})
            except Exception as error:
                failed.append(name)
                self.cleanup_outcomes.append({'resource': name, 'step': step, 'status': 'unverified', 'reason': fixed_reason(error)})
        if self.bucket:
            step = 'bucket_identity'
            try:
                base = '/r2/buckets/' + self.bucket['name']
                need(self.call(base) == self.bucket, 'RECOVERY_CLEANUP_BUCKET_IDENTITY_CHANGED')
                step = 'bucket_list'
                listing = self.call(base + '/objects?per_page=1000', envelope=True)
                items, info = listing['result'], listing.get('result_info')
                # Same REST cursor contract as the production metadata listing:
                # absent metadata/cursor is terminal; a supplied flag must agree.
                if info is None:
                    info = {}
                need(isinstance(info, dict), 'RECOVERY_CLEANUP_LIST_UNBOUNDED')
                cursor = info.get('cursor')
                need(cursor is None or isinstance(cursor, str), 'RECOVERY_CLEANUP_LIST_UNBOUNDED')
                if 'is_truncated' in info:
                    need(type(info['is_truncated']) is bool and info['is_truncated'] == bool(cursor),
                         'RECOVERY_CLEANUP_LIST_UNBOUNDED')
                need(not cursor and isinstance(items, list) and len(items) <= 33, 'RECOVERY_CLEANUP_LIST_UNBOUNDED')
                for obj in items:
                    need(obj['key'].startswith(('photos/' + self.space + '/', 'snapshots/' + self.space + '/')),
                         'RECOVERY_CLEANUP_FOREIGN_OBJECT')
                step = 'owned_objects_delete'
                for obj in items:
                    self.call(base + '/objects/' + urllib.parse.quote(obj['key'], safe='/'), method='DELETE')
                step = 'bucket_delete'
                self.call(base, method='DELETE')
                step = 'bucket_verify'
                need(self.call(base, absent=True) is None, 'RECOVERY_BUCKET_CLEANUP_UNVERIFIED')
                self.cleanup_outcomes.append({'resource': self.bucket['name'], 'step': step, 'status': 'verified_absent', 'reason': None})
            except Exception as error:
                failed.append(self.bucket['name'])
                self.cleanup_outcomes.append({'resource': self.bucket['name'], 'step': step, 'status': 'unverified', 'reason': fixed_reason(error)})
        for name in sorted(self.uncertain):
            self.cleanup_outcomes.append({'resource': name, 'step': 'creation_identity', 'status': 'unverified', 'reason': 'RECOVERY_CREATION_OUTCOME_UNCERTAIN'})
        return sorted(set(failed) | self.uncertain)


def run(config, artifact_dir, source_pin, request, *, approval, after_restore, receipt=None, external=private_request, billing_period=None):
    """Provision only after explicit admission; never read source object bodies here."""
    proof, candidate, bridge = frozen_modules(artifact_dir)
    need(isinstance(receipt, dict) and receipt.get('source') == proof['revision'], 'RECOVERY_PREPARED_SOURCE_MISMATCH')
    approval = admit(receipt, approval=approval, starting=False)
    require_billing_period(billing_period, approval, fresh=False)
    need(callable(after_restore), 'RECOVERY_DEPLOYMENT_CALLBACK_REQUIRED')
    need(isinstance(config.get('name'), str) and re.fullmatch('[a-z0-9-]{1,63}', config['name'])
         and approval.get('production_health_service') == config['name'], 'PRODUCTION_HEALTH_SERVICE_APPROVAL_REQUIRED')
    key, etag, size = source_pin.get('key'), source_pin.get('etag'), source_pin.get('size')
    need(isinstance(key, str) and re.fullmatch(r'snapshots/[a-z0-9-]{1,100}/[0-9]+-(daily|pre-release)-[a-f0-9]{48}\.json', key), 'INVALID_RECOVERY_SOURCE_PIN')
    need(isinstance(etag, str) and re.fullmatch(r'(?:[a-f0-9]{32}(?:-[0-9]+)?|"[a-f0-9]{32}(?:-[0-9]+)?")', etag), 'INVALID_RECOVERY_SOURCE_PIN')
    need(type(size) is int and size > 0, 'INVALID_RECOVERY_SOURCE_PIN')
    need(size <= BRIDGE_LIMITS['archive_bytes'],
         f'RECOVERY_ARCHIVE_BYTES_EXCEED_LIMIT size={size} limit={BRIDGE_LIMITS["archive_bytes"]}')
    # No capabilities are generated until every local input and approval passes.
    nonce = secrets.token_hex(12)
    stem, space = 'scry-recovery-' + nonce, 'recovery-' + nonce
    target, gateway, bucket = stem + '-target', stem + '-bridge', stem + '-data'
    internal, restore_key, transfer = [secrets.token_hex(32) for _ in range(3)]
    drill = Drill(request, external)
    drill.space = space
    outcome = None
    phase = 'restore'
    primary_reason = None
    failed = []
    try:
        for name in (target, gateway):
            drill.shell(name)
        bucket_path = '/r2/buckets/' + bucket
        need(drill.call(bucket_path, absent=True) is None, 'RECOVERY_BUCKET_ALREADY_EXISTS')
        drill.uncertain.add(bucket)
        created = drill.call('/r2/buckets', method='POST', payload={'name': bucket, 'storageClass': 'Standard'})
        need(created.get('name') == bucket and created.get('storage_class') == 'Standard' and created.get('creation_date'),
             'RECOVERY_BUCKET_CREATION_UNVERIFIED')
        drill.bucket = created
        drill.uncertain.discard(bucket)
        need(drill.call(bucket_path) == created, 'RECOVERY_BUCKET_READBACK_MISMATCH')
        need(drill.call(bucket_path + '/domains/managed').get('enabled') is False
             and drill.call(bucket_path + '/domains/custom').get('domains') == [], 'RECOVERY_BUCKET_NOT_PRIVATE')
        base_meta = {'compatibility_date': '2026-09-10', 'observability': {'enabled': False}, 'logpush': False, 'usage_model': 'standard',
                     'limits': {'cpu_ms': 30000, 'subrequests': 100}}
        def binding(name, value, secret=False):
            return {'name': name, 'type': 'secret_text' if secret else 'plain_text', 'text': str(value)}
        issued = int(time.time() * 1000)
        target_meta = dict(base_meta, main_module='target-adapter.mjs', migrations={'new_tag': 'recovery-v1', 'new_sqlite_classes': ['LearningSpace']}, bindings=[
            {'name': 'LEARNING_SPACE', 'type': 'durable_object_namespace', 'class_name': 'LearningSpace'},
            *[{'name': n, 'type': 'r2_bucket', 'bucket_name': bucket} for n in ('SCRY_ASSETS', 'SCRY_BACKUPS')],
            binding('SCRY_ENV', 'restore'), binding('SPACE_NAME', space), binding('INTERNAL_KEY', internal, True), binding('RESTORE_TOKEN', restore_key, True),
            binding('TRANSFER_ISSUED_MS', issued), binding('TRANSFER_UNTIL_MS', issued + 900000)])
        drill.bind(target, target_meta, candidate, 'LearningSpace')
        pins = {'SOURCE_KEY': key, 'SOURCE_ETAG': etag, 'SOURCE_BYTES': size, 'SPACE_NAME': space,
                'TRANSFER_ISSUED_MS': issued, 'TRANSFER_UNTIL_MS': issued + 900000}
        source_bucket = next(v['bucket_name'] for v in config['r2_buckets'] if v['binding'] == 'SCRY_BACKUPS')
        bridge_meta = dict(base_meta, main_module='cloud-restore.mjs', migrations={'new_tag': 'recovery-v1', 'new_sqlite_classes': ['RecoveryBridge']}, bindings=[
            {'name': 'GATE', 'type': 'durable_object_namespace', 'class_name': 'RecoveryBridge'},
            {'name': 'TARGET', 'type': 'durable_object_namespace', 'class_name': 'LearningSpace', 'script_name': target},
            {'name': 'SOURCE', 'type': 'r2_bucket', 'bucket_name': source_bucket},
            {'name': 'BACKUPS', 'type': 'r2_bucket', 'bucket_name': bucket},
            {'name': 'PRODUCTION', 'type': 'service', 'service': config['name'], 'environment': 'production'},
            *[binding(k, v) for k, v in pins.items()], binding('INTERNAL_KEY', internal, True),
            binding('RESTORE_TOKEN', restore_key, True), binding('TRANSFER_KEY', transfer, True)])
        staged_meta = dict(bridge_meta, bindings=[b for b in bridge_meta['bindings'] if b['name'] not in ('SOURCE', 'PRODUCTION')])
        drill.bind(gateway, staged_meta, bridge, 'RecoveryBridge')
        session = drill.call('/workers/scripts/' + gateway + '/subdomain/edge-preview')
        token, exchange = session.get('token'), session.get('exchange_url')
        need(isinstance(token, str) and len(token) >= 20, 'RECOVERY_PREVIEW_SESSION_INVALID')
        if exchange:
            status, body = drill.preview(exchange)
            need(status == 200, 'RECOVERY_PREVIEW_EXCHANGE_FAILED')
            token = json.loads(body)['token']
            host = urllib.parse.urlsplit(exchange).hostname
            host = gateway + host[host.index('.'):]
        else:
            subdomain = drill.call('/workers/subdomain').get('subdomain')
            need(isinstance(subdomain, str) and re.fullmatch('[a-z0-9-]+', subdomain), 'RECOVERY_EXISTING_SUBDOMAIN_REQUIRED')
            host = gateway + '.' + subdomain + '.workers.dev'
        # Preview foreign-binds exact recorded namespaces; it cannot create a
        # new gate when replayed, and never registers a public subdomain.
        preview_meta = dict(staged_meta)
        preview_meta.pop('migrations')
        preview_meta['bindings'] = [dict(v) for v in staged_meta['bindings']]
        # Keep explicit names: no untrusted caller chooses either namespace.
        preview_meta['bindings'][0] = {'name': 'GATE', 'type': 'durable_object_namespace', 'namespace_id': drill.namespace_ids[gateway]}
        preview_meta['bindings'][1] = {'name': 'TARGET', 'type': 'durable_object_namespace', 'namespace_id': drill.namespace_ids[target]}
        body, headers = multipart(preview_meta, bridge, session=True)
        headers['cf-preview-upload-config-token'] = token
        preview = drill.call('/workers/scripts/' + gateway + '/edge-preview', method='POST', body=body, headers=headers)
        preview_token = preview.get('preview_token')
        need(isinstance(preview_token, str) and len(preview_token) >= 20, 'RECOVERY_PREVIEW_TOKEN_INVALID')
        url = 'https://' + host + '/restore'
        headers = {'cf-workers-preview-token': preview_token}
        for auth in (None, 'Bearer synthetic-wrong-key'):
            status, _ = drill.preview(url, method='POST', headers=headers | ({'Authorization': auth} if auth else {}), body=b'')
            need(status == 403, 'RECOVERY_PREVIEW_AUTH_REFUSAL_UNVERIFIED')
        active_meta = dict(bridge_meta)
        active_meta.pop('migrations')
        gate_id = drill.namespace_ids[gateway]
        drill.bind(gateway, active_meta, bridge, 'RecoveryBridge')
        need(drill.namespace_ids[gateway] == gate_id, 'RECOVERY_GATE_IDENTITY_CHANGED')
        status, body = drill.preview(url, method='POST', headers=headers | {'Authorization': 'Bearer ' + transfer}, body=b'')
        need(status == 200, 'RECOVERY_TRANSFER_NOT_VERIFIED')
        outcome = json.loads(body)
        need(set(outcome) == {'format', 'canonical_application_equal', 'source_sha256', 'snapshot_sha256', 'source_bytes',
            'snapshot_bytes', 'photos_full_bytes_verified', 'restore_attempts', 'backup_attempts', 'elapsed_ms',
            'target_database_bytes', 'target_page_size'}, 'RECOVERY_PROOF_INVALID')
        need(all(type(outcome[k]) is int for k in ('source_bytes', 'snapshot_bytes', 'photos_full_bytes_verified', 'restore_attempts', 'backup_attempts', 'elapsed_ms', 'target_database_bytes', 'target_page_size')), 'RECOVERY_PROOF_INVALID')
        need(outcome['format'] == 'scry-cloud-restore-proof-v1' and outcome['canonical_application_equal'] is True
             and outcome['source_bytes'] == size and 0 < outcome['snapshot_bytes'] <= BRIDGE_LIMITS['archive_bytes']
             and outcome['restore_attempts'] == outcome['backup_attempts'] == 1
             and 0 <= outcome['target_database_bytes'] <= 32 * 1024 * 1024
             and outcome['target_page_size'] in (512, 1024, 2048, 4096)
             and 0 <= outcome['photos_full_bytes_verified'] <= 32 and 0 <= outcome['elapsed_ms'] <= 900000
             and all(re.fullmatch('[a-f0-9]{64}', outcome[k]) for k in ('source_sha256', 'snapshot_sha256')), 'RECOVERY_PROOF_INVALID')
        # Only a complete, strict restore proof may admit the parent's exact
        # production upload/readback. Once called, failure is never 'undeployed'.
        phase = 'deployment'
        after_restore()
        phase = 'recurrence'
        status, body = drill.preview('https://' + host + '/recurrence', method='POST',
            headers=headers | {'Authorization': 'Bearer ' + transfer}, body=b'')
        need(status == 200 and len(body) <= 64, 'RECOVERY_RECURRENCE_NOT_VERIFIED')
        recurrence = json.loads(body)
        need(isinstance(recurrence, dict) and set(recurrence) == {'backup_proof'}
             and recurrence['backup_proof'] is True, 'RECOVERY_RECURRENCE_NOT_VERIFIED')
        phase = 'verified'
    except Exception as error:
        primary_reason = fixed_reason(error)
        outcome = None
    finally:
        failed = drill.cleanup()
    if failed:
        error = RecoveryBlocked('RECOVERY_CLEANUP_FAILED')
        # Caller retains this only in the private native receipt, not public logs.
        error.retained_resources = failed
        error.recovery_phase = phase
        error.primary_reason = primary_reason
        error.cleanup_outcomes = drill.cleanup_outcomes
        error.deployment_attempted = phase != 'restore'
        raise error
    if outcome is None:
        reason = {'deployment': 'RECOVERY_DEPLOYMENT_CALLBACK_FAILED',
                  'recurrence': 'RECOVERY_RECURRENCE_NOT_VERIFIED'}.get(phase, 'RECOVERY_NOT_VERIFIED')
        error = RecoveryBlocked(reason)
        error.recovery_phase = phase
        error.primary_reason = primary_reason
        error.cleanup_outcomes = drill.cleanup_outcomes
        error.deployment_attempted = phase != 'restore'
        raise error
    return {'status': 'verified', 'proof': outcome, 'recurrence_verified': True, 'cleanup_verified': True, 'cloud_requests': drill.calls,
            'preview_requests': drill.external_calls, 'cost_quote_micro_usd': rounded_quote({n: v[2] for n, v in METERS.items()}),
            'learner_bodies_exported': False}


def main():
    print(json.dumps(plan(), sort_keys=True))
    return 3


if __name__ == '__main__':
    if sys.argv[1:]:
        print('Usage: scry-cloud-recovery.py (offline admission plan only)', file=sys.stderr)
        sys.exit(2)
    sys.exit(main())
