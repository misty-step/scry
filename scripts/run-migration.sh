#!/bin/bash
# Migration Runner with Safety Checks
#
# Usage:
#   ./scripts/run-migration.sh <migration-name> <environment>
#
# Examples:
#   ./scripts/run-migration.sh migrateQuestionsToConceptsV2 dev
#   ./scripts/run-migration.sh migrateQuestionsToConceptsV2 production
#
# Safety features:
# - Enforces dry-run first
# - Requires manual confirmation before actual migration
# - Validates environment target
# - Runs diagnostic query after completion

set -e  # Exit on error

MIGRATION_NAME=$1
ENVIRONMENT=$2

if [ -z "$MIGRATION_NAME" ] || [ -z "$ENVIRONMENT" ]; then
  echo "Usage: ./scripts/run-migration.sh <migration-name> <environment>"
  echo "Example: ./scripts/run-migration.sh migrateQuestionsToConceptsV2 dev"
  exit 1
fi

# Validate environment
if [ "$ENVIRONMENT" != "dev" ] && [ "$ENVIRONMENT" != "production" ]; then
  echo "Error: Environment must be 'dev' or 'production'"
  exit 1
fi

# Set deploy key based on environment
if [ "$ENVIRONMENT" = "production" ]; then
  # Load production deploy key from .env.production
  if [ ! -f .env.production ]; then
    echo "Error: .env.production not found"
    exit 1
  fi

  export CONVEX_DEPLOY_KEY=$(grep CONVEX_DEPLOY_KEY .env.production | cut -d= -f2)

  if [ -z "$CONVEX_DEPLOY_KEY" ]; then
    echo "Error: CONVEX_DEPLOY_KEY not found in .env.production"
    exit 1
  fi

  # Verify it's the production key
  if [[ ! $CONVEX_DEPLOY_KEY == prod:* ]]; then
    echo "Error: CONVEX_DEPLOY_KEY must start with 'prod:' for production environment"
    exit 1
  fi

  BACKEND_NAME=$(echo $CONVEX_DEPLOY_KEY | cut -d: -f2 | cut -d'|' -f1)
  echo "🔒 Production mode: Targeting $BACKEND_NAME"
else
  # Dev environment uses local .convex/config
  echo "🔧 Dev mode: Using local .convex/config"
  BACKEND_NAME="dev"
fi

echo ""
echo "=========================================="
echo "Migration Runner: $MIGRATION_NAME"
echo "Environment: $ENVIRONMENT"
echo "Backend: $BACKEND_NAME"
echo "=========================================="
echo ""

# Step 1: Dry-run
echo "Step 1: Running dry-run (no mutations)..."
echo ""

npx convex run "migrations:$MIGRATION_NAME" --args '{"dryRun":true}'

if [ $? -ne 0 ]; then
  echo ""
  echo "❌ Dry-run failed. Fix errors before proceeding."
  exit 1
fi

echo ""
echo "✅ Dry-run completed successfully"
echo ""

# Step 2: Manual confirmation
echo "Step 2: Review the dry-run output above."
echo ""
read -p "Do you want to proceed with the actual migration? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
  echo "Migration cancelled by user"
  exit 0
fi

echo ""
echo "Step 3: Running actual migration..."
echo ""

npx convex run "migrations:$MIGRATION_NAME" --args '{"dryRun":false}'

if [ $? -ne 0 ]; then
  echo ""
  echo "❌ Migration failed. Check logs and consider rollback."
  exit 1
fi

echo ""
echo "✅ Migration completed successfully"
echo ""

# Step 4: Run diagnostic query (if exists)
DIAGNOSTIC_NAME="${MIGRATION_NAME}Diagnostic"
echo "Step 4: Running diagnostic query ($DIAGNOSTIC_NAME)..."
echo ""

npx convex run "migrations:$DIAGNOSTIC_NAME" 2>/dev/null || {
  # Fallback to checkMigrationStatus if specific diagnostic doesn't exist
  echo "No specific diagnostic found, trying checkMigrationStatus..."
  npx convex run "migrations:checkMigrationStatus" 2>/dev/null || {
    echo "No diagnostic queries available"
  }
}

echo ""
echo "=========================================="
echo "Migration Complete"
echo "=========================================="
echo ""
echo "Next steps:"
echo "1. Verify data in Convex dashboard"
echo "2. Test review flow in $ENVIRONMENT environment"
echo "3. Monitor Sentry for errors"
echo ""
