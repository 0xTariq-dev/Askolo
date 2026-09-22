#!/usr/bin/env bash
set -Eeuo pipefail

# Run the database-backed native email auth checks against a disposable
# PostgreSQL instance. The test suite uses a capture-only sender, so this
# validation never contacts a real email provider.

die() {
  echo "native email auth validation failed: $*" >&2
  exit 1
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
service_root="$repo_root/services/askolo-backend"

[[ -d "$service_root" ]] || die "backend service directory not found"

temporary_root=""
pg_data=""
pg_started=false

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
  python_path="$(command -v python3 || true)"

  [[ -n "$initdb_path" ]] || die "initdb is required to provision the disposable PostgreSQL instance"
  [[ -n "$pg_ctl_path" ]] || die "pg_ctl is required to provision the disposable PostgreSQL instance"
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

  test_database_url="postgresql://postgres@127.0.0.1:${postgres_port}/postgres?sslmode=disable"
  echo "Disposable PostgreSQL instance is ready"
else
  echo "Using caller-provided disposable PostgreSQL test database"
fi

cd -- "$service_root"
ASKOLO_TEST_DATABASE_URL="$test_database_url" \
  go test ./internal/modules/auth -run '^(TestNativeEmailAuth|TestEmailChallengeCleanup)' -count=1 -v

echo "Native email auth release validation passed"