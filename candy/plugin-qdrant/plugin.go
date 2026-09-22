// Package qdrant is the importable form of the charly `qdrant` plugin: a
// `command:qdrant` CLI for a deployed Qdrant vector-search server PLUS the
// `qdrant:` check VERB (the declarative counterpart, verb.go).
//
// A command provider dispatches via the pb Invoke(OpRun) envelope — decode the
// pass-through `{"args":[...]}` and kong-parse them into the QdrantCmd tree
// (sdk.RunInProcCLI), so the handler runs in charly's OWN process with native
// stdio/TTY. The CLI talks to the server over the official Go gRPC client
// (github.com/qdrant/go-client, port 6334) and the REST API (port 6333) for
// health/version/snapshots; endpoint resolution is deliberately lightweight:
// --host flag > QDRANT_HOST env > http://127.0.0.1:6333 (REST) / :6334 (gRPC).
// The verb provider dispatches via Invoke with the full #Op as params_json: it is
// HOST-BASED (the herdr pattern) — it resolves the in-venue REST/gRPC ports over
// the reverse channel (cc.ResolveEndpoint) and drives the SAME official Go client,
// reading the admin key host-side from the credential store (verb:credential).
// Every verb method needs a running server, so all skip under `charly check box`.
//
// Usable OUT-OF-PROCESS by the cmd/serve shim (the default placement) OR
// COMPILED-IN (NewProvider()/NewMeta() via plugins_generated.go) — both
// placements run the SAME runCommand / runVerbQdrant (placement-invisible, F8).
package qdrant

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/alecthomas/kong"

	params "github.com/opencharly/plugin-qdrant/candy/plugin-qdrant/params"
	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/kit"
	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

const calver = "2026.249.2125"

//go:embed schema/*.cue
var schemaFS embed.FS

// NewProvider returns the provider for in-proc registration (compiled-in) or
// out-of-proc serving.
func NewProvider() pb.ProviderServer { return &provider{} }

// NewMeta advertises command:qdrant + verb:qdrant via a lazy Describe: the kong
// CLIModel is reflected INSIDE Describe (qdrantMeta) rather than eagerly in the
// constructor — a kong reflection regression then surfaces as a Describe error
// at plugin registration, loud but never a panic crashing every charly startup
// (the plugin-dsh/plugin-herdr pattern). The verb capability carries the
// #QdrantInput def served from the plugin's own schema/*.cue; the command
// capability carries no InputDef — a command's args are pass-through CLI tokens,
// not a structured plugin_input.
func NewMeta() pb.PluginMetaServer { return qdrantMeta{} }

// qdrantMeta is the plugin's PluginMetaServer: NewMeta stays trivial (it is
// called at process init by plugins_generated.go) and all fallible reflection
// happens in Describe, which can return an error.
type qdrantMeta struct {
	pb.UnimplementedPluginMetaServer
}

func (qdrantMeta) Describe(context.Context, *pb.Empty) (*pb.Capabilities, error) {
	model, err := commandModel()
	if err != nil {
		return nil, err
	}
	return sdk.BuildCapabilities(calver,
		[]sdk.ProvidedCapability{
			{Class: "command", Word: "qdrant", CommandModel: model},
			{Class: "verb", Word: "qdrant", InputDef: "#QdrantInput", Primary: "method"},
		},
		schemaFS, "schema")
}

type provider struct{ pb.UnimplementedProviderServer }

// Invoke dispatches one operation for the plugin's capabilities. A "command" op
// runs the pass-through CLI args in charly's own process (OpRun); a "verb" op
// runs one `qdrant:` check step (the full #Op as params_json + a CheckEnv
// snapshot as env — the dsh pattern). (Out-of-process command dispatch is
// fork/exec → CliMain, never this gRPC path.)
func (provider) Invoke(ctx context.Context, req *pb.InvokeRequest) (*pb.InvokeReply, error) {
	if req.GetClass() == "command" {
		return invokeCommand(req)
	}
	if req.GetClass() == "verb" {
		return invokeVerb(ctx, req)
	}
	return nil, fmt.Errorf("qdrant: unsupported class %q", req.GetClass())
}

// Reserved implements spec.CheckVerbProvider: the verb word.
func (p *provider) Reserved() string { return "qdrant" }

// RunVerb implements spec.CheckVerbProvider — the COMPILED-IN verb dispatch. The
// host recognizes a compiled-in pb.ProviderServer that ALSO implements this typed
// contract (hostVerbResolver.RunVerb) and threads the live host CheckContext in
// (hostCheckContext) — the executor-bearing surface the host-based verb needs
// (ResolveEndpoint + Mode). The out-of-process placement runs the SAME core via invokeVerb (the pb
// Invoke envelope with the broker attached) — placement-invisible, F8.
func (p *provider) RunVerb(ctx context.Context, cc spec.CheckContext, op *spec.Op) spec.CheckVerbResult {
	var in params.QdrantInput
	kit.DecodeInput(op.PluginInput, &in)
	method := in.Method
	if err := validateMethod(method, in); err != nil {
		return spec.CheckVerbResult{Status: spec.StatusFail, Message: err.Error()}
	}
	// The qdrant verb drives a RUNNING server over the host-resolved endpoint, so
	// every method skips under `charly check box` (a disposable `podman run --rm`
	// has no server). The deterministic in-image claims are the candy's own build
	// checks.
	if requiresLive(method) && cc.Mode() == spec.CheckModeBox {
		return spec.CheckVerbResult{Status: spec.StatusSkip, Message: fmt.Sprintf("qdrant: %s requires a running server (skip under charly check box)", method)}
	}
	out, runErr := runVerbQdrant(ctx, cc, in)
	return verbVerdict(method, out, runErr, op)
}

