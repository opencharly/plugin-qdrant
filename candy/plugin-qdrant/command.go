package qdrant

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// command.go is the `charly qdrant` kong command tree. Targeting flags live on
// the Globals struct embedded in the ROOT node only — kong treats embedded-root
// flags as GLOBAL (parseable before OR after any subcommand) — and the root is
// kong.Bind()'d so every leaf Runner receives the populated root and reads the
// resolved endpoint from it. Each leaf resolves the endpoint through the same
// ladder (config.go) before dialing gRPC/REST. Every data operation goes through
// the OFFICIAL Go client; only telemetry/metrics use the small REST fallback.

// QdrantCmd is the `charly qdrant` command tree.
type QdrantCmd struct {
	Globals

	Version     VersionCmd     `cmd:"" help:"Show the server version (Go client health check)"`
	Health      HealthCmd      `cmd:"" help:"Probe the server health (Go client)"`
	Collections CollectionsCmd `cmd:"" help:"Collection helpers (list, create, info, exists, delete)"`
	Points      PointsCmd      `cmd:"" help:"Point helpers (upsert, query, scroll, count, get, delete)"`
	Snapshots   SnapshotsCmd   `cmd:"" help:"Snapshot helpers (list, create, delete)"`
	Telemetry   TelemetryCmd   `cmd:"" help:"Print server telemetry (REST)"`
	Metrics     MetricsCmd     `cmd:"" help:"Print Prometheus metrics (REST)"`
}

// leafRun resolves the endpoint, builds the engine, and runs fn.
func leafRun(g Globals, fn func(context.Context, *engine) (string, error)) error {
	ep, err := resolveEndpoint(g)
	if err != nil {
		return err
	}
	eng, err := newEngine(ep)
	if err != nil {
		return err
	}
	defer eng.close()
	ctx := context.Background()
	out, err := fn(ctx, eng)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

// ---------------------------------------------------------------------------
// health / version
// ---------------------------------------------------------------------------

type VersionCmd struct{}

func (c VersionCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) { return e.version(ctx) })
}

type HealthCmd struct{}

func (c HealthCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) { return e.health(ctx) })
}

type TelemetryCmd struct{}

func (c TelemetryCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) { return e.telemetry(ctx) })
}

type MetricsCmd struct{}

func (c MetricsCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) { return e.metrics(ctx) })
}

// ---------------------------------------------------------------------------
// collections
// ---------------------------------------------------------------------------

type CollectionsCmd struct {
	List   CollectionsListCmd   `cmd:"" help:"List collections"`
	Create CollectionsCreateCmd `cmd:"" help:"Create a collection"`
	Info   CollectionsInfoCmd   `cmd:"" help:"Show collection info"`
	Exists CollectionsExistsCmd `cmd:"" help:"Assert a collection exists"`
	Delete CollectionsDeleteCmd `cmd:"" help:"Delete a collection"`
}

type CollectionsListCmd struct{}

func (c CollectionsListCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) { return e.collections(ctx) })
}

type CollectionsCreateCmd struct {
	Name     string `arg:"" name:"name" help:"Collection name"`
	Size     uint64 `name:"size" required:"" help:"Vector dimension"`
	Distance string `name:"distance" default:"cosine" help:"Distance metric (cosine, euclid, dot, manhattan)"`
}

func (c CollectionsCreateCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.createCollection(ctx, c.Name, c.Size, c.Distance)
	})
}

type CollectionsInfoCmd struct {
	Name string `arg:"" name:"name" help:"Collection name"`
}

func (c CollectionsInfoCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.collectionInfo(ctx, c.Name)
	})
}

type CollectionsExistsCmd struct {
	Name string `arg:"" name:"name" help:"Collection name"`
}

func (c CollectionsExistsCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.collectionExists(ctx, c.Name)
	})
}

type CollectionsDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Collection name"`
}

func (c CollectionsDeleteCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.deleteCollection(ctx, c.Name)
	})
}

// ---------------------------------------------------------------------------
// points
// ---------------------------------------------------------------------------

