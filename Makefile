UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
WAILS_TAGS := -tags webkit2_41
endif

export SEED_EMAIL SEED_PASSWORD SEED_NAME

.PHONY: help dev build test lint verify db-up db-down seed seed-catalogs

help:
	@echo "Targets: dev build test lint verify db-up db-down seed seed-catalogs"

dev:
	@set -e; \
	if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	if [ -z "$$SECRET_KEY" ]; then unset SECRET_KEY; fi; \
	APP_ENV=development wails dev $(WAILS_TAGS)

build:
	@set -e; \
	if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	unset APP_ENV ALLOW_DEV_ORIGINS; \
	wails build $(WAILS_TAGS)

test:
	go test ./...
	cd frontend && pnpm test

lint:
	go vet ./...
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required:"; echo "$$unformatted"; exit 1; \
	fi
	cd frontend && pnpm lint

verify:
	scripts/verify.sh

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

seed:
	@test -n "$$SEED_EMAIL" || { echo "SEED_EMAIL is required"; exit 1; }
	@test -n "$$SEED_PASSWORD" || { echo "SEED_PASSWORD is required"; exit 1; }
	go run ./cmd/seed

seed-catalogs:
	go run ./cmd/seed-catalogs
