#!/usr/bin/env bash
# End-to-end proof of the inference gateway MVP slice: Presidio + control
# plane + gateway, one harness run via the API, and assertions that the spine
# lit up. Safe to re-run; leaves Presidio running, kills its own Go processes.
set -euo pipefail
cd "$(dirname "$0")/.."

# Override when the dev launcher already holds the default ports:
#   CP_PORT=18122 GW_PORT=18125 ./scripts/e2e-gateway.sh
CP=127.0.0.1:${CP_PORT:-8122}
GW=127.0.0.1:${GW_PORT:-8125}

PASS=0; FAIL=0
check() { if eval "$2"; then echo "  ok  $1"; PASS=$((PASS+1)); else echo "  FAIL $1"; FAIL=$((FAIL+1)); fi; }

# Every control-plane WRITE below goes through requireAdminWrite
# (internal/server/spine_auth.go), so once AIRTRAFFIC_ADMIN_KEY is set they 401
# — and loopback is deliberately not a free pass. scripts/dev-env.sh mints one
# into .env, so read it the way GWKEY is read rather than assuming the open
# posture. Defined above the mode branch because both modes need it under -u.
# Empty is harmless: curl omits a header whose value is empty.
ADMKEY="${AIRTRAFFIC_ADMIN_KEY:-}"
if [ -z "$ADMKEY" ] && [ -f .env ]; then
  ADMKEY=$(sed -n 's/^AIRTRAFFIC_ADMIN_KEY=//p' .env | tail -n1)
fi
ADM=(-H "X-Air-Traffic-Admin-Key: $ADMKEY")

if [ "${E2E_COMPOSE:-}" = "1" ]; then
  # Compose mode: assert against an already-running root docker-compose.yml
  # stack instead of booting bare processes. The gateway there pulls policy at
  # the default 15s interval, so propagation waits are longer.
  echo "→ using running compose stack at $CP / $GW"
  PULL_WAIT=17
  # The running stack authenticates with whatever key it was started with.
  # After ./scripts/dev-env.sh that is a minted gwk-<random>, not the compose
  # fallback, so read the same .env compose read rather than assuming the demo
  # value — otherwise every proxy check here 401s and reads as a regression.
  GWKEY="${GATEWAY_CLIENT_KEYS:-}"
  if [ -z "$GWKEY" ] && [ -f .env ]; then
    GWKEY=$(sed -n 's/^GATEWAY_CLIENT_KEYS=//p' .env | tail -n1)
  fi
  GWKEY="${GWKEY:-gwk-demo}"
  curl -sf http://$CP/api/health >/dev/null || { echo "control plane not reachable — docker compose up -d --build first"; exit 1; }
  curl -sf http://$GW/readyz >/dev/null || { echo "gateway not ready"; exit 1; }
else
  PULL_WAIT=4
  # Bare mode boots its own gateway below with this exact key.
  GWKEY=gwk-demo

  # A stale listener would make every check below test the wrong build.
  for p in ${CP_PORT:-8122} ${GW_PORT:-8125}; do
    if lsof -nP -iTCP:$p -sTCP:LISTEN >/dev/null 2>&1; then
      echo "port $p already has a listener:"; lsof -nP -iTCP:$p -sTCP:LISTEN | tail -1
      echo "kill it or re-run with CP_PORT/GW_PORT overrides"; exit 1
    fi
  done

  echo "→ presidio"
  docker compose -f deploy/presidio/docker-compose.yml up -d --wait

  echo "→ control plane ($CP)"
  AIRTRAFFIC_ADDR=$CP AIRTRAFFIC_GATEWAY_KEY=gwk-demo go run ./cmd/air-traffic-server >/tmp/e2e-cp.log 2>&1 &
  CP_PID=$!
  echo "→ gateway ($GW)"
  GATEWAY_LISTEN_ADDR=$GW \
  GATEWAY_UPSTREAMS='{"anthropic":{"base_url":"http://'$CP'/synthetic/anthropic","credential_ref":"env:ANTHROPIC_UPSTREAM_KEY"}}' \
  ANTHROPIC_UPSTREAM_KEY=sk-ant-synthetic-e2e \
  GATEWAY_CLIENT_KEYS=gwk-demo \
  GATEWAY_DETECTORS=regex,presidio \
  GATEWAY_REDACT_ACTION=per_policy \
  GATEWAY_CONTROL_PLANE_URL=http://$CP \
  GATEWAY_OBS_PUSH_INTERVAL=2s \
  GATEWAY_POLICY_PULL_INTERVAL=3s \
  go run ./cmd/air-traffic-gateway >/tmp/e2e-gw.log 2>&1 &
  GW_PID=$!
  # go run spawns the real binary as a child — kill the whole family or the
  # orphaned child keeps the port and poisons the next run.
  trap 'pkill -P $CP_PID 2>/dev/null; pkill -P $GW_PID 2>/dev/null; kill $CP_PID $GW_PID 2>/dev/null; wait 2>/dev/null || true' EXIT

  for i in $(seq 1 30); do curl -sf http://$CP/api/health >/dev/null 2>&1 && break; sleep 1; done
  for i in $(seq 1 30); do curl -sf http://$GW/readyz >/dev/null 2>&1 && break; sleep 1; done
  # Fail fast on a dead process (port already bound → we'd silently talk to a stray)
  kill -0 $CP_PID 2>/dev/null || { echo "control plane died (port $CP busy?) — see /tmp/e2e-cp.log"; exit 1; }
  kill -0 $GW_PID 2>/dev/null || { echo "gateway died (port $GW busy?) — see /tmp/e2e-gw.log"; exit 1; }
  curl -sf http://$CP/api/health >/dev/null || { echo "control plane not healthy"; exit 1; }
  curl -sf http://$GW/readyz >/dev/null || { echo "gateway not ready"; exit 1; }