// verbVerdict grades the verb's output against the authored op matchers
// (exit_status / stdout / stderr) and returns the typed verdict — the SAME shared
// pipeline the out-of-process path runs (sdk.VerbVerdict), converted from the wire
// form to the typed spec.CheckVerbResult a compiled-in RunVerb returns (R3).
func verbVerdict(method, out string, runErr error, op *spec.Op) spec.CheckVerbResult {
	reply, err := sdk.VerbVerdict("qdrant", method, out, runErr, op, false)
	if err != nil {
		return spec.CheckVerbResult{Status: spec.StatusFail, Message: err.Error()}
	}
	var wire struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(reply.ResultJson, &wire); err != nil {
		return spec.CheckVerbResult{Status: spec.StatusFail, Message: err.Error()}
	}
	status := spec.StatusFail
	switch wire.Status {
	case "pass":
		status = spec.StatusPass
	case "skip":
		status = spec.StatusSkip
	}
	return spec.CheckVerbResult{Status: status, Message: wire.Message}
}

// invokeCommand handles OpRun for the COMPILED-IN (in-proc) dispatch: decode the
// pass-through {args} and run the command effect in charly's own process.
func invokeCommand(req *pb.InvokeRequest) (*pb.InvokeReply, error) {
	if req.GetOp() != sdk.OpRun {
		return nil, fmt.Errorf("qdrant: unsupported op %q (only %q)", req.GetOp(), sdk.OpRun)
	}
	var input struct {
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(req.GetParamsJson(), &input); err != nil {
		return nil, fmt.Errorf("qdrant: decode args: %w", err)
	}
	if err := runCommand(input.Args); err != nil {
		return nil, err
	}
	return &pb.InvokeReply{}, nil
}

// qdrantEnv is the plugin-side decode of the CheckEnv the host ships as
// Operation.Env for a `qdrant:` check step — only Mode matters here (the verb
// resolves the venue endpoints host-side and drives a running server; every
// method skips under box mode).
type qdrantEnv struct {
	Box  string `json:"box"`
	Mode string `json:"mode"` // "live" | "box"
}

// invokeVerb runs one `qdrant:` check operation (the out-of-process placement).
func invokeVerb(ctx context.Context, req *pb.InvokeRequest) (*pb.InvokeReply, error) {
	var op spec.Op
	if len(req.GetParamsJson()) > 0 {
		if err := json.Unmarshal(req.GetParamsJson(), &op); err != nil {
			return sdk.ResultJSON("fail", "qdrant: decode op: "+err.Error())
		}
	}
	var in params.QdrantInput
	kit.DecodeInput(op.PluginInput, &in)
	var env qdrantEnv
	if len(req.GetEnvJson()) > 0 {
		_ = json.Unmarshal(req.GetEnvJson(), &env)
	}
	method := in.Method
	if err := validateMethod(method, in); err != nil {
		return sdk.ResultJSON("fail", err.Error())
	}
	if requiresLive(method) && env.Mode == "box" {
		return sdk.ResultJSON("skip", fmt.Sprintf("qdrant: %s requires a running server (skip under charly check box)", method))
	}
	cc, err := sdk.NewCheckContext(req.GetExecutorBrokerId(), req.GetEnvJson())
	if err != nil {
		return sdk.ResultJSON("fail", fmt.Sprintf("qdrant: %s: %v", method, err))
	}
	out, runErr := runVerbQdrant(ctx, cc, in)
	return sdk.VerbVerdict("qdrant", method, out, runErr, &op, false)
}

// CliMain is the OUT-OF-PROCESS CLI-mode entry (charly fork/execs the binary with
// the pass-through tokens after `charly qdrant`). It runs the SAME effect as the
// in-proc Invoke(OpRun) path.
func CliMain(args []string) int {
	if err := runCommand(args); err != nil {
		fmt.Fprintf(os.Stderr, "charly qdrant: %v\n", err)
		return 1
	}
	return 0
}

// runCommand parses the pass-through args of the command — which runs in
// charly's OWN process under the compiled-in placement — so it must NEVER let
// kong terminate the host: kong's default Exit is os.Exit, and a raw
// kong.New/Parse would make `charly qdrant --help` kill charly whole.
// sdk.RunInProcCLI is the house in-proc helper (sdk/clidispatch.go documents the
// hazard).
func runCommand(args []string) error {
	var command QdrantCmd
	return sdk.RunInProcCLI("qdrant", &command, args,
		kong.Description("Manage a deployed Qdrant vector-search server: collections, points, snapshots, health"),
		kong.Bind(&command))
}

// commandModel reflects the kong grammar into a CLIModel. Every error propagates
// to Describe (no panic).
func commandModel() (*spec.CLIModel, error) {
	return sdk.BuildCLIModel(&QdrantCmd{}, "qdrant", calver, "qdrant")
}
