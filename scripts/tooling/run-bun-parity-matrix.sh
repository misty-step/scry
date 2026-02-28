#!/usr/bin/env bash
set -euo pipefail

TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
OUT="${1:-/tmp/bun-parity-$TIMESTAMP.md}"

if ! command -v bun >/dev/null 2>&1; then
  echo "bun is not installed. Install from https://bun.sh before running this matrix." >&2
  exit 1
fi

run_check() {
  local label="$1"
  local command="$2"

  set +e
  bash -lc "$command" >/tmp/bun-parity-last.out 2>/tmp/bun-parity-last.err
  local status=$?
  set -e

  if [[ "$status" -eq 0 ]]; then
    echo "| $label | \`$command\` | ✅ pass | |" >>"$OUT"
  else
    local note
    note="$(tail -n 1 /tmp/bun-parity-last.err | tr '|' '/' | tr -d '\r')"
    echo "| $label | \`$command\` | ❌ fail | ${note:-See stderr output} |" >>"$OUT"
  fi
}

cat >"$OUT" <<EOF
# Bun Parity Matrix

Generated: $(date -u +"%Y-%m-%d %H:%M:%SZ")

| Area | Command | Result | Notes |
|---|---|---|---|
EOF

run_check "Install" "bun install"
run_check "Dev command" "bun run dev --help"
run_check "Lint" "bun run lint"
run_check "Typecheck" "bun run tsc --noEmit"
run_check "Tests" "bun run test:ci"
run_check "QA smoke (local)" "bun run qa:dogfood:local -- --help"

echo "Wrote matrix: $OUT"
