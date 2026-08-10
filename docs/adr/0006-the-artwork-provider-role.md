# The artwork provider role

**Status:** Accepted; **built.** `RoleArtwork` and `ArtworkProvider` are in the
SDK, `CapabilityRegistry.ArtworkProviders()` resolves them, and
`enrich_artwork.go` runs the pass over a materialised work and its seasons —
asking every provider, unioning the results rather than stopping at the first,
and swallowing every failure so an artwork source being down cannot lose a user
the work they just added. `module-fanart-tv` fills the role and declares no
metadata role; a boundary test in that repository fails if it ever does, which
is the half of this record that had to be enforced rather than stated.
`RoleArtwork` does not count toward
[platform#23](https://github.com/mosaic-media/platform/blob/main/docs/adr/0023-metadata-as-required-capability.md)'s requirement, as decided.
**Date:** 2026-07-24

Adds an eighth role to [sdk#2](0002-modules-as-typed-capability-providers.md)'s
vocabulary. Applies [platform#46](https://github.com/mosaic-media/platform/blob/main/docs/adr/0046-stream-resolution-is-decoupled-from-metadata-provenance.md)'s
enrichment pattern to a second kind of provider. Consumes the value
[platform#47](https://github.com/mosaic-media/platform/blob/main/docs/adr/0047-artwork-is-a-candidate-set.md) defines.

## Context

Mosaic's artwork comes from whichever module described the content. Cinemeta has
a poster, a background and sometimes a logo; TMDB has more and discards most of
it; an addon proxying an artwork database has whatever it proxies. That is the
whole supply, and it is a by-product of asking a question about *titles*.

A dedicated artwork database is a different kind of source, and the shape of the
difference is what this record is about. **It answers only one question, and it
cannot answer any of the others.** Given an id it already trusts, fanart.tv
returns posters, backdrops, logos, clearart, banners, disc art and per-season
art, each with a language and a vote count. It has no titles, no overviews, no
years, no cast, no search, and no catalogs. There is no query that turns
"Blade Runner" into a fanart.tv result — you must already know which film you
mean.

None of [sdk#2](0002-modules-as-typed-capability-providers.md)'s seven roles
fit that. `RoleMetadata` is the near miss, and taking it would be a mistake with
a specific, foreseeable failure.

**`RoleMetadata` is load-bearing for boot.**
[platform#23](https://github.com/mosaic-media/platform/blob/main/docs/adr/0023-metadata-as-required-capability.md) made a registered
`RoleMetadata` *and* `RoleSearch` a composition-root requirement — the serving
composition refuses to start without them, because a Mosaic that cannot identify
or find content reads as broken. A module declaring `RoleMetadata` to smuggle
artwork through `ContentMetadata`'s `Poster`/`Backdrop`/`Logo` fields would
satisfy half of that check while being structurally incapable of describing
anything. The failure would not be a compile error or a red test; it would be a
deployment that boots and cannot name a film.

The same argument in the other direction is the one `module-tmdb` already
carries — *"do not let it grow into a source"* — and it generalises: **a role is
a claim about what a module can answer, and a module must not claim one to reach
a field.**

## Decision

**A new `RoleArtwork`, backed by an `ArtworkProvider`, for a source that supplies
artwork and nothing else. The Platform invokes it as a best-effort enrichment
pass over a materialised work, addressing it by shared external identity —
exactly as [platform#46](https://github.com/mosaic-media/platform/blob/main/docs/adr/0046-stream-resolution-is-decoupled-from-metadata-provenance.md)
invokes a stream provider.**

- **The role answers one question.** `Artwork(ctx, ArtworkRequest) →
  ArtworkResponse`, where the response is a list of
  [platform#47](https://github.com/mosaic-media/platform/blob/main/docs/adr/0047-artwork-is-a-candidate-set.md) candidates. It returns no title,
  no overview and no ref — it is not asked to identify anything, only to
  illustrate something already identified.

- **It is enrichment, not import.** An artwork provider never materialises and
  never appears in a search result. It has nothing to search over, so a
  deployment running *only* artwork providers has no content at all, and
  [platform#23](https://github.com/mosaic-media/platform/blob/main/docs/adr/0023-metadata-as-required-capability.md)'s check correctly refuses
  to serve. `RoleArtwork` does not count toward that requirement, and saying so
  is the point of defining it separately.

- **Addressed by shared external identity, never by a native id.** The request
  carries the work's `imdb`/`tvdb`/`tmdb` pairs and, for a season, its number.
  The provider derives its own addressing. This is
  [platform#46](https://github.com/mosaic-media/platform/blob/main/docs/adr/0046-stream-resolution-is-decoupled-from-metadata-provenance.md)'s
  load-bearing half restated: fanart.tv keys television on TVDB ids and films on
  TMDB or IMDb ids, and *which id goes in which path* is the module's business.
  A Platform that knew it would have a provider's dialect in the kernel, which
  [module-stremio-addons#2](https://github.com/mosaic-media/module-stremio-addons/blob/main/docs/adr/0002-modules-as-anti-corruption-layers.md) forbids in as many words.

- **Every provider is asked, and the results union.** Unlike stream enrichment —
  which stops at the first provider that answers, because cross-provider Part
  dedup does not exist — artwork candidates from several providers combine into
  one set. They can, because [platform#47](https://github.com/mosaic-media/platform/blob/main/docs/adr/0047-artwork-is-a-candidate-set.md)
  makes candidates additive and attributable: there is no dedup problem to solve
  when nothing has to be chosen at write time.

- **Best-effort, and it never fails an import.** A provider that is unreachable,
  unconfigured, or simply does not know the title yields nothing and the import
  keeps the art it already had. This is the same rule as stream enrichment and
  for the same reason: a source being down must not lose a user the work they
  just added.

- **It runs on the work and its seasons, not on every episode.** Series-level
  and season-level art is a bounded number of requests per import. Episode
  stills stay with the metadata provider, which already returns them per episode
  in one call — asking an artwork database per episode would be a request per
  episode for data the metadata module already fetched in bulk.

## Alternatives considered

**Declare `RoleMetadata` and return artwork-only `ContentMetadata`.**
*Rejected*, and it is the shortest path by a wide margin — no SDK role, no
registry change, no enrichment pass. It is rejected because it makes a false
claim that the composition root believes: a module that cannot name a film would
count toward the guarantee that Mosaic can name films
([platform#23](https://github.com/mosaic-media/platform/blob/main/docs/adr/0023-metadata-as-required-capability.md)). It would also make every
metadata consumer defend against a `ContentMetadata` with no title, which is a
shape the type does not otherwise have.

**Extend `MetadataProvider` with a second method rather than adding a role.**
*Rejected.* It reads as smaller and is larger: every existing metadata module
would have to implement a method it cannot answer, and `roleImplemented`'s
interface check — which is what makes a declared role a verified one — would
stop distinguishing the two capabilities. A module that fills both roles simply
implements both interfaces, which is what the role model is for.

**Let the metadata module call the artwork module.** *Rejected*, for the reason
[platform#46](https://github.com/mosaic-media/platform/blob/main/docs/adr/0046-stream-resolution-is-decoupled-from-metadata-provenance.md)
rejected it: the SDK gives a capability no way to reach the registry, and adding
one would make modules depend on each other's presence and behaviour. The
registry exists so they compose without knowing about each other.

**Fetch artwork lazily at render time instead of at import.** *Deferred, not
rejected*, and it is the mirror of the same alternative in [platform#46](https://github.com/mosaic-media/platform/blob/main/docs/adr/0046-stream-resolution-is-decoupled-from-metadata-provenance.md). Always-current
art with no storage cost is genuinely attractive, and it is exactly what
[platform#45](https://github.com/mosaic-media/platform/blob/main/docs/adr/0045-content-artwork-is-stored-on-the-node.md) rejected for list
surfaces: a rail re-deriving art per card is a round-trip per card, and from a
materialised node the provider-bearing ref is gone. Import-time enrichment is
what makes the art *readable from a node at all*.

**Make it a browse-time provider too, enriching virtual search results.**
*Not taken here.* A search returning forty results would fan out forty artwork
lookups to make a grid marginally prettier, on the latency path of the thing a
user is waiting for. Curation is what earns the extra calls
([platform#18](https://github.com/mosaic-media/platform/blob/main/docs/adr/0018-virtual-and-materialized-content.md)'s two planes), and browse
art is proxied rather than stored anyway
([platform#20](https://github.com/mosaic-media/platform/blob/main/docs/adr/0020-artwork-proxy-and-cache.md)).

## Consequences

- **A library item can now have art no metadata source offered** — clearlogos on
  titles Cinemeta left blank, clearart and banners nothing has ever supplied
  ([sdk#3](0003-rich-metadata-preview.md)'s recorded gap), and per-season
  posters where a series previously showed one poster four times.
- **Import grows a second fan-out.** It was one module plus every stream
  provider; it is now that plus every artwork provider. The artwork pass is
  bounded by the work and its seasons rather than by episodes, so it is a small
  addition next to the stream pass — but import is measurably not free any more,
  and this is the second record to make it slower.
- **A metadata module that binds only its own scheme is unenrichable, again.**
  [platform#46](https://github.com/mosaic-media/platform/blob/main/docs/adr/0046-stream-resolution-is-decoupled-from-metadata-provenance.md) said the shared external identity had become load-bearing; this makes
  it more so. Concretely: **Cinemeta binds only `imdb`, and fanart.tv keys
  television on TVDB**, so a *series* imported through Cinemeta gets no artwork
  enrichment while the same series imported through TMDB does. That is a real
  and visible inconsistency, it is not fixable in the artwork module, and it is
  recorded rather than papered over.
- **The eighth role is a precedent worth naming.** Roles one to seven were
  either sources of content or consumers of it. This is neither: it enriches
  content that already exists, and it is the shape a subtitles-database or
  ratings-aggregator module would also take. The vocabulary is open
  ([sdk#2](0002-modules-as-typed-capability-providers.md)) and this is the
  first extension of it since it was written.
- **It is the first genuinely optional module.**
  [platform#3](https://github.com/mosaic-media/platform/blob/main/docs/adr/0003-platform-as-execution-kernel.md)'s core tier is for coupling or guarantee,
  and an artwork provider has neither — nothing breaks without it, art simply
  stays as good as the metadata source made it. Even `module-remote-playback` is
  core under the guarantee clause. So this is the first candidate for the
  extension tier, which is not built ([platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md));
  until it is, the module composes statically like the others and the
  classification is a delivery decision waiting for its mechanism.

## Implementation implications

**SDK** (minor `v0.x` bump, same as [platform#47](https://github.com/mosaic-media/platform/blob/main/docs/adr/0047-artwork-is-a-candidate-set.md)'s): `RoleArtwork` in the `Role`
vocabulary; `ArtworkProvider`, `ArtworkRequest` (caller, settings, the external
identities, media type, optional season) and `ArtworkResponse` (candidates) in
`provider.go`.

**Platform**: `RoleArtwork` in `roleImplemented`, an `ArtworkProviders()`
enumerator on `CapabilityRegistry` beside `StreamProviders()`, and
`enrich_artwork.go` modelled closely on `enrich_streams.go` — same identity
read, same best-effort telemetry, differing in that it unions across providers
rather than stopping at the first. The selection rule and candidate cap from
[platform#47](https://github.com/mosaic-media/platform/blob/main/docs/adr/0047-artwork-is-a-candidate-set.md) resolve the slots before the node
is written.

**`module-fanart-tv`**: a new repository per
[platform#2](https://github.com/mosaic-media/platform/blob/main/docs/adr/0002-module-storage-and-delivery-model.md) — inbound, naming the foreign
system. SDK-only imports with the usual boundary test, a hermetic fake over
`httptest`, and the bundled-credential pattern `module-tmdb` established
(`-ldflags -X` with a `linkercheck` gate) since fanart.tv requires a project key.
It fills `RoleArtwork` and `RoleSettingsUI` and must never acquire another.
