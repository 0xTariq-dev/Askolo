#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
validation_script="$script_dir/run-native-email-auth-validation.sh"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/askolo-native-email-auth-test.XXXXXX")"
fake_bin="$fixture_root/bin"
test_tmpdir="$fixture_root/tmp"
fake_postgres_pid_file="$fixture_root/fake-postgres.pid"
fake_postgres_stopped="$fixture_root/fake-postgres-stopped"
go_started="$fixture_root/go-started"
go_args_log="$fixture_root/go-args.log"
validation_pid=""
postgres_pid=""

cleanup() {
  local exit_code=$?

  if [[ -n "$validation_pid" ]] && kill -0 "$validation_pid" 2>/dev/null; then
    kill -KILL "$validation_pid" 2>/dev/null || true
    wait "$validation_pid" 2>/dev/null || true
  fi

  if [[ -n "$postgres_pid" ]] && kill -0 "$postgres_pid" 2>/dev/null; then
    kill -KILL "$postgres_pid" 2>/dev/null || true
  fi

  rm -rf -- "$fixture_root"
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

fail() {
  echo "native email auth cleanup regression: $*" >&2
  exit 1
}

wait_for_file() {
  local path="$1"

  for _ in {1..100}; do
    [[ -e "$path" ]] && return
    sleep 0.05
  done

  fail "timed out waiting for $path"
}

assert_clean() {
  local case_name="$1"
  local temporary_root

  for _ in {1..100}; do
    if [[ -z "$postgres_pid" ]] || ! kill -0 "$postgres_pid" 2>/dev/null; then
      break
    fi
    sleep 0.05
  done

  if [[ -n "$postgres_pid" ]] && kill -0 "$postgres_pid" 2>/dev/null; then
    fail "$case_name left the disposable PostgreSQL process running (pid $postgres_pid)"
  fi

  temporary_root=""
  for temporary_root in "$test_tmpdir"/askolo-native-email-auth.*; do
    [[ ! -e "$temporary_root" ]] ||
      fail "$case_name left temporary data behind at $temporary_root"
  done

  [[ -e "$fake_postgres_stopped" ]] ||
    fail "$case_name did not invoke disposable PostgreSQL shutdown"
}

mkdir -p -- "$fake_bin" "$test_tmpdir"

cat >"$fake_bin/initdb" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail

pg_data=""
for argument in "$@"; do
  case "$argument" in
    --pgdata=*) pg_data="${argument#--pgdata=}" ;;
  esac
done

[[ -n "$pg_data" ]] || exit 2
mkdir -p -- "$pg_data"
STUB

cat >"$fake_bin/pg_ctl" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail

pg_data=""
action=""
while (($#)); do
  case "$1" in
    --pgdata=*) pg_data="${1#--pgdata=}" ;;
    -D) shift; pg_data="$1" ;;
    start|stop) action="$1" ;;
  esac
  shift
done

[[ -n "$pg_data" && -n "$action" ]] || exit 2
pid_file="$pg_data/fake-postgres.pid"

if [[ "$action" == start ]]; then
  "$FAKE_POSTGRES" &
  printf '%s\n' "$!" >"$pid_file"
  printf '%s\n' "$!" >"$FAKE_POSTGRES_PID_FILE"
  exit 0
fi

if [[ -f "$pid_file" ]]; then
  pid="$(cat -- "$pid_file")"
  if [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    for _ in {1..100}; do
      kill -0 "$pid" 2>/dev/null || break
      sleep 0.05
    done
  fi
  rm -f -- "$pid_file"
fi
rm -f -- "$FAKE_POSTGRES_PID_FILE"
touch -- "$FAKE_POSTGRES_STOPPED"
STUB

cat >"$fake_bin/psql" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail

cat >/dev/null
STUB

cat >"$fake_bin/go" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail

touch -- "$GO_STARTED"
printf '%s\n' "$*" >>"$GO_ARGS_LOG"
case "$GO_MODE" in
  success)
    exit 0
    ;;
  early-exit)
    exit 37
    ;;
  interrupted)
    parent_pid="$PPID"
    while kill -0 "$parent_pid" 2>/dev/null; do
      sleep 0.05
    done
    ;;
  *)
    echo "unknown test Go mode: $GO_MODE" >&2
    exit 2
    ;;
