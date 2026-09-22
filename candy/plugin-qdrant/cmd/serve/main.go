// Command serve is the OUT-OF-PROCESS placement shim for the qdrant plugin:
// `charly` fork/execs this binary with the pass-through tokens after
// `charly qdrant` when the plugin is served out-of-process. It runs the SAME
// effect as the compiled-in Invoke(OpRun) path (CliMain), so both placements are
// placement-invisible.
package main

import (
	"github.com/opencharly/sdk"

	qdrant "github.com/opencharly/plugin-qdrant/candy/plugin-qdrant"
)

func main() {
	sdk.Main(qdrant.NewProvider(), qdrant.NewMeta(), qdrant.CliMain)
}
