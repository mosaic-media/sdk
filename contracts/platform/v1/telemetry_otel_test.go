package v1_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	logembedded "go.opentelemetry.io/otel/log/embedded"
	"go.opentelemetry.io/otel/trace"
	traceembedded "go.opentelemetry.io/otel/trace/embedded"
	"go.opentelemetry.io/otel/trace/noop"

	v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
)

// The classification has to survive the conversion to OpenTelemetry, because
// OTel has no notion of one: `attribute.String(k, v)` carries `v`, full stop.
// This is the property platform#34 exists for, checked at the boundary sdk#5
// says it matters most — third-party code is where an unclassified value is
// most likely to originate.
func TestRedactionSurvivesTheConversionToOTel(t *testing.T) {
	logger := &recordingLogger{}
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Logger: logger})

	tel.Info("import starting",
		v1.String("provider", "tmdb"),
		v1.Int("results", 7),
		v1.Sensitive("query", "the terminator"),
		v1.Secret("api_key", "sk-live-4242"),
		// A Field built by hand, which is how a module author bypasses the
		// constructors without meaning to. Its zero-value class is not
		// RedactionNone, so it must fail closed.
		v1.Field{Key: "hand_written", Value: "postgres://user:hunter2@db/x"},
	)

	if len(logger.records) != 1 {
		t.Fatalf("want one record, got %d", len(logger.records))
	}
	got := attributesOf(logger.records[0])

	for _, leaked := range []string{"the terminator", "sk-live-4242", "hunter2"} {
		for k, v := range got {
			if strings.Contains(v, leaked) {
				t.Errorf("%q reached the record on %q", leaked, k)
			}
		}
	}
	if got["provider"] != "tmdb" {
		t.Errorf("a safe field was lost: provider=%q", got["provider"])
	}
	if got["results"] != "7" {
		t.Errorf("a safe count was lost: results=%q", got["results"])
	}
	for _, key := range []string{"query", "api_key", "hand_written"} {
		if _, present := got[key]; !present {
			t.Errorf("%q vanished entirely; a redacted field must still show it was there", key)
		}
	}
}

// Identifier is the one class that carries its value across the module boundary,
// because only the Platform holds the install salt. An encoder without one
// therefore cannot keep the promise the class makes — so it drops the field
// rather than emitting the raw value, which is the difference between a
// pseudonymisation feature and a leak.
func TestAnIdentifierWithoutASaltIsDroppedRatherThanEmitted(t *testing.T) {
	logger := &recordingLogger{}
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Logger: logger})
	tel.Info("resolved", v1.Identifier("user", "ada@example.com"))

	got := attributesOf(logger.records[0])
	if v, present := got["user"]; present {
		t.Fatalf("an unsalted Identifier was emitted as %q", v)
	}

	// With a salt, it is carried as the digest and never as the value.
	salted := &recordingLogger{}
	tel = v1.NewTelemetry(context.Background(), v1.TelemetryOptions{
		Logger: salted,
		Digest: func(value string) string { return "digest-of-" + value[:3] },
	})
	tel.Info("resolved", v1.Identifier("user", "ada@example.com"))

	got = attributesOf(salted.records[0])
	if got["user"] != "digest-of-ada" {
		t.Fatalf("user is %q, want the digest", got["user"])
	}
	if strings.Contains(got["user"], "@example.com") {
		t.Fatal("the value survived alongside the digest")
	}
}

// The record has to be an ordinary OpenTelemetry one, since the whole point of
// the change is that tooling nobody wrote can read it.
func TestTheRecordIsAnOrdinaryOTelRecord(t *testing.T) {
	logger := &recordingLogger{}
	at := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{
		Logger: logger,
		Now:    func() time.Time { return at },
	})

	tel.Debug("d")
	tel.Info("i")
	tel.Warn("w")
	tel.Error("e")

	want := []struct {
		severity log.Severity
		text     string
		body     string
	}{
		{log.SeverityDebug, "debug", "d"},
		{log.SeverityInfo, "info", "i"},
		{log.SeverityWarn, "warn", "w"},
		{log.SeverityError, "error", "e"},
	}
	if len(logger.records) != len(want) {
		t.Fatalf("want %d records, got %d", len(want), len(logger.records))
	}
	for i, w := range want {
		got := logger.records[i]
		if got.Severity() != w.severity {
			t.Errorf("record %d severity is %v, want %v", i, got.Severity(), w.severity)
		}
		// Set alongside the number because a human reading an exported record
		// should not have to know that 9 means info.
		if got.SeverityText() != w.text {
			t.Errorf("record %d severity text is %q, want %q", i, got.SeverityText(), w.text)
		}
		if got.Body().AsString() != w.body {
			t.Errorf("record %d body is %q, want %q", i, got.Body().AsString(), w.body)
		}
		if !got.Timestamp().Equal(at) {
			t.Errorf("record %d is stamped %s, want %s", i, got.Timestamp(), at)
		}
	}
}

