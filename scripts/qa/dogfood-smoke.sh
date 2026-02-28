#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/qa/dogfood-smoke.sh [--url <base-url>] [--session <name>] [--out <dir>] [--fail-on-high]

Runs a lightweight dogfood + agent-browser smoke pass and writes a markdown report
plus screenshots under the output directory.

Options:
  --url <base-url>     Base URL to test (default: https://scry.study)
  --session <name>     agent-browser session name (default: scry-dogfood-<timestamp>)
  --out <dir>          Output directory (default: /tmp/dogfood-scry-<timestamp>)
  --fail-on-high       Exit non-zero when high/critical findings are detected
  -h, --help           Show this help
EOF
}

TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
BASE_URL="https://scry.study"
SESSION="scry-dogfood-$TIMESTAMP"
OUT_DIR="/tmp/dogfood-scry-$TIMESTAMP"
FAIL_ON_HIGH=false
QA_TEST_EMAIL="${QA_TEST_EMAIL:-qa-bot@example.com}"
QA_TEST_PASSWORD="${QA_TEST_PASSWORD:-invalid-password}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --url)
      BASE_URL="$2"
      shift 2
      ;;
    --session)
      SESSION="$2"
      shift 2
      ;;
    --out)
      OUT_DIR="$2"
      shift 2
      ;;
    --fail-on-high)
      FAIL_ON_HIGH=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      # Accept pnpm argument forwarding separator and continue parsing.
      shift
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v agent-browser >/dev/null 2>&1; then
  echo "agent-browser not found. Install with: npm i -g agent-browser" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq not found. Install with: brew install jq" >&2
  exit 1
fi

mkdir -p "$OUT_DIR/screenshots"

ab() {
  agent-browser --session "$SESSION" "$@"
}

run_ab() {
  set +e
  ab "$@"
  local status=$?
  set -e
  if [[ "$status" -ne 0 ]]; then
    AB_FAILURE_COUNT=$((AB_FAILURE_COUNT + 1))
  fi
  return "$status"
}

soft_ab() {
  run_ab "$@" || true
}

AB_FAILURE_COUNT=0
LANDING_URL="$BASE_URL"
SIGN_IN_URL="${BASE_URL%/}/sign-in"

echo "→ Session: $SESSION"
echo "→ Output:  $OUT_DIR"
echo "→ Base URL: $BASE_URL"

soft_ab open "$LANDING_URL" >/dev/null
soft_ab wait --load domcontentloaded >/dev/null
soft_ab screenshot --annotate "$OUT_DIR/screenshots/initial.png" >/dev/null

soft_ab open "$SIGN_IN_URL" >/dev/null
soft_ab wait --load domcontentloaded >/dev/null
soft_ab screenshot --annotate "$OUT_DIR/screenshots/sign-in.png" >/dev/null

soft_ab fill "input[name='identifier']" "$QA_TEST_EMAIL" >/dev/null
soft_ab fill "input[name='password']" "$QA_TEST_PASSWORD" >/dev/null
soft_ab click "button:has-text('Continue')" >/dev/null
soft_ab wait "text=Couldn't find your account." >/dev/null
soft_ab screenshot --annotate "$OUT_DIR/screenshots/login-unknown-account.png" >/dev/null

DUP_EVAL_JSON="$OUT_DIR/dup-eval.json"
if run_ab eval "(() => { const txt = document.body?.innerText || ''; const needle = \"Couldn't find your account.\"; return { url: location.href, duplicateCount: Math.max(0, txt.split(needle).length - 1), hasError: txt.includes(needle) }; })()" >"$DUP_EVAL_JSON"; then
  DUP_COUNT="$(jq -r '.duplicateCount // 0' "$DUP_EVAL_JSON")"
else
  DUP_COUNT=0
fi

soft_ab click "a:has-text('Sign up')" >/dev/null
soft_ab wait --load domcontentloaded >/dev/null
soft_ab screenshot --annotate "$OUT_DIR/screenshots/sign-up.png" >/dev/null

