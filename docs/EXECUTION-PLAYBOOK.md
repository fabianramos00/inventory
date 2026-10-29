# Execution Playbook — Faster, Parallel, Evidence-Based Delivery

Status: active
Owner: manager
Applies to: every Phase 3 module after Sales

## Why this exists

The Products → Clients → Sales phase delivered correctly, but it took longer
than necessary. The main causes were contract drift discovered late, sequential
backend → frontend handoffs, docs claiming coverage before a run, and oversized
agent tasks. This playbook defines the operating rules that remove that
rework.

Legitimate verification is not the target. The target is avoiding a second
round of the same work.

## Operating model

1. **Contract first.** Freeze the request/response contract before backend or
   frontend implementation starts.
2. **Parallel by ownership.** Run tasks in parallel only when their file scopes
   do not overlap.
3. **Manager is the hub.** Subagents do not call or message each other
   directly. The manager collects results and relays cross-scope handoffs.
4. **Evidence before claims.** A module is "covered" only after the command
   that proves it has run.
5. **One verification command.** Prefer a single script that runs backend,
   frontend, lint, and build checks.
6. **Small tasks, explicit acceptance criteria.** One module or one
   cross-cutting concern per handoff.

## Contract-first gate

Before implementation, write a module contract using
[MODULE-CONTRACT-TEMPLATE.md](MODULE-CONTRACT-TEMPLATE.md). It must include:

- every endpoint and method;
- request keys, required/optional, null vs omitted;
- response keys and exact JSON types;
- status codes and error codes;
- enums and casing at the API boundary vs the database;
- money precision and serialization;
- timestamp format and timezone;
- pagination shape (`items`, `has_next`, `skip`, `limit`);
- search columns and wildcard behavior;
- PostgreSQL vs SQLite differences;
- frontend type mapping and query keys;
- test fixtures.

No backend or frontend implementation starts until the contract is frozen.
Changes after the freeze require a manager decision and an explicit contract
diff.

### Why this matters

The following defects would have been caught at freeze time:

| Defect | Missed contract detail |
|---|---|
| `min_stock_quantity` vs `low_stock_threshold` | exact write key |
| `payment_status` sent to a server-derived field | request key ownership |
| `total`/`pages` vs `has_next` | pagination shape |
| `YYYY-MM-DD` vs RFC 3339 date filters | timestamp format |
| empty string vs null when clearing clients | null vs omitted |
| manual payment-status control with no effect | server-derived field |

## Parallel execution matrix

OpenCode runs subagents in parallel when the primary agent emits multiple
`task` calls in one message. Use that deliberately; do not parallelize writes
to the same files.

| Workstream | Can run in parallel with | Precondition | Writes |
|---|---|---|---|
| `product` | `ux-designer`, `database` (read-only) | Request is defined | `docs/work/product/**` |
| `ux-designer` | `product`, `database` (read-only) | Product scope is clear | `docs/work/ux/**` |
| `database` | `product`, `ux-designer`, `backend`, `frontend` | Schema question is defined | `docs/work/database/**` |
| `backend` | `frontend` (after contract freeze), `database`, `security` | Contract frozen; no shared files | Go, migrations, backend docs |
| `frontend` | `backend` (after contract freeze), `database`, `security` | Contract frozen; no shared files | `frontend/**`, frontend docs |
| `qa` (test authoring) | `backend`, `frontend` | Contract frozen | test files only |
| `qa` (integration verification) | `security`, `devops` (read-only) | Backend + frontend artifacts exist | test files only |
| `security` | implemented modules; `database` | Scope defined | `docs/work/security/**` |
| `devops` | `backend`, `frontend`, `qa` | Build/CI scope defined | build/CI files |
| `manager` | all read-only reviews | Contract frozen | `docs/work/manager/**` |

Rules:

- Maximum **2–3 concurrent developer tasks**. More concurrency increases
  conflict and review cost.
- Never run two agents that write the same path or the same file family.
- Backend and frontend may run in parallel only after the contract is frozen.
  Until then, frontend mocks the contract; it does not guess.
- QA may write tests in parallel, but integration verification starts only
  after both implementations exist.
- `database` and `security` are read-only; they can run alongside development
  and report findings through the manager.
- `product` and `ux-designer` run before development when behavior or UI is
  changing; they can run alongside each other when the request is independent.

## Agent communication protocol

OpenCode subagents have no direct messaging channel. The manager is the
message bus. Configure every handoff to be explicit and self-contained.

A subagent that needs another scope must return:

```text
HANDOFF
Target: frontend | backend | devops | qa | database | security | product | ux-designer
Reason: <one sentence>
Files: <exact paths>
Change: <what must change>
Acceptance: <how to verify>
Blocked: yes | no
```

