# wlwasm

Wanted-board operations exposed as a `GOOS=js GOARCH=wasm`
WebAssembly bundle. See `docs/libwl-wasm.md` for the full design.

## Operations

Each operation is registered on `globalThis` and returns a JS
Promise that resolves to a JSON envelope.

### Common envelope

```json
{ "ok": true,  "data": <op-specific> }
{ "ok": false, "error": "<message>" }
```

The bridge returns a Promise that resolves with `ok: true` envelopes
and **rejects** with `ok: false` envelopes — JS callers can use
either `.then`/`.catch` or check `ok` after `await`. Errors
returned by the SDK preserve the underlying message (DoltHub HTTP
status, validation failure, etc.).

### Common input fields (`Env`)

Every operation accepts these as part of its input:

| Field            | Type   | Required | Notes |
|------------------|--------|----------|-------|
| `upstream`       | string | yes      | DoltHub upstream path, e.g. `"steveyegge/wl-commons"`. |
| `dolthub_token`  | string | yes      | DoltHub OAuth or API token with read+write access. |
| `user_id`        | string | yes      | Caller's identity. Used for the synthetic config; not sent to DoltHub. |
| `rig_handle`     | string | yes      | Rig handle that's embedded in branch names (`wl/<rig_handle>/<wanted_id>`). DoltHub branch names are 3-32 chars of letters/dashes/underscores — pick a sensible value. |
| `fork_org`       | string | no       | DoltHub org that owns the user's fork. Defaults to `user_id`, which is rarely correct outside testing — you almost always want to pass the user's actual DoltHub username here. |
| `fork_db`        | string | no       | Defaults to the upstream DB name (DoltHub forks share the upstream DB name). Override only for unusual fork layouts. |
| `direct`         | bool   | no       | When true, switches to wild-west mode (writes go to upstream `main` directly). Caller is responsible for the authorization check; the wasm bundle does not enforce it. |

### `wlBrowse`

Reads the wanted board.

Op-specific fields:

| Field        | Type        | Notes |
|--------------|-------------|-------|
| `status`     | string      | Filter by status (`open`, `claimed`, `in_review`, `completed`, `withdrawn`). |
| `project`    | string      | Filter by project tag. |
| `type`       | string      | Filter by type (`feature`, `bug`, `docs`, `other`). |
| `priority`   | int \| null | Filter by priority (0-3). Omit or send `null` to disable the filter. **Sending `0` filters to priority=0 items only**; this is correct but easy to miss. |
| `limit`      | int         | Defaults to 50 server-side. |
| `posted_by`  | string      | Filter by poster handle. |
| `claimed_by` | string      | Filter by claimer handle. |
| `search`     | string      | Substring match across title/description/tags. |
| `view`       | string      | `"all"` (default in wild-west), `"mine"` (default in PR mode), `"upstream"`. |
| `long`       | bool        | Include description and other detail fields. |

Returns `sdk.BrowseResult`:

```json
{
  "items": [...],
  "pending_ids": {"w-abc": 2},
  "upstream_pending": {"w-abc": [...]}
}
```

### `wlClaim`, `wlUnclaim`, `wlClose`

| Field     | Type   | Required |
|-----------|--------|----------|
| `item_id` | string | yes      |

Returns `sdk.MutationResult`.

### `wlDone`

| Field      | Type   | Required |
|------------|--------|----------|
| `item_id`  | string | yes      |
| `evidence` | string | yes      |

Returns `sdk.MutationResult`.

### `wlPost`

| Field          | Type     | Notes |
|----------------|----------|-------|
| `title`        | string   | required |
| `description`  | string   | |
| `project`      | string   | |
| `type`         | string   | one of `feature`, `bug`, `docs`, `other` |
| `priority`     | int      | 0-3; defaults to medium if omitted |
| `effort_level` | string   | `small`, `medium`, `large` |
| `tags`         | string[] | |

Returns `sdk.MutationResult`.

### `wlAccept`

| Field         | Type     | Notes |
|---------------|----------|-------|
| `item_id`     | string   | required |
| `quality`     | int      | 1-5 |
| `reliability` | int      | 1-5 |
| `severity`    | string   | `leaf`, `branch`, `root` |
| `skill_tags`  | string[] | |
| `message`     | string   | optional, attached to the stamp |

Returns `sdk.MutationResult`.

### `wlReject`

| Field     | Type   | Required |
|-----------|--------|----------|
| `item_id` | string | yes      |
| `reason`  | string | yes      |

Returns `sdk.MutationResult`.

## Building

From the repo root:

```bash
make wasm-build
```

See `docs/libwl-wasm.md` for the full architecture, build-tag
strategy, concurrency notes, and how to load the bundle from a
Cloudflare Worker.

## Smoke test

`wlwasm/cmd/wasm-smoke` is a host-build (`!js`) program that
exercises the same Go API the wasm bridge exposes, against a real
DoltHub upstream. Useful for regressing the pure-Go call graph
before debugging cross-language issues:

```bash
DOLTHUB_TOKEN=... DOLTHUB_ORG=jrf0110 \
  WLWASM_UPSTREAM=jrf0110/wl-commons \
  go run ./wlwasm/cmd/wasm-smoke browse
```

If `DOLTHUB_TOKEN` is unset, the smoke test exits cleanly without
hitting the network.
