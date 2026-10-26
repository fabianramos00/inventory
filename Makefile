SHELL := /bin/sh

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
WAILS_TAGS := -tags webkit2_41
endif

export SEED_EMAIL SEED_PASSWORD SEED_NAME

.PHONY: help dev build test lint db-up db-down seed

help:
	@echo "Inventory Desktop"
	@echo ""
	@echo "Targets:"
	@echo "  dev       Run the Wails app in development mode"
	@echo "  build     Build the desktop app into build/bin/"
	@echo "  test      Run Go and frontend unit tests"
	@echo "  lint      Run go vet, a gofmt check, and frontend lint"
	@echo "  db-up     Start PostgreSQL 17 on localhost:5433"
	@echo "  db-down   Stop the compose database (keeps its volume)"
	@echo "  seed      Create or update a user; needs SEED_EMAIL and SEED_PASSWORD"
	@echo "  help      Show this help"
	@echo ""
	@echo "On Linux, dev and build pass -tags webkit2_41 (WebKitGTK 4.1)."

dev:
	APP_ENV=development wails dev $(WAILS_TAGS)

build:
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

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

seed:
	@test -n "$$SEED_EMAIL" || { echo "SEED_EMAIL is required"; exit 1; }
	@test -n "$$SEED_PASSWORD" || { echo "SEED_PASSWORD is required"; exit 1; }
	go run ./cmd/seed
