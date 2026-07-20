# Mosaic SDK

The public contract language between the Mosaic Platform and the Modules that
extend it ([ADR 0008](https://github.com/mosaic-media/mosaic-architecture/blob/main/docs/adr/0008-sdk-as-public-contract-language.md)).
A Module compiles against this module and nothing else of the Platform's.

It is deliberately small. Today it carries the **content surface**
(`contracts/platform/v1`): the object-graph models (`Node`, `Part`,
`Relation`, `SourceBinding` and their vocabularies), the content command,
query and result types, the `ContentService` interface a capability calls, and
the opaque `Caller` a capability forwards from its invocation context
([ADR 0016](https://github.com/mosaic-media/mosaic-architecture/blob/main/docs/adr/0016-published-contract-surface.md),
[ADR 0017](https://github.com/mosaic-media/mosaic-architecture/blob/main/docs/adr/0017-how-a-capability-acts.md)).

It holds **no** storage contracts, no transaction type, no identity or
configuration models, and no Platform implementation — a capability calls
application services, never stores. It depends only on the Go standard
library.

```go
import v1 "github.com/mosaic-media/mosaic-sdk/contracts/platform/v1"
```

## Status

Extracted from `mosaic-platform` as a local dry-run: the Platform and the
reference capability build against it through a `replace` directive. Cutting
over to a tagged, `go get`-able version is the remaining publish step.

## License

Apache License, Version 2.0 (see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE)).
The SDK is deliberately permissive: it is the contract a Module builds against,
so a Module author may use it under any license. This is independent of the
Platform's license (AGPL-3.0 with a Module Linking Exception).