// A span is not a laxer channel than a log field, and since the logs API
// carries attribute.KeyValue the same code classifies both.
func TestSpanAttributesAreClassifiedAndFailureIsRecorded(t *testing.T) {
	tracer := &recordingTracer{}
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{Tracer: tracer})

	_, span := tel.Span(context.Background(), "fetch-metadata", v1.Secret("token", "sk-live-4242"))
	span.SetAttributes(v1.String("provider", "tmdb"), v1.Sensitive("title", "The Terminator"))
	span.Fail(nil) // the success path must not mark a span failed
	if tracer.span.status != codes.Unset {
		t.Fatalf("a nil error set the status to %v", tracer.span.status)
	}
	span.Fail(context.DeadlineExceeded)
	span.End()

	if tracer.name != "fetch-metadata" {
		t.Errorf("span name is %q", tracer.name)
	}
	for _, kv := range tracer.span.attrs {
		if strings.Contains(kv.Value.Emit(), "sk-live-4242") || strings.Contains(kv.Value.Emit(), "The Terminator") {
			t.Errorf("a classified value reached a span attribute: %s=%s", kv.Key, kv.Value.Emit())
		}
	}
	if tracer.span.status != codes.Error {
		t.Errorf("status is %v, want Error", tracer.span.status)
	}
	if tracer.span.err == nil {
		t.Error("the error was not recorded on the span")
	}
	if !tracer.span.ended {
		t.Error("End did not reach the span")
	}
}

// A Telemetry with nothing wired discards and never panics. The Platform builds
// these at an invocation seam that may be running before a pipeline exists, and
// a logging call must never be the thing that takes a request down.
func TestNothingWiredDiscardsRatherThanPanics(t *testing.T) {
	tel := v1.NewTelemetry(nil, v1.TelemetryOptions{})
	tel.Info("anything", v1.String("k", "v"))

	ctx, span := tel.Span(context.Background(), "work")
	if ctx == nil {
		t.Fatal("Span returned a nil context; a module's own nesting depends on it")
	}
	span.SetAttributes(v1.String("k", "v"))
	span.Fail(context.Canceled)
	span.End()
}

// The no-op tracer from OTel's own package must work as a Tracer, which is the
// cheap proof that this takes the API rather than a Mosaic-shaped thing.
func TestOTelsOwnNoopTracerSatisfiesTheOption(t *testing.T) {
	tel := v1.NewTelemetry(context.Background(), v1.TelemetryOptions{
		Tracer: noop.NewTracerProvider().Tracer("test"),
	})
	ctx, span := tel.Span(context.Background(), "work")
	span.End()
	if ctx == nil {
		t.Fatal("nil context")
	}
}

// attributesOf flattens a record's attributes into key → rendered value.
func attributesOf(r log.Record) map[string]string {
	out := map[string]string{}
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		out[string(kv.Key)] = kv.Value.Emit()
		return true
	})
	return out
}

// recordingLogger is a log.Logger that keeps what it was given.
//
// It embeds embedded.Logger deliberately. The OTel logs API is pre-1.0 and says
// in its own package documentation that its interfaces may gain methods without
// a major bump; embedding the `embedded` type is how an implementation chooses
// *compilation failure* over a runtime panic when that happens. A fake that
// declared the methods by hand would start panicking in somebody's build
// instead.
type recordingLogger struct {
	logembedded.Logger
	records []log.Record
}

func (l *recordingLogger) Emit(_ context.Context, record log.Record) {
	l.records = append(l.records, record)
}

func (l *recordingLogger) Enabled(context.Context, log.EnabledParameters) bool { return true }

type recordingTracer struct {
	traceembedded.Tracer
	name string
	span *recordingSpan
}

func (tr *recordingTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	cfg := trace.NewSpanStartConfig(opts...)
	tr.name = name
	tr.span = &recordingSpan{attrs: cfg.Attributes()}
	return ctx, tr.span
}

type recordingSpan struct {
	traceembedded.Span
	attrs  []attribute.KeyValue
	status codes.Code
	err    error
	ended  bool
}

func (s *recordingSpan) End(...trace.SpanEndOption)            { s.ended = true }
func (s *recordingSpan) AddEvent(string, ...trace.EventOption) {}
func (s *recordingSpan) AddLink(trace.Link)                    {}
func (s *recordingSpan) IsRecording() bool                     { return true }
func (s *recordingSpan) RecordError(err error, _ ...trace.EventOption) {
	s.err = err
}
func (s *recordingSpan) SpanContext() trace.SpanContext { return trace.SpanContext{} }
func (s *recordingSpan) SetStatus(code codes.Code, _ string) {
	s.status = code
}
func (s *recordingSpan) SetName(string) {}
func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.attrs = append(s.attrs, kv...)
}
func (s *recordingSpan) TracerProvider() trace.TracerProvider { return noop.NewTracerProvider() }
