package qdrant

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// config.go resolves the target server from the CLI globals + environment. The
// ladder mirrors `charly ollama`: --host/--api-key/--grpc-port flags >
// QDRANT_HOST / QDRANT_API_KEY env > http://127.0.0.1:6333 (REST) with gRPC on
// 6334. A schemeless --host gets http:// prepended; 0.0.0.0 (a server-bind
// value) maps to 127.0.0.1 for client use.

const (
	defaultRestPort = 6333
	defaultGrpcPort = 6334
)

// Globals are the targeting flags every subcommand accepts (global flags).
type Globals struct {
	Host     string `name:"host" env:"QDRANT_HOST" help:"Qdrant REST base URL or host[:port] (default http://127.0.0.1:6333)"`
	APIKey   string `name:"api-key" env:"QDRANT_API_KEY" help:"Admin API key (default: QDRANT_API_KEY env)"`
	GrpcPort int    `name:"grpc-port" help:"gRPC port (default 6334)"`
	RestPort int    `name:"rest-port" help:"REST port (default 6333)"`
	TLS      bool   `name:"tls" help:"Use TLS (https / grpcs) for the connection"`
}

// endpoint is a resolved, dialable server target.
type endpoint struct {
	restBase string // e.g. http://127.0.0.1:6333
	grpcHost string // hostname for the gRPC client
	grpcPort int
	apiKey   string
	tls      bool
}

// resolveEndpoint applies the targeting ladder for the CLI.
func resolveEndpoint(g Globals) (endpoint, error) {
	raw := strings.TrimSpace(g.Host)
	if raw == "" {
		raw = "http://127.0.0.1:6333"
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return endpoint{}, fmt.Errorf("parse --host %q: %w", g.Host, err)
	}
	host := u.Hostname()
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	scheme := u.Scheme
	tls := g.TLS || scheme == "https" || scheme == "grpcs"

	restPort := g.RestPort
	if restPort == 0 {
		if p := u.Port(); p != "" {
			if n, err := strconv.Atoi(p); err == nil {
				restPort = n
			}
		}
	}
	if restPort == 0 {
		restPort = defaultRestPort
	}
	grpcPort := g.GrpcPort
	if grpcPort == 0 {
		grpcPort = defaultGrpcPort
	}
	restScheme := "http"
	if tls {
		restScheme = "https"
	}
	return endpoint{
		restBase: fmt.Sprintf("%s://%s:%d", restScheme, host, restPort),
		grpcHost: host,
		grpcPort: grpcPort,
		apiKey:   g.APIKey,
		tls:      tls,
	}, nil
}

// endpointFromAddrs builds an endpoint from the verb's resolved host:port
// addresses (restAddr/grpcAddr) + a host-resolved API key.
func endpointFromAddrs(restAddr, grpcAddr, apiKey string) endpoint {
	grpcHost, grpcPort := splitHostPort(grpcAddr, defaultGrpcPort)
	return endpoint{
		restBase: "http://" + restAddr,
		grpcHost: grpcHost,
		grpcPort: grpcPort,
		apiKey:   apiKey,
	}
}

// splitHostPort splits a host:port, falling back to the default port.
func splitHostPort(addr string, defPort int) (string, int) {
	if addr == "" {
		return "127.0.0.1", defPort
	}
	if i := strings.LastIndex(addr, ":"); i > 0 {
		host := addr[:i]
		if p, err := strconv.Atoi(addr[i+1:]); err == nil {
			return host, p
		}
		return host, defPort
	}
	return addr, defPort
}

// apiKeyFromEnv reads the admin key from the environment — used by the verb,
// where the key may be injected from the credential store as
// QDRANT__SERVICE__API_KEY (the layer's secret_require env var) or set as
// QDRANT_API_KEY.
func apiKeyFromEnv() string {
	if v := os.Getenv("QDRANT_API_KEY"); v != "" {
		return v
	}
	return os.Getenv("QDRANT__SERVICE__API_KEY")
}

// ---------------------------------------------------------------------------
// REST fallback — ONLY for the two endpoints the Go client does not expose
// (/telemetry and /metrics). Everything else goes through the official client.
// ---------------------------------------------------------------------------

// restGet performs an authenticated GET against the REST API (CLI diagnostics).
func (e *engine) restGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.ep.restBase+path, nil)
	if err != nil {
		return nil, err
	}
	if e.ep.apiKey != "" {
		req.Header.Set("api-key", e.ep.apiKey)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (e *engine) telemetry(ctx context.Context) (string, error) {
	data, err := e.restGet(ctx, "/telemetry")
	if err != nil {
		return "", err
	}
	return prettyJSON(data)
}

func (e *engine) metrics(ctx context.Context) (string, error) {
	data, err := e.restGet(ctx, "/metrics")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
