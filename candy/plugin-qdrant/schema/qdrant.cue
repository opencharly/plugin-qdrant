// This external plugin's OWN CUE schema, served over the Describe channel — the
// typed plugin_input for the `qdrant` check verb. It is the SINGLE SOURCE for
// this plugin's verb params, used two ways (the same contract core `spec` uses):
//
//  1. GENERATE the Go param struct — `cue exp gengotypes` emits
//     ../params/cue_types_gen.go, so the provider decodes plugin_input into a
//     TYPED struct, never a hand-parsed map.
//  2. VALIDATE authored input AT RUNTIME — the plugin serves this source over the
//     Describe channel; the host splices it onto the base (base ++ plugin) and
//     validates every authored `qdrant:` step's plugin_input against #QdrantInput.
//
// The verb is HOST-BASED (the herdr pattern): the provider resolves the in-venue
// REST/gRPC ports to a host-routable address over the reverse channel
// (cc.ResolveEndpoint) and drives the server through the OFFICIAL Go client
// (github.com/qdrant/go-client) and the REST API — the SAME client the
// `charly qdrant` command uses (R3 — one protocol surface covers the CLI and
// every bed). The admin API key is resolved host-side from the credential store
// (verb:credential).
//
// The method surface below is the FULL management/testing capability: health,
// version, the auth boundary, collections (CRUD + exists + aliases), points
// (upsert/query/scroll/count/get/delete/set-payload), and snapshots. That lets a
// candy/box `check:` plan fully configure and exercise a Qdrant instance from
// charly.yml alone. `charly check box` (no running server) skips the
// live-requiring methods; the deterministic in-image methods still run.
//
// SELF-CONTAINED: it references NO base def, so it compiles standalone (the SDK's
// serve-side check + gengotypes) AND splices onto the base (base ++ plugin is a
// def-name collision check, not a base-reference resolver).

// #QdrantInput is the `qdrant` verb's plugin_input: the method name plus its
// method-exclusive modifiers.

#QdrantInput: {
	// method — the qdrant verb method name (the verb's PRIMARY input field, so
	// `qdrant: health` desugars to {method: "health"}).
	method: ("health" | "version" | "auth-required" | "collections-list" | "collection-create" | "collection-info" | "collection-delete" | "collection-exists" | "points-upsert" | "points-query" | "points-scroll" | "points-count" | "points-get" | "points-delete" | "snapshot-list" | "snapshot-create" | "snapshot-delete") @go(Method,type=string)
	// collection — the target collection for the collection/points/snapshot methods.
	collection?: string
	// size — the vector dimension for collection-create.
	size?: uint64
	// distance — the distance metric for collection-create (default cosine).
	distance?: ("cosine" | "euclid" | "dot" | "manhattan") @go(Distance,type=string)
	// id — the point id for points-upsert / points-get / points-delete.
	id?: uint64
	// vector — the dense vector for points-upsert / points-query.
	vector?: [...number] @go(Vector,type=[]float32)
	// payload — the payload for points-upsert (scalar values only).
	payload?: {[string]: string | number | bool} @go(Payload,type=map[string]any)
	// limit — the result limit for points-query / points-scroll.
	limit?: uint64
}
