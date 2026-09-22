package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	qc "github.com/qdrant/go-client/qdrant"
)

// client.go is the plugin's engine: BOTH the `charly qdrant` leaves and the
// `qdrant:` check verb drive the server through the OFFICIAL Go client
// (github.com/qdrant/go-client) — one protocol surface, two placements (R3). The
// client speaks gRPC on 6334 for collections/points/snapshots/health and there is
// a small REST fallback only for the two endpoints the client does not cover
// (/telemetry and /metrics, both CLI-only diagnostics).

// engine holds the resolved target and a lazily-created gRPC client.
type engine struct {
	ep   endpoint
	grpc *qc.Client
}

func newEngine(ep endpoint) (*engine, error) {
	return &engine{ep: ep}, nil
}

// grpcClient lazily dials the gRPC port. A fresh engine per command means the
// connection lives for the command's duration.
func (e *engine) grpcClient() (*qc.Client, error) {
	if e.grpc != nil {
		return e.grpc, nil
	}
	c, err := qc.NewClient(&qc.Config{
		Host:                   e.ep.grpcHost,
		Port:                   e.ep.grpcPort,
		APIKey:                 e.ep.apiKey,
		UseTLS:                 e.ep.tls,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect qdrant gRPC %s:%d: %w", e.ep.grpcHost, e.ep.grpcPort, err)
	}
	e.grpc = c
	return c, nil
}

// close releases the gRPC connection.
func (e *engine) close() {
	if e.grpc != nil {
		_ = e.grpc.Close()
		e.grpc = nil
	}
}

// ---------------------------------------------------------------------------
// health / version (Go client)
// ---------------------------------------------------------------------------

func (e *engine) health(ctx context.Context) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	reply, err := c.HealthCheck(ctx)
	if err != nil {
		return "", fmt.Errorf("health check: %w", err)
	}
	return fmt.Sprintf("qdrant healthy: %s %s (commit %s)", reply.GetTitle(), reply.GetVersion(), reply.GetCommit()), nil
}

func (e *engine) version(ctx context.Context) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	reply, err := c.HealthCheck(ctx)
	if err != nil {
		return "", fmt.Errorf("version: %w", err)
	}
	return fmt.Sprintf("qdrant %s (commit %s)", reply.GetVersion(), reply.GetCommit()), nil
}

// authRequired asserts that an UNAUTHENTICATED client cannot list collections —
// proving the admin key is enforced. It dials a second client with no API key and
// expects the request to be rejected.
func (e *engine) authRequired(ctx context.Context) (string, error) {
	anon, err := qc.NewClient(&qc.Config{
		Host:                   e.ep.grpcHost,
		Port:                   e.ep.grpcPort,
		APIKey:                 "",
		UseTLS:                 e.ep.tls,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return "", fmt.Errorf("dial anonymous client: %w", err)
	}
	defer anon.Close()
	if _, err := anon.ListCollections(ctx); err == nil {
		return "", fmt.Errorf("auth-required: an unauthenticated client was NOT rejected (is QDRANT__SERVICE__API_KEY set?)")
	}
	return "unauthenticated requests are rejected (admin key enforced)", nil
}

// ---------------------------------------------------------------------------
// collections (Go client)
// ---------------------------------------------------------------------------

func (e *engine) collections(ctx context.Context) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	names, err := c.ListCollections(ctx)
	if err != nil {
		return "", fmt.Errorf("list collections: %w", err)
	}
	if len(names) == 0 {
		return "(no collections)", nil
	}
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintln(&b, n)
	}
	fmt.Fprintf(&b, "%d collection(s)", len(names))
	return b.String(), nil
}

func (e *engine) collectionExists(ctx context.Context, name string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	ok, err := c.CollectionExists(ctx, name)
	if err != nil {
		return "", fmt.Errorf("collection exists %s: %w", name, err)
	}
	if !ok {
		return "", fmt.Errorf("collection %s does not exist", name)
	}
	return fmt.Sprintf("collection %s exists", name), nil
}

func (e *engine) collectionInfo(ctx context.Context, name string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	info, err := c.GetCollectionInfo(ctx, name)
	if err != nil {
		return "", fmt.Errorf("collection %s: %w", name, err)
	}
	var vecSize uint64
	var dist string
	if p := info.GetConfig().GetParams(); p != nil {
		if vp := p.GetVectorsConfig().GetParams(); vp != nil {
			vecSize = vp.GetSize()
			dist = vp.GetDistance().String()
		}
	}
	return fmt.Sprintf("collection %s: status=%s points=%d indexed=%d vector_size=%d distance=%s segments=%d",
		name, info.GetStatus().String(), info.GetPointsCount(),
		info.GetIndexedVectorsCount(), vecSize, dist, info.GetSegmentsCount()), nil
}

