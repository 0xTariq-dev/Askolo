#!/usr/bin/env bash
set -Eeuo pipefail

# Run the database-backed native email auth and cleanup readiness checks
# against a disposable PostgreSQL instance. The test suites use a
# capture-only sender, so this validation never contacts a real email
# provider.

die() {
  echo "native email auth and cleanup readiness validation failed: $*" >&2
  exit 1
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
service_root="$repo_root/services/askolo-backend"

if [[ "${GOSUMDB:-}" == "off" ]]; then
  export GOSUMDB=sum.golang.org
fi

[[ -d "$service_root" ]] || die "backend service directory not found"

temporary_root=""
pg_data=""
pg_started=false
pg_ctl_path=""
pg_start_options=""

cleanup() {
  local exit_code=$?

  if [[ "$pg_started" == true ]]; then
    echo "Stopping disposable PostgreSQL instance"
    pg_ctl -D "$pg_data" -m fast -w stop >/dev/null 2>&1 || true
  fi

  if [[ -n "$temporary_root" ]]; then
    rm -rf -- "$temporary_root"
  fi

  exit "$exit_code"
}
trap cleanup EXIT

test_database_url="${ASKOLO_TEST_DATABASE_URL:-}"
if [[ -z "$test_database_url" ]]; then
  initdb_path="$(command -v initdb || true)"
  pg_ctl_path="$(command -v pg_ctl || true)"
  psql_path="$(command -v psql || true)"
  python_path="$(command -v python3 || true)"

  [[ -n "$initdb_path" ]] || die "initdb is required to provision the disposable PostgreSQL instance"
  [[ -n "$pg_ctl_path" ]] || die "pg_ctl is required to provision the disposable PostgreSQL instance"
  [[ -n "$psql_path" ]] || die "psql is required to provision the disposable PostgreSQL instance"
  [[ -n "$python_path" ]] || die "python3 is required to select a free local PostgreSQL port"

  temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/askolo-native-email-auth.XXXXXX")"
  pg_data="$temporary_root/data"
  socket_dir="$temporary_root/socket"
  mkdir -m 700 -- "$socket_dir"

  echo "Provisioning disposable PostgreSQL instance"
  "$initdb_path" \
    --pgdata="$pg_data" \
    --username=postgres \
    --auth=trust \
    --no-locale \
    --encoding=UTF8 \
    >/dev/null

  postgres_port="$("$python_path" - <<'PY'
import socket

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
    probe.bind(("127.0.0.1", 0))
    print(probe.getsockname()[1])
PY
  )"

  "$pg_ctl_path" \
    --pgdata="$pg_data" \
    --log="$temporary_root/postgres.log" \
    --options="-h 127.0.0.1 -p $postgres_port -k $socket_dir" \
    --wait \
    start >/dev/null
  pg_started=true
  pg_start_options="-h 127.0.0.1 -p $postgres_port -k $socket_dir"

  test_role="askolo_native_auth_test"
  admin_database_url="postgresql://postgres@127.0.0.1:${postgres_port}/postgres?sslmode=disable"
  "$psql_path" \
    --dbname="$admin_database_url" \
    --set=ON_ERROR_STOP=1 \
    <<SQL
CREATE ROLE "$test_role"
  LOGIN
  NOSUPERUSER
  NOCREATEDB
  NOCREATEROLE
  NOINHERIT
  NOREPLICATION
  NOBYPASSRLS;
GRANT CONNECT, CREATE ON DATABASE postgres TO "$test_role";
SQL

  test_database_url="postgresql://${test_role}@127.0.0.1:${postgres_port}/postgres?sslmode=disable"
  echo "Disposable PostgreSQL instance is ready"
else
  echo "Using caller-provided disposable PostgreSQL test database"
  pg_data="${ASKOLO_TEST_DATABASE_PGDATA:-}"
  pg_ctl_path="${ASKOLO_TEST_DATABASE_PGCTL:-}"
  pg_start_options="${ASKOLO_TEST_DATABASE_PG_START_OPTIONS:-}"
fi

test_environment=(
  "ASKOLO_TEST_DATABASE_URL=$test_database_url"
)
if [[ -n "$pg_data" || -n "$pg_ctl_path" || -n "$pg_start_options" ]]; then
  [[ -n "$pg_data" && -n "$pg_ctl_path" && -n "$pg_start_options" ]] ||
    die "database outage controls require ASKOLO_TEST_DATABASE_PGDATA, ASKOLO_TEST_DATABASE_PGCTL, and ASKOLO_TEST_DATABASE_PG_START_OPTIONS"
  test_environment+=(
    "ASKOLO_TEST_DATABASE_PGDATA=$pg_data"
    "ASKOLO_TEST_DATABASE_PGCTL=$pg_ctl_path"
    "ASKOLO_TEST_DATABASE_PG_START_OPTIONS=$pg_start_options"
  )
fi

cd -- "$service_root"
env "${test_environment[@]}" \
  go test ./internal/modules/auth -run '^TestNativeEmailAuth' -count=1 -v

env "${test_environment[@]}" \
  go test ./internal/app -run '^TestEmailChallengeCleanupReadinessRecoversAfterDatabase' -count=1 -v

env "${test_environment[@]}" \
  go test ./internal/migrations -count=1 -v

echo "Native auth, cleanup readiness, and migration release validation passed"