# libwl WASM build target

**Experimental.** The `wlwasm/` package compiles the wanted-board
subset of the wasteland Go SDK to WebAssembly (`GOOS=js GOARCH=wasm`)
so that Cloudflare Workers — and other JS-hosted runtimes — can
invoke wanted-board operations directly without packaging the `wl`
CLI into a container or talking to a long-lived Go process.

This document is the steady-state reference for the build target.
For the operations the bundle exposes, see `wlwasm/README.md`.

## Why

The hosted wasteland service in the Kilo monorepo
(`cloud/services/wasteland/`) used to dispatch every wanted-board
operation to a Cloudflare Container, which itself shelled out to the
`wl` CLI on each request. That added container-cold-start latency, a
Bun runtime, a Docker image, and an HTTP layer between the Worker
and a real DoltHub call.

The observation that motivated this work: in non-interactive use, `wl`
already defaults to `BackendRemote`, and `internal/backend/remote.go`
is ~600 lines of pure `net/http` against DoltHub's REST API. There's
no need for the dolt subprocess on the wanted-board hot path. The
`internal/sdk.Client` already mediates everything; the missing piece
is shipping a JS-callable surface around it.

`wlwasm/` is that surface.

## What's in the bundle

The wasm bundle is built from `wlwasm/`, which depends on:

- `internal/sdk` — the orchestration brain (mode-aware mutations,
  branch overlays, idempotency).
- `internal/backend.RemoteDB` — pure HTTP against DoltHub's SQL +
  write APIs.
- `internal/remote.DoltHubProvider` — fork/PR/merge HTTP calls.
- `internal/commons` — DML builders and the SQL fragments that the
  CLI also uses. Subprocess-coupled helpers (`dolt_local.go`) are
  excluded by build tag.
- `internal/federation` — the value types in `config.go` only.
  Subprocess-coupled `Service`, `DoltCLI`, etc. live in
  `federation.go` (`!js`-tagged) and are excluded.
- A no-op `internal/observability` shim
  (`otel_wasm.go`) so the OTel HTTP exporters don't get pulled in.
- A no-op `internal/xdg` shim (`xdg_wasm.go`) — the wasm bundle has
  no filesystem, so paths from `XDG_CONFIG_HOME` would be
  meaningless.

What's **not** in the bundle:

- The `wl` CLI (`cmd/wl/`).
- The TUI (`internal/tui`).
- The embedded web UI (`web/`).
- `wl create` — bootstrapping a new commons still requires the
  local `dolt` CLI. Reimplementing it on the DoltHub REST API is
  follow-up work; the wasm bundle does not currently support it.
- `wl join` — similar story, depends on dolt clone/fetch. Hosted
  callers typically synthesize the post-join config directly
  (the rig has already been registered).
- `wl sync` and anything else that touches the local clone.

## Sizes

As of writing:

| Metric        | Bytes      |
|---------------|-----------:|
| Raw           | ~12.5 MB   |
| gzip `-9`     | ~3.2 MB    |

Build with `make wasm-build` to see current numbers and the
dependency list (`bin/libwl.deps.txt`).

Cloudflare Workers' compressed-bundle limit is 10 MB on the paid
plan, 3 MB on free. The current bundle fits the paid plan with
headroom. If you change something that drags in OTel exporters,
sentry-go, or the bubbletea TUI, expect the gzipped size to jump by
1-3 MB and re-evaluate.

## Building

```bash
make wasm-build
```

Builds `bin/libwl.wasm`, reports raw + gzipped sizes (and brotli, if
installed), and writes the dependency closure to
`bin/libwl.deps.txt`. The Makefile target uses
`-trimpath -ldflags="-s -w"` to strip debug info — non-trivial
size win and the bundle is not a debugging artifact.

The host build (`make build`) is unaffected.

## Build tags

The wasm bundle is gated by Go's `js`/`!js` build tags. Files that
shell out to `dolt`, touch the filesystem in a way that's
meaningless under wasm, or depend on the OTel HTTP exporters, are
tagged `//go:build !js` so they're excluded from the wasm build.
A small number of `//go:build js` files (`otel_wasm.go`,
`xdg_wasm.go`, `dolt_wasm.go`) provide the no-op replacements
needed to keep the wasm-side call graph compilable.

The convention is intentionally simple: `!js` excludes from wasm,
`js` is wasm-only. A future `wasip1` build target could use a
narrower constraint.

## Calling convention

Each operation in `wlwasm/api.go` has a Go struct input and
returns the SDK's typed result. `wlwasm/js_bridge.go` wraps each
function in a JS-callable that:

1. Accepts a single JSON string argument.
2. Spawns a Go goroutine to do the work — necessary because
   `js.FuncOf` callbacks otherwise deadlock the JS event loop the
   moment Go calls `fetch` (per `syscall/js` docs:
   <https://pkg.go.dev/syscall/js#FuncOf>).
3. Returns a JS Promise that resolves to a JSON envelope:
   `{ ok: true, data } | { ok: false, error }`.

Example from JS:

```js
const result = await globalThis.wlBrowse(JSON.stringify({
  upstream: "myorg/wl-commons",
  dolthub_token: "...",
  user_id: "...",
  rig_handle: "...",
  view: "all",
}));
const envelope = JSON.parse(result);
if (envelope.ok) {
  console.log(envelope.data.items);
}
```

The eight registered globals are `wlBrowse`, `wlClaim`,
`wlUnclaim`, `wlDone`, `wlPost`, `wlAccept`, `wlReject`,
`wlClose`. Inputs and outputs are documented in `wlwasm/README.md`.

## Loading from a Cloudflare Worker

This repo ships only the bundle and Go's standard `wasm_exec.js`
glue (`$GOROOT/lib/wasm/wasm_exec.js`). Worker integration lives in
the consumer (e.g. `cloud/services/wasteland/`). The integration
shape:

1. `import wasmModule from './libwl.wasm'` — Wrangler bundles
   `.wasm` imports as `WebAssembly.Module`.
2. Side-effect-import `wasm_exec.js`, which sets `globalThis.Go`.
3. Per request: `new Go()`, `WebAssembly.instantiate(wasmModule, go.importObject)`,
   kick off `go.run(instance)` without awaiting it (the wasm's
   `main()` blocks on `select{}`), then call `globalThis.wl*`.
4. JSON in, JSON out.

## Concurrency

`js_bridge.go` registers functions on `globalThis`. Two concurrent
requests on the same isolate would clobber each other's references.
Consumers should either:

- **Serialize calls** (POC-friendly; the cloud service does this).
- **Run a single long-lived Go runtime per isolate** with an
  internal work queue.
- **Move registration to a per-instance object** in a future revision
  of `js_bridge.go`. Tracked but not implemented; PRs welcome.

## Dependencies

`wlwasm/` deliberately depends only on `internal/sdk`,
`internal/backend`, `internal/remote/dolthub.go`, `internal/commons`,
`internal/federation` (config-only), and a couple of stub shims. Anything
that pulls in `os/exec`, `cloud.google.com/go/kms`, `jackc/pgx`,
the OTel HTTP exporters, or `getsentry/sentry-go` should fail the
build under `GOOS=js GOARCH=wasm` — that's by design. If you hit a
compile error after adding an import, treat it as a signal that the
new dependency needs build-tag gating, not as an obstacle to work
around.
