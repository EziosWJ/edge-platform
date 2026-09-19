# Database migrations

Migrations are deliberately split by responsibility and database dialect:

- `schema/` is the PostgreSQL stream retained at its historical path; it
  contains tables, indexes, constraints, and other schema changes.
- `seed/` is the PostgreSQL built-in-data stream retained at its historical
  path.
- `sqlite/schema/` and `sqlite/seed/` are the corresponding SQLite streams.
  Their logical version numbers must remain identical to the PostgreSQL
  streams even when DDL differs.

Run schema migrations before seed migrations. Each directory must use its own
Goose version table (`goose_schema_db_version` and
`goose_seed_db_version`), so the same migration number in one stream cannot
mark a migration in the other stream as applied. The selected database driver
chooses the dialect stream; PostgreSQL continues to read the historical paths
so already deployed version tables upgrade without being reset.

The API process must not run either stream automatically. The explicit migrate
command and deployment workflow own migration execution.