func (e *engine) createCollection(ctx context.Context, name string, size uint64, distance string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	dist, err := parseDistance(distance)
	if err != nil {
		return "", err
	}
	if _, err := c.CollectionExists(ctx, name); err == nil {
		exists, _ := c.CollectionExists(ctx, name)
		if exists {
			return fmt.Sprintf("collection %s already exists", name), nil
		}
	}
	if err := c.CreateCollection(ctx, &qc.CreateCollection{
		CollectionName: name,
		VectorsConfig:  qc.NewVectorsConfig(&qc.VectorParams{Size: size, Distance: dist}),
	}); err != nil {
		return "", fmt.Errorf("create collection %s: %w", name, err)
	}
	return fmt.Sprintf("created collection %s (size=%d distance=%s)", name, size, dist.String()), nil
}

func (e *engine) deleteCollection(ctx context.Context, name string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	if err := c.DeleteCollection(ctx, name); err != nil {
		return "", fmt.Errorf("delete collection %s: %w", name, err)
	}
	return fmt.Sprintf("deleted collection %s", name), nil
}

// ---------------------------------------------------------------------------
// points (Go client)
// ---------------------------------------------------------------------------

func (e *engine) upsertPoints(ctx context.Context, collection string, id uint64, vector []float32, payload map[string]any) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	res, err := c.Upsert(ctx, &qc.UpsertPoints{
		CollectionName: collection,
		Wait:           boolPtr(true),
		Points: []*qc.PointStruct{{
			Id:      qc.NewIDNum(id),
			Vectors: qc.NewVectors(vector...),
			Payload: qc.NewValueMap(payload),
		}},
	})
	if err != nil {
		return "", fmt.Errorf("upsert point %d into %s: %w", id, collection, err)
	}
	return fmt.Sprintf("upserted point %d into %s (status=%s)", id, collection, res.GetStatus().String()), nil
}

func (e *engine) queryPoints(ctx context.Context, collection string, vector []float32, limit uint64) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	if len(vector) == 0 {
		return "", fmt.Errorf("points-query needs `vector: [...]`")
	}
	points, err := c.Query(ctx, &qc.QueryPoints{
		CollectionName: collection,
		Query:          qc.NewQuery(vector...),
		Limit:          &limit,
		WithPayload:    qc.NewWithPayload(true),
	})
	if err != nil {
		return "", fmt.Errorf("query %s: %w", collection, err)
	}
	var b strings.Builder
	for _, p := range points {
		fmt.Fprintf(&b, "id=%s score=%.4f payload=%s\n", pointIDString(p.GetId()), p.GetScore(), valueMapString(p.GetPayload()))
	}
	fmt.Fprintf(&b, "%d result(s)", len(points))
	return b.String(), nil
}

func (e *engine) scrollPoints(ctx context.Context, collection string, limit uint32) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	points, err := c.Scroll(ctx, &qc.ScrollPoints{
		CollectionName: collection,
		Limit:          &limit,
		WithPayload:    qc.NewWithPayload(true),
	})
	if err != nil {
		return "", fmt.Errorf("scroll %s: %w", collection, err)
	}
	var b strings.Builder
	for _, p := range points {
		fmt.Fprintf(&b, "id=%s payload=%s\n", pointIDString(p.GetId()), valueMapString(p.GetPayload()))
	}
	fmt.Fprintf(&b, "%d point(s)", len(points))
	return b.String(), nil
}

func (e *engine) countPoints(ctx context.Context, collection string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	n, err := c.Count(ctx, &qc.CountPoints{CollectionName: collection, Exact: boolPtr(true)})
	if err != nil {
		return "", fmt.Errorf("count %s: %w", collection, err)
	}
	return fmt.Sprintf("collection %s: %d point(s)", collection, n), nil
}

func (e *engine) getPoints(ctx context.Context, collection string, id uint64) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	points, err := c.Get(ctx, &qc.GetPoints{
		CollectionName: collection,
		Ids:            []*qc.PointId{qc.NewIDNum(id)},
		WithPayload:    qc.NewWithPayload(true),
	})
	if err != nil {
		return "", fmt.Errorf("get point %d from %s: %w", id, collection, err)
	}
	if len(points) == 0 {
		return "", fmt.Errorf("point %d not found in %s", id, collection)
	}
	var b strings.Builder
	for _, p := range points {
		fmt.Fprintf(&b, "id=%s payload=%s\n", pointIDString(p.GetId()), valueMapString(p.GetPayload()))
	}
	fmt.Fprintf(&b, "%d point(s)", len(points))
	return b.String(), nil
}

func (e *engine) deletePoints(ctx context.Context, collection string, id uint64) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	res, err := c.Delete(ctx, &qc.DeletePoints{
		CollectionName: collection,
		Wait:           boolPtr(true),
		Points:         qc.NewPointsSelector(qc.NewIDNum(id)),
	})
	if err != nil {
		return "", fmt.Errorf("delete point %d from %s: %w", id, collection, err)
	}
	return fmt.Sprintf("deleted point %d from %s (status=%s)", id, collection, res.GetStatus().String()), nil
}

