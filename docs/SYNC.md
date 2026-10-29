# Sync Design Constraints

Cross-device sync is **not implemented**. This document records the constraints
that future work must respect so the current data model does not make sync
impossible.

## Current behavior

- PostgreSQL is the primary database.
- `DB_MODE=auto` falls back to local SQLite when PostgreSQL is unreachable.
- SQLite contains the auth tables and the full business schema. Catalog,
  products, clients, providers, supplier catalogs, sales, purchase orders, and
  the dashboard are implemented offline. Sync is still not implemented.
- Local writes are not sent anywhere. An offline session is a separate
  standalone dataset until sync exists.
- Auth sessions are per installation. Do not assume the same user exists in
  both databases.

## Non-negotiable constraints for future sync

1. **Stable global identity.** Local integer primary keys are not enough.
   Every syncable record needs a globally unique identifier (`sync_id` UUIDv7
   is the default candidate) that survives merges and re-keying.
2. **Reliable timestamps.** Every syncable record needs `updated_at` in UTC and
   a monotonic per-device sequence. Do not use wall-clock time alone to order
   writes.
3. **Tombstones.** Deletes cannot simply disappear locally; they need a
   `deleted_at` or a tombstone record so the delete can propagate.
4. **Outbox.** Local writes go into a durable `sync_outbox` in the same
   transaction as the business write. Sync retries the outbox; it does not
   scan tables looking for “new” rows.
5. **Idempotent apply.** Every remote change is applied by `sync_id` with an
   idempotency key. Replaying the same change must be safe.
6. **Stock is not last-write-wins.** Syncing `stock_quantity` as a scalar will
   corrupt stock across devices. Model stock as an append-only movement
   ledger (`sale`, `sale_cancel`, `purchase_receipt`, `adjustment`) and derive
   the quantity. The current schema stores mutable stock; that must be
   revisited before stock sync.
7. **Money is decimal text.** Sync payloads must carry money as decimal
   strings, never binary floats.
8. **Conflict policy is per entity.** Catalog names and contact data can use a
   documented last-writer rule. Sales and purchase orders are append-only
   business events; conflicts must be rejected or resolved explicitly, not
   overwritten.
9. **Device registration.** A device needs an identity, a sync cursor, and a
   revocation path. Do not use a raw user password or access token as the sync
   credential.
10. **Offline authentication is local.** The SQLite user table is local. A
    future design must decide how users and roles are provisioned offline and
    whether credentials ever sync. Do not sync refresh tokens.

## Recommended schema additions before sync

Do not add these speculatively now. Add them with the first sync feature, in a
dedicated migration, after the protocol is designed:

```text
sync_id        UUID/TEXT UNIQUE
updated_at     timestamptz / TEXT UTC
deleted_at     timestamptz / TEXT UTC NULL
version        integer
sync_outbox    id, entity, sync_id, operation, payload, created_at, attempts
sync_cursor    device_id, last_pulled_sequence
```

## Security

- The SQLite file is local and not encrypted at rest. It is protected by
  owner-only permissions, not by encryption.
- A future sync client must authenticate devices, use TLS, validate server
  signatures or certificates, and avoid logging payloads with customer PII.
- A stolen device exposes local business data and any cached tokens.

## Testing requirements for the future feature

- Offline create/edit/delete, then reconnect and sync.
- Duplicate delivery of the same outbox item.
- Two devices editing the same client/product.
- Two devices selling the last stock unit.
- Clock skew and out-of-order delivery.
- Delete/restore and tombstone retention.
- Schema upgrade on both PostgreSQL and SQLite.

## Out of scope now

- Server endpoints for sync.
- Device registration.
- Conflict UI.
- Sync of auth tokens.
- Encryption-at-rest for SQLite.
