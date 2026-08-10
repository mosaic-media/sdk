# go-plugin is the extension module harness

**Status:** Accepted; **built.** `hashicorp/go-plugin` v1.8.0 is the harness on
both sides — `sdk/host` serves a module and `internal/adapters/extension` in the
Platform launches and adapts one — and the four things this record leaves with
Mosaic (the `Caller` handle table, restart and crash-loop policy, the egress
proxy, the manifest check) are Mosaic's and are built. `sdk`'s own `go.mod`
still names no gRPC or protobuf module, which is the property the split exists
to hold. With [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) and
[platform#51](https://github.com/mosaic-media/platform/blob/main/docs/adr/0051-extension-installation-is-user-initiated-and-persistent.md) this
is what supersedes [platform#4](https://github.com/mosaic-media/platform/blob/main/docs/adr/0004-static-go-module-composition.md)'s rejection
of module RPC **for the extension tier only**; core modules stay statically
linked under [platform#3](https://github.com/mosaic-media/platform/blob/main/docs/adr/0003-platform-as-execution-kernel.md). Not done: the MPL-2.0
compatibility check this record asks to be "done and recorded" is recorded
nowhere, and both Open questions below are still open.
**Date:** 2026-07-24

Implements [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) within the decision it
already made — gRPC over a Unix domain socket — and does not supersede it.
Depends on [contracts#6](https://github.com/mosaic-media/contracts/blob/main/docs/adr/0006-contracts-protobuf-workspace.md)'s workspace and on
the `sdui` → `contracts` rename that record specifies. Nothing here is built.

## Context

[platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) decided the wire and the shape
around it: gRPC over a Unix domain socket, the SDK's Go interfaces staying the
contract, and a split so that `sdk` keeps an empty dependency graph while a
nested `sdk/host` module carries the serving harness. What it did not decide is
whether Mosaic *writes* that harness or adopts one.

The question was reopened by a concrete proposal: use **msgpack**
(`tinylib/msgp`) over the socket instead of protobuf, because msgp generates
marshallers *from* the hand-written Go structs. That would collapse the cost [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) named as ongoing rather than one-off — "the Go interfaces and the `.proto`
are two sources of truth that must agree. Nothing enforces it but codegen
discipline and tests."

**That argument is real, and it is the strongest case against protobuf here.**
Two things defeat it.

**msgp generates methods on the types.** `MarshalMsg` and `UnmarshalMsg` must
live in the same package as the type they serialize. So either `sdk` takes a
dependency on `github.com/tinylib/msgp` — the precise mechanism
[sdk#5](0005-modules-observe-through-the-sdk.md) refused when it declined to
re-export OpenTelemetry, and the property [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md)'s nested-module split exists
to preserve — or the wire uses separate DTO structs inside `sdk/host` that mirror
the SDK types by hand. The second keeps `sdk` clean and reinstates a dual source
of truth in a different place, Go against Go instead of Go against proto. The
saving is therefore smaller than it first appears. Protobuf generates into its
own package and has the problem in neither form.

**A codec is not a protocol.** msgpack is a serialization format and nothing
else. The call graph across this boundary is bidirectional and concurrent — the
Platform calls `Import` and the provider roles, and the module calls back into
`ContentService` many times per import over the same connection. Choosing
msgpack means owning framing, request correlation, stream multiplexing,
cancellation, deadlines and backpressure. [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) rejected exactly this under
*a hand-rolled stdlib-only protocol*, on the grounds that those are "the things
that are tedious to get right and painful to get wrong", and that rejection
stands.

The HashiCorp comparison that motivated the proposal is itself the evidence:
**go-plugin does not hand-roll any of that either.** It runs gRPC, or `net/rpc`
over yamux, precisely to avoid having to.

## Decision

**The wire stays gRPC over a Unix domain socket, as [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) decided, and
`hashicorp/go-plugin` is the harness on both sides of it.**

### What go-plugin supplies, and what stays Mosaic's

It supplies subprocess launch and handshake, protocol version negotiation, Unix
domain sockets — including for the broker sockets that carry the callbacks —
bidirectional calls through the broker, graceful kill, and log forwarding from
the child process.

It supplies none of the following, which stay Mosaic's and which this record
changes not at all: the invocation-scoped `Caller` handle table, restart and
crash-loop policy, the egress forward proxy, and the manifest check. [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md)
owns all four.

### The dependency lands in `sdk/host` and nowhere else

go-plugin is MPL-2.0 and brings grpc-go and protobuf with it. That closure sits
in the nested `sdk/host` module. **`sdk`'s `go.mod` stays a module line and a Go
version**, which is the property [sdk#5](0005-modules-observe-through-the-sdk.md) established and [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md)'s split was
designed to preserve — now carrying a heavier but battle-tested payload than
either record anticipated.

A module author importing only `sdk` still gets the contract and can be tested
with no transport at all. Adding `sdk/host` is still what makes the module
runnable as a process. **This should be asserted by a test rather than
remembered**, because it is the kind of property that decays silently.

### `HandshakeConfig.ProtocolVersion` is the SDK major version

[platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) requires that "a module and a Platform are compatible when they share an
SDK major version", and that this be **one number a user reasons about, not
two.** go-plugin's `ProtocolVersion` is that number, and its handshake is where
the check runs. While the SDK is pre-1.0 the compatibility unit remains the minor
version, exactly as [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) specifies — effectively exact pinning, which is
correct rather than unfortunate while there are no third-party authors to
inconvenience.

### The manifest check precedes the handshake — both gates, not either

**go-plugin's handshake happens by executing the binary.**
[platform#40](https://github.com/mosaic-media/platform/blob/main/docs/adr/0040-module-distribution-and-trust.md) requires that a manifest be
readable *without* executing it, because the alternative is running an unverified
binary to ask it what it is.

Both gates therefore run, in order: the Supervisor reads and verifies the
manifest and the signature first, and only then is the process launched, at which
point go-plugin's handshake verifies that the running binary agrees with what the
manifest claimed. [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) already said declaration and enforcement are separate
concerns; this record names where each of them lives.

### Containerised modules become an operator option, not the model

`hashicorp/go-secure-stdlib/plugincontainer` runs a plugin inside a container.
[platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) named "one container per module, with no network" as the strongest and
most portable form of its layer-3 requirement, and rejected it **as the model**
because it would require the Platform to drive a container runtime and turn it
into an orchestrator.

plugincontainer means an operator can have that deployment without the
architecture depending on it — which is precisely the disposition [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) asked
for when it named the option and declined to build on it.

## Alternatives considered

**msgpack (`tinylib/msgp`) over the socket, hand-framed.** *Rejected*, on the two
grounds argued in Context. Its merit is real and should not be lost: it is the
only option that would have collapsed the dual-source-of-truth cost, and if that
cost proves worse in practice than it looks on paper, this is where to come back
to. What defeats it is that the saving is partly illusory once `sdk`'s empty
dependency graph is protected, and that the remainder buys a protocol Mosaic
would then own end to end.

**Raw grpc-go, no go-plugin.** *A genuine alternative.* It is the same wire with
the lifecycle, handshake and broker written here instead of adopted, and it
avoids a dependency whose opinions about process launch Mosaic does not control.
Rejected because those are exactly the parts that are tedious to get right and
painful to get wrong — the same argument [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) used against a hand-rolled
protocol — and because go-plugin is what Terraform, Vault, Nomad and Packer run
in production, which is a quantity of adversarial testing this project cannot
reproduce.

**go-plugin's `net/rpc` mode rather than its gRPC mode.** *Rejected.* It is
lighter and it would drop protobuf from the closure, but `net/rpc` is
Go-specific and would close the door on a non-Go extension module permanently.
gRPC mode keeps that door open at no additional cost today.

**Adopt go-plugin's own plugin discovery and versioning conventions.**
*Deferred.* [platform#40](https://github.com/mosaic-media/platform/blob/main/docs/adr/0040-module-distribution-and-trust.md) owns distribution,
and its signed-index model is a deliberate choice that go-plugin's conventions do
not make for it.

## Consequences

- **[platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md) is unchanged and unsuperseded.** The proto module, the split SDK,
  the invocation-scoped handle, the egress proxy and the compatibility rule all
  stand exactly as written. This record fills in one thing that record left
  open.
- **MPL-2.0 enters the dependency graph.** It sits in `sdk/host` (Apache-2.0) and
  reaches the Platform (AGPL-3.0-only with a linking exception). MPL-2.0 is
  file-level copyleft and is normally compatible with both, but
  [platform#1](https://github.com/mosaic-media/platform/blob/main/docs/adr/0001-transactional-store-extensibility.md) governs and **the check should be done and
  recorded rather than assumed.**
- **[platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md)'s dual-source-of-truth cost is unchanged, not paid.** The Go
  interfaces and the `.proto` must still agree by codegen discipline and tests.
  The msgpack proposal was an attempt to pay that cost, and this record declines
  the price rather than disputing the debt.
- **A non-Go extension module becomes possible in principle**, because the wire
  is gRPC and the harness is a detail of the Go side. Not promised, not designed,
  and `sdk/host` is Go-only — but the door is not closed, which `net/rpc` would
  have closed.
- **Some of the Platform's planned process lifecycle work is supplied.** Spawn,
  handshake and graceful kill come with go-plugin; health checking, restart with
  backoff and crash-loop policy do not, and remain [platform#39](https://github.com/mosaic-media/platform/blob/main/docs/adr/0039-extension-module-boundary.md)'s requirement on the
  Platform.
- **A dependency now sits on the path every extension module takes.** A go-plugin
  regression or a breaking change is Mosaic's problem to absorb on behalf of every
  module author, which is the ordinary cost of adopting rather than writing, and
  is worth naming because the alternative was rejected partly on effort.

**Open.**

- **Whether go-plugin's log forwarding feeds the telemetry surface**
  ([sdk#5](0005-modules-observe-through-the-sdk.md)) or stays a separate
  stream. A module process's stderr is something [platform#31](https://github.com/mosaic-media/platform/blob/main/docs/adr/0031-telemetry-is-ambient-in-context.md)'s ambient-context
  design has no vocabulary for.
- **Whether `plugincontainer` becomes a supported deployment** with its own
  documentation and tests, or stays a possibility an operator assembles.
