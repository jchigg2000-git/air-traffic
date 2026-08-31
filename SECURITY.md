# Security

## Air-Traffic has no user model, and by default no write authentication

There is no sign-in, no account, and no per-human principal. The control plane is
single-operator by decision, which means an audit row can name the system but never a person
(`DECISIONS.md`, 2026-08-15).

No read is gated by the operator key — the observability surfaces are the product. (The four
`/api/gateway/*` spine routes, two of them reads, carry a separate gate — `AIRTRAFFIC_SPINE_KEY`,
below.) Writes are gated by one shared operator key, `AIRTRAFFIC_ADMIN_KEY`, covering adapter
patch, policy PUT, credential POST, harness run/sample, proposal approve/reject
(`internal/server/spine_auth.go`), and the two mutating synthetic-harness control paths
(`internal/synthetic/synthetic.go`).

**With `AIRTRAFFIC_ADMIN_KEY` unset, those writes are open to anyone who can reach the port.**
That is the posture the repo ships in and the one the one-command compose demo runs in. It is
stated rather than hidden: both binaries warn on every boot, and `GET /api/health` and
`GET /api/gateway/status` report `"admin_auth": "open"` until a key is set. Anyone who can reach
an open install can change the applied policy — and therefore what the inference gateway enforces
on live traffic.

**Binding differs between the two ways of running it; host exposure does not.**
`go run ./cmd/air-traffic-server` binds `127.0.0.1:8122` and the gateway binds `127.0.0.1:8125`.
Under compose both bind `0.0.0.0` *inside the container* (`AIRTRAFFIC_ADDR`,
`GATEWAY_LISTEN_ADDR`) — they have to, or neither a peer container nor the published port could
reach them — but every `ports:` entry in `docker-compose.yml` is pinned to the loopback
interface (`127.0.0.1:8122:8122`, `127.0.0.1:8125:8125`, `127.0.0.1:8126:3000`), so nothing is
reachable from off the host either way. **That prefix is the only thing holding the line:** drop
it — publish `8122:8122` — and an install whose writes are open by default is answering the
whole network.

Compose also falls back to throwaway shared secrets — `gwk-demo` for `GATEWAY_CLIENT_KEYS` and
`spine-dev-insecure` for `AIRTRAFFIC_SPINE_KEY` — so the stack comes up in one command. Both
binaries log a warning while those are live and `/api/gateway/status` reports
`spine_key_unrotated`. Run `./scripts/dev-env.sh` (add `--rotate` to replace existing keys)
before the stack is reachable by anything but you.

If you run this anywhere other than your own machine, put it behind something: keep it on
loopback and reach it over an SSH tunnel, or put a reverse proxy in front that does the
authentication, or keep it on a private network. Do not put it on the public internet.

## The gateway is the only thing on a request path

Everything else in this repo talks to a synthetic replica it hosts itself. The inference gateway
is different: pointed at a real upstream it carries a **real vendor credential** and **real prompt
bytes**, and it is the component where a security bug has consequences outside the process.

Three properties are worth knowing before you deploy it:

- **Detection is request-side only.** Responses are relayed byte-faithfully; a leak on the way
  back is scored informationally, not blocked. This is deferred work, not an oversight — see
  `docs/plans/TODO-gateway-deferred.md`.
- **Redaction happens before the upstream call, and the upstream sees the redacted text.** What
  it does with that is its retention policy, not ours.
- **`GATEWAY_FAIL_MODE` defaults to `closed`.** A detector that times out or errors fails the
  request rather than letting it through unredacted. Setting it to `open` inverts that; do so
  knowingly.

## Reporting a vulnerability

