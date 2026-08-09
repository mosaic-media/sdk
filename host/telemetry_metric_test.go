package host

import (
	"context"
	"testing"

	modulev1 "github.com/mosaic-media/contracts/gen/mosaic/module/v1"
	v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
	"google.golang.org/grpc"
)

// The metric calls have to survive the round trip through the harness, and this
// is the test that says so.
//
// **The failure it guards against is invisible in every other configuration.**
// Three of Mosaic's modules run out of process (ADR 0077) and the rest run in
// it, so a bridge that accepted a Count and dropped it would work perfectly in
// development, in every in-process test, and for three of six modules in
// production. It would also be the exact thing ADR 0059 refused to publish — a
// counter that discards silently — rebuilt by omission rather than by decision.
func TestMetricCallsCrossTheHarness(t *testing.T) {
	impl := &recordingTelemetry{}
	server := &telemetryServer{impl: impl}
	client := &telemetryClient{client: loopbackTelemetry{server: server}}

	client.Count("addon_requests", 3,
		v1.String("addon", "cinemeta"),
		v1.Secret("api_key", "sk-live-4242"),
	)
	client.Measure("payload", 2048, v1.UnitBytes, v1.String("addon", "cinemeta"))

	if len(impl.counts) != 1 {
		t.Fatalf("want one count on the Platform side, got %d", len(impl.counts))
	}
	if impl.counts[0].name != "addon_requests" || impl.counts[0].delta != 3 {
		t.Errorf("count arrived as %+v", impl.counts[0])
	}
	if len(impl.measures) != 1 {
		t.Fatalf("want one measure, got %d", len(impl.measures))
	}
	// The unit is the field most likely to be lost, because both sides map
	// through a closed enum and a mapping that silently returns the zero value
	// looks like working code.
	if impl.measures[0].unit != v1.UnitBytes {
		t.Errorf("unit arrived as %q, want %q", impl.measures[0].unit, v1.UnitBytes)
	}
	if impl.measures[0].value != 2048 {
		t.Errorf("value arrived as %v", impl.measures[0].value)
	}

	// A classified value must not be reconstructed on the far side. Secret
	// drops at construction (ADR 0056), so what crosses the wire is already
	// the placeholder — asserting it here is what proves the boundary did not
	// re-widen when the metric calls were added beside the log ones.
	for _, f := range impl.counts[0].attrs {
		if f.Key == "api_key" && f.Value == "sk-live-4242" {
			t.Error("a secret crossed the harness on a metric dimension")
		}
	}
}

// A unit this build does not know must arrive as unitless rather than as a
// value the Platform would pass along without understanding — the property that
// makes the vocabulary closed in the sense ADR 0130 means.
func TestAnUnknownUnitArrivesUnitlessRatherThanInvented(t *testing.T) {
	if got := unitToWire(v1.Unit("furlongs")); got != modulev1.MetricUnit_METRIC_UNIT_UNSPECIFIED {
		t.Errorf("an unknown unit went onto the wire as %v", got)
	}
	if got := unitFromWire(modulev1.MetricUnit(99)); got != "" {
		t.Errorf("an unknown wire unit came back as %q, want unitless", got)
	}
	// And the ones in the set round-trip, so the fallback above is not simply
	// swallowing everything.
	for _, u := range []v1.Unit{v1.UnitSeconds, v1.UnitBytes, v1.UnitItems} {
		if back := unitFromWire(unitToWire(u)); back != u {
			t.Errorf("%q round-tripped to %q", u, back)
		}
	}
}

// A telemetryServer with no implementation behind it must accept and discard
// rather than panic: it is the state a harness is in before the Platform hands
// it one.
func TestTheServerToleratesNoImplementation(t *testing.T) {
	server := &telemetryServer{}
	if _, err := server.Count(context.Background(), &modulev1.CountRequest{Name: "x", Delta: 1}); err != nil {
		t.Errorf("Count: %v", err)
	}
	if _, err := server.Measure(context.Background(), &modulev1.MeasureRequest{Name: "x", Value: 1}); err != nil {
		t.Errorf("Measure: %v", err)
	}
}

// ─── fakes ──────────────────────────────────────────────────────────────────

// loopbackTelemetry is a TelemetryServiceClient that calls the server directly,
// so a test exercises both halves of the conversion without standing up gRPC.
type loopbackTelemetry struct {
	modulev1.TelemetryServiceClient
	server *telemetryServer
}

func (l loopbackTelemetry) Count(ctx context.Context, in *modulev1.CountRequest, _ ...grpc.CallOption) (*modulev1.CountResponse, error) {
	return l.server.Count(ctx, in)
}

func (l loopbackTelemetry) Measure(ctx context.Context, in *modulev1.MeasureRequest, _ ...grpc.CallOption) (*modulev1.MeasureResponse, error) {
	return l.server.Measure(ctx, in)
}

type countCall struct {
	name  string
	delta int64
	attrs []v1.Field
}

type measureCall struct {
	name  string
	value float64
	unit  v1.Unit
	attrs []v1.Field
}

// recordingTelemetry stands in for the Platform's real implementation.
type recordingTelemetry struct {
	counts   []countCall
	measures []measureCall
}

var _ v1.Telemetry = (*recordingTelemetry)(nil)

func (r *recordingTelemetry) Debug(string, ...v1.Field) {}
func (r *recordingTelemetry) Info(string, ...v1.Field)  {}
func (r *recordingTelemetry) Warn(string, ...v1.Field)  {}
func (r *recordingTelemetry) Error(string, ...v1.Field) {}

func (r *recordingTelemetry) Span(ctx context.Context, _ string, _ ...v1.Field) (context.Context, v1.Span) {
	return ctx, noopSpan{}
}

func (r *recordingTelemetry) Count(name string, delta int64, attrs ...v1.Field) {
	r.counts = append(r.counts, countCall{name: name, delta: delta, attrs: attrs})
}

func (r *recordingTelemetry) Measure(name string, value float64, unit v1.Unit, attrs ...v1.Field) {
	r.measures = append(r.measures, measureCall{name: name, value: value, unit: unit, attrs: attrs})
}
