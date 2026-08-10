# Module-contributed settings UI

**Status:** Accepted (built)
**Date:** 2026-07-21

## Context

Stremio's `addon_catalog` resource lists *other addons* — it is how a user
discovers and manages the addons a Stremio deployment sources from. It is
meaningless outside Stremio: it is not a Platform content capability, and giving
the Platform an `addon_catalog` provider role would make the Platform encode one
ecosystem's addon model.

But users do need to manage a module's addons, and today a module's settings are
**opaque JSON** ([platform#17](https://github.com/mosaic-media/platform/blob/main/docs/adr/0017-module-settings.md)) with **no UI** — a user
configures Stremio addons by hand-writing `{"addons":[…]}` through a raw
mutation. That is fine for a contract and hostile as a product.

The tension: the configuration UI for a module is inherently *module-specific*
(the Platform cannot know Stremio's addon model, a TMDB module's API-key field,
a debrid module's account flow), yet [sdk#1](0001-sdk-as-public-contract-language.md)/[platform#19](https://github.com/mosaic-media/platform/blob/main/docs/adr/0019-sdui-emit-side.md)
establish that **modules contribute data, not screens** — the Platform owns the
emit-side so a second client renders the same payloads.

## Decision

**A module contributes its own *settings* UI as SDUI, which the Platform hosts in
a module-settings surface. This is a bounded exception to "modules contribute
data, not screens" — scoped to a module's own configuration, never to content
screens.**

- **A new capability role, `SettingsUIProvider` (`RoleSettingsUI`), returns an
  SDUI screen as JSON bytes.** The SDK stays SDUI-agnostic: the role returns
  `[]byte` (a serialised `UINode` tree), not a typed SDUI value, so the SDK does
  not depend on the SDUI contract. The Platform validates the bytes and hands
  them to the client to render in a bounded settings slot.
- **Modules may import the published SDUI contract (`mosaic-sdui`) to build that
  screen.** This is sanctioned, not new licence: `mosaic-sdui` is "the contract
  the Platform, **Modules** and Shell share" ([contracts#3](https://github.com/mosaic-media/contracts/blob/main/docs/adr/0003-sdui-contract-repository.md)),
  so a module producing SDUI with the Go producer binding is intended. The module
  import boundary — today "the SDK and the standard library only" — is **extended
  to also allow the published SDUI contract**, and the boundary test updated to
  match. A module still may not import Platform internals.
- **The "data not screens" rule is refined, not broken.** That rule governs
  *content* screens (search, detail, catalog) — the Platform owns them so the
  interface can evolve without a client deploy and a Flutter client renders the
  same payloads. A module's *own settings* screen is different in kind: only the
  module knows its configuration model, and the screen is hosted in a bounded,
  Platform-owned settings frame (the module fills a slot, it does not own the
  chrome or the navigation). Content stays Platform-emitted; module configuration
  is module-emitted into a Platform host.
- **`addon_catalog` is the first use.** The Stremio module fetches its addon
  catalog and renders a list of addons with enable/configure actions whose
  `Invoke` drives the existing `configureModule` mutation
  ([platform#17](https://github.com/mosaic-media/platform/blob/main/docs/adr/0017-module-settings.md)) — so the settings UI writes settings
  through the same command surface, and the Platform stays the one that persists
  them.

## Alternatives considered

**`addon_catalog` as a Platform capability/provider role.** *Rejected.* It is
Stremio-specific; the Platform would encode one ecosystem's addon model as a
first-class role, which the typed-provider model ([sdk#2](0002-modules-as-typed-capability-providers.md))
exists to avoid.

**The Platform renders module settings from a declared *form schema*.**
*Rejected.* It invents a second UI-description language alongside SDUI, and a
form schema is less expressive than the SDUI vocabulary the project already
shares with modules. SDUI is the richer, existing answer.

**Keep opaque JSON settings with no UI (status quo).** *Rejected.* A user cannot
manage addons without hand-writing JSON through a raw mutation — not a product.

**Let modules contribute content screens too (drop the rule entirely).**
*Rejected.* The "data not screens" rule is right for content — it is what keeps
the interface server-owned and client-agnostic. The exception is deliberately
narrow: a module's own settings, in a bounded host, only.

## Consequences

- **Modules gain a UI surface bounded to their own configuration**, distinct from
  content screens. The Platform hosts it; the module fills a slot.
- **The SDK grows `RoleSettingsUI`** (SDUI-agnostic, `[]byte` return); **the
  module depends on `mosaic-sdui`**; **the boundary is widened** to the published
  SDUI contract and the boundary test updated.
- **The Platform gains its first module-settings UI surface** — a settings screen
  in the emit-side that embeds module-contributed SDUI, plus Shell navigation to
  it. This is net-new surface and the larger part of the work; it is why this is
  a separate slice from the source-surface completion
  ([module-stremio-addons#1](https://github.com/mosaic-media/module-stremio-addons/blob/main/docs/adr/0001-completing-the-stremio-source-surface.md)).
- **Authority is unchanged.** The settings UI's actions run `configureModule` as
  the invoking user ([platform#13](https://github.com/mosaic-media/platform/blob/main/docs/adr/0013-how-a-capability-acts.md)); no system
  principal is needed, because configuration is a user act.
- **A validation burden.** Module-supplied SDUI is data crossing a trust-ish
  boundary; the Platform validates the `UINode` against the schema and confines
  it to the settings slot, so a malformed or oversized payload cannot escape into
  the chrome.

## Implementation implications

SDK: `RoleSettingsUI` + `SettingsUIProvider` returning `[]byte` (`v0.7.0`).
Module (`v0.5.0`): an `addon_catalog` client, a `SettingsUIProvider` that builds
the settings screen with the `mosaic-sdui` producer binding, the new
`mosaic-sdui` require, and the boundary-test widening. Platform: `ModuleSettingsUI`
(resolve the provider, read settings, validate the `UINode`) hosted by a
`settings` screen in the emit-side, a Settings nav item, and `configureModule`
made invoke-compatible (an `input: JSON` envelope, a JSON-scalar return) so the
contributed form's actions drive it. Runtime (`@mosaic-media/sdui-react@0.1.5`):
a `SubmitField` primitive that substitutes a typed value for `$value` in its
action — the input binding the add-by-URL form needs, closing a known gap.

**Built and verified live.** The Stremio settings page renders in the Platform's
settings host: add an addon by manifest URL (the typed URL flows into
`configureModule`), view and remove installed addons, toggle the bundled Cinemeta
default, and browse installable addons — the `addon_catalog` works out of the box
because Cinemeta's manifest serves a community-addons catalog. Add, remove and
browse all round-trip live. One robustness fix fell out of verification: an
unreachable or mis-typed addon is now **skipped** rather than failing the whole
operation, so a bad addon added through the UI cannot blank search, browse, or
metadata.

Sequenced after [module-stremio-addons#1](https://github.com/mosaic-media/module-stremio-addons/blob/main/docs/adr/0001-completing-the-stremio-source-surface.md); it
completes the Stremio module's *product* surface (addon management) now that the
source surface is done.
