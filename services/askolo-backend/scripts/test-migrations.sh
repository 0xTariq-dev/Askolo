#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
service_root="$(cd -- "$script_dir/.." && pwd)"
cd -- "$service_root"

initdb_bin="$(command -v initdb || true)"
pg_ctl_bin="$(command -v pg_ctl || true)"
if [[ -z "$initdb_bin" || -z "$pg_ctl_bin" ]]; then
  echo "PostgreSQL initdb and pg_ctl are required for the disposable migration gate" >&2
  exit 1
fi

postgres_version="$("$initdb_bin" --version)"
postgres_version_suffix="${postgres_version##*PostgreSQL}"
postgres_version_suffix="${postgres_version_suffix//)/}"
postgres_version_suffix="${postgres_version_suffix// /}"
if [[ "$postgres_version_suffix" != 16.* ]]; then
  echo "The migration release gate requires PostgreSQL 16.x; found: $postgres_version" >&2
  exit 1
fi

test_root="$(mktemp -d "${TMPDIR:-/tmp}/askolo-migration-check.XXXXXX")"
data_dir="$test_root/data"
socket_dir="$test_root/socket"
server_log="$test_root/postgres.log"
db_user="$(id -un)"
database_url=""

cleanup() {
  local status=$?
  if [[ -d "$data_dir" ]]; then
    "$pg_ctl_bin" -D "$data_dir" -m fast -w stop >/dev/null 2>&1 || true
  fi
  if [[ "$status" -ne 0 && -f "$server_log" ]]; then
    echo "Disposable PostgreSQL log tail:" >&2
    tail -n 30 "$server_log" >&2 || true
  fi
  rm -rf -- "$test_root"
  return "$status"
}
trap cleanup EXIT

mkdir -p -- "$socket_dir"
"$initdb_bin" -D "$data_dir" -U "$db_user" \
  --auth-local=trust --auth-host=reject --no-sync \
  >"$test_root/initdb.log" 2>&1
"$pg_ctl_bin" -D "$data_dir" -l "$server_log" \
  -o "-c listen_addresses='' -c unix_socket_directories='$socket_dir' -c port=5432" \
  -w start >/dev/null

encoded_socket_dir="${socket_dir//\//%2F}"
database_url="postgresql://$db_user@/postgres?host=$encoded_socket_dir&port=5432"

# Never let the normal application database URL become an integration-test
# target. The test suite receives only this script's disposable local URL.
env -u DATABASE_URL ASKOLO_TEST_DATABASE_URL="$database_url" \
  go test ./internal/migrations -count=1

# Exercise explicit target selection, including the restore gate, against this
# disposable database only.
if output="$(env -u ASKOLO_MIGRATION_APPROVED DATABASE_URL="$database_url" \
  go run ./cmd/askolo-migrate -target restore 2>&1)"; then
  echo "restore target unexpectedly ran without explicit approval" >&2
  exit 1
elif [[ "$output" != *"refusing restore target without ASKOLO_MIGRATION_APPROVED=yes"* ]]; then
  printf '%s\n' "$output" >&2
  echo "restore-target refusal failed for an unexpected reason" >&2
  exit 1
fi

env DATABASE_URL="$database_url" go run ./cmd/askolo-migrate -target development
env DATABASE_URL="$database_url" ASKOLO_MIGRATION_APPROVED=yes \
  go run ./cmd/askolo-migrate -target restore

echo "Disposable PostgreSQL migration and restore-target checks passed"