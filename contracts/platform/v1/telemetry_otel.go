package v1

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// OpenTelemetry behind the surface (ADR 0128).
//
// **The surface above this file did not change, and that is the decision.**
// A module still writes `v1.TelemetryFrom(ctx).Info("…", v1.String(…))`, still
// classifies its fields, and still configures nothing. What changed is what
// carries the record: an OpenTelemetry `log.Record` and an OpenTelemetry span
// rather than a Mosaic-shaped one, so everything Mosaic emits is readable by
// tooling nobody had to write.
//
// Mosaic had hand-written the same telemetry three times — the Platform's, this
// one, and the Supervisor's, whose record format is duplicated from the
// Platform's with a test naming the JSON keys as the only thing holding them
// together. This is the shared vocabulary that replaces all three, sourced from
// outside Mosaic so that no component owns it.
//
// **This file takes the OTel *API* and never the SDK.** A module gets `log.Logger`
// and `trace.Tracer` — interfaces it receives — and no provider, no exporter, no
// sampler and no global. That is ADR 0059's ownership rule unchanged: the
// Platform decides where a record goes, how long it lives and who may read it.
// A module that reached for `otel.SetLoggerProvider` would be configuring the
// Platform's observability plane from inside it, which is the thing this
// arrangement exists to prevent.

// Encoder converts classified Fields into OpenTelemetry attributes.
//
// **The redaction happens here, so an implementer cannot forget it.** OTel's
// attribute model has no notion of classification — `log.String(k, v)` carries
// `v` — so the conversion is the last place the rule can be applied, and it is
// applied to every field on every path rather than at each call site.
//
// It is exported because more than one component performs the conversion: the
// Platform for in-process modules, and the extension host for out-of-process
// ones (ADR 0077). A second copy of this rule living over there is exactly how a
// fail-closed guarantee quietly stops being one.
type Encoder struct {
	// Digest resolves an Identifier field to its stable pseudonym.
	//
	// **Nil drops Identifier fields rather than emitting them**, and that is the
	// deliberate failure direction. Identifier is the one class that carries its
	// value across the module boundary, because only the Platform holds the
	// install salt (ADR 0059) — so an encoder with no salt is one that cannot
	// keep the promise the class makes, and emitting the raw value instead would
	// turn a pseudonymisation feature into a leak. Nobody has to remember: an
	// unconfigured encoder is a safe one.
	Digest func(value string) string
}

// Attributes converts fields, dropping what cannot be recorded safely.
//
// **One conversion serves both records and spans**, because the OpenTelemetry
// logs API carries `attribute.KeyValue` rather than a type of its own. A span
// attribute is therefore not a laxer channel than a log field in any sense,
// including the mechanical one: the same code classifies both.
func (e Encoder) Attributes(fields []Field) []attribute.KeyValue {
	if len(fields) == 0 {
		return nil
	}
	out := make([]attribute.KeyValue, 0, len(fields))
	for _, f := range fields {
		if kv, ok := e.Attribute(f); ok {
			out = append(out, kv)
		}
	}
	return out
}

// Attribute converts one field, reporting false when it must not be recorded.
func (e Encoder) Attribute(f Field) (attribute.KeyValue, bool) {
	value, ok := e.resolve(f)
	if !ok {
		return attribute.KeyValue{}, false
	}
	return attributeOf(f.Key, value), true
}

// resolve applies the redaction rule, returning the value to record.
//
// Everything but Identifier goes through EmitValue, which re-applies redaction
// on the way out so a Field built as a struct literal — whose zero-value class
// is not RedactionNone — fails closed. Identifier is the exception in both
// directions: it still holds its original value at this point, and it is either
// digested here or dropped.
func (e Encoder) resolve(f Field) (any, bool) {
	if f.Redaction != RedactionIdentifier {
		return f.EmitValue(), true
	}
	if e.Digest == nil {
		return nil, false
	}
	if f.Value == nil {
		return "", true
	}
	return e.Digest(fmt.Sprint(f.Value)), true
}

