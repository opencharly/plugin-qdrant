package qdrant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// rest_test.go — the REST leg of the engine (restGet / telemetry / metrics /
// prettyJSON). The Go client covers collections/points/snapshots/health/version;
// telemetry and metrics are the two endpoints it does not expose, so the engine
// reaches them over HTTP. These tests drive the REAL HTTP code path against a
// local httptest server (a real HTTP boundary, not a mock of the qdrant service
// itself — the live-or-skip TestLiveCapabilities covers a real qdrant). They fail
// if restGet/telemetry/metrics/prettyJSON regress.

// newTestEngine points an engine at srv's URL, with an optional API key.
func newTestEngine(t *testing.T, srv *httptest.Server, apiKey string) *engine {
	t.Helper()
	host := strings.TrimPrefix(srv.URL, "http://")
	host, port := splitHostPort(host, 80)
	return &engine{ep: endpoint{restBase: "http://" + host + ":" + strconv.Itoa(port), grpcHost: host, grpcPort: 6334, apiKey: apiKey}}
}

func TestRestGetAuthAndError(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("api-key")
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("nope"))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	e := newTestEngine(t, srv, "secret-key")
	data, err := e.restGet(context.Background(), "/ok")
	if err != nil {
		t.Fatalf("restGet /ok: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("body = %q", data)
	}
	if gotKey != "secret-key" {
		t.Errorf("api-key header = %q, want %q", gotKey, "secret-key")
	}

	// A non-2xx is an error carrying the status + body.
	if _, err := e.restGet(context.Background(), "/fail"); err == nil {
		t.Fatalf("restGet /fail should error on 401")
	} else if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should name the status: %v", err)
	}
}

func TestTelemetryAndMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/telemetry":
			_, _ = w.Write([]byte(`{"result":{"status":"ok"},"n":1}`))
		case "/metrics":
			_, _ = w.Write([]byte("# HELP qdrant_points qdrant_points 3\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	e := newTestEngine(t, srv, "")

	tel, err := e.telemetry(context.Background())
	if err != nil {
		t.Fatalf("telemetry: %v", err)
	}
	// prettyJSON must have reformatted it (indented).
	if !strings.Contains(tel, "\n") || !strings.Contains(tel, `"status": "ok"`) {
		t.Errorf("telemetry not pretty-printed: %q", tel)
	}

	met, err := e.metrics(context.Background())
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if !strings.Contains(met, "qdrant_points 3") {
		t.Errorf("metrics = %q", met)
	}
}

func TestPrettyJSON(t *testing.T) {
	out, err := prettyJSON([]byte(`{"a":1,"b":[2,3]}`))
	if err != nil {
		t.Fatalf("prettyJSON: %v", err)
	}
	if !json.Valid([]byte(out)) || !strings.Contains(out, "\n") {
		t.Errorf("prettyJSON output not valid indented JSON: %q", out)
	}
	// Non-JSON input degrades to the trimmed raw string, never an error.
	raw, err := prettyJSON([]byte("  not json  "))
	if err != nil {
		t.Fatalf("prettyJSON non-json: %v", err)
	}
	if raw != "not json" {
		t.Errorf("prettyJSON non-json = %q", raw)
	}
}
