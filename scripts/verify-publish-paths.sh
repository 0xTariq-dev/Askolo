#!/usr/bin/env bash
set -Eeuo pipefail

# Run the same production build commands that the artifact publisher runs,
# from the repository root. Keep the contract checks before the builds so a
# path-only artifact change fails with an actionable message.

die() {
  echo "publish path verification failed: $*" >&2
  exit 1
}

contract_errors=()

record_contract_error() {
  contract_errors+=("$1")
}

report_contract_errors() {
  [[ "${#contract_errors[@]}" -gt 0 ]] || return 0

  echo "publish path verification failed:" >&2
  printf ' - %s\n' "${contract_errors[@]}" >&2
  exit 1
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
artifact_config="$repo_root/artifacts/personal-assistant/.replit-artifact/artifact.toml"
api_build_script="$repo_root/services/askolo-backend/scripts/build.sh"
api_root="$repo_root/services/askolo-backend"
policy_validator="$repo_root/monitoring/askolo-backend/verify-mfa-recovery-alert-policy.sh"

if [[ "${GOSUMDB:-}" == "off" ]]; then
  export GOSUMDB=sum.golang.org
fi

[[ -f "$artifact_config" ]] ||
  die "artifact config not found at artifacts/personal-assistant/.replit-artifact/artifact.toml"
[[ -f "$api_build_script" ]] ||
  die "API build script not found at services/askolo-backend/scripts/build.sh"
[[ -x "$policy_validator" ]] ||
  die "MFA alert-policy validator is missing at monitoring/askolo-backend/verify-mfa-recovery-alert-policy.sh"
"$policy_validator" "$repo_root/monitoring/askolo-backend/mfa-recovery-alerts.yaml" ||
  die "MFA alert-policy validation failed; inspect the file path and reason above"

require_artifact_line() {
  local expected="$1"
  local description="$2"

  grep -Fqx "$expected" "$artifact_config" || {
    record_contract_error \
      "$description changed in artifacts/personal-assistant/.replit-artifact/artifact.toml; expected: $expected"
  }
}

# These values are consumed from the publishing repository root. A changed
# path can otherwise leave local development healthy while publishing fails.
require_artifact_line \
  'build = [ "pnpm", "--filter", "@workspace/personal-assistant", "run", "build" ]' \
  "frontend production build command"
require_artifact_line \
  'publicDir = "artifacts/personal-assistant/dist/public"' \
  "frontend production public directory"
require_artifact_line \
  'build = ["bash", "services/askolo-backend/scripts/build.sh"]' \
  "API production build command"
extract_paths() {
  sed -E 's/^[^[]*\[//; s/\].*$//' |
    tr ',' '\n' |
    sed -E 's/^[[:space:]]*"//; s/"[[:space:]]*$//' |
    sed '/^$/d'
}

contains_path() {
  local needle="$1"
  shift
  local path
  for path in "$@"; do
    [[ "$path" == "$needle" ]] && return 0
  done
  return 1
}

backend_route_paths="$(
  cd -- "$api_root"
  go run ./cmd/published-route-paths
)" || die "could not read the backend published route registry"

artifact_route_paths_line="$(
  awk '
    /^\[\[services\]\]$/ {
      in_service = 1
      is_api = 0
      next
    }
    in_service && /^name = "api"$/ {
      is_api = 1
      next
    }
    in_service && is_api && /^paths = / {
      print
      exit
    }
  ' "$artifact_config"
)"
[[ -n "$artifact_route_paths_line" ]] ||
  die "API service paths are missing from artifacts/personal-assistant/.replit-artifact/artifact.toml"

mapfile -t backend_route_paths_array < <(printf '%s\n' "$backend_route_paths" | extract_paths)
mapfile -t artifact_route_paths_array < <(printf '%s\n' "$artifact_route_paths_line" | extract_paths)

[[ "${#backend_route_paths_array[@]}" -gt 0 ]] ||
  die "backend published route registry is empty"

missing_route_paths=()
for path in "${backend_route_paths_array[@]}"; do
  contains_path "$path" "${artifact_route_paths_array[@]}" ||
    missing_route_paths+=("$path")
done
if [[ "${#missing_route_paths[@]}" -gt 0 ]]; then
  record_contract_error \
    "backend route registry declares published path(s) missing from artifacts/personal-assistant/.replit-artifact/artifact.toml: ${missing_route_paths[*]}; add them to the API service paths list"
fi

unexpected_route_paths=()
for path in "${artifact_route_paths_array[@]}"; do
  contains_path "$path" "${backend_route_paths_array[@]}" ||
    unexpected_route_paths+=("$path")
done
if [[ "${#unexpected_route_paths[@]}" -gt 0 ]]; then
  record_contract_error \
    "artifact publishes path(s) absent from the backend route registry: ${unexpected_route_paths[*]}; remove them or register the backend routes before publishing"
fi

report_contract_errors

