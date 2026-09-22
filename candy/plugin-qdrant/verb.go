package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	params "github.com/opencharly/plugin-qdrant/candy/plugin-qdrant/params"
	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/spec"
)

// verb.go is the `qdrant:` check VERB — the declarative counterpart of the
// `charly qdrant` command plugin. It is HOST-BASED (the herdr pattern): the
// provider resolves the in-venue REST/gRPC ports to a host-routable address over
// the reverse channel (cc.ResolveEndpoint) and drives the server through the
// OFFICIAL Go client (github.com/qdrant/go-client) — the SAME client the command
// uses (R3 — one protocol surface covers the CLI and every bed). The admin API
// key is resolved host-side from the credential store (verb:credential) using the
// layer's declared key (charly/api-key/qdrant), with QDRANT__SERVICE__API_KEY /
// QDRANT_API_KEY env as overrides.
//
// The method surface is the FULL management/testing capability, so a candy/box
// `check:` plan can fully configure and exercise a Qdrant instance from
// charly.yml alone: health, version, the auth boundary, collections (CRUD +
// exists), points (upsert/query/scroll/count/get/delete), and snapshots. The
// live-requiring methods skip under `charly check box` (no running server).

// inVenueRestPort / inVenueGrpcPort are the fixed in-venue ports the qdrant
// candy's service listens on. The verb and the candy agree on these numbers.
const (
	inVenueRestPort = 6333
	inVenueGrpcPort = 6334
)

// secretService / secretKey are the credential-store coordinates the layer's
// `secret_require` key: charly/api-key/qdrant resolves to. secretEnv names the
// env override the operator may set instead (the injected container var).
const (
	secretService = "charly/api-key"
	secretKey     = "qdrant"
)

// requiresLive reports whether a method needs a RUNNING server (and so must skip
// under `charly check box`). health/version/auth-required all need the live
// server; every method here does (a disposable image has no server). The
// deterministic in-image claims are covered by the candy's own build checks.
func requiresLive(method string) bool { return true }

// validateMethod checks method-exclusive modifiers before dispatch.
func validateMethod(method string, in params.QdrantInput) error {
	switch method {
	case "collection-create":
		if strings.TrimSpace(in.Collection) == "" {
			return fmt.Errorf("qdrant: collection-create needs `collection: <name>`")
		}
		if in.Size == 0 {
			return fmt.Errorf("qdrant: collection-create needs `size: <dimension>`")
		}
	case "collection-info", "collection-delete", "collection-exists", "points-count",
		"points-query", "points-scroll", "snapshot-list", "snapshot-create":
		if strings.TrimSpace(in.Collection) == "" {
			return fmt.Errorf("qdrant: %s needs `collection: <name>`", method)
		}
	case "points-upsert":
		if strings.TrimSpace(in.Collection) == "" {
			return fmt.Errorf("qdrant: points-upsert needs `collection: <name>`")
		}
		if len(in.Vector) == 0 {
			return fmt.Errorf("qdrant: points-upsert needs `vector: [...]`")
		}
	case "points-get", "points-delete":
		if strings.TrimSpace(in.Collection) == "" {
			return fmt.Errorf("qdrant: %s needs `collection: <name>`", method)
		}
		if in.Id == 0 {
			return fmt.Errorf("qdrant: %s needs `id: <n>`", method)
		}
	case "snapshot-delete":
		if strings.TrimSpace(in.Collection) == "" {
			return fmt.Errorf("qdrant: snapshot-delete needs `collection: <name>`")
		}
	}
	return nil
}

// runVerbQdrant resolves the venue endpoint, builds a client engine, and
// dispatches the method. resolveAddr is injected so the tests point the verb at
// a fake server without the reverse channel.
func runVerbQdrant(ctx context.Context, cc kit.CheckContext, op *spec.Op, in params.QdrantInput) (string, error) {
	restAddr, err := cc.ResolveEndpoint(ctx, inVenueRestPort)
	if err != nil {
		return "", fmt.Errorf("resolve qdrant REST endpoint: %w", err)
	}
	if restAddr == "" {
		return "", fmt.Errorf("no live qdrant venue for the qdrant verb (box-mode or no published port)")
	}
	grpcAddr, err := cc.ResolveEndpoint(ctx, inVenueGrpcPort)
	if err != nil {
		return "", fmt.Errorf("resolve qdrant gRPC endpoint: %w", err)
	}
	if grpcAddr == "" {
		// Fall back to the REST host with the default gRPC port — a deployment
		// that publishes only REST still answers /healthz etc. over REST, and the
		// gRPC methods report a clear dial error if 6334 is truly absent.
		host := restAddr
		if i := strings.LastIndex(restAddr, ":"); i > 0 {
			host = restAddr[:i]
		}
		grpcAddr = fmt.Sprintf("%s:%d", host, inVenueGrpcPort)
	}
	apiKey := resolveAPIKey(ctx, cc)
	eng, err := newEngine(endpointFromAddrs(restAddr, grpcAddr, apiKey))
	if err != nil {
		return "", err
	}
	defer eng.close()
	return dispatchVerb(ctx, eng, in)
}

