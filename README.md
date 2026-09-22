# plugin-qdrant

The Qdrant plugin candy for [opencharly/charly](https://github.com/opencharly/charly):
a `command:qdrant` CLI plus the declarative `qdrant:` check verb, driving a Qdrant
vector-search server through the **official Go client**
([`github.com/qdrant/go-client`](https://github.com/qdrant/go-client), gRPC on
6334) and the REST API (6333). No upstream `qdrant` binary is needed on the host.

The plugin is an OUT-OF-TREE external plugin: projects compose it via the
`@github.com/opencharly/plugin-qdrant/candy/plugin-qdrant:<ref>` candy ref and
charly connects it OUT-OF-PROCESS by word at runtime (the `qdrant:` verb + the
`charly qdrant` CLI both dispatch through the `cmd/serve` gRPC shim) — zero
charly-module import, per the kernel/plugin boundary law.

- `charly qdrant` — manage a server: `version`, `health`, `collections
  list|create|info|exists|delete`, `points upsert|query|scroll|count|get|delete`,
  `snapshots list|create|delete`, `telemetry`, `metrics`.
- `qdrant:` — the declarative check verb for beds: `health`, `version`,
  `auth-required`, `collections-list`, `collection-create|info|exists|delete`,
  `points-upsert|query|scroll|count|get|delete`, `snapshot-list|create|delete`.

## Endpoint resolution

`--host` / `--api-key` / `--grpc-port` / `--rest-port` / `--tls` flags >
`QDRANT_HOST` / `QDRANT_API_KEY` env > `http://127.0.0.1:6333` (REST) with gRPC
on `6334`. The verb reads the admin key host-side from the credential store
(`charly/api-key/qdrant`) over `verb:credential`.

## Testing

- `go test ./...` — pure helpers plus `TestLiveCapabilities`, which boots a REAL
  qdrant (set `QDRANT_TEST_BINARY` or put `qdrant` on `PATH`) and exercises the
  full surface: health, version, auth boundary, collection CRUD, point
  upsert/query/scroll/count/get/delete, and snapshots. With no binary it SKIPS
  cleanly rather than faking the gRPC boundary.

```bash
cd candy/plugin-qdrant && go test ./...
QDRANT_TEST_BINARY=/path/to/qdrant go test ./...
```

- [`pod-qdrant`](https://github.com/opencharly/pod-qdrant) — the box image + the
  `check-qdrant-pod` R10 bed that prove the whole stack.
- [`layer-qdrant`](https://github.com/opencharly/layer-qdrant) — the qdrant
  service candy the box composes.