esac
STUB

cat >"$fixture_root/fake-postgres" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail

stop() {
  rm -f -- "$FAKE_POSTGRES_PID_FILE"
  exit 0
}
trap stop TERM INT

while :; do
  sleep 0.05
done
STUB

chmod +x "$fake_bin/initdb" "$fake_bin/pg_ctl" "$fake_bin/psql" "$fake_bin/go" \
  "$fixture_root/fake-postgres"

run_validation() {
  local case_name="$1"
  local go_mode="$2"
  local log_file="$fixture_root/$case_name.log"

  rm -f -- "$fake_postgres_pid_file" "$fake_postgres_stopped" "$go_started"
  postgres_pid=""

  env \
    -u ASKOLO_TEST_DATABASE_URL \
    "PATH=$fake_bin:$PATH" \
    "TMPDIR=$test_tmpdir" \
    "FAKE_POSTGRES=$fixture_root/fake-postgres" \
    "FAKE_POSTGRES_PID_FILE=$fake_postgres_pid_file" \
    "FAKE_POSTGRES_STOPPED=$fake_postgres_stopped" \
    "GO_MODE=$go_mode" \
    "GO_STARTED=$go_started" \
    "GO_ARGS_LOG=$go_args_log" \
    bash "$validation_script" >"$log_file" 2>&1 &
  validation_pid=$!

  wait_for_file "$go_started"
  if [[ -f "$fake_postgres_pid_file" ]]; then
    postgres_pid=""
    for _ in {1..20}; do
      postgres_pid="$(cat -- "$fake_postgres_pid_file" 2>/dev/null || true)"
      [[ "$postgres_pid" =~ ^[0-9]+$ ]] && break
      sleep 0.05
    done
    if [[ ! "$postgres_pid" =~ ^[0-9]+$ ]]; then
      grep -q "Disposable PostgreSQL instance is ready" "$log_file" ||
        fail "$case_name recorded an invalid PostgreSQL pid"
      postgres_pid=""
    fi
  else
    # The early-exit fake Go process can finish and trigger validation cleanup
    # between the startup signal and this read. The validation log is the
    # durable startup marker in that narrow race; cleanup still verifies the
    # fake PostgreSQL shutdown marker below.
    grep -q "Disposable PostgreSQL instance is ready" "$log_file" ||
      fail "$case_name did not start the disposable PostgreSQL process"
  fi
}

run_interruption_case() {
  local status

  run_validation interrupted interrupted
  kill -TERM "$validation_pid"

  set +e
  wait "$validation_pid"
  status=$?
  set -e
  validation_pid=""

  [[ "$status" -ne 0 ]] ||
    fail "interrupting validation unexpectedly succeeded"
  assert_clean interrupted-validation
}

run_success_case() {
  local status

  run_validation success success

  set +e
  wait "$validation_pid"
  status=$?
  set -e
  validation_pid=""

  [[ "$status" -eq 0 ]] ||
    fail "successful validation returned status $status"
  grep -Fq -- "./internal/modules/auth" "$go_args_log" ||
    fail "successful validation did not run native email auth checks"
  grep -Fq -- "./internal/app" "$go_args_log" ||
    fail "successful validation did not run cleanup readiness checks"
  assert_clean successful-validation
}

run_early_exit_case() {
  local status

  run_validation early-exit early-exit

  set +e
  wait "$validation_pid"
  status=$?
  set -e
  validation_pid=""

  [[ "$status" -eq 37 ]] ||
    fail "early Go-test exit returned status $status instead of 37"
  assert_clean early-go-test-exit
}

run_success_case
run_interruption_case
run_early_exit_case

echo "Native email auth interruption cleanup regression check passed"