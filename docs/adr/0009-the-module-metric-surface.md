# The module metric surface

**Status:** Built. `v1.Telemetry` carries `Count` and `Measure`, the harness carries them to out-of-process modules, the Platform backs them with an OpenTelemetry meter over a `ManualReader`, and the Metrics screen reads them. Not built: retention. The values live in the Platform process and reset when it restarts, which is stated on the screen rather than left for a reader to discover.
**Date:** 2026-08-09

Completes the condition [sdk#5](0005-modules-observe-through-the-sdk.md) set
rather than reversing it: that record declined to publish a counter and said
they would join the surface when the Platform could back them. It supersedes
nothing.

## Context

[sdk#5](0005-modules-observe-through-the-sdk.md) gave modules levelled logging with classified fields and spans, and
deliberately no metric of any kind:

> **No counter and no histogram.** The Platform has no metrics yet, and
> publishing a counter that silently discards is worse than publishing nothing:
> a module author would instrument against it and get no data with no indication
> why. They join the surface when the Platform can back them.

That reasoning was right and its premise has changed. [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md)
put the OpenTelemetry SDK in the Platform, so a meter and a reader are now
things it has rather than things it would have to write.

**The question that prompted this was about the learning curve, not the
feature.** A third-party contributor arriving from a Java shop expects an
observability library that weaves the right behaviour into an ordinary
`log.warn`, and asks what the equivalent costs here. For logging and tracing the
answer is already good: a module that never mentions telemetry is fully traced,
because the Platform spans the invocation at the seam
([platform#33](https://github.com/mosaic-media/platform/blob/main/docs/adr/0033-instrument-at-the-seams.md), seam 8), the context carries the
trace opaquely, and the `*http.Client` the composition root hands over propagates
it (seam 9). Across six module repositories there are six hand-written telemetry
call sites, and every module is fully traced regardless.

Metrics were the honest hole in that answer: not "hard to do correctly" but
*absent*, with no counter to reach for at all.

**Two facts about the OpenTelemetry Go metric API decide the shape**, and both
matter because [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) had to swallow a pre-1.0 logs API to land:

- `go.opentelemetry.io/otel/metric` is **v1.45.0 and stable**, covered by that
  project's compatibility guarantee — unlike `otel/log` at `v0.21.0`.
- It is an API module, so it joins the SDK's allowlist on the same terms as the
  other three: a module receives a `metric.Meter` and never a `MeterProvider`.

## Decision

**`v1.Telemetry` gains `Count` and `Measure`, on the same ambient handle. The
Platform owns the meter, bounds the cardinality, and reads the values back on a
screen.**

- **Two instruments and no more.** `Count(name, delta, attrs…)` accumulates;
  `Measure(name, value, unit, attrs…)` records into a distribution. No gauge:
  a gauge in OpenTelemetry is an observable with a callback and a lifetime, and
  a module has neither — it is invoked and returns.
- **No instrument to construct, hold or thread.** A module calls a method on the
  handle it already has. The meter caches by identifying fields, so a repeat call
  is a lookup rather than an allocation. **This is the whole point:** the floor
  for a contributor stays "do nothing and be traced", and metrics cost one line
  rather than a lifecycle. A design where instruments are constructed and held
  as state is the design that does not get adopted.
- **The unit is a closed vocabulary** — seconds, bytes, items. A unit is what
  lets a backend convert and a reader label an axis, and a free string produces
  `ms`, `millis`, `milliseconds` and `Ms` across four modules describing one
  quantity, reconciling to nothing. A unit outside the set is recorded as
  unitless rather than refused: losing the measurement over its annotation is
  the wrong way round, and an unrecognised unit is worse than none because a
  backend converts against it.
- **Metric attributes are classified exactly as log fields are** ([platform#34](https://github.com/mosaic-media/platform/blob/main/docs/adr/0034-redaction-classes-are-the-pii-boundary.md)),
  and the property matters *more* here rather than less: a log record ages out
  under retention, while a value used as a dimension is a permanent label on a
  running counter.
- **Cardinality is bounded per scope, for the life of the process, and an
  over-cap series is folded rather than dropped.** This is the metric-shaped
  version of [sdk#5](0005-modules-observe-through-the-sdk.md)'s record quota and it needed a different lifetime — a
  record quota is per invocation because a chatty module should degrade its own
  call, but a series is created once and outlives the invocation, so a
  per-invocation cap would reset and admit the same unbounded growth. Folding
  keeps every total exact and loses only the breakdown; **dropping would make a
  counter under-report, which is the worst outcome available for a
  measurement** — the number a person reads would be quietly wrong. The fold is
  recorded once per scope, because a module in a loop would otherwise turn the
  diagnostic into the flood the cap prevents.
- **A refused instrument is reported through the channel the module is already
  reading.** The only way to create one wrongly is to name it wrongly, which
  produces no error anywhere and no failing test — it is the exact
  silently-discarding counter [sdk#5](0005-modules-observe-through-the-sdk.md) refused to ship, arrived at by a typo.
- **The harness carries both calls.** Three modules run out of process
  ([sdk#7](0007-go-plugin-as-the-extension-harness.md)), so a bridge that
  accepted a `Count` and dropped it would rebuild the discarding counter for
  exactly those three, in the configuration nobody exercises locally.
- **The destination is a reader, not a sink, because a metric is state rather
  than an event.** A record and a span are produced, written once and aged out;
  a counter has no moment of production and nothing to age. So the Platform
  holds a `ManualReader`: no background goroutine, no export interval, no queue
  that can drop, and a snapshot that is the values as of the question rather
  than as of the last flush.
- **The screen lands with the surface.** A capability with no client path is
  [owed rather than done](https://github.com/mosaic-media/architecture/blob/main/docs/unreachable-capability.md), and a metric nobody can
  look at is the thing this record exists to avoid. It is composed from SDUI
  that already exists, so it cost no client release and no new definition.

## Alternatives

**Wait, and add metrics when there is a store to retain them in.** *Rejected*,
and it is the closest call here. It would have produced a better feature later:
history, rollups, a time axis. But the gap being closed is a contributor
reaching for a counter and finding none, and "none until the schema exists" does
not close it — while "in memory, readable now, resets on restart" does, with the
limitation printed on the screen. The retained series remains available to build
and is not prejudged by this.

**Publish a metric surface that discards until a backend exists.** *Rejected,
and it is what [sdk#5](0005-modules-observe-through-the-sdk.md) refused* — correctly. It is worth naming because the
out-of-process bridge is where it would have been rebuilt by accident rather
than chosen.

**Let modules use the OpenTelemetry metric API directly**, since it is stable
and they will often already have it. *Rejected*, for the reasons [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) gave
for the surface as a whole and one specific to metrics: it hands a module the
global `MeterProvider`, where a view, a reader or an exporter is configured —
the ownership [sdk#5](0005-modules-observe-through-the-sdk.md) placed with the Platform. It also loses classification at
construction, which on a dimension is worse than on a record.

**A free-form unit string.** *Rejected.* It is one field and it is exactly the
field that decides whether two modules' data can sit on one axis. Mosaic's
open-vs-closed test ([platform#11](https://github.com/mosaic-media/platform/blob/main/docs/adr/0011-open-and-closed-vocabularies.md)) asks whether Platform code branches on it; this
one is branched on in three places already.

**Bound cardinality with OpenTelemetry's own aggregation limit.** *Rejected for
now.* The Go SDK's `AggregationLimit` is behind an experimental flag, and its
overflow is silent — it folds into an attribute and tells nobody. The bound here
is a dozen lines, is tested, and reports the fold to the module that caused it,
which is the part that makes it a diagnosable failure rather than a mystery.

**Add a gauge.** *Rejected*, and it may not stay rejected. OpenTelemetry's
gauge is an observable read by callback; a module is invoked and returns, so it
has nowhere to put one. A synchronous gauge exists in the API and measures
"current value of something", which is a shape no module has yet asked for.

## Consequences

- **The SDK's OpenTelemetry allowlist goes from three modules to four.** The
  rule is unchanged in kind — the API and never the SDK — and the metric API is
  the *stable* half of OpenTelemetry, unlike the logs API [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md) had to take
  at `v0.21.0`.
- **`v1.Telemetry` gained two methods, which breaks every implementer.** Pre-1.0
  minor, sanctioned by [supervisor#12](https://github.com/mosaic-media/supervisor/blob/main/docs/adr/0012-upgrade-automation-is-staged-against-the-contract-version.md);
  in practice it broke the Platform's adapter, the harness bridge and three test
  fakes, all fixed in the same change. A third party's own fake breaks on the
  bump, which is what a pre-1.0 minor means.
- **Metrics do not survive a restart, and nothing warns except the screen.** The
  honest failure mode is a reader concluding a module has done nothing when the
  process was recycled, which is why the screen says so in its lead rather than
  in a doc comment.
- **Nothing exports to a collector yet.** The Platform holds a reader rather
  than an OTLP exporter, so the values are readable in Mosaic and nowhere else.
  [sdk#8](0008-opentelemetry-is-the-telemetry-implementation.md)'s benefit — everything Mosaic emits being readable by tooling nobody
  had to write — is available here for the cost of an exporter and is not taken
  in this change.
- **A module still cannot be *made* to instrument.** `log.Printf` remains a
  convention rather than a gate, because a boundary test cannot reach another
  repository ([sdk#5](0005-modules-observe-through-the-sdk.md)'s own stated weakness). Metrics inherit that: the surface
  is now there to reach for, and reaching for it is still voluntary.
