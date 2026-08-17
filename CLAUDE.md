# Claude Instructions — sdk

`github.com/mosaic-media/sdk` is the published contract a Mosaic module compiles
against: the content models and service types, the provider roles, the opaque
`Caller`, and the module telemetry surface. `README.md` describes that surface
and carries its per-version changelog; this file is how to work here.

Fleet-wide conventions — commits, decision records, citation form, the roadmap —
are in [`architecture`](https://github.com/mosaic-media/architecture/blob/main/CLAUDE.md).
This file is what is specific to `sdk`.

## Two Go modules, and the nesting is load-bearing

| Module | Directory | What it is |
|---|---|---|
| `github.com/mosaic-media/sdk` | `contracts/platform/v1/` | the contract a module compiles against |
| `github.com/mosaic-media/sdk/host` | `host/` | the go-plugin/gRPC harness a module serves itself with, and the Platform's client for one |

`host` is nested so its dependencies — go-plugin, gRPC, the SDUI contract — stay
out of the parent's graph. Read each `go.mod` for what either requires; do not
describe one from the other, and do not add to the parent's on the way to fixing
something in the harness.

They are tagged separately (`vX.Y.Z` and `host/vX.Y.Z`), and `host/go.mod`
requires the parent by version like any other consumer — so a change spanning
both is two steps, not one.

## This is hand-written Go. It is not generated.

There are no `.proto` files here, no codegen and no build step: a new type is a
`.go` file beside `capability.go`. Mosaic's *other* published contract
repository, [`contracts`](https://github.com/mosaic-media/contracts), is the
generated one, and carrying a rule across from it is the easy mistake to make.
The split follows what each contract is — this one is Go interfaces a module
implements in its own process, which protobuf cannot express.

## The two tests that hold the boundaries

Read both before touching what they cover; each asserts something narrower than
its name suggests.

- **`host/nodeps_test.go`** reads `../go.mod` as text and fails on any require
  outside `allowedParentDependencies`. It is an **allowlist, not an emptiness
  assertion**, and because it reads text it checks indirect requires like any
  other. What is outside it — the OTel SDK and anything under it, an exporter, a
  collector client — is what a *binary* wires, so admitting one is a decision
  rather than a `go get`. The test lives in `host` because `host` is the module
  whose own dependency list is the temptation.
- **`contracts/platform/v1/telemetry_seam_test.go`** asserts that
  `telemetry.go` imports no `go.opentelemetry.io/…` path and that
  `telemetry_otel.go` imports at least one — the second half so the split cannot
  be satisfied by collapsing the two files. It reads those **two files by
  name**: a module-facing declaration moved into a third file is covered by
  neither. `telemetry.go` is the surface a module compiles against; anything
  needing an OTel type belongs next door.

## Rules for a change here

- **Nothing in this repository imports the Platform.** The dependency points one
  way. If something here needs a private Platform import, the contract is not
  ready to publish — that is the stop point.
- **No store contract, no transaction type, no identity or configuration
  model.** `contracts/platform/v1/doc.go` says so and means it: a capability
  calls application services, never stores. A facility a module needs is reached
  *declaratively* — a settings field the Platform seals — never as a primitive
  here, because `Seal`/`Open` in a published contract names an implementation
  and hands a module an encryption oracle
  ([sdk#10](docs/adr/0010-the-sdk-carries-no-implementation.md)).
- **The redaction rule is applied here, in pure Go, and only once.**
  `Field.EmitValue` re-applies the class on the way out, so a `Field` built as a
  struct literal — whose zero-value class is not `RedactionNone` — fails closed.
  It is exported precisely so the Platform applies *this* rule instead of
  reimplementing it; a second copy of a fail-closed rule is how the guarantee
  quietly stops being one.
- **The contract is tested only through its exported surface.** Every test file
  under `contracts/platform/v1` is `package v1_test`, and a new interface earns
  a stub that satisfies it from outside the package with no Platform types. That
  is the check that somebody holding only this module can implement it.
- **Every exported identifier carries a doc comment saying *why*.** This is read
  by people who cannot read the Platform's source; the comments are the
  documentation.
- **Nothing in `host` may write to stdout** beyond `serve.go`'s manifest mode,
  which never serves. go-plugin's handshake owns that stream.
- **Update `README.md`'s Status section in the same change.** It is the only
  account of what a version contains.
- **Apache-2.0**, with a `NOTICE`. Go files here carry no SPDX header — match
  the files already present.

## Decision records

[`docs/adr/README.md`](docs/adr/README.md) is the generated index; read it rather
than the directory, and regenerate it with `scripts/adr_index.py` rather than
editing it.

`scripts/adr_index.py` and `scripts/adr_lint.py` are **vendored from
[`architecture`](https://github.com/mosaic-media/architecture)** and say so in
their headers. Do not edit them here: a drifted copy is this repository's gate
enforcing a rule that has moved. Change them there and re-vendor.

## The gate

Nothing is built or tested on the host.

```bash
docker compose -f docker-compose.test.yml run --rm test
```

In order, that runs `adr_index.py --check`, `adr_lint.py`, `gofmt -l` over the
tree, and then `go build`, `go vet` and `go test` **twice — once at the root and
again in `host/`**, because `./...` does not descend into a nested module. A gate
that ran only at the root would report green having never compiled the harness.
The image's Go version and `go.mod`'s `go` directive are pinned to each other;
bump both together. Append `bash` for a shell in the same environment.

**There is no verify workflow in this repository.** `.github/workflows/pages.yml`
is the only workflow and it publishes the README and the records to GitHub Pages
through a reusable workflow in `architecture`. Nothing refuses a push on your
behalf — the container is the whole gate, and running it is on you.
