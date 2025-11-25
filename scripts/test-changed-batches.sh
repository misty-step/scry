#!/usr/bin/env bash
# Run Vitest on changed test files in small batches to avoid worker OOMs when
# many files are touched (e.g., config changes invalidate the cache).
set -euo pipefail

fallback() {
  git rev-parse --verify "$1" >/dev/null 2>&1
}

BASE_REF=${BASE_REF:-origin/master}
if ! fallback "$BASE_REF"; then
  if fallback master; then
    BASE_REF=master
  elif fallback main; then
    BASE_REF=main
  else
    BASE_REF=$(git rev-list --max-parents=0 HEAD)
  fi
fi
BATCH_SIZE=${BATCH_SIZE:-10}
NODE_OPTIONS=${NODE_OPTIONS:-"--max-old-space-size=4096"}

merge_base=$(git merge-base "$BASE_REF" HEAD)
changed_tests=$(git diff --name-only "$merge_base"...HEAD \
  -- '*.test.ts' '*.test.tsx' '*.test.js' '*.test.jsx' \
  '*.spec.ts' '*.spec.tsx' '*.spec.js' '*.spec.jsx')

if [[ -z "$changed_tests" ]]; then
  echo "No changed test files detected relative to $BASE_REF; skipping batch run."
  exit 0
fi

file_count=$(echo "$changed_tests" | wc -l | tr -d ' ')
if [[ $file_count -gt 50 ]]; then
  echo "⚠️ Too many changed files ($file_count), likely comparing to initial commit. Skipping batched test run."
  echo "Run 'pnpm test' manually to verify all tests pass."
  exit 0
fi

echo "Running Vitest on changed test files relative to $BASE_REF (batch size $BATCH_SIZE)..."

batch=()
count=0

run_batch() {
  if [[ ${#batch[@]} -eq 0 ]]; then
    return
  fi
  echo "→ vitest run ${batch[*]}"
  NODE_OPTIONS=$NODE_OPTIONS \
  SKIP_HEAVY_TESTS=1 \
  pnpm exec vitest run --pool=forks --maxWorkers=1 "${batch[@]}"
  batch=()
  count=0
}

while IFS= read -r file; do
  if [[ -f "$file" ]]; then
    # Skip Playwright e2e specs (handled by dedicated runner)
    if [[ "$file" == tests/e2e/* ]]; then
      continue
    fi
    batch+=("$file")
    count=$((count + 1))
    if [[ $count -ge $BATCH_SIZE ]]; then
      run_batch
    fi
  fi
done <<< "$changed_tests"

# Run any remaining files
run_batch
