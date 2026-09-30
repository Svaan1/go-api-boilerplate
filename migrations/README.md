# PostgreSQL migrations

Store versioned schema changes here as paired, timestamped files:

```text
YYYYMMDDHHMMSS_short_description.up.sql
YYYYMMDDHHMMSS_short_description.down.sql
```

Use unique, increasing UTC timestamps. Each `.up.sql` applies one deliberate schema change; its matching `.down.sql` reverses that change where reversal is safe. The first application migration establishes the initial schema, with its timestamp as the migration version. Do not add an empty, no-op, or placeholder baseline migration: no domain schema exists yet.

Before review, verify both directions against a disposable PostgreSQL database, inspect generated SQL, and consider existing data, locks, table size, indexes, constraints, and deployment compatibility. Prefer additive expand/contract changes when old and new application versions may run concurrently. Destructive or data-transforming migrations need an explicit recovery plan and verified backup; a down migration cannot restore discarded data. Never edit a migration after it has been applied in a shared environment. Add a new forward migration instead.

Apply and roll back through the repository's migration targets, using `APP_DATABASE_URL` for the target database. Confirm target environment and backup before production changes. Roll back one version at a time only after assessing effects on writes made since deployment; restore from backup or forward-fix when rollback would lose data. Keep credentials out of migration files and command output.

`sqlc.yaml` reads schema only from `.up.sql` files and queries from `internal/database/query/`. Both locations intentionally contain no SQL until the first real application schema and query exist. Do not run sqlc generation or commit generated code before then. Add reviewed schema and parameterized queries first, then generate the pgx/v5 package.
