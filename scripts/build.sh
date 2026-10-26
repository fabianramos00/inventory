#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ -f .env ]; then
	set -a
	. ./.env
	set +a
fi

# Release builds must never include dev origins.
unset APP_ENV ALLOW_DEV_ORIGINS

exec make build
