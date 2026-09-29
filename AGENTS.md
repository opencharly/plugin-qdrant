# AGENTS.md — plugin-qdrant

Standalone plugin repo for the Qdrant `command:qdrant` CLI and the `qdrant:`
check verb. The plugin is a Go module at `candy/plugin-qdrant/` (module path
`github.com/opencharly/plugin-qdrant/candy/plugin-qdrant`); the root `charly.yml`
declares `discover: candy` **and** the `qdrant-cli-skill` `skill:` entity (the
corpus source for `/charly-qdrant:qdrant-cli`).

Canonical files:

- `candy/plugin-qdrant/charly.yml` — the `plugin-qdrant:` candy entity
  (`plugin:` block, `plan:` check) + the `qdrant-cli-skill` skill entity.
- `candy/plugin-qdrant/client.go` — the official Go-client backend.
- `candy/plugin-qdrant/command.go` — the `charly qdrant` CLI tree.
- `candy/plugin-qdrant/verb.go` — the declarative `qdrant:` check verb.
- `candy/plugin-qdrant/config.go` — endpoint resolution.
- `candy/plugin-qdrant/schema/qdrant.cue` — the self-contained schema.
- `.github/workflows/ci.yml` + `.github/workflows/tag-on-merge.yml`.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the unified Provider model, the external command/verb shape, the
  per-plugin CUE-schema contract, placement. Load before touching the provider or
  schema.
- `/charly-qdrant:qdrant-cli` — the `charly qdrant` CLI and the `qdrant:` check
  verb this candy owns.
- `/charly-check:check` — the check orchestrator, beds and the R10 sequence.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-qdrant/` — compile the plugin module.
- `go test ./...` in `candy/plugin-qdrant/` — pure helpers plus
  `TestLiveCapabilities`, which boots a REAL qdrant and **SKIPS cleanly** when no
  binary is present (never fakes the gRPC boundary).
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema, the skill entity).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- R10 consumer: the `check-qdrant-pod` bed in `opencharly/pod-qdrant`.

## Modify this repo

- Edit the `plugin-qdrant:` candy entity, the Go source, and `schema/qdrant.cue`
  **together** — the schema is the single source for the `params/` struct, so a
  field change not mirrored in the schema desyncs the generated types.
- The `skill:` entity is the corpus source for `/charly-qdrant:qdrant-cli`; a
  change to the CLI/verb surface belongs in BOTH the candy and the skill body.
- The plugin is **out-of-tree external** (connected out-of-process by word); do
  not describe it as compiled-in.

## Landing

- PR-only. Every change lands through a pull request; the org-required
  `charly/pr-validator` validates the diff and body and arms native auto-merge on
  PASS. Direct pushes to `main` are blocked.
- History lives in `CHANGELOG/` (written by `tag-on-merge` at merge time); the PR
  body IS the changelog.
- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Do not
  restate its rules here.