type PointsCmd struct {
	Upsert PointsUpsertCmd `cmd:"" help:"Upsert a point (vector + optional payload)"`
	Query  PointsQueryCmd  `cmd:"" help:"Query nearest neighbours"`
	Scroll PointsScrollCmd `cmd:"" help:"Scroll points"`
	Count  PointsCountCmd  `cmd:"" help:"Count points"`
	Get    PointsGetCmd    `cmd:"" help:"Get a point by id"`
	Delete PointsDeleteCmd `cmd:"" help:"Delete a point"`
}

type PointsUpsertCmd struct {
	Collection string   `arg:"" name:"collection" help:"Collection name"`
	ID         uint64   `name:"id" required:"" help:"Point id (unsigned integer)"`
	Vector     string   `name:"vector" required:"" help:"Comma-separated vector, e.g. 0.1,0.2,0.3"`
	Payload    []string `name:"payload" help:"key=value payload entry (repeatable)"`
}

func (c PointsUpsertCmd) Run(root *QdrantCmd) error {
	vector, err := parseVector(c.Vector)
	if err != nil {
		return err
	}
	payload, err := parsePayload(c.Payload)
	if err != nil {
		return err
	}
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.upsertPoints(ctx, c.Collection, c.ID, vector, payload)
	})
}

type PointsQueryCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
	Vector     string `name:"vector" required:"" help:"Comma-separated query vector"`
	Limit      uint64 `name:"limit" default:"5" help:"Maximum results"`
}

func (c PointsQueryCmd) Run(root *QdrantCmd) error {
	vector, err := parseVector(c.Vector)
	if err != nil {
		return err
	}
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.queryPoints(ctx, c.Collection, vector, c.Limit)
	})
}

type PointsScrollCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
	Limit      uint32 `name:"limit" default:"10" help:"Maximum points"`
}

func (c PointsScrollCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.scrollPoints(ctx, c.Collection, c.Limit)
	})
}

type PointsCountCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
}

func (c PointsCountCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.countPoints(ctx, c.Collection)
	})
}

type PointsGetCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
	ID         uint64 `name:"id" required:"" help:"Point id"`
}

func (c PointsGetCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.getPoints(ctx, c.Collection, c.ID)
	})
}

type PointsDeleteCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
	ID         uint64 `name:"id" required:"" help:"Point id"`
}

func (c PointsDeleteCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.deletePoints(ctx, c.Collection, c.ID)
	})
}

// ---------------------------------------------------------------------------
// snapshots
// ---------------------------------------------------------------------------

type SnapshotsCmd struct {
	List   SnapshotsListCmd   `cmd:"" help:"List snapshots for a collection"`
	Create SnapshotsCreateCmd `cmd:"" help:"Create a snapshot"`
	Delete SnapshotsDeleteCmd `cmd:"" help:"Delete a snapshot"`
}

type SnapshotsListCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
}

func (c SnapshotsListCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.snapshots(ctx, c.Collection)
	})
}

type SnapshotsCreateCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
}

func (c SnapshotsCreateCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.createSnapshot(ctx, c.Collection)
	})
}

type SnapshotsDeleteCmd struct {
	Collection string `arg:"" name:"collection" help:"Collection name"`
	Snapshot   string `arg:"" name:"snapshot" help:"Snapshot file name"`
}

func (c SnapshotsDeleteCmd) Run(root *QdrantCmd) error {
	return leafRun(root.Globals, func(ctx context.Context, e *engine) (string, error) {
		return e.deleteSnapshot(ctx, c.Collection, c.Snapshot)
	})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// parsePayload parses repeated key=value flags into a payload map.
func parsePayload(entries []string) (map[string]any, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(entries))
	for _, e := range entries {
		k, v, ok := splitKV(e)
		if !ok {
			return nil, fmt.Errorf("payload entry %q must be key=value", e)
		}
		out[k] = v
	}
	return out, nil
}

func splitKV(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// parseVector parses a vector literal ("0.1,0.2,0.3") into []float32.
func parseVector(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty vector")
	}
	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := strconv.ParseFloat(p, 32)
		if err != nil {
			return nil, fmt.Errorf("parse vector element %q: %w", p, err)
		}
		out = append(out, float32(f))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty vector")
	}
	return out, nil
}
