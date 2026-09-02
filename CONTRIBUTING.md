# Contributing to Air-Traffic

Issues, bug reports and pull requests are welcome.

## Licensing of contributions

Air-Traffic is released under the [MIT License](LICENSE).

A contribution you deliberately submit for inclusion is licensed under the same terms as the
rest of the software, unless you explicitly say otherwise. There is no separate contributor
license agreement to sign and no copyright assignment.

**By opening a pull request you confirm that:**

1. You wrote the contribution yourself, or you otherwise have the right to submit it under the
   MIT License.
2. You retain your own copyright in your contribution — nothing here is an assignment, and you
   may continue to use your own work however you like.

## Signing your commits (optional)

`git commit -s` adds a `Signed-off-by` line as a record of the two points above, and it is
welcome:

```bash
git commit -s -m "your message"
```

It is not required, and no PR will be held for it. Nothing in CI checks for a DCO line, and not
one commit in this repo's own history carries one — a rule the maintainer has never followed is
worse than no rule, so this is a preference rather than a gate.

## Before you open a PR

```bash
gofmt -l .          # must print nothing
go vet ./...
go build ./...
go test -race ./...

cd web
npm ci
npm audit --omit=dev --audit-level=high   # production tree only; a dev advisory is not a gate
npm run build       # tsc --noEmit && vite build
npm test            # vitest run
```

All of it should be clean. `.github/workflows/ci.yml` runs those same commands across four jobs —
`go`, `log-leak-guard`, `web`, `docker` — plus three checks you cannot run as a one-liner: a guard
that fails the build if a `go.sum` appears, the redaction log-leak test as its own signal, and both
Docker image targets. Anything that fails locally fails there too. The Go tests need no network, no
database and no Docker; the ones that exercise the gateway stand up their own listeners on
ephemeral ports.

The Go floor is whatever `go.mod` says, currently **go 1.26** with the toolchain pinned to
**go1.26.7** — the pin is deliberate and is there to pick up stdlib security fixes, so don't relax
it to make a local build work. Node's floor is **22**, set by CI.

Two optional extras are not part of the gate. `E2E_COMPOSE=1 ./scripts/e2e-gateway.sh` asserts a
running compose stack end to end and needs Docker; `docker compose -f
deploy/presidio/docker-compose.yml up -d` brings up the NER tier alone. Neither runs in CI.

## Getting it running

See the Quickstart in [README.md](README.md). Short version: `docker compose up -d --build`, then
open <http://127.0.0.1:8122/>. `go run ./cmd/air-traffic-server` serves the API only until
`cd web && npm run build` writes the gitignored `web/dist`; for the SPA in dev, `npm run dev` on
:5202.

You need no credential of any kind to run the control plane, the synthetic surfaces, the SPA or
the test suite. The only thing that ever wants a real vendor key is the inference gateway pointed
at a real upstream, and that is opt-in.

## Orientation

A few things about this codebase are deliberate and will look like bugs if you don't know them.

[`CLAUDE.md`](CLAUDE.md) is the short form of the working agreement;
[`ROADMAP.md`](ROADMAP.md) is the long form of what is open.

**The backend is synthetic, and that is the product rather than a placeholder.** Nothing in the
control plane makes an outbound call to a vendor. `/synthetic/{vendor}/{native-path}` serves a
replica the binary hosts itself — a hand-written fixture for seven vendors, a labeled generic
envelope for the other nine — and adapter `proxy` mode is a deliberate stub
that returns `501 proxy_not_normalized` (`internal/synthetic/synthetic.go`). A change that gives
an adapter a real network path is a conversation before it is a pull request. The one component
that touches real bytes is the gateway, and only when you point it at a real upstream.

**Stdlib only, and it is enforced.** There is no `go.sum`, and CI fails the build if one appears.
Adding a Go dependency is not a code-review decision — raise it first. The web side has
dependencies; the Go side does not.

**Disposition honesty is the thesis, not a UI detail.** Every capability carries one of five
dispositions — `vendor_native`, `env_managed`, `proxy_enforced`, `monitor_only`, `unverified` —
and every `env_managed` one carries an enforcement tier (`server_side` / `mdm_locked` /
`seed_only`). A `seed_only` control is a suggestion a user can undo, and the UI must never render
it as "enforced". If a change makes a weaker control read as a stronger one, that is the bug,
even when the code is correct.

**`ROADMAP.md` is the single source of truth, and closure is deletion.** A finished item is
deleted in the closing edit, not ticked and kept. Git and the commit history are the history
layer. `DECISIONS.md` is append-only — supersede an entry, never rewrite it. Two files under
`docs/plans/` (`TODO-gateway-deferred.md`, `TODO-vendor-auth.md`) deliberately stay standalone
because live source cites them by path; keep them current in place.

**The gateway takes secret *references*, not secrets.** `GATEWAY_UPSTREAMS` entries carry
`credential_ref: env:NAME`, and a value that merely looks like a raw credential kills the boot
before a port is bound (`internal/gateway/config/config.go`). This is why the config is more
ceremonious than it first appears; don't simplify it by accepting an inline key.

**Redaction is proven behaviorally, not asserted.** The harness sends seeded synthetic PII
through the gateway and then checks what the *upstream capture actually received* — a test that
only inspects the gateway's own report of itself proves nothing. The matching guarantee, that a
redacted value never reaches a log line or the audit stream, has its own CI job
(`TestNoRedactedValueInLogsOrAudit`). If you touch `internal/gateway/detect`,
`internal/gateway/redact` or `internal/redact`, expect to touch that guard too.

**Request-side only.** Responses are relayed byte-faithfully and a leak on the way back is scored
informationally rather than blocked. That is deferred work (G4), documented in
[`docs/plans/TODO-gateway-deferred.md`](docs/plans/TODO-gateway-deferred.md) — a scan will keep
rediscovering it, and it is not a defect to fix in passing.

**One port pair, two ways to run.** The control plane is `:8122`, the gateway `:8125`, the
Presidio sidecar `:8126`, and the Vite dev server `:5202`. The compose stack and the bare `go run`
flow use the same ports, so run one or the other, not both. Compose images bake built source: after
a code change use `docker compose up -d --build <service>`, because a bare `restart` reuses the old
image. `.devlauncher.json` at the repo root registers that compose command (`docker compose up
--build`, `:8122`) with an external dev-launcher tool the author uses; nothing in this repo
reads it, and you can ignore it.

**In-memory by decision.** Observations, gateway request reports, drift and audit are ring buffers
that a restart clears. Three things are written through to `AIRTRAFFIC_DATA_DIR`: the keystore
(`keys.json`, because issued credentials are not reconstructible), the applied policy
(`policy.json`, because the gateway is already enforcing it), and the harness flywheel state
(`ratchet.jsonl`, `corpus/*.json`, `patterns.json` — a ratchet that resets is not a ratchet).
Adding a fourth means arguing with `DECISIONS.md` first.

## Scope

Air-Traffic is a control- and observability-plane spine for enterprise AI vendors, built against
synthetic replicas of their admin surfaces. The most useful changes are ones that make the vendor
surface collection more faithful, the enforcement claims more honest, or the gateway's detection
measurably better. If a change affects detection or enforcement, say what you measured and how —
a recall or precision number with the run configuration beside it, not an assertion.
