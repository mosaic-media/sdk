# Claude Instructions — Mosaic SDK

This repository is the **published contract surface** between the Platform and
the Modules that extend it ([sdk#1](docs/adr/0001-sdk-as-public-contract-language.md)).
It is `github.com/mosaic-media/sdk`, consumed as an ordinary dependency with no
`replace`.

`README.md` describes the surface and carries the per-version changelog. This
file is how to *work* here — the rules, and the reasons they exist.

Two Go modules live in this repository, and the distinction is load-bearing:

| Module | Path | What it is |
|---|---|---|
| the contract | `github.com/mosaic-media/sdk` | `contracts/platform/v1/` — what a module compiles against |
| the harness | `github.com/mosaic-media/sdk/host` | `host/` — what an out-of-process module links to serve itself |

`host` is **nested so the contract's dependency graph does not have to carry
it**. It requires go-plugin, gRPC and the SDUI contract; the parent must not.
That is the whole reason for the nesting, and `host/nodeps_test.go` is the
assertion that keeps it honest — see below.

## This is hand-written Go. It is not generated, and it is not protobuf.

**Read this before adding a file.** Mosaic has two published contract
repositories and they are built in opposite ways, which is a reasonable thing to
get wrong. This one is hand-written Go;
[`contracts`](https://github.com/mosaic-media/contracts) is generated, and its
own `CLAUDE.md` says from what.

The reason is not historical accident. **This SDK's job is Go interfaces with
behaviour** — `Capability`, `ContentService`, the provider roles, `Telemetry` —
which a module *implements* in its own process. Protobuf describes messages and
RPC services; it cannot express an interface a third party satisfies in-process.
A wire format consumed by several client languages is the opposite case, and
that is why it lives in the other repository rather than here.
[contracts#6](https://github.com/mosaic-media/contracts/blob/main/docs/adr/0006-contracts-protobuf-workspace.md)
made the SDUI and session contracts protobuf; its title names that scope, and it
does not extend here.

So: add a `.go` file beside `capability.go` and `provider.go`. Do not add a
`.proto`, do not add a `buf.yaml`, and do not generate anything. There are no
generated files here and no build step.

## Non-negotiable rules

- **This SDK names no implementation.** The principle governs every change here:
  **the SDK says how a module interacts with the Platform; the Platform holds the
  implementations.** The surface may be **wide** — a module nobody has imagined
  must be expressible, and a missing verb is found only by someone trying to
  build the thing it blocks — but never **deep**. Those are not in tension: a
  contract can declare a great many interfaces and still name no library, which
  is what the content surface already does.

- **The dependency rule is an allowlist, and its current state is not what
  [sdk#10](docs/adr/0010-the-sdk-carries-no-implementation.md) wants.** Read both
  the record and `go.mod` before writing anything about it, because they
  disagree on purpose and the record's `**Status:**` line says so.

  As this is written, `go.mod` **requires four OpenTelemetry API modules** —
  `go.opentelemetry.io/otel`, `.../log`, `.../metric`, `.../trace` — plus
  `github.com/cespare/xxhash/v2` as an indirect they pull in.
  `host/nodeps_test.go` enforces exactly that five-entry allowlist and fails on
  anything else. It is **not** an emptiness assertion, and describing it as one
  has already put a false sentence in this file.

  [sdk#10](docs/adr/0010-the-sdk-carries-no-implementation.md) decides to remove
  them and is **unbuilt**. Until it lands: **do not widen the allowlist**, and do
  not write "no require block at all" as though the removal had happened. The
  live rule is the allowlist; the intended one is in the record.

  What is *not* on the list and cannot be added without a decision:
  `go.opentelemetry.io/otel/sdk` and anything under it, any exporter, any
  collector client. Those are what a *binary* wires, and a module that could
  reach them could configure the observability plane from inside it.

- **The module-facing surface must stay free of OpenTelemetry types, and this is
  checked.** `telemetry.go` is the module-facing half (`Telemetry`, `Span`,
  `Field`, `TelemetryFrom`); `telemetry_otel.go` is the host-facing half
  (`NewTelemetry`, `TelemetryOptions`, `Encoder`). The split is physical rather
  than a convention, and `telemetry_seam_test.go` parses `telemetry.go`'s imports
  and fails the build if an OTel path appears there. "Abstract it so the
  implementation can change" is a claim; that test is what makes it checkable.

  **The classification rule stays here, in pure Go.** Resolving a `Field` to the
  value a sink may record is `(any, bool)`; only the last step, building an
  `attribute.KeyValue`, needs OTel. A second copy of a fail-closed rule is how
  the guarantee quietly stops being one.

- **A facility a module needs is reached declaratively, never as a primitive.**
  This settles the crypto question without a second argument: the Platform's
  secret facility is reached through a settings field marked secret and sealed by
  the Platform — never through `Seal`/`Open` primitives here, which would publish
  an implementation and hand a module an encryption oracle.

- **Nothing here imports the Platform.** The dependency points one way. If a
  capability needs a private Platform import, the contracts are not ready to
  publish — that is the stop point, and it governs any change here.

- **No storage contracts, no transaction type, no identity or configuration
  models.** A capability calls application services, never stores
  ([platform#8](https://github.com/mosaic-media/platform/blob/main/docs/adr/0008-capabilities-do-not-own-stores.md)).

- **Apache-2.0**, unlike the Platform's AGPL. This is the permissive surface a
  third party compiles against. Files here carry **no SPDX header** — match the
  files already present rather than importing the Platform's convention.

## Decision records

[`docs/adr/README.md`](docs/adr/README.md) is the generated index of the records
this repository owns, with each one's status. **Read the index rather than
counting files, and do not restate a status here** — it is generated from the
records and this file is not.

The index is produced by `adr_index.py`, which
[`architecture`](https://github.com/mosaic-media/architecture) owns for the
fleet. **Neither that script nor the citation lint is vendored here yet**, so
nothing in this repository's gate checks the index is current or that a citation
resolves. Until they are, both are on you: regenerate the index when you add a
record, and write every citation as a `repo#N` link.

## Versioning and release

Pre-1.0 on purpose. A change is a **minor** bump, and `README.md`'s **Status**
section is updated in the same change — it is the per-version changelog and the
only way anyone finds out what a version contains. Read it for where the
numbering has got to rather than trusting an example written here.

**Tag pushes are currently refused from this environment**, and this repository
has no tags at all as a result. Do not present tagging as a step you have
completed, and do not tell anyone a version is published when only its commit is
on `main`.

What works instead: a Go consumer moves onto an untagged commit as an ordinary
pseudo-version `require` resolved from the proxy. That keeps the standing rule
intact — **a `replace` must never land in a commit** — and it is how every
consumer is presently pinned. For local cross-repo work add a `replace` to the
*consumer's* `go.mod` temporarily, then move it to a pseudo-version and remove
the `replace` before committing.

Note that `host` requires the parent SDK by version like any other consumer, so
a change spanning both is two steps, not one.

## Everything runs in the container, nothing runs on the host

**Do not run `go build`, `go test`, `go vet` or `gofmt` directly on this
machine.** This repository's gate runs inside its test container:

```bash
docker compose -f docker-compose.test.yml run --rm test
```

That runs gofmt over the tree, then `go build`, `go vet` and `go test` **twice —
once for the contract and once for `host`**, against the Go version pinned in the
compose file, which must stay equal to `go.mod`'s. Append `bash` for a shell in
the same environment.

**The second pass is the point, not a formality.** `./...` does not descend into
a nested module, so a gate that ran only in the root would report green while
never compiling the harness — and `host` is where `nodeps_test.go` lives, so it
would also never check the dependency rule the whole nesting exists to protect.

This repository has **no CI workflows**, so the container is the only gate there
is. Nothing will refuse a push on your behalf.

The container's other argument is weaker here than elsewhere and still worth
keeping: there is no hidden dependency to supply — no database, no generator —
but **this is the surface a third party compiles against**, and the only claim
worth making about it is that it builds under a pinned toolchain rather than
under whatever a particular machine happens to have installed. A contract that
compiles only where its author works is not a contract.

## Workflow

- Every exported type and function carries a doc comment that says *why*, not
  only what. This is a published contract read by people who cannot read the
  Platform's source; the comments are the documentation.
- A module that cannot express something is reporting a **finding**, not hitting
  an obstacle to work around. Take it as an additive minor bump, or record it in
  the roadmap as an open gap. What a finding may ask for is a shape — a type or a
  verb. One that can only be closed by naming a library is a Platform change
  reached through a declarative surface, not a bump here.

<!-- shared-rules:begin -->
<!-- shared-rules:end -->
