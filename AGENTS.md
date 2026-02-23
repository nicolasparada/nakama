# Guidelines

## Database stack

- Use `github.com/jackc/pgx/v5` for CockroachDB access.
- Prefer `github.com/jackc/pgxutil` where practical.
- Use `const` SQL strings when the query is static.
- Use `pgx.StrictNamedArgs` for query params.

## `cockroach` package boundaries

- `cockroach` is for data access only (no business logic).
- Keep method names straightforward (`create`, `update`, `delete`, etc.).
- One method = one SQL query.
- If multiple queries are needed, add a higher-level wrapper method in `cockroach`.
- Prefer one file per table; place cross-table tx methods where they fit best.
- Avoid exposing data structs from `cockroach`; if a shared type is needed, add it to `types`.

## Transactions

- Callers should not pass transaction references.
- Encapsulate transaction handling inside `cockroach` methods.
- Do not use `cockroach.ExecuteTx` (legacy; planned for removal).

## Method shape

- Method signature: `ctx` first, then optional input struct when needed.
- Return either `error` or `(result, error)`.
- Use result structs for multi-value outputs.
- Avoid exposing internal DB types (for example, prefer `*string` over `sql.NullString`).

## Error handling

- Use unique error context messages.
- Map common DB errors (no rows, not-null, FK, unique) to `types/errs`.
- Use `github.com/nicolasparada/go-db` utilities to classify DB errors.