validate_port_override() {
  local variable_name="$1"
  local value="$2"

  [[ "$value" =~ ^[0-9]{1,5}$ ]] ||
    die "$variable_name must be a port number between 1 and 65535; received \"$value\""

  local port=$((10#$value))
  (( port >= 1 && port <= 65535 )) ||
    die "$variable_name must be a port number between 1 and 65535; received \"$value\""
}

frontend_port_override="${PUBLISH_SMOKE_FRONTEND_PORT-}"
api_port_override="${PUBLISH_SMOKE_API_PORT-}"
router_port_override="${PUBLISH_SMOKE_ROUTER_PORT-}"

[[ -z "$frontend_port_override" ]] ||
  validate_port_override "PUBLISH_SMOKE_FRONTEND_PORT" "$frontend_port_override"
[[ -z "$api_port_override" ]] ||
  validate_port_override "PUBLISH_SMOKE_API_PORT" "$api_port_override"
[[ -z "$router_port_override" ]] ||
  validate_port_override "PUBLISH_SMOKE_ROUTER_PORT" "$router_port_override"

line_number() {
  local needle="$1"
  local line
  line="$(grep -nF "$needle" "$api_build_script" | head -n 1 | cut -d: -f1 || true)"
  [[ -n "$line" ]] || die "API build script no longer contains: $needle"
  printf '%s\n' "$line"
}

script_dir_line="$(line_number 'script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"')"
service_root_line="$(line_number 'service_root="$(cd -- "$script_dir/.." && pwd)"')"
cd_root_line="$(line_number 'cd -- "$service_root"')"
go_test_line="$(line_number 'go test ./...')"
go_vet_line="$(line_number 'go vet ./...')"
go_build_line="$(line_number 'go build -trimpath -o "$tmp_binary" ./cmd/askolo-backend')"

(( script_dir_line < service_root_line )) ||
  die "API build script must resolve script_dir before service_root"
(( service_root_line < cd_root_line )) ||
  die "API build script must resolve service_root before changing directories"
(( cd_root_line < go_test_line && cd_root_line < go_vet_line && cd_root_line < go_build_line )) ||
  die "API build script must enter its module root before running Go commands"

cd -- "$repo_root"

echo "Verifying frontend production build from repository root"
PORT=18131 BASE_PATH=/ pnpm --filter @workspace/personal-assistant run build

api_pid_file="$repo_root/services/askolo-backend/tmp/askolo-backend.pid"
api_pid_file_stash=""

restore_api_pid_file() {
  [[ -n "$api_pid_file_stash" ]] || return 0

  if [[ -f "$api_pid_file" ]]; then
    local current_pid
    local stashed_pid
    current_pid="$(cat -- "$api_pid_file" 2>/dev/null || true)"
    stashed_pid="$(cat -- "$api_pid_file_stash" 2>/dev/null || true)"
    if [[ "$current_pid" != "$stashed_pid" ]]; then
      rm -f -- "$api_pid_file_stash"
      api_pid_file_stash=""
      return
    fi
  fi

  mv -- "$api_pid_file_stash" "$api_pid_file"
  api_pid_file_stash=""
}

if [[ -f "$api_pid_file" ]]; then
  api_pid_file_stash="$(mktemp "${TMPDIR:-/tmp}/askolo-backend-pid.XXXXXX")"
  rm -f -- "$api_pid_file_stash"
  mv -- "$api_pid_file" "$api_pid_file_stash"
  trap restore_api_pid_file EXIT
fi

echo "Verifying API production build from repository root"
bash services/askolo-backend/scripts/build.sh
restore_api_pid_file
trap - EXIT

smoke_dir="$(mktemp -d)"
frontend_pid=""
api_pid=""
router_pid=""

port_is_available() {
  local port="$1"

  node - "$port" >/dev/null 2>&1 <<'NODE'
const net = require("node:net");

const port = Number(process.argv[2]);
const server = net.createServer();

server.once("error", () => process.exit(1));
server.listen({ host: "0.0.0.0", port }, () => {
  server.close(() => process.exit(0));
});
NODE
}

port_was_selected() {
  local port="$1"
  local selected

  for selected in "${selected_ports[@]}"; do
    [[ "$selected" == "$port" ]] && return 0
  done
  return 1
}

select_smoke_port() {
  local variable_name="$1"
  local override="$2"
  local first_candidate="$3"
  local port="$override"

  if [[ -n "$override" ]]; then
    port_was_selected "$port" &&
      die "$variable_name is already selected for another smoke service: $port"
    port_is_available "$port" ||
      die "$variable_name is already in use: $port"
    printf '%s\n' "$port"
    return
  fi

  for ((port = first_candidate; port <= 65535; port++)); do
    port_was_selected "$port" && continue
    if port_is_available "$port"; then
      printf '%s\n' "$port"
      return
    fi
  done

  die "could not find an unused port for $variable_name"
}

cleanup_smoke() {
  local exit_code=$?

  for pid in "$router_pid" "$api_pid" "$frontend_pid"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  wait "$router_pid" "$api_pid" "$frontend_pid" 2>/dev/null || true

  if (( exit_code != 0 )); then
    for log in "$smoke_dir"/*.log; do
      [[ -f "$log" ]] || continue
      echo "--- $(basename "$log") ---" >&2
      tail -n 40 "$log" >&2 || true
    done
  fi

  rm -rf -- "$smoke_dir"
  exit "$exit_code"
}
trap cleanup_smoke EXIT INT TERM

selected_ports=()
frontend_port="$(select_smoke_port "PUBLISH_SMOKE_FRONTEND_PORT" "$frontend_port_override" 18000)"
selected_ports+=("$frontend_port")
api_port="$(select_smoke_port "PUBLISH_SMOKE_API_PORT" "$api_port_override" 18001)"
selected_ports+=("$api_port")
router_port="$(select_smoke_port "PUBLISH_SMOKE_ROUTER_PORT" "$router_port_override" 18002)"
selected_ports+=("$router_port")

echo "Using smoke ports: frontend=$frontend_port api=$api_port router=$router_port"

wait_for_http() {
  local url="$1"
  local description="$2"

  for _ in {1..60}; do
    if curl --fail --silent --show-error --max-time 2 "$url" >/dev/null 2>&1; then
      return
    fi
    sleep 0.2
  done

  die "$description did not respond successfully at $url"
}

echo "Starting built frontend and API for published routing smoke check"
(
  cd "$repo_root"
  PORT="$frontend_port" BASE_PATH=/ NODE_ENV=production \
    pnpm --filter @workspace/personal-assistant run serve
) >"$smoke_dir/frontend.log" 2>&1 &
frontend_pid=$!

(
  cd "$repo_root"
  env \
    -u DATABASE_URL \
    ASKOLO_ENVIRONMENT=development \
    ASKOLO_INTERNAL_TOKEN=publish-smoke-token \
    BACKEND_HOST=127.0.0.1 \
    PORT="$api_port" \
    "$repo_root/services/askolo-backend/bin/askolo-backend"
) >"$smoke_dir/api.log" 2>&1 &
api_pid=$!

node - "$frontend_port" "$api_port" "$router_port" >"$smoke_dir/router.log" 2>&1 <<'NODE' &
const http = require("node:http");

const frontendPort = Number(process.argv[2]);
const apiPort = Number(process.argv[3]);
const routerPort = Number(process.argv[4]);
const apiPrefixes = ["/api", "/healthz", "/readyz", "/ws", "/webhooks"];

const isApiPath = (requestUrl) => {
  const pathname = new URL(requestUrl, "http://published-smoke").pathname;
  return apiPrefixes.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
};

const server = http.createServer((request, response) => {
  const targetPort = isApiPath(request.url || "/") ? apiPort : frontendPort;
  const proxy = http.request(
    {
      hostname: "127.0.0.1",
      port: targetPort,
      path: request.url,
      method: request.method,
      headers: request.headers,
    },
    (targetResponse) => {
      response.writeHead(targetResponse.statusCode || 502, targetResponse.headers);
      targetResponse.pipe(response);
    },
  );

  proxy.on("error", (error) => {
    response.statusCode = 502;
    response.end(`published smoke router error: ${error.message}`);
  });
  request.pipe(proxy);
});

server.listen(routerPort, "127.0.0.1");
NODE
router_pid=$!

wait_for_http "http://127.0.0.1:$frontend_port/" "built frontend"
wait_for_http "http://127.0.0.1:$api_port/healthz" "built API"
wait_for_http "http://127.0.0.1:$router_port/" "published smoke router"

root_body="$smoke_dir/root.html"
route_body="$smoke_dir/route.html"
health_body="$smoke_dir/health.json"
api_health_body="$smoke_dir/api-health.json"

curl --fail --silent --show-error "http://127.0.0.1:$router_port/" >"$root_body" ||
  die "published root path failed"
grep -Fq "<!doctype html>" "$root_body" ||
  die "published root path did not return the built frontend"

curl --fail --silent --show-error "http://127.0.0.1:$router_port/dashboard" >"$route_body" ||
  die "published SPA route failed"
grep -Fq "<!doctype html>" "$route_body" ||
  die "published SPA route did not receive the frontend fallback"

curl --fail --silent --show-error "http://127.0.0.1:$router_port/healthz" >"$health_body" ||
  die "published API health path failed"
grep -Fq '"status":"ok"' "$health_body" ||
  die "published API health path did not return the API health response"

curl --fail --silent --show-error "http://127.0.0.1:$router_port/api/healthz" >"$api_health_body" ||
  die "published API health alias failed"
grep -Fq '"status":"ok"' "$api_health_body" ||
  die "published API health alias did not return the API health response"

echo "Published frontend/API routing smoke check passed"

echo "Verifying native email auth lifecycle against disposable PostgreSQL"
bash scripts/run-native-email-auth-validation.sh

echo "Publish path verification passed"