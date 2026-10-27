// Package testdb holds helpers shared by Go tests that use a real PostgreSQL
// database. It is test support only and never linked into the application.
package testdb

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// AdvisoryLockKey serializes test suites that share one PostgreSQL database.
// Both the persistence contract suite and the httpapi auth integration test
// take it, so parallel package test binaries cannot truncate each other's rows.
const AdvisoryLockKey int64 = 7421337001

// Lock blocks until the advisory lock is held and returns a release function.
// The lock lives on a dedicated connection because advisory locks are
// session-scoped.
func Lock(ctx context.Context, dsn string) (func(), error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open lock connection: %w", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("acquire lock connection: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", AdvisoryLockKey); err != nil {
		conn.Close()
		db.Close()
		return nil, fmt.Errorf("acquire test database lock: %w", err)
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", AdvisoryLockKey)
		conn.Close()
		db.Close()
	}, nil
}
