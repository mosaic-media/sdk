package v1_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"

	v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
)

// A metric attribute goes through the same classification as a log field, and
// this is the test that says so mechanically rather than by convention.
//
// It matters more here than on a record, not less: a log record ages out under
// retention, and a metric series lives for the life of the process. A leaked
// value on a record is eventually gone; a leaked value as a *dimension* is a
// permanent label on a running counter.
func TestMetricAttributesAreClassifiedLikeLogFields(t *testing.T) {
	meter := &recordingMeter{}
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Meter: meter})

	tel.Count("addon_requests", 1,
		v1.String("addon", "cinemeta"),
		v1.Sensitive("query", "the terminator"),
		v1.Secret("api_key", "sk-live-4242"),
		// Built by hand, so its class is the zero value rather than
		// RedactionNone. It must fail closed here exactly as it does on a
		// record.
		v1.Field{Key: "hand_written", Value: "postgres://user:hunter2@db/x"},
	)

	if len(meter.adds) != 1 {
		t.Fatalf("want one counter add, got %d", len(meter.adds))
	}
	got := attributeMap(meter.adds[0].attrs)

	for _, leaked := range []string{"the terminator", "sk-live-4242", "hunter2"} {
		for k, v := range got {
			if strings.Contains(v, leaked) {
				t.Errorf("%q reached the counter on dimension %q", leaked, k)
			}
		}
	}
	if got["addon"] != "cinemeta" {
		t.Errorf("the unclassified dimension did not survive: %q", got["addon"])
	}
}

// The value and the instrument name have to arrive as given, or a counter is
// counting something other than what the module said.
func TestCountReachesTheMeter(t *testing.T) {
	meter := &recordingMeter{}
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Meter: meter})

	tel.Count("results_returned", 7, v1.String("provider", "tmdb"))
	tel.Count("results_returned", 3, v1.String("provider", "tmdb"))

	if len(meter.adds) != 2 {
		t.Fatalf("want two adds, got %d", len(meter.adds))
	}
	for i, want := range []int64{7, 3} {
		if meter.adds[i].name != "results_returned" {
			t.Errorf("add %d named the instrument %q", i, meter.adds[i].name)
		}
		if meter.adds[i].delta != want {
			t.Errorf("add %d carried %d, want %d", i, meter.adds[i].delta, want)
		}
	}
}

// The unit is the whole reason Measure takes one: an exported histogram without
// it cannot be labelled or converted by anything downstream.
func TestMeasureCarriesItsUnit(t *testing.T) {
	for _, tc := range []struct {
		unit v1.Unit
		want string
	}{
		{v1.UnitSeconds, "s"},
		{v1.UnitBytes, "By"},
		{v1.UnitItems, "{item}"},
		// The type is a string, so a caller can always write one that is not in
		// the closed set. The measurement is kept and the annotation dropped —
		// an unrecognised unit is worse than none, because a backend converts
		// against it.
		{v1.Unit("furlongs"), ""},
	} {
		meter := &recordingMeter{}
		tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Meter: meter})

		tel.Measure("payload", 1024, tc.unit)

		if len(meter.records) != 1 {
			t.Fatalf("%s: want one histogram record, got %d", tc.unit, len(meter.records))
		}
		if got := meter.records[0].unit; got != tc.want {
			t.Errorf("unit %q reached the instrument as %q, want %q", tc.unit, got, tc.want)
		}
		if meter.records[0].value != 1024 {
			t.Errorf("%s: value arrived as %v", tc.unit, meter.records[0].value)
		}
	}
}

// **This is the failure ADR 0059 refused to ship**, and the reason it declined
// to publish a counter at all until the Platform could back one: a module author
// instruments against something that discards, gets no data, and has nothing
// telling them why.
//
// The only way to create an instrument wrongly is to name it wrongly, which no
// test against a working meter reveals — so the diagnostic goes through the
// channel the module is already reading.
func TestARefusedInstrumentIsReportedRatherThanDroppedSilently(t *testing.T) {
	logger := &recordingLogger{}
	meter := &recordingMeter{refuse: errors.New("invalid instrument name")}
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Logger: logger, Meter: meter})

	tel.Count("has a space", 1)

	if len(logger.records) != 1 {
		t.Fatalf("want one record telling the module its instrument was refused, got %d", len(logger.records))
	}
	attrs := attributesOf(logger.records[0])
	if attrs["instrument"] != "has a space" {
		t.Errorf("the record does not name the instrument: %v", attrs)
	}
	if !strings.Contains(attrs["error"], "invalid instrument name") {
		t.Errorf("the record does not carry the reason: %v", attrs)
	}
}

