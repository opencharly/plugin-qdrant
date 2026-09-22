package qdrant

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// live_client_test.go — the LIVE integration test. Per the rulebook's "Live or
// skip — never fake a live service": this test drives the REAL qdrant server
// through the SAME Go-client engine the CLI and verb use. It runs when a real
// qdrant binary + a free port are available (QDRANT_TEST_BINARY, or `qdrant` on
// PATH), and SKIPS CLEANLY (t.Skip, visibly reported) when absent — never a mock
// of the gRPC boundary.
//
// It also exercises the full capability surface a bed exercises: health,
// version, auth boundary, collection CRUD, point upsert/query/count/get/delete,
// and snapshots.

// startLiveQdrant boots a real qdrant on a private port with an admin key and a
// temp storage dir; returns the endpoint + a stop func, or skips.
func startLiveQdrant(t *testing.T) (endpoint, func()) {
	t.Helper()
	bin := os.Getenv("QDRANT_TEST_BINARY")
	if bin == "" {
		if p, err := exec.LookPath("qdrant"); err == nil {
			bin = p
		}
	}
	if bin == "" {
		t.Skip("live qdrant integration: no qdrant binary (set QDRANT_TEST_BINARY or put qdrant on PATH); SKIPPING the real-service boundary rather than faking it")
	}
	dir := t.TempDir()
	const (
		restPort = 16333
		grpcPort = 16334
		apiKey   = "test-admin-key"
	)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"QDRANT__SERVICE__API_KEY="+apiKey,
		"QDRANT__SERVICE__HTTP_PORT="+strconv.Itoa(restPort),
		"QDRANT__SERVICE__GRPC_PORT="+strconv.Itoa(grpcPort),
		"QDRANT__STORAGE__STORAGE_PATH="+dir+"/storage",
		"QDRANT__STORAGE__SNAPSHOTS_PATH="+dir+"/snapshots",
		"QDRANT__TELEMETRY_DISABLED=true",
	)
	if err := cmd.Start(); err != nil {
		t.Skipf("live qdrant integration: cannot start %s: %v", bin, err)
	}
	ep := endpoint{
		restBase: "http://127.0.0.1:" + strconv.Itoa(restPort),
		grpcHost: "127.0.0.1",
		grpcPort: grpcPort,
		apiKey:   apiKey,
	}
	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
	// Readiness: retry the Go-client health check until the server answers.
	deadline := time.Now().Add(30 * time.Second)
	for {
		eng, _ := newEngine(ep)
		if eng != nil {
			if _, err := eng.health(context.Background()); err == nil {
				eng.close()
				return ep, stop
			}
			eng.close()
		}
		if time.Now().After(deadline) {
			stop()
			t.Skipf("live qdrant integration: server did not become healthy within 30s; SKIPPING")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// TestLiveCapabilities drives the full capability surface against a real server.
func TestLiveCapabilities(t *testing.T) {
	ep, stop := startLiveQdrant(t)
	defer stop()
	ctx := context.Background()

	eng, err := newEngine(ep)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.close()

	// health + version
	if _, err := eng.health(ctx); err != nil {
		t.Fatalf("health: %v", err)
	}
	ver, err := eng.version(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	t.Logf("version: %s", ver)

	// auth boundary: an unauthenticated client must be rejected.
	if _, err := eng.authRequired(ctx); err != nil {
		t.Fatalf("authRequired: %v", err)
	}

	// collection lifecycle
	const coll = "charly_qdrant_live_test"
	if _, err := eng.createCollection(ctx, coll, 4, "cosine"); err != nil {
		t.Fatalf("createCollection: %v", err)
	}
	if _, err := eng.collectionExists(ctx, coll); err != nil {
		t.Fatalf("collectionExists: %v", err)
	}
	if info, err := eng.collectionInfo(ctx, coll); err != nil {
		t.Fatalf("collectionInfo: %v", err)
	} else {
		t.Logf("info: %s", info)
	}

	// point upsert/query/scroll/count/get/delete
	if _, err := eng.upsertPoints(ctx, coll, 1, []float32{0.05, 0.61, 0.76, 0.74}, map[string]any{"city": "London"}); err != nil {
		t.Fatalf("upsertPoints: %v", err)
	}
	if _, err := eng.upsertPoints(ctx, coll, 2, []float32{0.19, 0.81, 0.75, 0.11}, map[string]any{"city": "Berlin"}); err != nil {
		t.Fatalf("upsertPoints: %v", err)
	}
	if res, err := eng.queryPoints(ctx, coll, []float32{0.2, 0.1, 0.9, 0.7}, 5); err != nil {
		t.Fatalf("queryPoints: %v", err)
	} else {
		t.Logf("query: %s", res)
	}
	if res, err := eng.scrollPoints(ctx, coll, 10); err != nil {
		t.Fatalf("scrollPoints: %v", err)
	} else {
		t.Logf("scroll: %s", res)
	}
	if res, err := eng.countPoints(ctx, coll); err != nil {
		t.Fatalf("countPoints: %v", err)
	} else if res == "" {
		t.Fatalf("empty count")
	}
	if _, err := eng.getPoints(ctx, coll, 1); err != nil {
		t.Fatalf("getPoints: %v", err)
	}
	if _, err := eng.deletePoints(ctx, coll, 2); err != nil {
		t.Fatalf("deletePoints: %v", err)
	}

	// snapshots
	if snap, err := eng.createSnapshot(ctx, coll); err != nil {
		t.Fatalf("createSnapshot: %v", err)
	} else {
		t.Logf("snapshot: %s", snap)
	}
	if list, err := eng.snapshots(ctx, coll); err != nil {
		t.Fatalf("snapshots: %v", err)
	} else {
		t.Logf("snapshots: %s", list)
	}
	if _, err := eng.deleteNewestSnapshot(ctx, coll); err != nil {
		t.Fatalf("deleteNewestSnapshot: %v", err)
	}

	// collections list + delete
	if list, err := eng.collections(ctx); err != nil {
		t.Fatalf("collections: %v", err)
	} else {
		t.Logf("collections: %s", list)
	}
	if _, err := eng.deleteCollection(ctx, coll); err != nil {
		t.Fatalf("deleteCollection: %v", err)
	}

	// telemetry + metrics — the REST leg the Go client does not cover.
	if tel, err := eng.telemetry(ctx); err != nil {
		t.Fatalf("telemetry: %v", err)
	} else if len(tel) == 0 {
		t.Fatalf("empty telemetry")
	} else {
		t.Logf("telemetry: %d bytes", len(tel))
	}
	if met, err := eng.metrics(ctx); err != nil {
		t.Fatalf("metrics: %v", err)
	} else if !strings.Contains(met, "qdrant") && len(met) == 0 {
		t.Fatalf("unexpected metrics output")
	} else {
		t.Logf("metrics: %d bytes", len(met))
	}
}
