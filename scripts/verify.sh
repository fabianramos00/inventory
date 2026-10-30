#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

step() { printf '\n==> %s\n' "$1"; }

step "gofmt"
test -z "$(gofmt -l internal cmd main.go app.go)"

# app.go embeds frontend/dist, so build it before any Go compile step.
step "frontend: pnpm test / lint / build"
(
	cd frontend
	pnpm test
	pnpm lint
	pnpm build
)

step "go vet"
go vet ./...

step "go test (offline: SQLite + unit)"
INVENTORY_TEST_DATABASE_URL= go test ./... -count=1

if [ -n "${INVENTORY_TEST_DATABASE_URL:-}" ]; then
	step "go test (PostgreSQL: $INVENTORY_TEST_DATABASE_URL)"
	go test ./... -count=1
else
	step "go test (PostgreSQL skipped: INVENTORY_TEST_DATABASE_URL unset)"
fi

step "git diff --check"
git diff --check

printf '\nverify: all checks passed\n'
