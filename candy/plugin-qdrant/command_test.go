package qdrant

import (
	"testing"

	params "github.com/opencharly/plugin-qdrant/candy/plugin-qdrant/params"
	"github.com/qdrant/go-client/qdrant"
)

// TestResolveEndpoint covers the CLI targeting ladder: default, schemeless,
// explicit host:port, and 0.0.0.0 → 127.0.0.1.
func TestResolveEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		in       Globals
		wantRest string
		wantGrpc string
		wantTLS  bool
	}{
		{"default", Globals{}, "http://127.0.0.1:6333", "127.0.0.1", false},
		{"schemeless", Globals{Host: "qdrant.example"}, "http://qdrant.example:6333", "qdrant.example", false},
		{"explicit port", Globals{Host: "127.0.0.1:7000"}, "http://127.0.0.1:7000", "127.0.0.1", false},
		{"bind addr maps to loopback", Globals{Host: "0.0.0.0:6333"}, "http://127.0.0.1:6333", "127.0.0.1", false},
		{"https implies tls", Globals{Host: "https://qdrant.example"}, "https://qdrant.example:6333", "qdrant.example", true},
		{"tls flag", Globals{Host: "qdrant.example", TLS: true}, "https://qdrant.example:6333", "qdrant.example", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ep, err := resolveEndpoint(tc.in)
			if err != nil {
				t.Fatalf("resolveEndpoint: %v", err)
			}
			if ep.restBase != tc.wantRest {
				t.Errorf("restBase = %q, want %q", ep.restBase, tc.wantRest)
			}
			if ep.grpcHost != tc.wantGrpc {
				t.Errorf("grpcHost = %q, want %q", ep.grpcHost, tc.wantGrpc)
			}
			if ep.tls != tc.wantTLS {
				t.Errorf("tls = %v, want %v", ep.tls, tc.wantTLS)
			}
		})
	}
}

func TestParseDistance(t *testing.T) {
	for in, want := range map[string]qdrant.Distance{
		"":          qdrant.Distance_Cosine,
		"cosine":    qdrant.Distance_Cosine,
		"euclid":    qdrant.Distance_Euclid,
		"euclidean": qdrant.Distance_Euclid,
		"L2":        qdrant.Distance_Euclid,
		"dot":       qdrant.Distance_Dot,
		"manhattan": qdrant.Distance_Manhattan,
		"l1":        qdrant.Distance_Manhattan,
	} {
		got, err := parseDistance(in)
		if err != nil {
			t.Fatalf("parseDistance(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("parseDistance(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parseDistance("hamming"); err == nil {
		t.Errorf("parseDistance(hamming) should error")
	}
}

func TestParseVector(t *testing.T) {
	v, err := parseVector("0.1, 0.2,0.3")
	if err != nil {
		t.Fatalf("parseVector: %v", err)
	}
	if len(v) != 3 || v[0] != 0.1 || v[2] != 0.3 {
		t.Errorf("parseVector = %v, want [0.1 0.2 0.3]", v)
	}
	for _, bad := range []string{"", " ", "a,b"} {
		if _, err := parseVector(bad); err == nil {
			t.Errorf("parseVector(%q) should error", bad)
		}
	}
}

func TestParsePayload(t *testing.T) {
	p, err := parsePayload([]string{"city=London", "age=32"})
	if err != nil {
		t.Fatalf("parsePayload: %v", err)
	}
	if p["city"] != "London" || p["age"] != "32" {
		t.Errorf("parsePayload = %v", p)
	}
	if _, err := parsePayload([]string{"novalue"}); err == nil {
		t.Errorf("parsePayload(novalue) should error")
	}
	if p, err := parsePayload(nil); err != nil || p != nil {
		t.Errorf("parsePayload(nil) = %v, %v", p, err)
	}
}

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		addr     string
		defPort  int
		wantHost string
		wantPort int
	}{
		{"127.0.0.1:6334", 6334, "127.0.0.1", 6334},
		{"qdrant.example:7000", 6334, "qdrant.example", 7000},
		{"qdrant.example", 6334, "qdrant.example", 6334},
		{"127.0.0.1:notaport", 6334, "127.0.0.1", 6334},
		{"", 6334, "127.0.0.1", 6334},
	}
	for _, tc := range tests {
		h, p := splitHostPort(tc.addr, tc.defPort)
		if h != tc.wantHost || p != tc.wantPort {
			t.Errorf("splitHostPort(%q) = %q,%d want %q,%d", tc.addr, h, p, tc.wantHost, tc.wantPort)
		}
	}
}

func TestValidateMethod(t *testing.T) {
	input := func(coll string) params.QdrantInput { return params.QdrantInput{Collection: coll} }

	// collection required
	if err := validateMethod("collection-info", input("")); err == nil {
		t.Errorf("collection-info without collection should error")
	}
	if err := validateMethod("collection-info", input("c")); err != nil {
		t.Errorf("collection-info with collection: %v", err)
	}
	// create needs size
	in := input("c")
	in.Size = 0
	if err := validateMethod("collection-create", in); err == nil {
		t.Errorf("collection-create without size should error")
	}
	in.Size = 128
	if err := validateMethod("collection-create", in); err != nil {
		t.Errorf("collection-create with size: %v", err)
	}
	// upsert needs vector
	up := input("c")
	if err := validateMethod("points-upsert", up); err == nil {
		t.Errorf("points-upsert without vector should error")
	}
	up.Vector = []float32{0.1}
	if err := validateMethod("points-upsert", up); err != nil {
		t.Errorf("points-upsert with vector: %v", err)
	}
	// points-get needs id
	pg := input("c")
	if err := validateMethod("points-get", pg); err == nil {
		t.Errorf("points-get without id should error")
	}
	pg.Id = 7
	if err := validateMethod("points-get", pg); err != nil {
		t.Errorf("points-get with id: %v", err)
	}
}

func TestRequiresLive(t *testing.T) {
	for _, m := range []string{"health", "version", "collection-create", "points-upsert"} {
		if !requiresLive(m) {
			t.Errorf("requiresLive(%q) = false, want true", m)
		}
	}
}

func TestPointAndValueString(t *testing.T) {
	if got := pointIDString(qdrant.NewIDNum(42)); got != "42" {
		t.Errorf("pointIDString(num) = %q", got)
	}
	if got := pointIDString(qdrant.NewIDUUID("abc")); got != "abc" {
		t.Errorf("pointIDString(uuid) = %q", got)
	}
	if got := pointIDString(nil); got != "?" {
		t.Errorf("pointIDString(nil) = %q", got)
	}
	m := qdrant.NewValueMap(map[string]any{"city": "London"})
	if got := valueMapString(m); got != "{city=London}" {
		t.Errorf("valueMapString = %q", got)
	}
	if got := valueMapString(nil); got != "{}" {
		t.Errorf("valueMapString(nil) = %q", got)
	}
}
