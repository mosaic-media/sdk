# Modules as typed capability providers

**Status:** Accepted
**Date:** 2026-07-20

## Context

The module contract to date is a single write-only verb. A `Capability`
declares a `Manifest` and implements `Import(ctx, ContentService, ImportRequest)`
([platform#15](https://github.com/mosaic-media/platform/blob/main/docs/adr/0015-module-capability-and-invocation.md), [platform#17](https://github.com/mosaic-media/platform/blob/main/docs/adr/0017-module-settings.md)):
the Platform hands the module a caller and a query string, and the module
sources content and writes it into the object graph. That was the right shape
to prove invocation, but it casts a module as a one-shot importer and nothing
else.

Building the Stremio module against real addons showed the shape is too narrow
for what a source actually offers. A Stremio addon is not an importer; it is a
set of **resources** — `meta`, `catalog`, `catalog/…/search`, `stream` — that a
client reads. The current contract collapses all of that into `Import(query)`,
which forces the caller to already hold a raw provider id (`movie/tt1254207`).
There is no way for the Platform to *search* a source, *browse* what it offers,
or *enrich* an existing node — the three things a user or an admin actually
does before anything is imported.

The requirement is broader than "add search." When the Stremio module is
enabled, the Platform should gain metadata, search, collections and streams as
**capabilities of the Platform**, and other modules should be able to use them —
a future scanner module enriching a local file's metadata through whatever
metadata provider is installed, without knowing Stremio exists. A module should
contribute typed verbs the Platform inherits, not just push content once.

Two constraints from [platform#15](https://github.com/mosaic-media/platform/blob/main/docs/adr/0015-module-capability-and-invocation.md) still
bound the design: a module imports **only** the SDK ([sdk#1](0001-sdk-as-public-contract-language.md)),
so it contributes no transport of its own; and a capability originates no
authority ([platform#13](https://github.com/mosaic-media/platform/blob/main/docs/adr/0013-how-a-capability-acts.md)), acting as the caller it
is handed.

## Decision

**A module declares the provider roles it fills, and the Platform holds a
registry of providers keyed by role. The single `Import` verb is joined by
read-side provider interfaces the Platform and — by design — other modules
resolve and call.**

The SDK (`v0.4.0`) gains a provider role per Stremio resource kind, each a
small interface a module implements only if it fills that role:

- **`MetadataProvider`** — resolve full metadata for a provider-native id
  (the `meta` resource). Backs enrichment and materialization.
- **`SearchProvider`** — given free text, return candidate results (the
  `catalog/…/search` resource). Backs user search with no raw id.
- **`CatalogProvider`** — enumerate the source's collections and their items
  (the `catalog` resource). Backs the admin collection browser.
- **`StreamProvider`** — resolve watchable locations for a materialized item's
  bound id (the `stream` resource).

The read verbs return the **virtual result types** [platform#18](https://github.com/mosaic-media/platform/blob/main/docs/adr/0018-virtual-and-materialized-content.md)
defines (`SearchResult`, `CatalogItem`) — transient projections, not object-graph
nodes. Only materialization writes nodes, and it keeps the `Import` shape,
narrowed: it takes a virtual item reference (provider, native id, media type)
rather than a free-form query, so "import" becomes "materialize *this* result"
rather than "parse an id from a string."

The `Manifest` grows the declaration [platform#15](https://github.com/mosaic-media/platform/blob/main/docs/adr/0015-module-capability-and-invocation.md)
predicted: a `Provides []Role` naming which roles the module fills, mirroring an
addon's `resources`. It is a self-declaration the Platform trusts; a role a
module names but does not implement is a composition-time error, not a runtime
surprise.

The Platform gains a **provider registry keyed by role**. It resolves providers
to serve each surface (search fan-out, catalog browse, enrich, stream resolve),
and the registry is the seam through which one module reaches another's
providers. Per the decision to design-for-cross-module-use but build
platform-first, the registry's shape supports module-to-module resolution, but
only the Platform resolves through it in this slice; a module receives the
registry (or a scoped view of it) on invocation the same way it receives its
`ContentService` and `Settings` today. Precedence when several modules fill one
role, and the authority a cross-module call carries, are named open questions
below, not settled here.

Every provider call still runs as the invoking caller ([platform#13](https://github.com/mosaic-media/platform/blob/main/docs/adr/0013-how-a-capability-acts.md)),
and background enrichment with no user in the loop needs the system principal
that ADR reserved — the same gap module-declared jobs reach.

## Alternatives considered

**Keep `Import(query)` and add a sibling `Search(query)` only.** *Rejected:* it
solves the raw-id complaint but not the shape problem. Catalogs, enrichment and
stream resolution are also resources; bolting on one verb at a time re-opens the
interface repeatedly. Roles name the whole surface once.

**One fat `Provider` interface with every method, and let a module stub the
ones it does not fill.** *Rejected:* a meta-only addon would have to implement a
`Stream` method that returns "unsupported," and the Platform could not tell from
the type which resources exist. Separate interfaces plus a `Provides`
declaration make the capability set inspectable and honest.

**A module contributes its verbs as GraphQL directly.** *Rejected*, unchanged
from [platform#15](https://github.com/mosaic-media/platform/blob/main/docs/adr/0015-module-capability-and-invocation.md): a module cannot import
Platform transport without breaking the SDK-only boundary. The Platform owns the
transports and projects the providers.

**Build module-to-module resolution now, with precedence and cross-module
authority.** *Deferred:* there is one module. Designing the registry so
cross-module use is possible costs nothing; building and testing precedence,
conflict resolution and a distinct module authority before a second consumer
exists would be inventing a contract against an imagined caller. It lands when a
second module needs the first.

## Consequences

The extension story gains its read half. A module is no longer a one-shot
importer but a set of typed sources the Platform inherits on enable — the
Platform can search, browse and enrich through it, and materialization becomes
one deliberate action among several rather than the only thing a module does.

The `Manifest`'s `Provides` is the first real growth of the manifest shape, and
it is what the `media_types` registry ([platform#11](https://github.com/mosaic-media/platform/blob/main/docs/adr/0011-open-and-closed-vocabularies.md))
has been waiting on: a module that declares the roles it fills is a short step
from declaring the media types it sources.

Three honest limits:

1. **Provider precedence is unspecified.** With one module per role the Platform
   takes the only provider; the rule for many is deferred with cross-module use.
2. **Cross-module authority is unspecified.** A module calling another's provider
   raises whose authority the call carries — the invoking user's, or a module
   principal distinct from it ([platform#13](https://github.com/mosaic-media/platform/blob/main/docs/adr/0013-how-a-capability-acts.md)). Named,
   not answered.
3. **Enrichment with no user needs the system principal.** Background metadata
   refresh has no session caller to forward; it converges on the same reserved
   principal as module-declared jobs.

## Implementation implications

The SDK change is a `v0.4.0`: new provider interfaces, the `SearchResult` /
`CatalogItem` types (defined by [platform#18](https://github.com/mosaic-media/platform/blob/main/docs/adr/0018-virtual-and-materialized-content.md)),
a `Provides` field on `Manifest`, and the narrowing of the import verb to a
virtual-item reference. The Stremio module implements all four roles: it already
has `meta` and `stream`; it gains `catalog` and `catalog/…/search`. The Platform
gains the provider registry and the surfaces that consume it, recorded in
[platform#18](https://github.com/mosaic-media/platform/blob/main/docs/adr/0018-virtual-and-materialized-content.md). The boundary test that
pins the module to SDK-only imports is unchanged and still governs: if a
provider needs a private Platform import, the contract is not ready to publish.
