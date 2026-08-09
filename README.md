# Mosaic SDK

The public contract language between the Mosaic Platform and the Modules that
extend it ([ADR 0008](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0008-sdk-as-public-contract-language.md)).
A Module compiles against this module and nothing else of the Platform's.

It is deliberately small. Today it carries the **content surface**
(`contracts/platform/v1`): the object-graph models (`Node`, `Part`,
`Relation`, `SourceBinding` and their vocabularies), the content command,
query and result types, the `ContentService` interface a capability calls, the
**provider roles** a module declares in `Manifest.Provides` — source roles that
populate the virtual plane (metadata, search, catalog, stream, subtitles) and
the consumer role that acts on the materialised library (playback) — and
the opaque `Caller` a capability forwards from its invocation context
([ADR 0016](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0016-published-contract-surface.md),
[ADR 0017](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0017-how-a-capability-acts.md)).

It holds **no** storage contracts, no transaction type, no identity or
configuration models, and no Platform implementation — a capability calls
application services, never stores. It depends only on the Go standard
library.

```go
import v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
```

## Hand-written Go, not generated

Mosaic has two published contract repositories built in opposite ways, and it
is worth stating which is which. **This one is hand-written Go**; there are no
`.proto` files, no codegen and no build step. [`contracts`](https://github.com/mosaic-media/contracts)
is the protobuf one, with Go and TypeScript generated from `proto/`
([ADR 0044](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0044-contracts-protobuf-workspace.md),
which is scoped to the SDUI and session contracts).

The split follows what each contract *is*. This SDK is Go interfaces with
behaviour — `Capability`, `ContentService`, the provider roles, `Telemetry` —
which a module implements in its own process, and which protobuf cannot
express. `sdui` is a wire format consumed by several client languages, where
codegen is exactly right.

## Build and test

**Everything runs in a container; nothing is built or tested on the host.**

```bash
docker compose -f docker-compose.test.yml run --rm test
```

That runs gofmt, `go build`, `go vet` and `go test` against a pinned toolchain.
There is no hidden dependency and `go build ./...` really is the whole thing —
the container is here because **this is the surface a third party compiles
against**, and the only claim worth making about it is that it builds under a
pinned toolchain rather than under whatever a given machine happens to have.

## Status

Extracted from `platform` into a standalone module and published. The Platform
and modules build against it as an ordinary tagged dependency, with no
`replace`.

**`v0.29.0` — counters and histograms**
([ADR 0130](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0130-the-module-metric-surface.md)).
`v1.Telemetry` gains `Count(name, delta, attrs…)` and
`Measure(name, value, unit, attrs…)`, on the same ambient handle as the log and
span calls — there is no instrument to construct, hold or thread anywhere.

[ADR 0059](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0059-modules-observe-through-the-sdk.md)
deliberately published neither, on the grounds that a counter which silently
discards is worse than no counter at all, and said they would join the surface
when the Platform could back them. It now can.

**One rule this asks of you that logging does not: metric attributes must be
low-cardinality.** Every distinct combination of values is a series that lives
as long as the process, where a log record ages out. An addon id or a status
code is a dimension; a title, a search term or a URL is not, whatever its
redaction class — a digest has exactly as many distinct values as the thing it
digests. The Platform caps how many series a module may create and records that
it did.

Classification is unchanged and applies to metric attributes exactly as it does
to log fields. Adding `go.opentelemetry.io/otel/metric` takes the API allowlist
from three modules to four.

**`v0.28.0` — OpenTelemetry behind the telemetry surface**
([ADR 0128](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0128-opentelemetry-is-the-telemetry-implementation.md)).
**The surface you write against did not change.** `v1.TelemetryFrom(ctx)`,
`v1.Telemetry`, `v1.Span` and the classified `v1.Field` constructors keep their
shapes and their semantics, so a module needs no change beyond this bump. What
changed is what carries the record: an OpenTelemetry `log.Record` and an
OpenTelemetry span, so everything Mosaic emits is readable by any collector,
backend or dashboard that speaks OTLP.

**This is the SDK's first dependency**, and the rule that said "none" is now
"the OpenTelemetry API modules and nothing else" — `go.opentelemetry.io/otel`,
`.../otel/log` and `.../otel/trace`, three modules, enforced by a test in
`sdk/host`. Not the OTel SDK, not an exporter, not a collector client: those are
what a *binary* wires, and a module that could reach them could configure the
Platform's observability plane from inside it.

`v1.NewTelemetry` and `v1.Encoder` are new and are for a *host* to call — the
Platform at its invocation seam, or the out-of-process extension host. A module
never builds a Telemetry; it receives one. `Encoder` is where classification
becomes an OTel attribute, and it is exported so the two hosts apply one rule
rather than two copies of it.

**One asymmetry to know about.** `Identifier` is the only class that carries its
value across the boundary, because only the Platform holds the install salt — so
an `Encoder` with no `Digest` **drops** those fields rather than emitting them
raw. An unconfigured encoder is a safe one.

Note that `go.opentelemetry.io/otel/log` is `v0.21.0` and says in its own
package documentation that its interfaces may gain methods without a major
bump. The tracing half is `v1.45.0` and carries that project's compatibility
guarantee. This SDK wraps rather than re-exports, so most of what could change
there is absorbed here.

**`v0.27.0` — the two fields that let a consumer avoid transcoding.**
`StreamLink` gains `Width`, `Height` and `HDRFormat`, named and typed as
`Part`'s so a consumer moves them across rather than translating.

They complete the ranking inputs `v0.26.0` started. A Platform chooses between
candidates before it fetches anything, and the two outcomes worth avoiding are
sending a client more pixels than it can display and tone-mapping a stream it
cannot render — the second being the most expensive thing a Platform does with
a release. Neither was rankable: `Quality` is a display label rather than a
number, and nothing carried dynamic range at all, so a source offering an HDR
and an SDR cut of one title could not be told to prefer the one that plays
untouched.

The dimensions are recovered rather than invented — **both stream providers
already derived them from the resolution label and discarded them at this
boundary**, because `Quality` was the only place a resolution could go.
`Quality` keeps its meaning as the label a source displayed. `HDRFormat` is a
new parse in each module; release text names it far less reliably than it names
resolution, which is why empty means "SDR or not stated" and both rank the same.

`v0.1.0` carried the content surface; `v0.2.0` added the `Capability`
interface; `v0.3.0` added the `ImportRequest` that hands a module its settings;
`v0.4.0` added the source provider roles and the virtual content DTOs;
`v0.5.0` grew `ContentMetadata` into the rich detail surface; `v0.7.0` added
the subtitles role and richer `StreamLink`, and `v0.8.0` the settings-UI role.

**`v0.9.0` opens the consumer side** — `RolePlayback` and `PlaybackProvider`
([ADR 0045](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0045-playback-consumer-and-media-origin.md)),
the first role that *acts on* the materialised library rather than sourcing
into it. It is one method: a provider resolves a `Part` to a playable location
and never serves bytes, because the Platform owns transports. The `Served`
variant (a module producing bytes for the Platform to serve) and its `Open`
method arrive with the torrent engine that needs them.

**`v0.10.0`** adds `ListContentParts` — the read side of `AttachContentPart`,
missing since the content model landed. A capability could write a Part and
never read one back, so it could not see what it had itself created. A
re-import needing to know which releases were already stored is what finally
forced it.

**`v0.21.0` turns artwork into a candidate set and adds the eighth role** —
`ArtworkCandidate`, `ArtworkSlot`, `Artwork.Candidates`, `RoleArtwork`,
`ArtworkProvider` and `ExternalIdentity`
([ADR 0074](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0074-artwork-is-a-candidate-set.md),
[ADR 0075](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0075-the-artwork-provider-role.md)).

`Artwork` held one URL per slot, so a source that returns forty posters — which
is what a dedicated artwork database *is* — had thirty-nine discarded, and a
user could never be offered a choice because the alternatives never left the
module. The four flat slots stay and keep their type; what changes is their
**meaning**, from "the artwork the provider gave" to "the artwork that was
*chosen*", with `Candidates` holding what it was chosen from. That is what keeps
the change additive: every consumer still reads `Poster`, and there is still
exactly one answer to "what is this node's poster".

A candidate carries `Source`, `Language` and `Rank` because those are what a
choice gets made on. Two of them are easy to misread and are documented at
length: an **empty `Language` means textless**, which is a positive property and
usually the right backdrop to sit under a clearlogo; and **`Rank` is not
comparable across sources**, since one source's likes and another's vote average
measure different things — it orders within a source and nowhere else.

`RoleArtwork` is neither a source role nor a consumer role: it enriches content
that already exists. Its doc comment says plainly that it does **not** satisfy
[ADR 0035](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0035-metadata-as-required-capability.md)'s
metadata requirement, because the tempting shortcut — declaring `RoleMetadata`
to reach `ContentMetadata`'s image fields — would pass the composition-root
check with a module that cannot name a film.

**`v0.20.0` adds `StreamRequest.Season` and `StreamRequest.Episode`** — neutral
coordinates for an episode, so a stream provider can be asked about content it
did not source ([ADR 0073](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0073-stream-resolution-is-decoupled-from-metadata-provenance.md)).

The Ref in that case carries a *shared* external identity (`imdb`, `tvdb`)
rather than the provider's own id, and the provider composes its native
addressing from the identity plus these two numbers. That is the point: one real
source addresses an episode as `tt0903747:1:2`, and a Platform that built that
string would have a provider's dialect in its kernel. Season and episode are
facts about television the Platform already models; the format is the module's
business.

`StreamProvider`'s doc says plainly what follows — a provider asked about
content it does not recognise returns an empty response and **no error**, since
failing there would fail a user's import over a title some other source
described.

**`v0.19.0` adds `SearchContentQuery.AttributesContain`** — a containment filter
over a node's module-owned `Attributes` document, so a capability can ask
"which of my works did some module tag *this* way".

Containment rather than a typed filter, because attributes are opaque to the
Platform by design (ADR 0013 assigns their correctness to the writing
capability). A typed filter would make the Platform learn what a module put
there, which is the coupling the arrangement exists to avoid; *does this
document contain this shape* is the one question that can be asked without
understanding the answer. It is the counterpart of `FindContentByExternalID`,
which has always worked this way over the neighbouring external-ids document.

It is a **storage-engine obligation**: any `StorageAdapter` must support
containment over both JSON documents. And a module can see what another module
wrote — deliberate, matching the rest of the read surface, but it means a
module's attribute keys are a published shape rather than a private one.

**`v0.18.0` adds `ContentMetadata.Watch`** — where a title can be watched
*outside* Mosaic, in one region, with `WatchAvailability`, `WatchOffer` and
`WatchOfferType` as new types.

It is the first read field that does not describe the title itself, and the
doc comments lean hard on one distinction because it is the easy thing to get
wrong: **an offer is not a source.** Every other read role answers "what can
Mosaic get you"; this answers "where else does this exist". Nothing in it
becomes a `Part`, nothing in it is playable through the Platform, and a client
that renders an offer as a play control is making a promise the Platform cannot
keep. `Link` is where an informational control should send the viewer.

Two smaller shapes follow from what the data is. It is **strictly regional**, so
a value names the one `Region` it describes and empty offers mean "none known
*here*" rather than "unavailable". And it carries an `Attribution` string,
because the upstreams that compile availability data generally require being
credited wherever it is shown — carrying it in the contract is what keeps the
Platform from having to know which upstream imposed that.

**`v0.17.0` grows `ContentMetadata` for a source that models more than one
title at a time** — `Keywords`, `Certification`, `Similar []RelatedItem`,
`Collection *Collection` and `Trailers []Trailer`, with `RelatedItem`,
`Collection` and `Trailer` as new types.

Three of those close gaps
[ADR 0034](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0034-rich-metadata-preview.md)
recorded rather than invented: a franchise ("the other Avatar films"), related
titles, and the finer-grained tags that make "more like this" mean anything. The
addon protocol carries none of them, so the fields waited for a source that does.

Two shape decisions are worth stating. **`Collection` is a descriptive
projection, not the object graph's collection** — the graph expresses membership
as a `RelationCollectionMember` edge between Works, and an edge needs two Works
to exist, which a virtual item is not; a detail screen must render a franchise
before anything has been materialised. And **`Trailer` carries a site and that
site's key rather than a URL**, because assembling a watch or embed URL is an
embed-policy decision that belongs to the client rather than to the contract.

`Certification` is display-only text for the same reason `Runtime` is: age
scales are national and not comparable, so the label is carried rather than
mapped onto an invented common scale. Empty means unknown, which a consumer must
not read as "suitable for everyone".

**`v0.16.0` adds `Artwork.Landscape`** — wide 16:9 key art, distinct from
`Backdrop`: a backdrop is scenery to sit *behind* a hero, this is a composed card
image to sit *in* one, which is what a resume rail wants. Added because a real
source supplies it and it was being dropped — Cinemeta has no such field, but an
addon proxying an artwork database returns one alongside the poster. Empty rather
than substituted when a source has none.

**`v0.15.0` stores artwork on the node** — an `Artwork` value (poster,
backdrop, logo) on `Node` and on `AddContentWorkCommand` / `AddContentChildCommand`
([ADR 0071](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0071-content-artwork-is-stored-on-the-node.md)).
Descriptive metadata is otherwise re-derived live from the provider (ADR 0034);
artwork alone is written at materialisation and read back, because it is
rendered in bulk on list surfaces like the continue-watching rail — one node
read instead of a metadata round-trip per card — and because it is the one piece
of art a user may later want to override, which is possible only for something
the library owns.

**`v0.14.0` opens the per-user tier** — playback state
([ADR 0046](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0046-playback-state-is-platform-owned.md)):
`RecordPlaybackProgress`, `SetPlaybackFinished`, and the reads behind resume, a
watched mark and a continue-watching list. Every other method on this surface
operates on an install-global graph; a position belongs to a person, and this is
the first thing here that differs between two users of one Mosaic. It is keyed
by node rather than Part on purpose — a viewer resumes an episode, not the
1080p release of an episode — and it lives on the Platform rather than in a
playback module so it survives swapping one, and so anything that has no
business resolving bytes can still read progress.

**`v0.13.0` gives modules a voice** — `Telemetry`, reached with
`TelemetryFrom(ctx)`, and the redaction-classed `Field` that crosses with it
([ADR 0059](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0059-modules-observe-through-the-sdk.md)).
Before it, a module could return an error or print to the Platform's stdout,
and neither could be filtered, correlated or classified. The Platform owns the
observability plane and implements the interface; a module emits and configures
nothing. `TelemetryFrom` never returns nil, so a module records nothing and
works unchanged on a Platform that provides none.

Pre-1.0 on purpose: the surface still changes as modules find its gaps.

## License

Apache License, Version 2.0 (see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE)).
The SDK is deliberately permissive: it is the contract a Module builds against,
so a Module author may use it under any license. This is independent of the
Platform's license (AGPL-3.0 with a Module Linking Exception).

