# Presidio analyzer sidecar

The gateway's heavy NER detector (`GATEWAY_DETECTORS=regex,presidio`). Runs
self-hosted so PHI never leaves the local boundary — cloud DLP is the
BAA-gated exception this project deliberately defers.

```sh
docker compose -f deploy/presidio/docker-compose.yml up -d   # start
curl -s http://127.0.0.1:8126/health                          # "Presidio Analyzer service is up"
docker compose -f deploy/presidio/docker-compose.yml down     # stop
```

## Local footprint (macOS / Docker Desktop)

- **Image:** ~1.5–2.5 GB (bundles spaCy `en_core_web_lg`); pinned to `2.2.359`.
- **RAM:** ~1–1.5 GB resident; the compose file caps at 2 GB. Make sure Docker
  Desktop's VM allocation covers it.
- **Cold start:** 10–45 s of spaCy model load; the healthcheck (`start_period:
  45s`) gates readiness. The gateway treats analyzer errors per
  `GATEWAY_FAIL_MODE` (default `closed`: requests are refused rather than
  forwarded unscanned).
- **Per-call latency:** ~20–150 ms CPU-only for short prompts, vs microseconds
  for the in-process regex floor — which is why the chain runs regex first and
  the analyzer call carries an 800 ms default timeout
  (`GATEWAY_DETECTOR_TIMEOUT_MS`).

## What it adds over the regex floor

Names, addresses, and other free-text PII/PHI (`PERSON` → `PERSON_NAME`,
`LOCATION` → `ADDRESS`). Flywheel-approved pattern-pack rules are passed per
call as `ad_hoc_recognizers`, so pack updates need no container restart.
`DATE_TIME` results are dropped on the gateway side (FP noise vs the
semver/order-number traps).
