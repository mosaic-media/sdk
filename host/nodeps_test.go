package host

import (
	"os"
	"strings"
	"testing"
)

// allowedParentDependencies is the whole of what `sdk`'s go.mod may require.
//
// **The rule used to be "nothing", and ADR 0128 replaced it with this rather
// than deleting it.** The property being protected was never zero for its own
// sake — it was that a third party compiles against a contract rather than
// against the Platform's taste. A vendor-neutral, CNCF-governed API that the
// third party very likely already has in their build is a different thing from
// a Mosaic-flavoured one, so the list is an allowlist and not an absence.
//
// Note what is **not** here and cannot be added without a decision:
// `go.opentelemetry.io/otel/sdk` and anything under it, any exporter, and any
// collector client. Those are what a *binary* wires (ADR 0128), and a module
// that could reach them could configure the Platform's observability plane from
// inside it — the ownership ADR 0059 placed with the Platform and ADR 0128 did
// not move. Twelve modules come with the OTel SDK against three with the API,
// and the difference is exactly this boundary.
var allowedParentDependencies = map[string]bool{
	// The API. `otel` itself carries `attribute` and `codes`.
	"go.opentelemetry.io/otel":       true,
	"go.opentelemetry.io/otel/log":   true,
	"go.opentelemetry.io/otel/trace": true,
	// Pulled in by the API modules above rather than chosen here.
	"github.com/cespare/xxhash/v2": true,
}

// TestParentSDKDependenciesAreTheOTelAPIOnly is the executable form of the
// reason this nested module exists.
//
// The property decays silently — a `go get` in the wrong directory is all it
// takes, and nothing else fails. So it is asserted from the one module that
// would notice, and whose own dependency list is the temptation: ADR 0064 split
// the harness out here precisely so gRPC would not land in the contract.
func TestParentSDKDependenciesAreTheOTelAPIOnly(t *testing.T) {
	data, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatalf("reading the parent go.mod: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "", strings.HasPrefix(trimmed, "//"):
			continue
		case trimmed == "require (", trimmed == ")":
			continue
		case strings.HasPrefix(trimmed, "module "), strings.HasPrefix(trimmed, "go "):
			continue
		case strings.HasPrefix(trimmed, "toolchain "):
			// A toolchain directive is a Go version statement, not a dependency.
			continue
		}

		path := strings.Fields(strings.TrimPrefix(trimmed, "require "))
		if len(path) == 0 || allowedParentDependencies[path[0]] {
			continue
		}
		t.Errorf("sdk/go.mod may require only the OpenTelemetry API, found: %q\n"+
			"If the SDK genuinely needs this, it belongs in sdk/host instead — "+
			"see ADR 0059, ADR 0064 and ADR 0128.", path[0])
	}
}
