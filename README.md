# plugin-qdrant

Qdrant vector-search management for OpenCharly — the `charly qdrant` CLI and the
`qdrant:` check verb.

The plugin drives a Qdrant server through the **official Go client**
([`github.com/qdrant/go-client`](https://github.com/qdrant/go-client), gRPC on
6334) for collections/points/snapshots/health, and the REST API (6333) only for
telemetry/metrics. No upstream `qdrant` binary is needed on the host.

It is an **out-of-tree external plugin**: projects compose it via the
`@github.com/opencharly/plugin-qdrant/candy/plugin-qdrant:<ref>` candy ref and
charly connects it out-of-process by word at runtime — zero charly-module import.

- **`charly qdrant`** — manage a server: `version`, `health`, `collections
  list|create|info|exists|delete`, `points upsert|query|scroll|count|get|delete`,
  `snapshots list|create|delete`, `telemetry`, `metrics`.
- **`qdrant:`** — the declarative check verb for beds: `health`, `version`,
  `auth-required`, `collections-list`, `collection-create|info|exists|delete`,
  `points-upsert|query|scroll|count|get|delete`, `snapshot-list|create|delete`.

The `qdrant:` verb is host-based: it resolves the in-venue REST/gRPC ports over
the reverse channel and drives the same Go client, reading the admin key
host-side from the credential store (`charly/api-key/qdrant`) over
`verb:credential`. All methods skip under `charly check box` (no running server on
a disposable `podman run --rm`).

## What it provides

| Capability | Surface |
|---|---|
| `command:qdrant` | the `charly qdrant` management CLI |
| `verb:qdrant` | the declarative `qdrant:` check verb |

## How to use it

Compose the plugin candy in a box or check bed:

```yaml
- '@github.com/opencharly/plugin-qdrant/candy/plugin-qdrant:<tag>'
```

Then author the verb in a plan:

```yaml
- check: create the demo collection
  id: qdrant-create
  context: [runtime]
  qdrant: {method: collection-create, collection: demo, size: 4, distance: cosine}
- check: upsert a point
  id: qdrant-upsert
  context: [runtime]
  qdrant: {method: points-upsert, collection: demo, id: 1, vector: [0.1, 0.2, 0.3, 0.4], payload: {city: London}}
- check: query nearest neighbours
  id: qdrant-query
  context: [runtime]
  qdrant: {method: points-query, collection: demo, vector: [0.1, 0.2, 0.3, 0.4], limit: 5}
- check: the admin key is enforced
  id: qdrant-auth
  context: [runtime]
  qdrant: {method: auth-required}
```

## Endpoint resolution

`--host` / `--api-key` / `--grpc-port` / `--rest-port` / `--tls` flags >
`QDRANT_HOST` / `QDRANT_API_KEY` env > `http://127.0.0.1:6333` (REST) with gRPC
on `6334`. A schemeless `--host` gets `http://` prepended; `0.0.0.0` (the candy's
server bind) maps to `127.0.0.1` for client use. The deployment's actual host port
comes from `charly status <name>`.

## Testing

- `go test ./...` — pure helpers plus `TestLiveCapabilities`, which boots a REAL
  qdrant (set `QDRANT_TEST_BINARY` or put `qdrant` on `PATH`) and exercises the
  full surface. With no binary it **SKIPS cleanly** rather than faking the gRPC
  boundary.

```bash
cd candy/plugin-qdrant && go test ./...
QDRANT_TEST_BINARY=/path/to/qdrant go test ./...
```

## Layout

- `candy/plugin-qdrant/` — the plugin module: `client.go` (the Go-client
  backend), `command.go` (the CLI), `verb.go` (the `qdrant:` verb),
  `config.go` (endpoint resolution), `schema/qdrant.cue`,
  `params/cue_types_gen.go`, `cmd/serve/main.go`.
- `charly.yml` — the root project manifest (`discover: candy` + the
  `qdrant-cli-skill` skill entity).
- `.github/workflows/ci.yml` + `.github/workflows/tag-on-merge.yml` — the Go
  gates and the CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-qdrant:qdrant-cli` — the `charly qdrant` CLI and the
  `qdrant:` check verb, authored in this candy's `skill:` entity.
- [`pod-qdrant`](https://github.com/opencharly/pod-qdrant) — the box image + the
  `check-qdrant-pod` R10 bed that prove the whole stack.
- [`layer-qdrant`](https://github.com/opencharly/layer-qdrant) — the qdrant
  service candy the box composes.
