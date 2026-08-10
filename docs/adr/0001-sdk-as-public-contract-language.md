# SDK as public contract language

**Status:** Accepted
**Date:** 2026-07-14

## Context

Mosaic Modules need a stable way to integrate with the Platform without importing Platform internals. The Platform owns orchestration and implementation while Modules own capability implementations, so both sides need a shared language for capability contracts, models, events, registration, permissions and testing.

If the SDK becomes a framework or an implementation library instead, Platform concerns will leak into Modules and SDK versioning will become unstable.

## Decision

The Mosaic SDK is the public contract language between the Platform and Modules. The SDK owns:

- capability interfaces
- canonical Platform models
- the Event Envelope
- Event Bus interfaces
- core Platform events
- Module registration APIs
- small validation and version helpers
- testing utilities

The SDK must remain lightweight, which means it must not contain database logic, storage implementation, GraphQL implementation, scheduler implementation, HTTP server code, caching implementation, business logic, Capability Manager orchestration or rendering.

Both the Platform and Modules depend on the SDK, but Modules must not depend directly on the Platform and the Platform must not depend directly on Modules. SDK tooling may generate Module manifests from Go Module definitions, and the generated manifest still remains the build-time artefact consumed and validated by the Supervisor.

## Alternatives considered

**SDK as a framework.** *Rejected:* it would accumulate behaviour and reduce the clarity of Platform ownership.

**SDK as a helper library.** *Rejected:* it under-specifies the contract authority that Modules need.

**Modules import Platform internals.** *Rejected:* it couples Modules to Runtime implementation details.

**Platform imports Modules directly.** *Rejected:* it breaks selected build-time composition and Module independence.

**Source code as the discovery authority.** *Rejected:* Supervisor discovery must remain manifest-driven and non-executing.

## Consequences

The SDK should be among the most stable Mosaic repositories, so breaking SDK changes should be rare and deliberate. Module authors can build against the Platform contract without understanding Platform implementation, and the Platform can in turn evolve its internals while preserving SDK compatibility. Manifest generation can therefore improve developer ergonomics without weakening Supervisor validation.

## Implementation implications

SDK repository structure should reinforce contract vocabulary, with the proposed shape below.

```text
mosaic-sdk/

    capabilities/
    events/
    models/
    module/
    permissions/
    registration/
    helpers/
    testing/
```

SDK tests should include contract assertions and provider fixtures, and CLI tooling should provide developer workflows such as the following.

```text
mosaic new module
mosaic dev
mosaic build
mosaic validate
mosaic publish
```

The CLI owns developer workflow, whereas the SDK owns developer contract.
