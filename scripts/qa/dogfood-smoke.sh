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

BASE_URL="https://scry.study"
SESSION="scry-dogfood-$(date +%Y%m%d-%H%M%S)"
OUT_DIR="/tmp/dogfood-scry-$(date +%Y%m%d-%H%M%S)"
FAIL_ON_HIGH=false

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

mkdir -p "$OUT_DIR/screenshots" "$OUT_DIR/videos"

ab() {
  agent-browser --session "$SESSION" "$@"
}

safe_ab() {
  set +e
  ab "$@"
  local status=$?
  set -e
  return "$status"
}

LANDING_URL="$BASE_URL"
SIGN_IN_URL="${BASE_URL%/}/sign-in"

echo "→ Session: $SESSION"
echo "→ Output:  $OUT_DIR"
echo "→ Base URL: $BASE_URL"

safe_ab open "$LANDING_URL" >/dev/null
safe_ab wait --load domcontentloaded >/dev/null
safe_ab screenshot --annotate "$OUT_DIR/screenshots/initial.png" >/dev/null

safe_ab open "$SIGN_IN_URL" >/dev/null
safe_ab wait --load domcontentloaded >/dev/null
safe_ab screenshot --annotate "$OUT_DIR/screenshots/sign-in.png" >/dev/null

safe_ab click "button:has-text('Continue')" >/dev/null
safe_ab wait 1000 >/dev/null
safe_ab screenshot --annotate "$OUT_DIR/screenshots/login-empty-submit.png" >/dev/null

safe_ab fill "input[name='identifier']" "qa-bot@example.com" >/dev/null
safe_ab fill "input[name='password']" "wrongpassword123" >/dev/null
safe_ab click "button:has-text('Continue')" >/dev/null
safe_ab wait 4000 >/dev/null
safe_ab screenshot --annotate "$OUT_DIR/screenshots/login-unknown-account.png" >/dev/null

DUP_EVAL_JSON="$OUT_DIR/dup-eval.json"
if safe_ab eval "(() => { const txt = document.body?.innerText || ''; const needle = \"Couldn't find your account.\"; return { url: location.href, duplicateCount: Math.max(0, txt.split(needle).length - 1), hasError: txt.includes(needle) }; })()" >"$DUP_EVAL_JSON"; then
  DUP_COUNT="$(jq -r '.duplicateCount // 0' "$DUP_EVAL_JSON" 2>/dev/null || echo 0)"
else
  DUP_COUNT=0
fi

safe_ab click "a:has-text('Sign up')" >/dev/null
safe_ab wait --load domcontentloaded >/dev/null
safe_ab screenshot --annotate "$OUT_DIR/screenshots/sign-up.png" >/dev/null

CF_EVAL_JSON="$OUT_DIR/cloudflare-eval.json"
if safe_ab eval "(() => { const text = (document.body?.innerText || '').toLowerCase(); const title = (document.title || '').toLowerCase(); const isCloudflareChallenge = title.includes('just a moment') || text.includes('performing security verification') || text.includes('ray id'); return { url: location.href, title: document.title, isCloudflareChallenge }; })()" >"$CF_EVAL_JSON"; then
  CF_CHALLENGE="$(jq -r '.isCloudflareChallenge // false' "$CF_EVAL_JSON" 2>/dev/null || echo false)"
  SIGNUP_URL="$(jq -r '.url // ""' "$CF_EVAL_JSON" 2>/dev/null || echo "")"
else
  CF_CHALLENGE=false
  SIGNUP_URL=""
fi

DATE_STR="$(date +%Y-%m-%d)"
REPORT_PATH="$OUT_DIR/report.md"

HIGH_COUNT=0
MEDIUM_COUNT=0
LOW_COUNT=0

if [[ "$CF_CHALLENGE" == "true" ]]; then
  HIGH_COUNT=$((HIGH_COUNT + 1))
fi

if [[ "$DUP_COUNT" =~ ^[0-9]+$ ]] && [[ "$DUP_COUNT" -gt 1 ]]; then
  LOW_COUNT=$((LOW_COUNT + 1))
fi

MEDIUM_COUNT=$((MEDIUM_COUNT + 1))
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

## Issues

### ISSUE-001: Sign-up automation path reliability

| Field | Value |
|-------|-------|
| **Severity** | $( [[ "$CF_CHALLENGE" == "true" ]] && echo "high" || echo "medium" ) |
| **Category** | functional |
| **URL** | ${SIGNUP_URL:-$SIGN_IN_URL} |
| **Repro Video** | N/A |

**Description**

Sign-up transition was exercised from sign-in. $( [[ "$CF_CHALLENGE" == "true" ]] && echo "Cloudflare bot verification challenge was detected, which blocks unattended UAT on this host." || echo "No Cloudflare challenge detected in this run." )

**Repro Steps**

1. Open sign-in page.
   ![Step 1](screenshots/sign-in.png)

2. Click **Sign up**.
   ![Step 2](screenshots/login-unknown-account.png)

3. Observe resulting page.
   ![Result](screenshots/sign-up.png)

---

### ISSUE-002: Browser automation wait strategy

| Field | Value |
|-------|-------|
| **Severity** | medium |
| **Category** | performance |
| **URL** | $SIGN_IN_URL |
| **Repro Video** | N/A |

**Description**

For this smoke suite, 'domcontentloaded' is used as the default wait strategy because it is consistently reliable for auth surfaces with ongoing background network activity.

---

### ISSUE-003: Unknown-account error rendering duplication

| Field | Value |
|-------|-------|
| **Severity** | $( [[ "$DUP_COUNT" -gt 1 ]] && echo "low" || echo "low (not reproduced)" ) |
| **Category** | ux |
| **URL** | $SIGN_IN_URL |
| **Repro Video** | N/A |

**Description**

The unknown-account message 'Couldn't find your account.' appeared **$DUP_COUNT** time(s) in this run after submitting unknown credentials.

**Evidence**

![Result](screenshots/login-unknown-account.png)
EOF

echo "✓ Report written: $REPORT_PATH"
echo "✓ Screenshots:    $OUT_DIR/screenshots"

if [[ "$FAIL_ON_HIGH" == "true" ]] && [[ "$HIGH_COUNT" -gt 0 ]]; then
  echo "High/critical findings detected and --fail-on-high was set." >&2
  exit 2
fi
