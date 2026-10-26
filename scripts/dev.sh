#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ -f .env ]; then
	set -a
	. ./.env
	set +a
fi

# A blank SECRET_KEY in .env means "generate one"; an empty exported variable
# would otherwise be treated as a supplied (invalid) secret.
if [ -z "${SECRET_KEY:-}" ]; then
	unset SECRET_KEY
fi

export APP_ENV=development

exec make dev
