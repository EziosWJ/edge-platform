# SQLite migrations

This stream has the same logical version numbers as the PostgreSQL stream in
the sibling `schema/` and `seed/` directories. SQLite uses `INTEGER PRIMARY
KEY AUTOINCREMENT`, `DATETIME`, portable constraints and application-level
checksum validation where PostgreSQL's DDL would require a dialect-specific
operator. The migration command selects this directory only when
`database.driver: sqlite` is explicit.
