package host

import (
	"os"
	"strings"
	"testing"
)

// allowedParentDependencies is the whole of what the parent sdk module's go.mod
// may require.
//
// It is an allowlist, not an assertion that the list is empty (sdk#8). The
// property being protected was never zero dependencies for its own sake — it is
// that a third party compiles against a contract rather than against the
// Platform's taste, and a vendor-neutral, CNCF-governed API the third party very
// likely already has in their build is a different thing from a Mosaic-flavoured
// one.
//
// Note what is not here and cannot be added without a decision:
// go.opentelemetry.io/otel/sdk and anything under it, any exporter, and any
// collector client. Those are what a binary wires (sdk#8), and a module that
// could reach them could configure the Platform's observability plane from
// inside it — the ownership sdk#5 placed with the Platform and sdk#8 did not
// move. The OTel SDK pulls in a dozen modules against the API's four, and the
// difference is exactly this boundary.
//
// The metric API joined the three in sdk#9, on the same terms: metric.Meter is
// an interface the Platform hands over, and metric.MeterProvider — where a view,
// a reader or an exporter would be configured — is not reachable from anything a
// module holds.
var allowedParentDependencies = map[string]bool{
	// The API. `otel` itself carries `attribute` and `codes`.
	"go.opentelemetry.io/otel":        true,
	"go.opentelemetry.io/otel/log":    true,
	"go.opentelemetry.io/otel/metric": true,
	"go.opentelemetry.io/otel/trace":  true,
	// Pulled in by the API modules above rather than chosen here.
	"github.com/cespare/xxhash/v2": true,
}

// TestParentSDKDependenciesAreTheOTelAPIOnly is the executable form of the
// reason this nested module exists.
//
// The property decays silently — a go get in the wrong directory is all it
// takes, and nothing else fails. So it is asserted from the one module that
// would notice, and whose own dependency list is the temptation: platform#39
// split the harness out here precisely so gRPC would not land in the contract.
//
// It reads the parent go.mod as text, so an entry marked indirect is checked
// like any other; that is why xxhash is on the list above.
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
			"see sdk#5, platform#39 and sdk#8.", path[0])
	}
}