// Bounded, because a bad name is usually in a loop. One record naming the
// instrument is the whole of what an author needs, and a record per call would
// be the flood the quota exists to prevent.
func TestTheRefusedInstrumentReportIsBoundedPerInvocation(t *testing.T) {
	logger := &recordingLogger{}
	meter := &recordingMeter{refuse: errors.New("invalid instrument name")}
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Logger: logger, Meter: meter})

	for range 100 {
		tel.Count("has a space", 1)
		tel.Measure("also bad", 1, v1.UnitBytes)
	}

	if len(logger.records) != 1 {
		t.Fatalf("200 refused instruments produced %d records, want exactly 1", len(logger.records))
	}
}

// A Telemetry built without a meter must discard rather than panic — the same
// property Logger and Tracer already have, and what makes a partially wired
// host safe.
func TestMetricsAreDiscardedWithoutAMeter(t *testing.T) {
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{})
	tel.Count("nothing_listening", 1, v1.String("k", "v"))
	tel.Measure("nothing_listening", 1.5, v1.UnitSeconds)
}

// The no-op returned by TelemetryFrom on an unseeded context has to answer the
// metric calls too, or a module running standalone — in its own tests, or
// against an older Platform — panics on the one surface that is newest.
func TestTheNoOpTelemetryAnswersTheMetricCalls(t *testing.T) {
	tel := v1.TelemetryFrom(context.Background())
	tel.Count("standalone", 1)
	tel.Measure("standalone", 1, v1.UnitItems)
}

// ─── fakes ──────────────────────────────────────────────────────────────────

// recordingMeter is a metric.Meter that keeps what it was given.
//
// It embeds the API's own no-op meter rather than implementing the interface
// outright: `metric.Meter` declares an instrument constructor for every kind
// and every value type, and a hand-written stub of all of them would need
// editing each time the API grows one — which is exactly the churn a pre-1.0
// dependency is expected to produce.
type recordingMeter struct {
	metricnoop.Meter
	// refuse, when set, is returned instead of an instrument.
	refuse  error
	adds    []addCall
	records []recordCall
}

type addCall struct {
	name  string
	delta int64
	attrs attribute.Set
}

type recordCall struct {
	name  string
	value float64
	unit  string
	attrs attribute.Set
}

func (m *recordingMeter) Int64Counter(name string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if m.refuse != nil {
		return nil, m.refuse
	}
	return &recordingCounter{meter: m, name: name}, nil
}

func (m *recordingMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	if m.refuse != nil {
		return nil, m.refuse
	}
	return &recordingHistogram{
		meter: m,
		name:  name,
		unit:  metric.NewFloat64HistogramConfig(opts...).Unit(),
	}, nil
}

type recordingCounter struct {
	metricnoop.Int64Counter
	meter *recordingMeter
	name  string
}

func (c *recordingCounter) Add(_ context.Context, delta int64, opts ...metric.AddOption) {
	c.meter.adds = append(c.meter.adds, addCall{
		name:  c.name,
		delta: delta,
		attrs: metric.NewAddConfig(opts).Attributes(),
	})
}

type recordingHistogram struct {
	metricnoop.Float64Histogram
	meter *recordingMeter
	name  string
	unit  string
}

func (h *recordingHistogram) Record(_ context.Context, value float64, opts ...metric.RecordOption) {
	h.meter.records = append(h.meter.records, recordCall{
		name:  h.name,
		value: value,
		unit:  h.unit,
		attrs: metric.NewRecordConfig(opts).Attributes(),
	})
}

func attributeMap(set attribute.Set) map[string]string {
	out := map[string]string{}
	for _, kv := range set.ToSlice() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}
