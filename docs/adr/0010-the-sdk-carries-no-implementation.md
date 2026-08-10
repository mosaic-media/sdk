# The SDK carries no implementation

**Status:** Proposed. The SDK's `go.mod` still requires four OpenTelemetry API modules as this is written.
**Date:** 2026-08-10

Reverses the dependency clause of
[sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) and reinstates
two of the three grounds
[sdk#5](0005-modules-observe-through-the-sdk.md) originally rejected it on.
[sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md)'s central decision — OpenTelemetry is Mosaic's telemetry
implementation, in every process — **stands unchanged**.

## Context

[sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) gave the SDK its first dependency: four OpenTelemetry API modules,
replacing the zero-dependency rule with "the OTel API and nothing else". Two
days of building on it show the line was drawn in the wrong place.

**The module-facing surface is clean, and that part worked.**
`contracts/platform/v1/telemetry.go` declares `Telemetry`, `Span`, `Field` and
`TelemetryFrom` and names no OpenTelemetry type; a test fails the build if it
ever does. A module author writes `v1.TelemetryFrom(ctx).Info(…)` and never sees
OTel.

**The rest of it is host code living in a module's contract.**
`telemetry_otel.go` sits in the same published package and holds `NewTelemetry`,
`TelemetryOptions{Logger, Tracer, Meter}` and `Encoder` — and it is what puts
four modules in the SDK's `go.mod`. Its two callers are the Platform and the
extension harness. **Neither is a module.** Nothing a module does needs any of
it, and every module pays for all of it.

That is the distinction [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) missed, and [sdk#5](0005-modules-observe-through-the-sdk.md) had already drawn it:

> It also publishes an implementation choice as a contract, so replacing OTel
> later would be a breaking change to a surface third parties compile against,
> when it should be an internal detail.

[sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) overrode that on the grounds that OpenTelemetry is vendor-neutral and
CNCF-governed, which is true and is not the point. **The objection was never
about which library — it was about a contract carrying one at all.**

Of [sdk#5](0005-modules-observe-through-the-sdk.md)'s three reasons, the second (a module could reach the configuration
surface) genuinely was answered: a module receives a `log.Logger` and never a
provider. The first and third were not answered, only outweighed, and this
record puts them back.

**The governing principle, which is not new but has never been written down:**
the SDK says **how a module interacts with the Platform**; the Platform holds
**the implementations**. Mosaic's build path depends on it — the Platform is
where functionality is implemented and modules are how functionality is added,
so a contract that names an implementation makes the Platform's private choices
into everyone's public constraint.

## Decision

**The SDK names no implementation and depends on nothing. Its `go.mod` returns
to a module line and a Go version.**

- **The classification rule stays in the SDK, expressed in Go.** It is the one
  genuinely shared thing: the Platform converts classified fields for in-process
  modules and the harness does it for out-of-process ones, and a second copy of
  a fail-closed rule is how the guarantee quietly stops being one. It does not
  need an OpenTelemetry type to say what it is — resolving a `Field` to the
  value a sink may record is `(any, bool)`, pure Go. **Only the last step —
  building an `attribute.KeyValue` — needs OTel, and that is three lines each
  host writes for itself.**
- **`NewTelemetry`, `TelemetryOptions` and the OTel wiring move to the hosts.**
  The Platform holds its own; the harness holds its own.
- **OpenTelemetry remains the implementation, everywhere.** Nothing about what
  Mosaic emits changes, no module changes beyond a version bump, and [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md)'s
  benefit — everything Mosaic emits readable by tooling nobody had to write —
  is untouched. What changes is who declares the dependency.
- **The rule that replaces "the OTel API and nothing else" is "nothing".** Not
  as an aesthetic: a third party compiles against a contract rather than against
  the Platform's taste, and the moment the contract carries a version floor for
  somebody else's library, it is distributing taste.
- **The surface may be wide; it may not be deep.** The SDK should be *broad* at
  1.0 — a module nobody has imagined must be expressible, and a missing verb is
  found only by someone trying to build the thing it blocks. Breadth and
  implementation-freedom are not in tension: a contract can declare a great many
  interfaces and still name no library, which is precisely what the content
  surface already does.
- **The same rule settles the crypto question without a second argument.** The
  Platform's secret facility ([platform#81](https://github.com/mosaic-media/platform/blob/main/docs/adr/0081-the-install-key.md) and its
  successor) is reached by modules through a *declarative* surface — a settings
  field marked secret, sealed by the Platform — and never through `Seal`/`Open`
  primitives, which would publish an implementation and hand a module an
  encryption oracle.

## Alternatives

**Leave it.** *Rejected.* The dependency is small today and the property being
protected is not about size — a contract with one dependency has already
conceded the argument, and the next one is easier than the first.

**Move the OTel half into `sdk/host`**, the nested module that already carries
gRPC and that no *contract* consumer imports. *Rejected on the facts, and it was
the obvious answer.* All three extension modules require
`github.com/mosaic-media/sdk/host` in their own `go.mod` — an out-of-process
module imports it to serve itself — so the dependency lands in a third party's
build regardless. The nesting protects the contract module's graph and not the
module author's.

**Duplicate the classification rule in the Platform and in the harness**, and
keep nothing shared. *Rejected.* It is the arrangement [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) was written to
end: the Supervisor's duplicated record format, guarded only by a test naming
its JSON keys, is what made three hand-written telemetry stacks a decision
rather than a preference.

**Re-export OTel types from the SDK behind Mosaic names**, so the contract reads
as Mosaic's while the dependency remains. *Rejected.* It keeps every cost and
hides it, and the `go.mod` still says what it says.

## Consequences

- **The SDK's `go.mod` returns to a module line and a Go version**, and
  `sdk/host/nodeps_test.go` goes back to asserting emptiness — which is what it
  did before [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) widened it to an allowlist.
- **The seam test broadens.** It currently asserts one file names no OTel type;
  it should assert the whole contract module does, which is a stronger and
  simpler claim.
- **A minor SDK bump, and the modules bump to match.** No module's code changes:
  the surface they compile against is the half that was already clean.
- **The Platform and the harness each gain a small amount of OTel wiring** they
  did not have, and the harness's copy is the one to watch — it is the one that
  could drift from the classification rule if somebody later inlines it.
- **[sdk#5](0005-modules-observe-through-the-sdk.md)'s first and third rejections stand again**, and the record should
  be read that way: it was right about the contract and wrong about the
  implementation, which is a more useful thing to have on file than a record
  that was simply overruled.
- **The principle is now written down** — in `architecture.md` and in the
  `CLAUDE.md` of every repository that could violate it — rather than being a
  thing each session rediscovers or, as here, gets wrong.