Please report security issues privately, through GitHub's
[private vulnerability reporting](https://docs.github.com/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability)
on this repository, rather than opening a public issue.

This is a small project maintained by one person. There is no bounty and no guaranteed response
window, but reports are read and taken seriously.

Given the sections above, "there is no authentication on reads" and "writes are open when the key
is unset" are known properties rather than findings. What is worth reporting is anything that
makes the damage worse than the model described here — for example a way to reach a resolved
upstream credential, to get a detected PII value into a log line or the audit stream, to make the
gateway forward to a host of your choosing, or to bypass detection with an input the detector
chain should have caught.

## What is worth scrutiny

- `internal/gateway/config/config.go` — the boot-time refusal of inline credentials. Config
  carries references (`env:NAME`), never values, and a value matching a raw-credential shape
  aborts startup before a port is bound. This is the first line and the cheapest to break.
- `internal/gateway/credbroker/` — where a credential reference becomes an actual secret, and the
  one place a resolved value exists in memory.
- `internal/redact/` and `internal/gateway/redact/` — the masking itself, and the guarantee that a
  redacted value never appears in a log line or the audit stream. That guarantee has its own CI
  job (`TestNoRedactedValueInLogsOrAudit`); treat a change that weakens it as a security change.
- `internal/server/spine_auth.go` — the three gates: `requireAdminWrite` (operator key),
  `requireAdminIngest` (two legitimate writers, two credentials), and `requireSpineKey`. Note
  that with `AIRTRAFFIC_SPINE_KEY` unset, the spine routes fall back to **loopback callers only**
  — and a container-network peer does not qualify.
- `internal/gateway/keystore.go` and `internal/store/keystore_persist.go` — issued keys are stored
  as SHA-256 digests and printed exactly once. The keystore admin API takes **a loopback caller or
  `AIRTRAFFIC_ADMIN_KEY`, and deliberately not the spine key**: the gateway holds that key, and a
  gateway that can mint its own credentials makes the keystore pointless. With no admin key set,
  the posture is loopback-only.
- `internal/gateway/proxy.go` — upstream request construction, the connect/TLS/response-header
  timeouts, and the byte-faithful response relay.

## Known properties that are not vulnerabilities

- **Revocation is eventual.** Gateways verify against a pulled snapshot, so a revoked keystore key
  stops working within one `GATEWAY_POLICY_PULL_INTERVAL` (15s by default), not instantly.
- **Observations, gateway request reports, drift and audit are in-memory ring buffers.** A restart
  loses them. Three things persist under `AIRTRAFFIC_DATA_DIR`: the keystore (`keys.json`), the
  applied policy (`policy.json`) and the harness flywheel state (`ratchet.jsonl`, `corpus/*.json`,
  `patterns.json`).
- **The synthetic vendor surfaces are unauthenticated and answer anyone.** They serve fabricated
  data and hold nothing real. Their two mutating control paths are the exception, and both carry
  the operator key: `_harness/scenario` writes the shared adapter record, `_harness/reset` drops
  that adapter's recorded calls.
- **`GATEWAY_CLIENT_KEYS` authenticates but does not identify.** Callers using it are attributed
  as app `env`. Per-caller attribution is what the keystore adds.

## Operational notes

- **Set `AIRTRAFFIC_ADMIN_KEY` before the control plane is reachable by anything but you**, and
  rotate the compose defaults with `./scripts/dev-env.sh --rotate`. Once it is set, paste it into
  the SPA sidebar's "Operator key" field or every console write will 401.
- **Set `AIRTRAFFIC_DATA_DIR` to a persistent path in any deployment.** On a container host the
  default `data/harness` is replaced on every deploy, which silently discards the keystore, the
  applied policy and the harness ratchet. A corrupt `policy.json` warns and boots with no policy
  applied rather than refusing to start — which means enforcement can silently drop to none after
  a bad write.
- **Vendor upstream credentials are read from the environment server-side only**, resolved through
  the credential broker. Never put one in `GATEWAY_UPSTREAMS` itself; the config loader will
  refuse to start, which is the intended behaviour.
- **The OpenAI-compatible route's upstream credential has no compose fallback on purpose.** Unset,
  the route returns 502 locally rather than sending a placeholder to a real vendor.
- **Presidio is self-hosted.** The NER tier runs in a container you control
  (`deploy/presidio/docker-compose.yml`); no prompt text goes to a third-party analysis service.
