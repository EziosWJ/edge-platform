# Seed migrations

Authentication owns the initial root department, administrator, `ADMIN` role,
menus, and their relationships. Later modules may append their own built-in
data only after the corresponding schema migration exists.

Seed migrations must:

- use normal versioned Goose SQL files;
- contain built-in data only, never table or index definitions;
- run after all schema migrations, using the separate
  `goose_seed_db_version` version table;
- remain one-time migrations and never be replayed by API startup logic.

Do not add placeholder or speculative data. Each owning migration issue must
define the tables and seed contract together.
