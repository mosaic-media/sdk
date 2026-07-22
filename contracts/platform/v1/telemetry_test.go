package v1_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
)

const fakeSecret = "hunter2-super-secret-password-AKIAFAKEEXAMPLE1234"

// TestTelemetryFromIsAlwaysUsable is the property that lets a module call
// telemetry unconditionally: a module under its own unit tests, or running
// against a Platform that provides nothing, must record nothing and work.
func TestTelemetryFromIsAlwaysUsable(t *testing.T) {
	tel := v1.TelemetryFrom(context.Background())
	tel.Info("nobody is listening", v1.String("k", "v"))
	tel.Error("still fine", v1.Err(errors.New("boom")))

	ctx, span := tel.Span(context.Background(), "work")
	span.SetAttributes(v1.Int("n", 1))
	span.Fail(errors.New("failed"))
	span.End()
	if ctx == nil {
		t.Fatal("Span must return a usable context even when unconfigured")
	}

	//nolint:staticcheck // deliberately nil: TelemetryFrom must survive it.
	v1.TelemetryFrom(nil).Info("even this")
}

func TestSensitiveAndSecretDropTheirValueAtConstruction(t *testing.T) {
	for name, f := range map[string]v1.Field{
		"sensitive": v1.Sensitive("username", "alice@example.com"),
		"secret":    v1.Secret("token", fakeSecret),
	} {
		got, _ := f.Value.(string)
		if strings.Contains(got, "alice") || strings.Contains(got, "hunter2") {
			t.Fatalf("%s carried its value forward: %v", name, f.Value)
		}
	}
}

// TestUnclassifiedFieldFailsClosed — a Field built as a struct literal has an
// empty class, which is not RedactionNone, so it must not be recorded.
func TestUnclassifiedFieldFailsClosed(t *testing.T) {
	f := v1.Field{Key: "raw", Value: fakeSecret}
	if got := f.StringValue(); strings.Contains(got, "hunter2") {
		t.Fatalf("an unclassified field must fail closed, got %q", got)
	}
}

// TestIdentifierCarriesItsValueForThePlatformToDigest documents the one
// asymmetry in the design: only the Platform holds the install salt, so
// Identifier hands the value across rather than dropping it.
func TestIdentifierCarriesItsValueForThePlatformToDigest(t *testing.T) {
	f := v1.Identifier("session", "session-abc")
	if f.Redaction != v1.RedactionIdentifier {
		t.Fatalf("class = %q, want identifier", f.Redaction)
	}
	if f.Value != "session-abc" {
		t.Fatalf("Identifier must hand the value to the Platform to digest, got %v", f.Value)
	}
}

func TestEmptyValuesStayEmptyRatherThanShowingAPlaceholder(t *testing.T) {
	if got := v1.Sensitive("reason", "").StringValue(); got != "" {
		t.Fatalf("an empty value should stay empty, got %q", got)
	}
}

// recordingTelemetry is the shape a module's own tests would install.
type recordingTelemetry struct {
	messages []string
	fields   []v1.Field
}

func (r *recordingTelemetry) Debug(m string, f ...v1.Field) { r.record(m, f) }
func (r *recordingTelemetry) Info(m string, f ...v1.Field)  { r.record(m, f) }
func (r *recordingTelemetry) Warn(m string, f ...v1.Field)  { r.record(m, f) }
func (r *recordingTelemetry) Error(m string, f ...v1.Field) { r.record(m, f) }
func (r *recordingTelemetry) record(m string, f []v1.Field) {
	r.messages = append(r.messages, m)
	r.fields = append(r.fields, f...)
}

func (r *recordingTelemetry) Span(ctx context.Context, _ string, _ ...v1.Field) (context.Context, v1.Span) {
	return ctx, noopSpanForTest{}
}

type noopSpanForTest struct{}

func (noopSpanForTest) SetAttributes(...v1.Field) {}
func (noopSpanForTest) Fail(error)                {}
func (noopSpanForTest) End()                      {}

// TestModuleCanInstallItsOwnTelemetryInTests is what makes an instrumented
// module testable without the Platform.
func TestModuleCanInstallItsOwnTelemetryInTests(t *testing.T) {
	rec := &recordingTelemetry{}
	ctx := v1.WithTelemetry(context.Background(), rec)

	v1.TelemetryFrom(ctx).Info("sourced", v1.Int("results", 3))

	if len(rec.messages) != 1 || rec.messages[0] != "sourced" {
		t.Fatalf("recorded %v, want one \"sourced\"", rec.messages)
	}
}