**`host/v0.1.0` — the module runs as its own process, and the contract does
not change.** `sdk/host` is a **nested module** with its own `go.mod`
([ADR 0064](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0064-extension-module-boundary.md),
[ADR 0077](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0077-go-plugin-as-the-extension-harness.md)).
A module gains a `main.go` of `func main() { host.Serve(mymodule.New()) }` and
otherwise keeps the code it has.

**Nothing in `sdk` itself moved, and that is the point.** The harness needs
`hashicorp/go-plugin`, gRPC and the generated wire bindings; this module's
`go.mod` is still a module line and a Go version. That is the property
[ADR 0059](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0059-modules-observe-through-the-sdk.md)
established when it declared a telemetry interface rather than re-exporting
OpenTelemetry, and `host` carries the test that asserts it — a `go get` in the
wrong directory is all it would take, and nothing else would fail.

Import only `sdk` to write and unit-test a module with no transport at all.
Add `sdk/host` when you want it to run as a process. The nested module is
tagged `host/vX.Y.Z` alongside the parent's `vX.Y.Z`.

Two things the boundary deliberately does **not** change. A module still
cannot classify an error: the Platform's categories are its own internal
vocabulary and are not published here, so in process and out of process alike a
module receives an error it can read but not branch on. And `Caller` is still
something a module forwards rather than inspects — across the boundary it
becomes a handle the Platform mints per invocation and revokes on return, which
module code never has to know.

