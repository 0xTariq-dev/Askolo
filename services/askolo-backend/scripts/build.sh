#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
service_root="$(cd -- "$script_dir/.." && pwd)"
bin_dir="$service_root/bin"
binary="$bin_dir/askolo-backend"
tmp_binary="$bin_dir/.askolo-backend.$$.tmp"
pid_file="$service_root/tmp/askolo-backend.pid"

cd -- "$service_root"

# Go's automatic toolchain download must be checksum-verified even when the
# workspace disables checksums by default.
if [[ "${GOSUMDB:-}" == "off" ]]; then
  export GOSUMDB=sum.golang.org
fi

cleanup() {
  rm -f -- "$tmp_binary"
}
trap cleanup EXIT

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

  echo "Stopping existing Askolo backend process $pid before rebuilding"
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

mkdir -p -- "$bin_dir" "$service_root/tmp"
find "$bin_dir" -maxdepth 1 -type f -name '.askolo-backend.*.tmp' -delete
stop_recorded_process

go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o "$tmp_binary" ./cmd/askolo-backend
chmod 755 "$tmp_binary"
mv -f -- "$tmp_binary" "$binary"

echo "Built $binary"