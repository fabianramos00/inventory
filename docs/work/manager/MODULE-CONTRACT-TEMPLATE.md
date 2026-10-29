# Module Contract — <name>

Status: draft | frozen
Owner: manager
Frozen at: <commit or date>
Applies to: backend, frontend, qa, database (review)

Freeze this document before implementation. After freeze, any change requires
a manager decision and a contract diff in the commit.

## Scope

- Module:
- Endpoints:
- In scope:
- Out of scope:
- Legacy reference files:

## Endpoints

| Method | Path | Auth | Request | Response | Status |
|---|---|---|---|---|---|
| GET | | active user | query | | 200 |
| POST | | active user | body | | 200 |
| PUT | | active user | body | | 200 |
| DELETE | | active user | path | | 200 |

## Request keys

| Key | Type | Required | Null allowed | Omitted behavior | Validation |
|---|---|---|---|---|---|
| | | | | | |

## Response keys

| Key | Type | Always present | Notes |
|---|---|---|---|
| | | | |

Money: JSON number, `<precision>` decimals.
Timestamps: RFC 3339 UTC.
Pagination: `{items, has_next, skip, limit}`.
Ordering: `<column> DESC|ASC`.

## Errors

| Case | Status | Code | Detail |
|---|---|---|---|
| Validation | 422 | `VALIDATION_ERROR` | |
| Missing | 404 | | |
| Duplicate | 400 | `UNIQUE_VIOLATION` | |
| Reference | 400 | `FOREIGN_KEY_VIOLATION` | |
| Insufficient stock | 400 | | |

## Enums and casing

| API value | Database value | Notes |
|---|---|---|
| | | |

## Database differences

| Concern | PostgreSQL | SQLite | Adapter rule |
|---|---|---|---|
| Money | `NUMERIC(12,2)` | integer cents | |
| Timestamps | `timestamptz` | fixed-width UTC text | |
| Search | `strpos(lower(...))` | Go case folding | literal `%`/`_` |
| Row locks | `FOR UPDATE` | single connection | |
| Enums | uppercase | uppercase | map at boundary |

## Frontend mapping

- Types:
- API module:
- Query keys:
- Invalidation keys:
- Routes:
- Cursor shape:
- Null/omitted form behavior:

## Test fixtures

| Case | Request | Expected |
|---|---|---|
| Create minimum | | |
| Create full | | |
| Duplicate | | |
| Missing reference | | |
| List default | | |
| List search | | |
| List filters | | |
| Update partial | | |
| Update clear | | |
| Delete conflict | | |
| Money exactness | | |
| Timestamps | | |

## Acceptance criteria

- [ ] Contract frozen before implementation
- [ ] Backend service/handler/repository tests pass on PostgreSQL + SQLite
- [ ] Frontend types and payloads match the contract
- [ ] Contract drift test added
- [ ] Docs updated
- [ ] `gofmt`, `go vet`, `go test`, `go test -race` pass
- [ ] `pnpm test`, `pnpm lint`, `pnpm build` pass
- [ ] `git diff --check` clean
- [ ] Parity matrix label is the weakest true label

## Handoffs

| From | To | Change | Acceptance | Blocked |
|---|---|---|---|---|
| | | | | |