**`host/v0.2.0` — the module's egress is pre-wired to the Platform's proxy.**
`host.Serve` now routes the process's default HTTP transport through the forward
proxy the Platform names in `MOSAIC_EGRESS_PROXY`
([ADR 0064](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0064-extension-module-boundary.md)),
so an out-of-process module's outbound calls carry the same deny list the
in-process client had — a module cannot reach the host's own PostgreSQL, its
LAN, or the cloud metadata endpoint through a URL a user supplied.

This is why the record can call the client "pre-wired" rather than merely
env-configured. The standard `HTTP_PROXY`/`HTTPS_PROXY` variables are not
sufficient on their own, and the gap is the target that matters most: Go's
`ProxyFromEnvironment` hardcodes a bypass for `localhost` and loopback, so a
module using an ordinary client would reach `127.0.0.1` *around* the proxy.
Forcing the transport's `Proxy` to always return the proxy URL has no such
exception. A module that builds a fully custom transport with an explicit nil
`Proxy` can still bypass it; that residual gap is what OS-level network denial
(ADR 0064's layer 3) closes.

**`host/v0.3.0` — a module can print its own manifest.** Run a module binary
with `--mosaic-manifest` and it prints its `Manifest()` — id, version, name,
roles — as JSON and exits, rather than serving. A module's release uses this to
learn the module's identity for the distribution manifest without hardcoding it
in a workflow, where it would drift from the code: a role added in Go appears in
the published manifest with no second edit. It is the one thing `Serve` prints
to stdout deliberately, and only in a mode that never serves, so there is no
go-plugin handshake to corrupt.

**`host/v0.5.0` — the harness compiles against what the Platform ships.** Its
requires named `contracts v0.16.0` and `sdk v0.22.0` while the Platform shipped
`contracts v0.53.0` and `sdk v0.24.0`. Nothing failed and nothing was going to:
minimal version selection raises both in any build that also includes the
Platform, so the binary that runs was never the one this repository's gate
compiled. A gate that green-lights a version nobody runs is the failure — the
requires now name what ships, and the two are the same statement again.

No source file changed, which is the useful part of the result: twenty-one
minor versions of `contracts` and two of `sdk` were additive to everything the
harness touches, and the wire package it imports
(`gen/mosaic/module/v1`) is byte-identical across the whole range.

**`host/v0.6.0` — three fields the SDK had and the wire could not carry.**
`ContentMetadata.Crew` and `EpisodePreview.RuntimeMinutes` (`v0.24.0`) and
`CatalogItemsResponse.HasMore` (`v0.23.0`) were added to the SDK against a
`module.proto` with no fields to hold them, so the converters here dropped all
three: an out-of-process module could set them and the Platform received a
zero. Nothing failed, because a dropped field is indistinguishable from a
source that did not supply one — and the one module that populates them,
`module-tmdb`, is compiled into the Platform binary and never crosses this
boundary, so the gap was invisible from both ends.

`contracts v0.54.0` adds the three wire fields and this release maps them in
each direction. `TestLaterSDKFieldsCrossTheBoundary` asserts them on the far
side of a real gRPC round trip rather than on the converters, so the thing it
proves is that they *arrive*; `TestHasMoreDefaultsToLastPage` pins the
compatibility claim that a provider which never sets `HasMore` still reads as
the last page. **A field added to the SDK's virtual-content DTOs needs a
`module.proto` field and a line in each direction of the converter** — those
two tests are where forgetting shows up.

**`v0.25.0` — a catalog can be narrowed, and a node carries its genres.** The
two halves of M2's faceting slice, and they are independent surfaces that
happen to share a release.

*Provider side.* `CatalogItemsRequest` carried only `Skip`, so browsing a source
by genre was not expressible at all. `Catalog.Filters` now declares the
narrowings a catalog accepts — `CatalogFilter`, each with `CatalogFilterOption`
values — and `CatalogItemsRequest.Filters` carries the selection back, keyed by
the filter's source-native name.

**The options are declared rather than free text, and that is the design rather
than a convenience.** A consumer builds its control from the source's own list,
so a value it sends back is one the source named — the discipline that stops a
catalog id being mistyped into a rule that silently matches nothing. Value and
label are separate fields because sources address a genre by numeric id and name
it in words, and carrying one of them would force either an unreadable control
or a reverse lookup per request. A provider must **decline** a name or value it
does not recognise rather than returning the unfiltered page: quietly widening a
query answers a question nobody asked, and the answer looks right.

`CatalogItemsResponse.HasMore`'s doc now names the **two grades of true** a
provider can make. An upstream reporting a total supports the exact statement;
an upstream paging by offset with no total supports only "this page came back
full", which is the inference the field exists to keep out of the *Platform* and
which is nonetheless the provider's to make, because only the provider knows its
page size. Its cost is bounded and visible — a final page that happens to be
exactly full asks for one more that comes back empty. A provider with neither a
total nor a known page size still has no basis for either and leaves it false.

*Library side.* `Node.Genres`, `AddContentWorkCommand.Genres` and
`SearchContentQuery.Genres`. Genres are stored on the node rather than in the
untyped `Attributes` document for
[ADR 0071](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0071-content-artwork-is-stored-on-the-node.md)'s
reason — universal display data, not per-media-type variation — plus one artwork
does not have: a genre is **filtered** in bulk rather than rendered in bulk, and
a facet whose source is a cache omits every title the cache has not reached,
which is an omission from a filtered list and therefore invisible.

Genres are stored **as the source named them**. Two sources disagree about the
words — one says "Sci-Fi" where another says "Science Fiction" — and nothing
here reconciles them, because a synonym table quietly rewriting a source's
vocabulary would be the Platform inventing a fact. A library fed by two sources
offers both spellings, which is a true statement about the library rather than a
tidy false one. `SearchContentQuery.Genres` is **conjunctive**: two lit chips on
a facet mean "crime *and* comedy", and the union would widen where a user asked
to narrow.

Genres are set on a **Work** and empty beneath it. A season and an episode
belong to their work's genres and do not have their own — the one place this
differs from artwork, which an episode genuinely does have.

**`host/v0.7.0` — the five fields `v0.25.0` added, mapped in each direction.**
`contracts v0.57.0` carries the wire fields; this maps them.
`TestLaterSDKFieldsCrossTheBoundary` grows to cover `Catalog.Filters` on the way
back and `CatalogItemsRequest.Filters` on the way out — **the first field on
that request a caller sets rather than a provider answers**, so the first a
converter could drop in a direction the existing assertions do not travel in.
`TestGenresCrossTheContentBoundary` does the same for the content service, where
the failure has its worst shape: nothing errors, the import succeeds, the node
is written, and the facet the whole slice exists for silently offers fewer
titles than the library holds.

**`v0.26.0` — a candidate can say what it is, and a subtitle request can name an
episode.** Two gaps in the source roles, batched into one release because they
cost the same round of module bumps either way.

*`StreamLink.Container`, `.VideoCodec`, `.AudioCodec`.* A module parses all
three at its boundary
([ADR 0051](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0051-modules-as-anti-corruption-layers.md))
and had nowhere to put them, so the parse was narrowed to `Quality`,
`SizeBytes` and `Seeders` on the way out of `Streams` and every candidate
arrived saying less than its source had said. The names are `Part`'s, spelled
identically, because that is where a resolved candidate ends up — a consumer
should be moving values across rather than translating them.

They are a **guess, not a measurement**, and the doc comment says so: release
text lies and no parse can see inside a file, so what a release actually
contains is still settled by probing the bytes
([ADR 0050](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0050-probing-and-the-per-stream-playback-decision.md)).
What they buy is a candidate list that is *rankable* before anything has been
fetched. Codecs are spelled the way ffprobe spells them — `hevc`, `h264`,
`eac3` — so a parsed guess and a probed fact are comparable; a container is the
bare lowercased extension.

*`SubtitlesRequest.Season` and `.Episode`.* The same two coordinates
`StreamRequest` gained in `v0.20.0`, for
[ADR 0073](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0073-stream-resolution-is-decoupled-from-metadata-provenance.md)'s
reason. The two requests matched only by accident: streams grew the coordinates
when the enrichment pass began asking providers about content they had not
sourced, and subtitles did not, because nothing consumed subtitles yet. The
consequence was concrete rather than theoretical — a subtitles provider handed a
foreign ref could answer for a film and for nothing else, and both modules
filling the role called their own addressing helper with two zeroes and a
comment explaining why.

Both are additive and both zero values are the old behaviour exactly, so a
provider written before this release keeps working unchanged.

**`host/v0.8.0` — those five fields, mapped in each direction.** `contracts
v0.60.0` carries the wire fields; this maps them.
`TestStreamAndSubtitleFieldsCrossTheBoundary` asserts every one on the far side
of a real gRPC round trip — the three codec fields on the way back from the
provider, the two subtitle coordinates on the way out to it, which can only be
asserted by the module reporting what it was handed. A converter that dropped
one direction while carrying the other passes half of that test and fails the
other half.

**A field added to the SDK needs a `module.proto` field and a line in each
direction of the converter.** This is the third time that has been forgotten,
and `TestLaterSDKFieldsCrossTheBoundary` and this test are the two places it
now shows up.
