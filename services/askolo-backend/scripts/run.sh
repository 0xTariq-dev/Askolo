#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
service_root="$(cd -- "$script_dir/.." && pwd)"
binary="$service_root/bin/askolo-backend"
pid_dir="$service_root/tmp"
pid_file="$pid_dir/askolo-backend.pid"

stop_recorded_process() {
  if [[ ! -f "$pid_file" ]]; then
    return
  fi

  local pid
  pid="$(cat -- "$pid_file" 2>/dev/null || true)"
  if [[ ! "$pid" =~ ^[0-9]+$ ]] || ! kill -0 "$pid" 2>/dev/null; then
    rm -f -- "$pid_file"
    return
  fi

  local command_line
  command_line="$(tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null || true)"
  if [[ "$command_line" != *"askolo-backend"* ]]; then
    rm -f -- "$pid_file"
    return
  fi

  echo "Stopping existing Askolo backend process $pid"
  kill -TERM "$pid" 2>/dev/null || true
  for _ in {1..50}; do
    if ! kill -0 "$pid" 2>/dev/null; then
      break
    fi
    sleep 0.1
  done
  if kill -0 "$pid" 2>/dev/null; then
    echo "Askolo backend process $pid did not stop gracefully; terminating it"
    kill -KILL "$pid" 2>/dev/null || true
  fi
  rm -f -- "$pid_file"
}

mkdir -p -- "$pid_dir"
stop_recorded_process

if [[ ! -x "$binary" ]]; then
  echo "Backend binary is missing; building it before startup"
  bash "$script_dir/build.sh"
fi

backend_pid=""
cleanup() {
  local exit_code=$?
  if [[ -n "$backend_pid" ]] && kill -0 "$backend_pid" 2>/dev/null; then
    echo "Stopping Askolo backend process $backend_pid"
    kill -TERM "$backend_pid" 2>/dev/null || true
    for _ in {1..50}; do
      if ! kill -0 "$backend_pid" 2>/dev/null; then
        break
      fi
      sleep 0.1
    done
    if kill -0 "$backend_pid" 2>/dev/null; then
      echo "Askolo backend process $backend_pid did not stop gracefully; terminating it"
      kill -KILL "$backend_pid" 2>/dev/null || true
    fi
  fi
  if [[ -f "$pid_file" ]] && [[ "$(cat -- "$pid_file" 2>/dev/null || true)" == "$backend_pid" ]]; then
    rm -f -- "$pid_file"
  fi
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

"$binary" &
backend_pid=$!
printf '%s\n' "$backend_pid" >"$pid_file"
wait "$backend_pid"