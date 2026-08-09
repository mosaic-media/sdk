package v1_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Where OpenTelemetry is allowed to appear, and where it is not.
//
// **"Abstract it so the implementation can change" is a claim, and this is what
// makes it checkable.** The surface a *module* compiles against — `Telemetry`,
// `Span`, `Field` and its constructors, `TelemetryFrom` — must be expressible
// without naming an OpenTelemetry type, because that is what lets the
// implementation underneath be replaced without a breaking change to a contract
// third parties build against. The surface a *host* uses to wire the pipeline —
// `NewTelemetry`, `TelemetryOptions`, `Encoder` — cannot be, and should not
// pretend to be: it exists precisely to hand OTel's logger and tracer in.
//
// So the split is physical rather than a convention: one file per side, and this
// test is the line between them. If a module-facing signature ever needs an OTel
// type, that is the abstraction failing, and it should fail here loudly rather
// than in somebody's build a year later.
const (
	moduleFacingFile = "telemetry.go"
	hostFacingFile   = "telemetry_otel.go"
	otelPrefix       = "go.opentelemetry.io/"
)

func TestTheModuleFacingSurfaceNamesNoOpenTelemetryType(t *testing.T) {
	imports := importsOf(t, moduleFacingFile)
	for _, path := range imports {
		if strings.HasPrefix(path, otelPrefix) {
			t.Errorf("%s imports %q — the surface a module compiles against must be "+
				"expressible without OpenTelemetry, so the implementation underneath "+
				"can be replaced without breaking a published contract (ADR 0128). "+
				"Anything needing an OTel type belongs in %s.",
				moduleFacingFile, path, hostFacingFile)
		}
	}
	if len(imports) == 0 {
		t.Fatalf("no imports parsed from %s — the check is not running", moduleFacingFile)
	}
}

// The other half, asserted so the split cannot be satisfied by moving everything
// into one file and calling the property true.
func TestTheHostFacingSurfaceIsWhereOpenTelemetryLives(t *testing.T) {
	var found bool
	for _, path := range importsOf(t, hostFacingFile) {
		if strings.HasPrefix(path, otelPrefix) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s imports no OpenTelemetry package — either the implementation moved "+
			"and this pair of tests is now checking nothing, or the split has collapsed",
			hostFacingFile)
	}
}

func importsOf(t *testing.T, name string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	out := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		out = append(out, strings.Trim(imp.Path.Value, `"`))
	}
	return out
}