// resolveAPIKey resolves the admin API key host-side: the operator env override
// first (QDRANT_API_KEY / QDRANT__SERVICE__API_KEY), then the credential store
// using the layer's declared key (charly/api-key/qdrant → service=charly/api-key,
// key=qdrant) over verb:credential. An empty key is returned when neither is
// available (an unsecured instance still answers health/version).
func resolveAPIKey(ctx context.Context, cc kit.CheckContext) string {
	if v := apiKeyFromEnv(); v != "" {
		return v
	}
	return execCredential(ctx, cc, secretService, secretKey)
}

// dispatchVerb runs the resolved method against the client engine.
func dispatchVerb(ctx context.Context, e *engine, in params.QdrantInput) (string, error) {
	switch in.Method {
	case "health":
		return e.health(ctx)
	case "version":
		return e.version(ctx)
	case "auth-required":
		return e.authRequired(ctx)
	case "collections-list":
		return e.collections(ctx)
	case "collection-create":
		return e.createCollection(ctx, in.Collection, uint64(in.Size), in.Distance)
	case "collection-info":
		return e.collectionInfo(ctx, in.Collection)
	case "collection-delete":
		return e.deleteCollection(ctx, in.Collection)
	case "collection-exists":
		return e.collectionExists(ctx, in.Collection)
	case "points-upsert":
		return e.upsertPoints(ctx, in.Collection, uint64(in.Id), in.Vector, in.Payload)
	case "points-query":
		limit := uint64(in.Limit)
		if limit == 0 {
			limit = 5
		}
		return e.queryPoints(ctx, in.Collection, in.Vector, limit)
	case "points-scroll":
		limit := uint32(in.Limit)
		if limit == 0 {
			limit = 10
		}
		return e.scrollPoints(ctx, in.Collection, limit)
	case "points-count":
		return e.countPoints(ctx, in.Collection)
	case "points-get":
		return e.getPoints(ctx, in.Collection, uint64(in.Id))
	case "points-delete":
		return e.deletePoints(ctx, in.Collection, uint64(in.Id))
	case "snapshot-list":
		return e.snapshots(ctx, in.Collection)
	case "snapshot-create":
		return e.createSnapshot(ctx, in.Collection)
	case "snapshot-delete":
		// The snapshot name is not a separate schema field; list first and delete
		// the newest — the common bed operation (create-then-clean-up).
		return e.deleteNewestSnapshot(ctx, in.Collection)
	default:
		return "", fmt.Errorf("unknown qdrant method %q", in.Method)
	}
}

// execCredential fetches one key from verb:credential's "get" method over the
// check context's PROVIDER channel (the same single-broker dial the verb already
// made — a second Dial hangs). It mirrors candy/plugin-adb's credential_shim.go
// (the cross-module credential wire contract).
func execCredential(ctx context.Context, cc kit.CheckContext, service, key string) string {
	if cc == nil || service == "" || key == "" {
		return ""
	}
	paramsJSON, err := json.Marshal(struct {
		Method  string `json:"method"`
		Service string `json:"service,omitempty"`
		Key     string `json:"key,omitempty"`
	}{Method: "get", Service: service, Key: key})
	if err != nil {
		return ""
	}
	reply, err := cc.InvokeProvider(ctx, "verb", "credential", sdk.OpRun, paramsJSON, nil)
	if err != nil {
		return ""
	}
	var out struct {
		Value string `json:"value,omitempty"`
	}
	if len(reply) > 0 {
		_ = json.Unmarshal(reply, &out)
	}
	return out.Value
}
