package v1

import (
	"context"
	"fmt"
	"time"
)

// The module observability surface (ADR 0059).
//
// A module is an anti-corruption layer against a system it does not control,
// which makes it the code most likely to meet a shape nobody predicted — and
// until this existed a module had exactly two ways to say anything: return an
// error, or print. Printing went to the Platform's stdout from another
// repository, unstructured, unattributed, and with no way to classify what it
// carried.
//
// The shape of the answer: **the Platform owns the observability plane, and a
// module hooks into it.** A module emits; the Platform decides where records
// go, how long they live and who may read them. A module configures no
// exporter, no sink, no sampling and no retention. That is the difference
// between a hook and a delegation, and it is why this is an interface the
// Platform implements rather than a re-export of whatever the Platform happens
// to use internally.
//
// Nothing here adds a dependency. The SDK has none, deliberately: a third
// party compiles against the contract and against nothing the Platform chose.

// RedactionClass says how a field's value may be recorded. It is the same
// fail-closed vocabulary the Platform uses on its own records, and it crosses
// the module boundary because a containment property that stops at a
// repository boundary is not a containment property — third-party code is
// exactly where an unclassified value is most likely to originate, and module
// text is rendered into an administrator's browser.
type RedactionClass string

const (
	// RedactionNone marks a value safe to record verbatim: identifiers,
	// states, counts, error categories. Never use it for anything that could
	// be personal data or a credential.
	RedactionNone RedactionClass = "none"
	// RedactionSensitive marks personal or identifying data. The value is
	// dropped; only the key survives.
	RedactionSensitive RedactionClass = "sensitive"
	// RedactionSecret marks credential material — an API key, a token, a
	// signed URL. Dropped in every sink, at every level, always.
	RedactionSecret RedactionClass = "secret"
	// RedactionIdentifier marks a value recorded as a stable salted digest
	// rather than verbatim, so occurrences correlate without the record
	// holding who or what they refer to. The Platform applies the salt.
	RedactionIdentifier RedactionClass = "identifier"
)

// redacted replaces a classified value. It is applied at construction, so a
// dropped value is never carried into the Platform at all.
const redacted = "[REDACTED]"

// Field is one structured value on a record or a span.
//
// The zero value is *not* safe-by-default: a Field built as a struct literal
// has an empty RedactionClass, which is not RedactionNone, and the Platform
// redacts it on the way out. Forgetting to classify costs you the value, not
// your users their privacy.
type Field struct {
	Key       string
	Value     any
	Redaction RedactionClass
}

// String records a value verbatim. For identifiers, states and categories —
// never for anything a user typed or a provider returned about a person.
func String(key, value string) Field {
	return Field{Key: key, Value: value, Redaction: RedactionNone}
}

// Int records a count or size verbatim.
func Int(key string, value int) Field {
	return Field{Key: key, Value: value, Redaction: RedactionNone}
}

// Int64 records a 64-bit count, size or offset verbatim.
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value, Redaction: RedactionNone}
}

// Bool records a flag verbatim.
func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value, Redaction: RedactionNone}
}

// Duration records an elapsed time verbatim, rendered readably.
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value.String(), Redaction: RedactionNone}
}

// Err records an error's message verbatim.
//
// Error text is authored by code rather than by a user — with one caveat worth
// stating, because it is the common way this leaks: an error that interpolates
// a response body, a URL or a username into its message has smuggled that past
// this classification. That is a bug in the error, not in this call.
func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: "", Redaction: RedactionNone}
	}
	return Field{Key: "error", Value: err.Error(), Redaction: RedactionNone}
}

// Sensitive records that a field was present without recording its value.
// Use it for anything personal or identifying: a username, a search term, a
// title someone is watching.
func Sensitive(key string, value any) Field {
	return Field{Key: key, Value: dropValue(value), Redaction: RedactionSensitive}
}

// Secret records that a credential-bearing field was present, without its
// value. An API key, a token, a signed or authenticated URL.
func Secret(key string, value any) Field {
	return Field{Key: key, Value: dropValue(value), Redaction: RedactionSecret}
}

// Identifier records a stable digest of value rather than value itself, so two
// records about the same subject can be tied together without either record
// holding the subject.
//
// Unlike Sensitive and Secret this carries the value to the Platform, because
// only the Platform holds the install salt. It is pseudonymous, not anonymous:
// treat a digest as personal data that is merely harder to read.
func Identifier(key string, value any) Field {
	if value == nil {
		return Field{Key: key, Value: "", Redaction: RedactionIdentifier}
	}
	return Field{Key: key, Value: value, Redaction: RedactionIdentifier}
}

