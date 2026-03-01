#!/usr/bin/env bash
# scripts/tooling/topology-hygiene-check.sh
# Verifies current root filesystem against documented inventory.

set -e

REPO_ROOT=$(git rev-parse --show-toplevel)
INVENTORY_FILE="$REPO_ROOT/docs/architecture/root-topology-inventory.md"
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# 1. Extract snapshot from inventory
sed -n '/^## Current root snapshot$/,/^## Classification and disposition$/p' "$INVENTORY_FILE" \
  | sed -n '/^```text$/,/^```$/p' \
  | grep -v '^```' \
  | sort -f > "$TEMP_DIR/documented_snapshot.txt"

# 2. Capture current root state (depth 1, strip trailing slashes)
(ls -1 -p "$REPO_ROOT" | sed 's/\/$//' && \
 ls -1 -a -p "$REPO_ROOT" | grep '^\.' | grep -v -E '^\.\.?/?$' | sed 's/\/$//') \
  | sort -f > "$TEMP_DIR/current_root.txt"

# 3. Compare
DIFF=$(diff -u "$TEMP_DIR/documented_snapshot.txt" "$TEMP_DIR/current_root.txt" || true)

if [ -n "$DIFF" ]; then
  echo "❌ ROOT TOPOLOGY DRIFT DETECTED"
  echo "Documented inventory in $INVENTORY_FILE is out of sync with filesystem."
  echo ""
  echo "Diff (-documented, +actual):"
  echo "$DIFF"
  echo ""
  echo "Action: update docs/architecture/root-topology-inventory.md section 'Current root snapshot'."
  exit 1
fi

echo "✅ Root topology hygiene check passed."