fi
sleep 2  # first heartbeat + policy pull

echo "→ apply healthcare baseline (pre-coverage gate: block until ZDR attested)"
curl -sf "${ADM[@]}" -X PUT http://$CP/api/policies -d '{"baseline":"healthcare"}' >/dev/null
sleep $PULL_WAIT  # one policy-pull interval

echo "→ direct proxy checks"
BLOCKED=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://$GW/v1/messages \
  -H "Authorization: Bearer $GWKEY" -H 'content-type: application/json' \
  -d '{"model":"m","messages":[{"role":"user","content":"SSN 123-45-6789 please"}]}')
check "healthcare gate blocks PII (got $BLOCKED)" "[ \"$BLOCKED\" = 400 ]"
UNAUTH=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://$GW/v1/messages -d '{}')
check "bad gateway key rejected (got $UNAUTH)" "[ \"$UNAUTH\" = 401 ]"

echo "→ attest ZDR (block → mask, no restart)"
curl -sf "${ADM[@]}" -X PUT http://$CP/api/policies -d '{"baseline":"healthcare","vendors":{"anthropic":{"zdr_attested":true}}}' >/dev/null
sleep $PULL_WAIT
MASKED=$(curl -s -X POST http://$GW/v1/messages \
  -H "Authorization: Bearer $GWKEY" -H 'content-type: application/json' \
  -d '{"model":"m","messages":[{"role":"user","content":"SSN 123-45-6789 please"}]}' -o /dev/null -w '%{http_code}')
check "attested ZDR masks instead of blocking (got $MASKED)" "[ \"$MASKED\" = 200 ]"

echo "→ harness run (120 requests)"
START_BODY=$(curl -s "${ADM[@]}" -X POST http://$CP/api/harness/runs \
  -d '{"count":120,"concurrency":4,"seed":4242,"include_traps":true,"include_presidio_only":true,"include_straddle":true,"replay_percent":10}')
RUN_ID=$(echo "$START_BODY" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("run",{}).get("id") or sys.exit(f"start failed: {d}"))')
echo "  run $RUN_ID"
for i in $(seq 1 120); do
  STATUS=$(curl -sf http://$CP/api/harness/runs/$RUN_ID | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["status"])')
  [ "$STATUS" != running ] && break
  sleep 2
done
curl -sf http://$CP/api/harness/runs/$RUN_ID | python3 -c '
import json, sys
run = json.load(sys.stdin)["run"]
s = run["score"]
print("  status=%s recall_behavioral=%.3f precision=%.3f trap_fps=%d orphans=%d promoted=%d"
      % (run["status"], s["recall_behavioral"], s["precision"], s["trap_fps"], s["orphan_requests"], run["promoted_count"]))
assert run["status"] == "done"
assert s["trap_fps"] == 0, "traps fired"
assert s["orphan_requests"] == 0, "join incomplete"
'
check "harness run scored" "true"

echo "→ spine lit up"
curl -s "http://$CP/api/observations?limit=1000" | python3 -c '
import json, sys
from collections import Counter
obs = json.load(sys.stdin)["observations"]
print("  newest 1000 observation records by connector:", dict(Counter(o["connector_type"] for o in obs)))'
# grep -c (not -q): under pipefail, grep -q's early exit SIGPIPEs curl on
# large responses and fails the pipeline despite a match.
check "gateway observations on the spine" \
  "curl -sf 'http://$CP/api/observations?limit=400' | grep -c '\"gw_requests\"' >/dev/null"
check "ratchet metric self-ingested" \
  "curl -sf 'http://$CP/api/observations?limit=400' | grep -c 'detector_recall_ratchet' >/dev/null"
check "gateway block events in audit" \
  "curl -sf http://$CP/api/audit | grep -c 'gateway.block' >/dev/null"
check "proxy_enforced flipped in coverage" \
  "curl -sf -H 'X-Air-Traffic-Admin-Key: $ADMKEY' -X PUT http://$CP/api/policies -d '{\"baseline\":\"healthcare\",\"vendors\":{\"anthropic\":{\"zdr_attested\":true}}}' | grep -c 'applied_proxy' >/dev/null"
check "gateway status fresh" \
  "curl -sf http://$CP/api/gateway/status | grep -c '\"fresh\": true' >/dev/null"

echo
echo "$PASS passed, $FAIL failed"
[ $FAIL -eq 0 ]