// attributeOf maps a field value onto an OTel attribute.
//
// The typed cases are the ones the Field constructors produce. Anything else is
// rendered as text rather than dropped: a value that reached here has already
// been classified, so the only question left is how to write it down.
func attributeOf(key string, value any) attribute.KeyValue {
	switch v := value.(type) {
	case nil:
		return attribute.String(key, "")
	case string:
		return attribute.String(key, v)
	case bool:
		return attribute.Bool(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	default:
		return attribute.String(key, fmt.Sprint(v))
	}
}

// TelemetryOptions is what NewTelemetry needs. Every field is something the
// caller supplies, which is the point: a module receives a Telemetry and never
// builds one, so nothing it can reach configures the pipeline.
type TelemetryOptions struct {
	// Logger receives the levelled records. Nil discards them.
	Logger log.Logger
	// Tracer starts the spans. Nil yields spans that record nothing and still
	// return a usable context, so a module's own nesting is unaffected.
	Tracer trace.Tracer
	// Meter creates the counters and histograms. Nil discards them.
	//
	// It is a `metric.Meter` — an interface the caller supplies — and never a
	// `MeterProvider`, for the same reason Logger is not a LoggerProvider: a
	// provider is where views, readers and exporters are configured, and that
	// is the Platform's (ADR 0059). A module receives the one thing it needs to
	// record and nothing it could use to decide where the recording goes.
	Meter metric.Meter
	// Digest resolves Identifier fields. See Encoder.Digest — nil drops them.
	Digest func(value string) string
	// Now is the clock, injectable so a test can pin a record's timestamp.
	// Nil takes time.Now.
	Now func() time.Time
}

// NewTelemetry builds the OpenTelemetry-backed implementation of Telemetry.
//
// **ctx is bound, and it is bound because the interface has no room for it.**
// `Telemetry.Info` takes a message and fields, matching how a caller wants to
// write a log line; OTel's `Logger.Emit` takes a context, because that is where
// the active span lives. Something has to hold it, and the invocation's context
// is the honest thing to hold: this Telemetry belongs to one invocation, which
// is also the scope its quota and its attribution belong to. Span takes its own
// ctx explicitly, since a span's parent is whatever the caller is inside at that
// moment rather than whatever it was when this was built.
//
// The Platform calls this at the invocation seam. A module never does.
func NewTelemetry(ctx context.Context, opts TelemetryOptions) Telemetry {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &otelTelemetry{ctx: ctx, opts: opts, enc: Encoder{Digest: opts.Digest}}
}

type otelTelemetry struct {
	ctx  context.Context
	opts TelemetryOptions
	enc  Encoder
	// badInstrument bounds the "that name is not usable" diagnostic to one
	// record per invocation. See reportInstrument.
	badInstrument sync.Once
}

func (t *otelTelemetry) Debug(message string, fields ...Field) {
	t.emit(log.SeverityDebug, message, fields)
}

func (t *otelTelemetry) Info(message string, fields ...Field) {
	t.emit(log.SeverityInfo, message, fields)
}

func (t *otelTelemetry) Warn(message string, fields ...Field) {
	t.emit(log.SeverityWarn, message, fields)
}

func (t *otelTelemetry) Error(message string, fields ...Field) {
	t.emit(log.SeverityError, message, fields)
}

// emit builds and emits one record.
//
// The severity text is set alongside the number because a human reading an
// exported record should not have to know that 9 means info.
func (t *otelTelemetry) emit(severity log.Severity, message string, fields []Field) {
	if t.opts.Logger == nil {
		return
	}
	// Asked before the record is built, which is the whole reason the API
	// offers it: a Debug call in a hot loop otherwise pays for its attribute
	// conversion whether or not anything is listening.
	if !t.opts.Logger.Enabled(t.ctx, log.EnabledParameters{Severity: severity}) {
		return
	}
	var record log.Record
	record.SetTimestamp(t.opts.Now())
	record.SetSeverity(severity)
	record.SetSeverityText(severityTextOf(severity))
	record.SetBody(attribute.StringValue(message))
	record.AddAttributes(t.enc.Attributes(fields)...)
	t.opts.Logger.Emit(t.ctx, record)
}

// severityTextOf names a severity for a reader.
func severityTextOf(severity log.Severity) string {
	switch severity {
	case log.SeverityDebug:
		return "debug"
	case log.SeverityWarn:
		return "warn"
	case log.SeverityError:
		return "error"
	default:
		return "info"
	}
}

// Span starts a child span of whatever ctx is inside.
func (t *otelTelemetry) Span(ctx context.Context, name string, attrs ...Field) (context.Context, Span) {
	if t.opts.Tracer == nil {
		return ctx, noopSpan{}
	}
	ctx, span := t.opts.Tracer.Start(ctx, name, trace.WithAttributes(t.enc.Attributes(attrs)...))
	return ctx, &otelSpan{span: span, enc: t.enc}
}

// otelSpan narrows an OTel span to what the SDK exposes. The narrowing is the
// contract: a module may attribute, fail and end a span, and may not set its
// name, its kind, its links or its start time — those are the Platform's, set
// at the seam.
type otelSpan struct {
	span trace.Span
	enc  Encoder
}

func (s *otelSpan) SetAttributes(attrs ...Field) {
	s.span.SetAttributes(s.enc.Attributes(attrs)...)
}

// Fail marks the span failed and records err. A nil error is ignored, so
// `span.Fail(err)` is safe on the success path.
//
// No error *category* is taken from the module: the seven categories are a
// Platform contract, and letting a module assert one would let it describe its
// own failure in the Platform's vocabulary.
func (s *otelSpan) Fail(err error) {
	if err == nil {
		return
	}
	s.span.RecordError(err)
	s.span.SetStatus(codes.Error, err.Error())
}

func (s *otelSpan) End() { s.span.End() }

// The metric half (ADR 0130).
//
// **The instrument is created on every call, and that is not the oversight it
// looks like.** An OpenTelemetry meter caches by the instrument's identifying
// fields — name, kind, unit, description — and returns the same instrument for
// a repeat request, so this is a map lookup rather than an allocation. The
// alternative is a per-module instrument cache, which buys a lock-free lookup
// and costs the thing that makes this surface usable: a module would have to
// hold instruments as state, construct them somewhere, and thread them to every
// call site. The whole point of an ambient handle is that there is nothing to
// hold.

// Count adds delta to a counter.
func (t *otelTelemetry) Count(name string, delta int64, attrs ...Field) {
	if t.opts.Meter == nil {
		return
	}
	counter, err := t.opts.Meter.Int64Counter(name)
	if err != nil {
		t.reportInstrument(name, err)
		return
	}
	counter.Add(t.ctx, delta, metric.WithAttributes(t.enc.Attributes(attrs)...))
}

// Measure records one observation into a distribution.
func (t *otelTelemetry) Measure(name string, value float64, unit Unit, attrs ...Field) {
	if t.opts.Meter == nil {
		return
	}
	hist, err := t.opts.Meter.Float64Histogram(name, metric.WithUnit(unitAnnotation(unit)))
	if err != nil {
		t.reportInstrument(name, err)
		return
	}
	hist.Record(t.ctx, value, metric.WithAttributes(t.enc.Attributes(attrs)...))
}

// unitAnnotation renders a Unit for OpenTelemetry, normalising anything outside
// the closed set to unitless.
//
// A caller can always write `Unit("furlongs")`, since the type is a string. The
// measurement is kept and the annotation is dropped, because losing the data
// over a label would be the wrong way round — and an instrument carrying a unit
// nothing recognises is worse than one carrying none, since a backend will
// convert against it.
func unitAnnotation(u Unit) string {
	switch u {
	case UnitSeconds, UnitBytes, UnitItems:
		return string(u)
	default:
		return ""
	}
}

// reportInstrument says once, per invocation, that an instrument could not be
// created.
//
// **A metric that silently discards is the specific failure ADR 0059 refused to
// ship**, and the only way to create an instrument wrongly is to name it
// wrongly — a name OpenTelemetry rejects, which no test against a no-op meter
// would ever reveal. So it is reported through the channel the module is
// already reading.
//
// Once per invocation rather than once per call: a bad name in a loop would
// otherwise turn a diagnostic into the flood the Platform's quota exists to
// prevent, and one record naming the instrument is the whole of what a module
// author needs.
func (t *otelTelemetry) reportInstrument(name string, err error) {
	t.badInstrument.Do(func() {
		t.emit(log.SeverityWarn, "module metric instrument refused; the measurement was dropped",
			[]Field{String("instrument", name), Err(err)})
	})
}
