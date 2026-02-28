#!/usr/bin/env bash
set -euo pipefail

TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
GENERATED_UTC="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
OUT="${1:-/tmp/bun-parity-${TIMESTAMP}-$$.md}"
PARITY_PORT="${PARITY_PORT:-3100}"
PARITY_URL="http://localhost:${PARITY_PORT}"

DEV_PID=""
DEV_STDOUT=""
DEV_STDERR=""

cleanup() {
  if [[ -n "$DEV_PID" ]] && kill -0 "$DEV_PID" >/dev/null 2>&1; then
    kill "$DEV_PID" >/dev/null 2>&1 || true
    wait "$DEV_PID" >/dev/null 2>&1 || true
  fi

  if [[ -n "$DEV_STDOUT" ]]; then
    rm -f "$DEV_STDOUT"
  fi

  if [[ -n "$DEV_STDERR" ]]; then
    rm -f "$DEV_STDERR"
  fi
}
trap cleanup EXIT

if ! command -v bun >/dev/null 2>&1; then
  echo "bun is not installed. Install from https://bun.sh before running this matrix." >&2
  exit 1
fi

sanitize_note() {
  tr '|' '/' | tr -d '\r' | sed '/^\s*$/d'
}

write_row() {
  local label="$1"
  local bun_cmd="$2"
  local result="$3"
  local note="$4"
  local owner_followup="$5"

  echo "| $label | \`$bun_cmd\` | $result | $note | $owner_followup |" >>"$OUT"
}

run_check() {
  local label="$1"
  local bun_cmd="$2"
  local owner_followup="${3:-TODO (#272): assign owner/follow-up}"

  local tmp_out
  local tmp_err
  tmp_out="$(mktemp)"
  tmp_err="$(mktemp)"

  set +e
  bash -c "$bun_cmd" >"$tmp_out" 2>"$tmp_err"
  local status=$?
  set -e

  if [[ "$status" -eq 0 ]]; then
    write_row "$label" "$bun_cmd" "✅ pass" "" ""
  else
    local note
    note="$(tail -n 5 "$tmp_err" | sanitize_note | paste -sd ' ' -)"
    write_row "$label" "$bun_cmd" "❌ fail" "${note:-See stderr output}" "$owner_followup"
  fi

  rm -f "$tmp_out" "$tmp_err"
}

start_dev_server_check() {
  local bun_cmd="PORT=${PARITY_PORT} bun run dev"

  DEV_STDOUT="$(mktemp)"
  DEV_STDERR="$(mktemp)"

  set +e
  bash -c "$bun_cmd" >"$DEV_STDOUT" 2>"$DEV_STDERR" &
  DEV_PID=$!
  set -e

  local ready=false
  for _ in {1..60}; do
    if curl -sf "$PARITY_URL" >/dev/null 2>&1; then
      ready=true
      break
    fi
    sleep 1
  done

  if [[ "$ready" == "true" ]]; then
    write_row "Dev startup" "$bun_cmd" "✅ pass" "Server responded at ${PARITY_URL}" ""
  else
    local note
    note="$(tail -n 5 "$DEV_STDERR" | sanitize_note | paste -sd ' ' -)"
    write_row "Dev startup" "$bun_cmd" "❌ fail" "${note:-Timed out waiting for ${PARITY_URL}}" "TODO (#272): investigate dev startup under Bun"

    if [[ -n "$DEV_PID" ]] && kill -0 "$DEV_PID" >/dev/null 2>&1; then
      kill "$DEV_PID" >/dev/null 2>&1 || true
      wait "$DEV_PID" >/dev/null 2>&1 || true
    fi
    DEV_PID=""
  fi
}

run_qa_smoke_check() {
  local bun_cmd="bun run qa:dogfood -- --url ${PARITY_URL}"

  if [[ -z "$DEV_PID" ]]; then
    write_row "QA smoke (local)" "$bun_cmd" "❌ fail" "Skipped because dev startup check did not pass" "TODO (#272): unblock dev startup before QA smoke parity"
    return
  fi

  run_check "QA smoke (local)" "$bun_cmd" "TODO (#272): investigate QA smoke parity failure"
}

cat >"$OUT" <<EOF
# Bun Parity Matrix

Generated: $GENERATED_UTC

| Area | Command | Result | Blocker note | Owner / follow-up |
|---|---|---|---|---|
EOF

run_check "Install" "bun install" "TODO (#272): investigate bun install parity"
run_check "Lint" "bun run lint" "TODO (#272): resolve Bun lint parity blocker"
run_check "Typecheck" "bun run tsc --noEmit" "TODO (#272): resolve Bun typecheck parity blocker"
run_check "Tests" "bun run test:ci" "TODO (#272): resolve Bun test parity blocker"
start_dev_server_check
run_qa_smoke_check

echo "Wrote matrix: $OUT"