// dropValue discards a classified value at construction. An empty value has
// nothing to drop, so it stays empty rather than becoming a misleading
// placeholder.
func dropValue(value any) any {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok && s == "" {
		return ""
	}
	return redacted
}

// Telemetry is what a module records through. Obtain it with TelemetryFrom on
// the context the Platform handed you:
//
//	func (c *Capability) Import(ctx context.Context, svc v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error) {
//	    t := v1.TelemetryFrom(ctx)
//	    t.Info("import starting", v1.String("native_type", req.Ref.NativeType))
//
//	    ctx, span := t.Span(ctx, "fetch-metadata")
//	    meta, err := c.fetch(ctx, req.Ref)
//	    span.Fail(err)
//	    span.End()
//	    ...
//	}
//
// The message is a constant and the data goes in fields. A value formatted
// into the message with fmt.Sprintf has bypassed classification entirely,
// which is how every scheme like this is actually defeated in practice.
//
// A module that never touches this is still traced: the Platform spans the
// invocation, the context already carries the trace, and the HTTP client the
// Platform hands you propagates it. Using this adds detail, not correctness.
type Telemetry interface {
	// Debug records detail useful while diagnosing. Off unless an operator
	// turns the level down, so it is the right place for volume.
	Debug(message string, fields ...Field)
	// Info records the normal narration of what the module is doing.
	Info(message string, fields ...Field)
	// Warn records something that did not stop the operation but should not be
	// routine — a provider returning a shape you had to work around.
	Warn(message string, fields ...Field)
	// Error records an operation that failed.
	Error(message string, fields ...Field)

	// Span measures a unit of work and places it in the trace. The returned
	// context must be passed to whatever the span covers, so anything nested
	// inside — including an outbound HTTP call — appears beneath it rather
	// than beside it.
	Span(ctx context.Context, name string, attrs ...Field) (context.Context, Span)
}

// Span is one measured unit of work inside a module.
//
// Nothing is recorded until End. Call it — a deferred End is the usual shape —
// or the span is simply dropped, which is deliberate: an abandoned span is
// better lost than recorded with a meaningless duration.
type Span interface {
	// SetAttributes adds attributes to a span in flight. They are classified
	// exactly like log fields; a span attribute is not a laxer channel.
	SetAttributes(attrs ...Field)
	// Fail marks the span failed and records err. A nil error is ignored, so
	// `span.Fail(err)` is safe on the success path.
	Fail(err error)
	// End completes the span. Idempotent.
	End()
}

// telemetryKey is unexported, so only the Platform can install a Telemetry.
// A module cannot forge one into a context and cannot attribute its records
// to anything other than itself.
type telemetryKey struct{}

// WithTelemetry returns a context carrying t.
//
// It exists for the Platform to install the implementation, and for a module's
// own tests to install a fake. A module does not call it in normal operation —
// it receives a context that already has one.
func WithTelemetry(ctx context.Context, t Telemetry) context.Context {
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, telemetryKey{}, t)
}

// TelemetryFrom returns the Telemetry carried by ctx, or a working no-op.
//
// It never returns nil and never panics, so a call site needs no check and no
// error handling. A module run outside the Platform — in its own unit tests,
// or against a Platform too old to provide one — records nothing and works
// exactly as before.
func TelemetryFrom(ctx context.Context) Telemetry {
	if ctx == nil {
		return noopTelemetry{}
	}
	if t, ok := ctx.Value(telemetryKey{}).(Telemetry); ok && t != nil {
		return t
	}
	return noopTelemetry{}
}

// noopTelemetry discards everything.
type noopTelemetry struct{}

func (noopTelemetry) Debug(string, ...Field) {}
func (noopTelemetry) Info(string, ...Field)  {}
func (noopTelemetry) Warn(string, ...Field)  {}
func (noopTelemetry) Error(string, ...Field) {}

func (noopTelemetry) Span(ctx context.Context, _ string, _ ...Field) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) SetAttributes(...Field) {}
func (noopSpan) Fail(error)             {}
func (noopSpan) End()                   {}

// EmitValue returns what a sink should record for f, re-applying redaction so
// a Field built as a struct literal — whose zero-value class is not
// RedactionNone — fails closed.
//
// The Platform calls this; a module has no reason to. It is exported because
// the Platform is a different module and must be able to apply the rule the
// same way, rather than reimplementing it and drifting.
func (f Field) EmitValue() any {
	switch f.Redaction {
	case RedactionNone, RedactionIdentifier:
		return f.Value
	default:
		return dropValue(f.Value)
	}
}

// StringValue renders f's value for a sink that wants text.
func (f Field) StringValue() string {
	v := f.EmitValue()
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
