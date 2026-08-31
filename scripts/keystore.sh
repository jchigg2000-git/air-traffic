#!/usr/bin/env bash
# Administer the gateway keystore: register apps, issue keys against them,
# revoke.
#
# The admin API takes a loopback caller or AIRTRAFFIC_ADMIN_KEY — key minting is
# the one surface that creates credentials, and it deliberately does NOT share
# the gateway's spine key (a gateway that can push reports must not be able to
# mint itself a credential). With no admin key set the posture is loopback-only,
# and that is why this script exists: when the control plane
# runs in compose, a request from your shell reaches the container over the
# Docker bridge, not loopback, so plain `curl 127.0.0.1:8122` gets a 401. This
# routes the call through the container's own network namespace instead.
#
#   ./scripts/keystore.sh apps
#   ./scripts/keystore.sh add-app hf-sandbox [baseline]
#   ./scripts/keystore.sh set-baseline hf-sandbox fintech    # "" to inherit global
#   ./scripts/keystore.sh disable-app hf-sandbox [true|false]
#   ./scripts/keystore.sh issue hf-sandbox user-42 [route] [expires-in-days]
#   ./scripts/keystore.sh keys hf-sandbox
#   ./scripts/keystore.sh revoke <key-id>
#   ./scripts/keystore.sh snapshot                          # what the gateway pulls
#
# The issued key is printed once. It is stored as a SHA-256 digest and cannot
# be recovered afterwards.
set -euo pipefail

cd "$(dirname "$0")/.."
CP_PORT="${AIRTRAFFIC_PORT:-8122}"
CP_CONTAINER="${AIRTRAFFIC_CONTAINER:-air-traffic-control-plane-1}"
CURL_IMAGE="${AIRTRAFFIC_CURL_IMAGE:-curlimages/curl:latest}"

# call METHOD PATH [BODY]
#
# Prefers a direct loopback call (control plane running natively on this host)
# and falls back to a throwaway curl sharing the container's netns.
call() {
  local method="$1" path="$2" body="${3:-}"
  local url="http://127.0.0.1:$CP_PORT$path"
  local args=(-sS -X "$method" -H 'Content-Type: application/json')
  [[ -n "${SPINE_HDR:-}" ]] && args+=(-H "$SPINE_HDR")
  [[ -n "$body" ]] && args+=(-d "$body")

  if [[ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$CP_PORT/api/apps" || true)" == "200" ]]; then
    curl "${args[@]}" "$url"
    return
  fi
  if ! docker inspect "$CP_CONTAINER" >/dev/null 2>&1; then
    echo "control plane not reachable on 127.0.0.1:$CP_PORT and container '$CP_CONTAINER' not found" >&2
    echo "(set AIRTRAFFIC_PORT / AIRTRAFFIC_CONTAINER if yours differ)" >&2
    exit 1
  fi
  docker run --rm --network "container:$CP_CONTAINER" "$CURL_IMAGE" "${args[@]}" "$url"
}

json_str() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

cmd="${1:-help}"
case "$cmd" in
  apps)
    call GET /api/apps
    ;;
  add-app)
    app="${2:?usage: add-app <app-id> [baseline]}"
    call POST /api/apps "{\"id\":\"$(json_str "$app")\",\"baseline\":\"$(json_str "${3:-}")\"}"
    ;;
  set-baseline)
    app="${2:?usage: set-baseline <app-id> <baseline|\"\">}"
    call PATCH "/api/apps/$app" "{\"baseline\":\"$(json_str "${3:-}")\"}"
    ;;
  disable-app)
    app="${2:?usage: disable-app <app-id> [true|false]}"
    call PATCH "/api/apps/$app" "{\"disabled\":${3:-true}}"
    ;;
  issue)
    app="${2:?usage: issue <app-id> <subject> [route] [expires-in-days]}"
    subject="${3:-}"
    routes=""
    [[ -n "${4:-}" ]] && routes=",\"routes\":[\"$(json_str "$4")\"]"
    expiry=""
    [[ -n "${5:-}" ]] && expiry=",\"expires_in_days\":$5"
    call POST "/api/apps/$app/keys" "{\"subject\":\"$(json_str "$subject")\"$routes$expiry}"
    ;;
  keys)
    app="${2:?usage: keys <app-id>}"
    call GET "/api/apps/$app/keys"
    ;;
  revoke)
    kid="${2:?usage: revoke <key-id>}"
    call DELETE "/api/keys/$kid"
    ;;
  snapshot)
    # What the gateway pulls. Digests, never secrets. Unlike the /api/apps
    # routes this one is spine-gated (requireSpineKey), and that check runs
    # BEFORE the loopback fallback — so the key is required even from inside
    # the container's netns whenever AIRTRAFFIC_SPINE_KEY is set.
    SPINEKEY="${AIRTRAFFIC_SPINE_KEY:-}"
    if [[ -z "$SPINEKEY" && -f .env ]]; then
      SPINEKEY=$(sed -n 's/^AIRTRAFFIC_SPINE_KEY=//p' .env | tail -n1)
    fi
    SPINE_HDR="X-Air-Traffic-Key: $SPINEKEY" call GET /api/gateway/keys
    ;;
  *)
    sed -n '2,23p' "$0" | sed 's/^# \{0,1\}//'
    ;;
esac
