#!/bin/bash
# Migration Validation Helper
#
# Runs a series of queries to validate migration quality
# Use after running migration on dev environment

set -e

echo "=========================================="
echo "Migration Validation Report"
echo "=========================================="
echo ""

echo "1. Migration Status..."
npx convex run migrations:checkMigrationStatus
echo ""

echo "2. Sample of created concepts (first 5)..."
npx convex run migrations:sampleConcepts --args '{"limit":5}' 2>/dev/null || {
  echo "Note: sampleConcepts query not yet implemented"
}
echo ""

echo "3. Cluster size distribution..."
echo "   (Check Convex dashboard → Data → concepts for phrasingCount distribution)"
echo ""

echo "4. Check for concepts with no phrasings..."
echo "   Query: concepts where phrasingCount === 0"
echo ""

echo "=========================================="
echo "Manual Validation Checklist"
echo "=========================================="
echo ""
echo "✓ All orphaned questions migrated (orphaned count = 0)"
echo "✓ Concept titles are semantic and atomic (no 'and', 'vs')"
echo "✓ Phrasings within concepts are actually related"
echo "✓ FSRS state preserved correctly (check a concept that was reviewed)"
echo "✓ Review flow works (try reviewing a migrated concept)"
echo "✓ Interaction recording works (answer a question, check history)"
echo ""
echo "To manually inspect concepts:"
echo "  - Convex dashboard → Data → concepts"
echo "  - Pick random concepts and check phrasings table"
echo "  - Verify question similarity within each concept"
echo ""