CF_EVAL_JSON="$OUT_DIR/cloudflare-eval.json"
if run_ab eval "(() => { const text = (document.body?.innerText || '').toLowerCase(); const title = (document.title || '').toLowerCase(); const isCloudflareChallenge = title.includes('just a moment') || text.includes('performing security verification') || text.includes('ray id'); return { url: location.href, title: document.title, isCloudflareChallenge }; })()" >"$CF_EVAL_JSON"; then
  CF_CHALLENGE="$(jq -r '.isCloudflareChallenge // false' "$CF_EVAL_JSON")"
  SIGNUP_URL="$(jq -r '.url // ""' "$CF_EVAL_JSON")"
else
  CF_CHALLENGE=false
  SIGNUP_URL=""
fi

if [[ ! "$DUP_COUNT" =~ ^[0-9]+$ ]]; then
  DUP_COUNT=0
fi

DATE_STR="$(date +%Y-%m-%d)"
REPORT_PATH="$OUT_DIR/report.md"

HIGH_COUNT=0
MEDIUM_COUNT=0
LOW_COUNT=0

if [[ "$CF_CHALLENGE" == "true" ]]; then
  HIGH_COUNT=$((HIGH_COUNT + 1))
fi

if [[ "$AB_FAILURE_COUNT" -gt 0 ]]; then
  MEDIUM_COUNT=$((MEDIUM_COUNT + 1))
fi

if [[ "$DUP_COUNT" -gt 1 ]]; then
  LOW_COUNT=$((LOW_COUNT + 1))
fi

TOTAL_COUNT=$((HIGH_COUNT + MEDIUM_COUNT + LOW_COUNT))

cat >"$REPORT_PATH" <<EOF
# Dogfood Report: Scry smoke QA

| Field | Value |
|-------|-------|
| **Date** | $DATE_STR |
| **App URL** | $BASE_URL |
| **Session** | $SESSION |
| **Scope** | Unauthenticated smoke test (landing, sign-in validation, sign-up transition) |

## Summary

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | $HIGH_COUNT |
| Medium | $MEDIUM_COUNT |
| Low | $LOW_COUNT |
| **Total** | **$TOTAL_COUNT** |

## Findings
EOF

if [[ "$CF_CHALLENGE" == "true" ]]; then
  cat >>"$REPORT_PATH" <<EOF

### ISSUE-001: Cloudflare challenge blocks unattended sign-up flow

| Field | Value |
|-------|-------|
| **Severity** | high |
| **Category** | functional |
| **URL** | ${SIGNUP_URL:-$SIGN_IN_URL} |
| **Repro Video** | N/A |

**Description**

Sign-up transition from sign-in encountered Cloudflare bot verification, which blocks unattended UAT on this host.

**Evidence**

![Result](screenshots/sign-up.png)

---
EOF
fi

if [[ "$AB_FAILURE_COUNT" -gt 0 ]]; then
  cat >>"$REPORT_PATH" <<EOF

### ISSUE-002: Browser automation command failures during smoke run

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | performance |
| **URL** | $SIGN_IN_URL |
| **Repro Video** | N/A |

**Description**

$AB_FAILURE_COUNT agent-browser command(s) failed during this run. The report was still produced, but reliability of individual steps may be reduced.

---
EOF
fi

if [[ "$DUP_COUNT" -gt 1 ]]; then
  cat >>"$REPORT_PATH" <<EOF

### ISSUE-003: Unknown-account error message appears multiple times

| Field | Value |
|-------|-------|
| **Severity** | low |
| **Category** | ux |
| **URL** | $SIGN_IN_URL |
| **Repro Video** | N/A |

**Description**

The unknown-account message 'Couldn't find your account.' appeared **$DUP_COUNT** time(s) after submitting unknown credentials.

**Evidence**

![Result](screenshots/login-unknown-account.png)

---
EOF
fi

if [[ "$TOTAL_COUNT" -eq 0 ]]; then
  cat >>"$REPORT_PATH" <<'EOF'

No actionable findings were detected in this smoke run.
EOF
fi

cat >>"$REPORT_PATH" <<EOF

## Artifacts

- report: $REPORT_PATH
- screenshots: $OUT_DIR/screenshots
- diagnostics: $DUP_EVAL_JSON, $CF_EVAL_JSON
EOF

echo "✓ Report written: $REPORT_PATH"
echo "✓ Screenshots:    $OUT_DIR/screenshots"

if [[ "$FAIL_ON_HIGH" == "true" ]] && [[ "$HIGH_COUNT" -gt 0 ]]; then
  echo "High/critical findings detected and --fail-on-high was set." >&2
  exit 2
fi