// ---------------------------------------------------------------------------
// snapshots (Go client)
// ---------------------------------------------------------------------------

func (e *engine) snapshots(ctx context.Context, collection string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	snaps, err := c.ListSnapshots(ctx, collection)
	if err != nil {
		return "", fmt.Errorf("list snapshots for %s: %w", collection, err)
	}
	if len(snaps) == 0 {
		return "(no snapshots)", nil
	}
	var b strings.Builder
	for _, s := range snaps {
		fmt.Fprintf(&b, "%s size=%d\n", s.GetName(), s.GetSize())
	}
	fmt.Fprintf(&b, "%d snapshot(s)", len(snaps))
	return b.String(), nil
}

func (e *engine) createSnapshot(ctx context.Context, collection string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	snap, err := c.CreateSnapshot(ctx, collection)
	if err != nil {
		return "", fmt.Errorf("create snapshot for %s: %w", collection, err)
	}
	return fmt.Sprintf("created snapshot %s for %s (size=%d)", snap.GetName(), collection, snap.GetSize()), nil
}

// deleteSnapshot removes a named snapshot from a collection.
func (e *engine) deleteSnapshot(ctx context.Context, collection, snapshot string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	if err := c.DeleteSnapshot(ctx, collection, snapshot); err != nil {
		return "", fmt.Errorf("delete snapshot %s: %w", snapshot, err)
	}
	return fmt.Sprintf("deleted snapshot %s from %s", snapshot, collection), nil
}

// deleteNewestSnapshot lists the collection's snapshots and deletes the newest —
// the common bed cleanup operation (create-then-remove).
func (e *engine) deleteNewestSnapshot(ctx context.Context, collection string) (string, error) {
	c, err := e.grpcClient()
	if err != nil {
		return "", err
	}
	snaps, err := c.ListSnapshots(ctx, collection)
	if err != nil {
		return "", fmt.Errorf("list snapshots for %s: %w", collection, err)
	}
	if len(snaps) == 0 {
		return "", fmt.Errorf("no snapshots to delete for %s", collection)
	}
	newest := snaps[0]
	for _, s := range snaps {
		if s.GetCreationTime().AsTime().After(newest.GetCreationTime().AsTime()) {
			newest = s
		}
	}
	if err := c.DeleteSnapshot(ctx, collection, newest.GetName()); err != nil {
		return "", fmt.Errorf("delete snapshot %s: %w", newest.GetName(), err)
	}
	return fmt.Sprintf("deleted snapshot %s from %s", newest.GetName(), collection), nil
}

// ---------------------------------------------------------------------------
// diagnostics (REST — the Go client does not expose telemetry/metrics)
// ---------------------------------------------------------------------------

// telemetry / metrics are CLI-only diagnostics: the Go client has no binding, so
// they use the REST API through the host-vantage HTTP leg is not available here;
// these methods are therefore dispatched only by the CLI (which has cc.HTTPDo
// unavailable too) — see command.go's httpFallback. Kept out of the verb schema.

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }

func parseDistance(s string) (qc.Distance, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "cosine":
		return qc.Distance_Cosine, nil
	case "euclid", "euclidean", "l2":
		return qc.Distance_Euclid, nil
	case "dot":
		return qc.Distance_Dot, nil
	case "manhattan", "l1":
		return qc.Distance_Manhattan, nil
	default:
		return qc.Distance_UnknownDistance, fmt.Errorf("unknown distance %q (cosine, euclid, dot, manhattan)", s)
	}
}

func prettyJSON(data []byte) (string, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return strings.TrimSpace(string(data)), nil
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return strings.TrimSpace(string(data)), nil
	}
	return string(out), nil
}

// pointIDString renders a PointId (num or uuid) for output.
func pointIDString(id *qc.PointId) string {
	if id == nil {
		return "?"
	}
	if n, ok := id.GetPointIdOptions().(*qc.PointId_Num); ok {
		return fmt.Sprintf("%d", n.Num)
	}
	if u, ok := id.GetPointIdOptions().(*qc.PointId_Uuid); ok {
		return u.Uuid
	}
	return "?"
}

// valueMapString renders a payload map compactly.
func valueMapString(m map[string]*qc.Value) string {
	if len(m) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+valueString(v))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func valueString(v *qc.Value) string {
	if v == nil {
		return "null"
	}
	switch kind := v.GetKind().(type) {
	case *qc.Value_StringValue:
		return kind.StringValue
	case *qc.Value_IntegerValue:
		return fmt.Sprintf("%d", kind.IntegerValue)
	case *qc.Value_DoubleValue:
		return fmt.Sprintf("%g", kind.DoubleValue)
	case *qc.Value_BoolValue:
		return fmt.Sprintf("%t", kind.BoolValue)
	default:
		return "(value)"
	}
}
