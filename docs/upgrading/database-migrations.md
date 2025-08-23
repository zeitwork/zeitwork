# Database Migration Guide

This guide covers database schema migrations for the Zeitwork platform using PlanetScale PostgreSQL.

## Overview

Zeitwork uses PlanetScale for PostgreSQL hosting, which provides:

- Zero-downtime schema migrations
- Automatic backups and point-in-time recovery
- Branch-based database development
- Built-in high availability

## Migration Tools

The platform uses Drizzle ORM for schema management:

- Migration files: `packages/database/migrations/`
- Schema definitions: `packages/database/schema/`
- Configuration: `packages/database/drizzle.config.ts`

## Migration Process

### 1. Prepare Migration

```bash
cd packages/database

# Install dependencies
npm install

# Check current migration status
npm run db:status

# Generate new migration from schema changes
npm run db:generate
```

### 2. Test Migration in Development

```bash
# Create development branch in PlanetScale
pscale branch create zeitwork-production dev-migration

# Connect to dev branch
export DATABASE_URL="postgresql://username:password@host.connect.psdb.cloud/zeitwork-production/dev-migration?sslmode=require"

# Run migration on dev branch
npm run db:migrate

# Test the schema
psql "$DATABASE_URL" -c "\dt"
psql "$DATABASE_URL" -c "\d+ table_name"
```

### 3. Production Migration

```bash
#!/bin/bash
# run-production-migration.sh

# Configuration
PROD_DATABASE_URL="postgresql://username:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require"
MIGRATION_LOG="/var/log/zeitwork-migration-$(date +%Y%m%d-%H%M%S).log"

echo "=== Zeitwork Database Migration ===" | tee ${MIGRATION_LOG}
echo "Starting at: $(date)" | tee -a ${MIGRATION_LOG}

# Pre-migration backup point
echo "Creating backup point..." | tee -a ${MIGRATION_LOG}
# PlanetScale automatically maintains continuous backups

# Check current version
echo "Current schema version:" | tee -a ${MIGRATION_LOG}
psql "$PROD_DATABASE_URL" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 1;" | tee -a ${MIGRATION_LOG}

# Run migrations
cd packages/database
echo "Running migrations..." | tee -a ${MIGRATION_LOG}
npm run db:migrate 2>&1 | tee -a ${MIGRATION_LOG}

# Verify new version
echo "New schema version:" | tee -a ${MIGRATION_LOG}
psql "$PROD_DATABASE_URL" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 1;" | tee -a ${MIGRATION_LOG}

echo "Migration completed at: $(date)" | tee -a ${MIGRATION_LOG}
```

## Migration Types

### 1. Backward Compatible Migrations

These can be run before upgrading services:

```sql
-- Adding nullable columns
ALTER TABLE projects ADD COLUMN description TEXT;

-- Adding new tables
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Adding indexes
CREATE INDEX idx_projects_org_id ON projects(org_id);
```

### 2. Breaking Migrations

Require coordinated service upgrades:

```sql
-- Removing columns
ALTER TABLE projects DROP COLUMN deprecated_field;

-- Changing column types
ALTER TABLE instances ALTER COLUMN memory TYPE BIGINT;

-- Renaming columns
ALTER TABLE nodes RENAME COLUMN ip_address TO internal_ip;
```

### 3. Data Migrations

Migrations that modify existing data:

```sql
-- Backfill new column
UPDATE projects SET status = 'active' WHERE status IS NULL;

-- Transform data
UPDATE deployments
SET config = jsonb_set(config, '{version}', '"2.0"'::jsonb)
WHERE config->>'version' IS NULL;

-- Clean up old data
DELETE FROM sessions WHERE created_at < NOW() - INTERVAL '30 days';
```

## PlanetScale-Specific Features

### Branch-Based Development

```bash
# Create a development branch
pscale branch create zeitwork-production feature-branch

# Make schema changes on branch
pscale shell zeitwork-production feature-branch

# Create deploy request (pull request for databases)
pscale deploy-request create zeitwork-production feature-branch

# Review and deploy
pscale deploy-request deploy zeitwork-production 1
```

### Safe Migration Practices

```bash
# Enable safe migrations mode
pscale safe-migrations enable zeitwork-production

# This prevents:
# - Dropping tables with data
# - Removing columns in use
# - Breaking foreign key constraints
```

