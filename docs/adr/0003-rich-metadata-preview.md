# Rich metadata: the descriptive surface grows a preview

**Status:** Accepted. Partly superseded by [platform#45](https://github.com/mosaic-media/platform/blob/main/docs/adr/0045-content-artwork-is-stored-on-the-node.md): artwork (poster, backdrop, logo) is now stored on the node at materialisation rather than re-derived live; the live re-derivation of the textual descriptive surface (cast, overview, rating, episode preview) stands.
**Date:** 2026-07-20

## Context

The detail screen ([platform#19](https://github.com/mosaic-media/platform/blob/main/docs/adr/0019-sdui-emit-side.md)) is thin: a poster, a
title, an overview, genres, and one action. Wiring a real product surface — the
kind a user recognises from Plex or Jellyfin — surfaced how little of what the
source actually provides ever reaches the screen.

The pipeline is a double funnel. Cinemeta (the default Stremio metadata addon)
returns cast, a clearlogo, an IMDB rating, a runtime, and — for a series — a
`videos` array with a per-episode overview, thumbnail and air date. The Stremio
module decodes almost none of it, and even if it did, the published
`v1.ContentMetadata` DTO ([sdk#2](0002-modules-as-typed-capability-providers.md))
has only seven flat fields (`Title, Year, Overview, Poster, Backdrop, Genres`,
plus `Ref`) with nowhere to put them. So the screen is thin because the
*contract* is thin, not because the data is missing.

Two honest limits bound what is worth adding, and both come from the source, not
the contract:

- **Cinemeta has no clearart, no banner, no franchise collection, and no
  reliable "similar".** Those are Fanart.tv / TMDB concepts. A detail page can
  show a backdrop-plus-logo hero (which Cinemeta *does* provide) but not a
  clearart hero, and cannot show "the other Avatar films" or "similar shows"
  from this source. Those wait on a different provider and are **out of scope
  here** (recorded, not built).
- **The DTO was deliberately flat** — "descriptive, not the tree" ([sdk#2](0002-modules-as-typed-capability-providers.md)),
  because a work's materialised children (seasons, episodes as `Node`s) are
  `Import`'s concern. That reasoning holds for the *materialised* tree. It does
  **not** cover the *preview* a detail screen needs for a **virtual** series —
  the episode list a user reads *before* deciding to add it, which is never
  materialised.

## Decision

**Grow `v1.ContentMetadata` into the full descriptive surface the detail screen
reads, and let it carry a read-only *episode preview* for a series. It stays a
projection a read role returns — never a node, never written — but it is no
longer artificially flat.** This is an additive, pre-1.0 `v0.x` change (SDK
`v0.5.0`).

New fields on `ContentMetadata`:

- `Logo string` — the clearlogo/title-treatment image (Cinemeta `logo`). The one
  new artwork type; renders as the hero's title.
- `Cast []Person` — top billed cast, `Person{Name, Role}` (Cinemeta gives names;
  `Role` is reserved and usually empty).
- `Rating float64` — the source rating (Cinemeta `imdbRating`), 0 when unknown.
- `Runtime string` — a display runtime (Cinemeta's `runtime`, whose format
  varies — a string, not a parsed integer, so nothing is lost or invented).
- `Episodes []EpisodePreview` — for a series, a **flat** list of
  `EpisodePreview{Season, Episode, Title, Overview, Thumbnail, Released}`. This
  is the deliberate refinement of [sdk#2](0002-modules-as-typed-capability-providers.md)'s
  "no tree" rule: it is a **preview projection, not the materialised tree**.
  The UI groups it by season for display; the Platform never persists it. When a
  virtual series is materialised, `Import` still builds the `Node` tree from the
  source's own structure exactly as before — this list does not feed
  materialisation, it feeds the *screen*.

**The preview is the single path for both planes.** `PreviewContent` stops
short-circuiting for an in-library ref: it now returns the full metadata *and*
`InLibrary`/`NodeID`. So one ref-based detail builder serves a virtual item and
a library item identically — the only difference is the primary action
(*Add to library* vs an *In library* marker). A library item's rich detail is
therefore **re-derived live** from the provider, not read from stored columns —
which means it is as current as the source and needs no new storage, at the cost
of depending on a reachable metadata addon (the honest trade below).

## Alternatives considered

**Store the rich metadata on the `Node` at import, read it back for library
items.** *Rejected for now.* `Node` has only a `Title` and an untyped
`Attributes []byte`; carrying cast/logo/episode-synopsis through the write
commands and into `Attributes`, then back out through `GetContentNode`, is a
larger change than re-deriving from the provider, and it makes a library item's
detail only as fresh as its last import. Re-deriving live is smaller and always
current. A durable metadata cache is a separate, later concern (it pairs
naturally with the artwork cache, [platform#20](https://github.com/mosaic-media/platform/blob/main/docs/adr/0020-artwork-proxy-and-cache.md)
slice 2). The cost — a library detail needs a reachable metadata addon — is
acceptable for a self-hosted server whose addons are configured, and is noted as
a gap, not hidden.

**A separate `Episodes(ref)` provider method / `EpisodeProvider` role.**
*Rejected as premature.* It is more surface for one consumer; the episode
preview rides on the metadata a detail already fetches, in one call. If a second
consumer needs episodes without the rest of the metadata, promote it then.

**Keep the DTO flat; put episodes only in the materialised tree.** *Rejected:* a
**virtual** series has no materialised tree, so its episodes would be invisible
until added — precisely backwards, since the episode list is part of what a user
reads to *decide* to add it.

**Add clearart / banner / collection / similar now.** *Rejected — no data.*
Cinemeta does not provide them; adding fields with no source behind them would be
contract theatre. They wait on a TMDB/Fanart-class provider and the relation-read
gap (`ListFrom`/`ListTo`, still open), and are recorded there.

## Consequences

- The detail screen becomes rich from data that already existed and was thrown
  away: a backdrop+logo hero, top cast, rating/runtime/genres, and — for a
  series — a season selector over an episode list with per-episode synopses and
  stills. It works for virtual and in-library items through one path.
- **The SDK grows** (`v0.5.0`), the **Stremio module grows** (`v0.3.0` — it now
  decodes `logo`, `imdbRating`, `runtime`, cast from the top-level field and the
  `links` array, and per-`Video` `overview`/`thumbnail`/`released`), and the
  **SDUI vocabulary** gains one binding — a `logo` image on `HeroBanner` (the
  only detail-page piece that had no component; everything else — `PersonChip`,
  `EpisodeRow`, `SeasonSelector`, `Carousel` — already existed).
- **A library detail now depends on a reachable metadata provider**, because it
  re-derives rather than reads stored fields. Documented trade; a durable
  metadata cache is the follow-up that removes it.
- **Still out, by data availability, not decision:** clearart/banner artwork,
  franchise collections, and similar/related — they need a different source and
  the relation-read surface, and remain recorded gaps.

## Implementation implications

SDK: new `Person`/`EpisodePreview` types and the five `ContentMetadata` fields,
tagged `v0.5.0`. Stremio module: decode the dropped Cinemeta fields, map them
(incl. the `videos` → `Episodes` projection), tagged `v0.3.0`. `mosaic-sdui`:
a `Logo` option and a `logo` binding on the `HeroBanner` definition (canonical
JSON + Go binding + React runtime), no schema change (props are open).
Platform: `PreviewContent` always fetches; the emit-side detail builder composes
the hero, cast rail, genres, and the series season/episode list from the grown
metadata, ref-based for both planes. Cross-repo version coordination follows the
usual flow (tag the SDK and module, bump the requires); local `replace`
directives bridge it until the owner tags and publishes.
