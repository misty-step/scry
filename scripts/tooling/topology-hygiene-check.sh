#!/usr/bin/env bash
# scripts/tooling/topology-hygiene-check.sh
# Verifies current root filesystem against documented inventory.

set -euo pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)
INVENTORY_FILE="$REPO_ROOT/docs/architecture/root-topology-inventory.md"
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

if [ ! -f "$INVENTORY_FILE" ]; then
  echo "❌ Inventory file not found: $INVENTORY_FILE"
  exit 1
fi

# 1. Extract snapshot from inventory
# Robust range match using start/end anchors
sed -n '/^## Current root snapshot$/,/^## Classification and disposition$/p' "$INVENTORY_FILE" \
  | sed -n '/^```text$/,/^```$/p' \
  | grep -v '^```' \
  | sort -f > "$TEMP_DIR/documented_snapshot.txt"

if [ ! -s "$TEMP_DIR/documented_snapshot.txt" ]; then
  echo "❌ Failed to extract 'Current root snapshot' from $INVENTORY_FILE"
  echo "Ensure the section is wrapped in \`\`\`text and ends before '## Classification and disposition'."
  exit 1
fi

# 2. Capture current root state (depth 1, strip trailing slashes)
(ls -1 -p "$REPO_ROOT" | sed 's/\/$//' && \
 ls -1 -a -p "$REPO_ROOT" | grep '^\.' | grep -v -E '^\.\.?/?$' | sed 's/\/$//') \
  | sort -f > "$TEMP_DIR/current_root.txt"

if [ ! -s "$TEMP_DIR/current_root.txt" ]; then
  echo "❌ Failed to capture current root filesystem state."
  exit 1
fi

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