### Migration Monitoring

```bash
# Monitor migration progress
pscale deploy-request show zeitwork-production 1

# Check for blocking queries
psql "$DATABASE_URL" -c "
SELECT pid, now() - pg_stat_activity.query_start AS duration, query
FROM pg_stat_activity
WHERE (now() - pg_stat_activity.query_start) > interval '5 minutes';"

# Monitor table sizes during migration
psql "$DATABASE_URL" -c "
SELECT schemaname, tablename,
       pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;"
```

## Migration Strategies

### 1. Online Migration (Zero Downtime)

For large tables, use online migrations:

```sql
-- Create new column alongside old
ALTER TABLE instances ADD COLUMN cpu_new INTEGER;

-- Backfill in batches
DO $$
DECLARE
    batch_size INTEGER := 1000;
    offset_val INTEGER := 0;
BEGIN
    LOOP
        UPDATE instances
        SET cpu_new = cpu * 1000
        WHERE id IN (
            SELECT id FROM instances
            WHERE cpu_new IS NULL
            LIMIT batch_size
        );

        EXIT WHEN NOT FOUND;
        PERFORM pg_sleep(0.1); -- Prevent blocking
    END LOOP;
END $$;

-- Switch to new column
ALTER TABLE instances RENAME COLUMN cpu TO cpu_old;
ALTER TABLE instances RENAME COLUMN cpu_new TO cpu;

-- Drop old column later
ALTER TABLE instances DROP COLUMN cpu_old;
```

### 2. Blue-Green Migration

For complex migrations:

```bash
# 1. Create new tables with _new suffix
CREATE TABLE projects_new (LIKE projects INCLUDING ALL);

# 2. Migrate data
INSERT INTO projects_new SELECT * FROM projects;

# 3. In a transaction, swap tables
BEGIN;
ALTER TABLE projects RENAME TO projects_old;
ALTER TABLE projects_new RENAME TO projects;
COMMIT;

# 4. Drop old table after verification
DROP TABLE projects_old;
```

### 3. Staged Migration

For gradual rollout:

```sql
-- Stage 1: Add feature flag
ALTER TABLE organisations ADD COLUMN new_feature_enabled BOOLEAN DEFAULT FALSE;

-- Stage 2: Enable for specific orgs
UPDATE organisations SET new_feature_enabled = TRUE WHERE id IN ('org1', 'org2');

-- Stage 3: Enable for all
UPDATE organisations SET new_feature_enabled = TRUE;

-- Stage 4: Remove flag
ALTER TABLE organisations DROP COLUMN new_feature_enabled;
```

## Rollback Procedures

### Automatic Rollback

```bash
#!/bin/bash
# rollback-migration.sh

# Check last migration
LAST_VERSION=$(psql "$DATABASE_URL" -t -c "
    SELECT version FROM schema_migrations
    ORDER BY applied_at DESC LIMIT 1;"
)

echo "Rolling back migration: ${LAST_VERSION}"

# Rollback using Drizzle
cd packages/database
npm run db:rollback

# Verify rollback
psql "$DATABASE_URL" -c "
    SELECT version, applied_at FROM schema_migrations
    ORDER BY applied_at DESC LIMIT 5;"
```

### Manual Rollback

For complex rollbacks:

```sql
-- Example: Rollback column addition
ALTER TABLE projects DROP COLUMN IF EXISTS new_column;

-- Example: Rollback index creation
DROP INDEX IF EXISTS idx_projects_new;

-- Example: Restore renamed column
ALTER TABLE nodes RENAME COLUMN internal_ip TO ip_address;

-- Update migration tracking
DELETE FROM schema_migrations WHERE version = '0003_add_new_feature';
```

### Point-in-Time Recovery

With PlanetScale:

```bash
# Request point-in-time restore
pscale database restore-request create zeitwork-production \
    --restore-to "2024-01-15 10:30:00" \
    --branch restore-branch

# Verify restored data
pscale shell zeitwork-production restore-branch

# Switch to restored branch if needed
# Update DATABASE_URL in all services
```

## Best Practices

### 1. Migration Guidelines

