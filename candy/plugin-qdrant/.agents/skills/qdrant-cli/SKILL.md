---
name: qdrant-cli
description: 'The `charly qdrant` management CLI for a deployed Qdrant

  vector-search server PLUS the declarative `qdrant:` check verb

  (command:qdrant + verb:qdrant plugin, plugin-qdrant). MUST be invoked

  before managing collections, points, or snapshots on a running Qdrant

  deployment from the host, or before authoring/verifying a qdrant check

  bed.'
---

# qdrant-cli

The `charly qdrant` management CLI for a deployed Qdrant vector-search
server PLUS the declarative `qdrant:` check verb — a `command:qdrant` +
`verb:qdrant` plugin, composed out-of-tree via
`@github.com/opencharly/plugin-qdrant/candy/plugin-qdrant` and connected
out-of-process by word at runtime.

The plugin drives the server through the OFFICIAL Go client
(`github.com/qdrant/go-client`, gRPC on 6334) for
collections/points/snapshots/health and the REST API only for
telemetry/metrics. It needs no upstream `qdrant` binary on the host.

## Endpoint resolution

`--host` / `--api-key` / `--grpc-port` / `--rest-port` / `--tls` flags >
`QDRANT_HOST` / `QDRANT_API_KEY` env > `http://127.0.0.1:6333` (REST) with
gRPC on `6334`. A schemeless `--host` gets `http://` prepended; `0.0.0.0`
(the candy's server bind) maps to `127.0.0.1` for client use. The
deployment's actual host port comes from `charly status <name>`
(auto-allocated on 127.0.0.1 at deploy).

## Command tree

- `charly qdrant version` — the server version (Go-client health check).
- `charly qdrant health` — the server health (Go client).
- `charly qdrant collections list` — list collections.
- `charly qdrant collections create <name> --size <n> [--distance cosine|euclid|dot|manhattan]`.
- `charly qdrant collections info <name>` / `collections exists <name>` / `collections delete <name>`.
- `charly qdrant points upsert <coll> --id <n> --vector 0.1,0.2,… [--payload k=v …]`.
- `charly qdrant points query <coll> --vector 0.1,0.2,… [--limit N]`.
- `charly qdrant points scroll <coll> [--limit N]`.
- `charly qdrant points count <coll>` / `points get <coll> --id <n>` / `points delete <coll> --id <n>`.
- `charly qdrant snapshots list|create|delete <coll> [<snapshot>]`.
- `charly qdrant telemetry` / `metrics` — REST-only diagnostics.

## The `qdrant:` check verb

The declarative counterpart, authored as a `check:` step in a candy/box
plan. It is HOST-BASED: it resolves the in-venue REST/gRPC ports over the
reverse channel and drives the SAME Go client, reading the admin key
host-side from the credential store (`charly/api-key/qdrant`) over
`verb:credential`. The full method surface lets a plan fully configure
and exercise a Qdrant instance from `charly.yml` alone:

- `qdrant: health` / `version` / `auth-required`.
- `qdrant:` `collections-list` / `collection-create` (`size`, `distance`) /
  `collection-info` / `collection-exists` / `collection-delete`.
- `qdrant:` `points-upsert` (`id`, `vector`, `payload`) / `points-query`
  (`vector`, `limit`) / `points-scroll` / `points-count` / `points-get`
  (`id`) / `points-delete` (`id`).
- `qdrant:` `snapshot-list` / `snapshot-create` / `snapshot-delete`.

```yaml
plan:
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

All methods skip under `charly check box` (no running server on a
disposable `podman run --rm`).

## Cross-References

- `/charly-qdrant:qdrant` — the qdrant service candy (layer-qdrant).
- the qdrant box + `check-qdrant-pod` R10 bed (the `pod-qdrant` repo). Named as a
  REPO, not a `/charly-<family>:<skill>` ref: `pod-qdrant` owns no `skill:` entity,
  so such a ref would not resolve (and an unresolvable ref is a hard error in the
  docs build).
- `/charly-internals:plugin` — external command/verb plugins, the provider model.