The manager then:

1. decides whether the handoff is in scope;
2. delegates it to the owning agent;
3. relays the result back to the original agent if it is still running;
4. records the decision in `docs/work/manager/`.

Subagents must not edit another agent's scope, and must not wait for a direct
message that will never arrive.

## Verification gate

Run verification once per module, in this order:

```bash
# backend
gofmt -l internal cmd main.go app.go
go vet ./...
INVENTORY_TEST_DATABASE_URL="postgresql://inventory:inventory@localhost:5433/inventory_desktop_test" \
  go test ./... -count=1
go test -race ./... -count=1

# frontend
cd frontend
pnpm test
pnpm lint
pnpm build

# repository hygiene
git diff --check
```

Preferred follow-up: a single `scripts/verify.sh` (owned by `devops`) that runs
these checks and fails on the first error. Until it exists, the manager uses
the sequence above.

Do not mark a module "covered" until the relevant command has run. Distinguish:

- **Implemented** — code exists.
- **Backend contract covered** — Go service/handler/repository tests pass on
  PostgreSQL and SQLite.
- **Frontend covered** — Vitest/build pass.
- **Live legacy parity** — the FastAPI-vs-Go fixture run has actually executed.

The parity matrix must use the weakest true label.

## Docs and claims

- Update docs in the same change as the code.
- Do not claim Playwright coverage for specs excluded by `testMatch`.
- Do not claim legacy parity without a recorded run or fixture replay.
- Record intentional differences in `MIGRATION.md` and `DECISIONS.md`.
- If docs and code disagree, stop and resolve the conflict before merging.

## Reuse before rebuilding

Use the patterns that already exist:

- CRUD: catalog, clients, and users are the template.
- Money: `ADR-0016` and the product/sale adapters.
- Timestamps: `gormstore.Timestamp`.
- Search: PostgreSQL `strpos(lower(...))`; SQLite Go folding with a `ponytail:`
  note.
- Stock and transactions: `UnitOfWork` + `StockRepository` (`ADR-0018`).
- Validation: `domain.ErrValidation`, `NotFoundError`,
  `InsufficientStockError`.
- Frontend: `usePagedList`, `DataTable`, `Pagination`, `apiError`, `saleCache`.

Do not create a second abstraction for an existing pattern.

## Task sizing

Prefer this shape:

1. Contract freeze (manager + product/UX/database as needed).
2. Backend implementation (one module).
3. Frontend implementation in parallel or immediately after.
4. QA test authoring in parallel; integration verification after both.
5. One docs/commit pass.

Avoid:

- one prompt covering three modules;
- one agent editing both backend and frontend;
- QA only after everything is merged;
- docs written before verification;
- re-running the whole suite after every small edit when a targeted test
  suffices.

## Capacity and retries

Upstream capacity refusals (`ttft_admission_refused`) are environmental. On a
refusal:

1. wait 10–40 seconds;
2. retry the same task once;
3. if it fails again, split the task into smaller scopes;
4. do not duplicate the work in another agent.

## Metrics to track per phase

| Metric | Target |
|---|---|
| Contract defects found after freeze | 0 |
| First-pass QA verdict | pass or pass-with-docs |
| Rework rounds per module | ≤ 1 |
| Backend + frontend parallel time | ≥ 30% of implementation |
| Docs claims without a command | 0 |
| Modules verified on PostgreSQL + SQLite + race | 100% |

## Next-phase plan

### Providers + Supplier Catalogs (CRUD)

- Contract freeze: list shape, search columns, provider/product filters,
  supplier catalog unique behavior.
- Backend and frontend in parallel after freeze.
- Reuse the catalog/client CRUD pattern.
- QA: CRUD, filters, pagination, FK delete, money for `last_quoted_cost`.

### Purchase Orders (stock mutation)

- Contract freeze: create/update/delete, item mutations, receiving transition.
- Reuse `UnitOfWork` + `StockRepository`.
- Receiving is idempotent: only the first transition to `received` increments
  stock and upserts supplier catalogs.
- QA: concurrent receiving, no double stock, delete does not reverse stock.

### Dashboard (read-only aggregates)

- Contract freeze: date boundaries in `America/Bogota`, trend keys, money.
- Backend aggregates and frontend charts can run in parallel.
- QA: fixture aggregates, empty data, top-products quirks.

### Cross-cutting

- `devops`: create `scripts/verify.sh` and wire it into CI.
- `qa`: add contract fixtures for every module and execute the live parity run
  once credentials are available.
- `security`: review loopback/auth boundaries for the new sales/order flows.