- **Test in development branch first**
- **Keep migrations small and focused**
- **Always include rollback scripts**
- **Avoid locking operations during peak hours**
- **Monitor query performance after migration**

### 2. Schema Design

```sql
-- Use UUIDs for primary keys
id UUID PRIMARY KEY DEFAULT gen_random_uuid()

-- Include audit columns
created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
deleted_at TIMESTAMP -- For soft deletes

-- Use appropriate indexes
CREATE INDEX CONCURRENTLY idx_table_column ON table(column);

-- Foreign keys with proper cascading
FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
```

### 3. Migration Checklist

- [ ] Schema changes reviewed by team
- [ ] Migration tested in development branch
- [ ] Rollback script prepared and tested
- [ ] Performance impact assessed
- [ ] Monitoring alerts configured
- [ ] Documentation updated
- [ ] Customer impact evaluated

## Monitoring Migrations

### Real-Time Monitoring

```bash
#!/bin/bash
# monitor-migration.sh

while true; do
    clear
    echo "=== Migration Monitor ==="
    echo "Time: $(date)"

    # Check for long-running queries
    echo -e "\nLong-running queries:"
    psql "$DATABASE_URL" -c "
        SELECT pid, now() - query_start AS duration,
               left(query, 50) AS query
        FROM pg_stat_activity
        WHERE state = 'active'
        AND now() - query_start > interval '30 seconds';"

    # Table locks
    echo -e "\nTable locks:"
    psql "$DATABASE_URL" -c "
        SELECT relation::regclass, mode, granted
        FROM pg_locks
        WHERE relation IS NOT NULL
        AND mode != 'AccessShareLock';"

    # Migration status
    echo -e "\nLatest migrations:"
    psql "$DATABASE_URL" -c "
        SELECT version, applied_at
        FROM schema_migrations
        ORDER BY applied_at DESC
        LIMIT 3;"

    sleep 5
done
```

### Post-Migration Validation

```bash
#!/bin/bash
# validate-migration.sh

echo "=== Post-Migration Validation ==="

# Check table structure
tables=(projects deployments instances nodes)
for table in "${tables[@]}"; do
    echo "Table: $table"
    psql "$DATABASE_URL" -c "\d $table" | head -20
done

# Verify constraints
psql "$DATABASE_URL" -c "
    SELECT conname, contype, conrelid::regclass
    FROM pg_constraint
    WHERE connamespace = 'public'::regnamespace
    ORDER BY conrelid::regclass;"

# Check indexes
psql "$DATABASE_URL" -c "
    SELECT indexname, tablename, indexdef
    FROM pg_indexes
    WHERE schemaname = 'public'
    ORDER BY tablename, indexname;"

# Test critical queries
echo -e "\nTesting critical queries..."
time psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM deployments;"
time psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM instances WHERE status = 'running';"
```

## Troubleshooting

### Migration Stuck

```bash
# Find blocking queries
psql "$DATABASE_URL" -c "
    SELECT pid, usename, pg_blocking_pids(pid) AS blocked_by, query
    FROM pg_stat_activity
    WHERE pg_blocking_pids(pid)::text != '{}';"

# Kill blocking query (use carefully)
psql "$DATABASE_URL" -c "SELECT pg_terminate_backend(PID);"
```

### Schema Conflicts

```bash
# Check for conflicts
psql "$DATABASE_URL" -c "
    SELECT * FROM schema_migrations
    WHERE version IN (SELECT version FROM schema_migrations
                     GROUP BY version HAVING COUNT(*) > 1);"

# Fix duplicate migrations
DELETE FROM schema_migrations
WHERE version = 'duplicate_version'
AND applied_at != (SELECT MIN(applied_at) FROM schema_migrations WHERE version = 'duplicate_version');
```

### Performance Issues After Migration

```sql
-- Update statistics
ANALYZE table_name;

-- Rebuild indexes
REINDEX TABLE table_name;

-- Check query plans
EXPLAIN ANALYZE SELECT * FROM table_name WHERE condition;
```

## Related Documentation

- [Platform Upgrade Guide](./platform-upgrade.md)
- [Operator Upgrade Guide](./operator-upgrade.md)
- [Rollback Procedures](./rollback-procedures.md)
- [PlanetScale Documentation](https://docs.planetscale.com)
