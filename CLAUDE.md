# Claude Instructions — Mosaic SDK

This repository is the **published contract surface** between the Platform and
the Modules that extend it (ADR 0008, ADR 0016). It is `github.com/mosaic-media/sdk`,
consumed as an ordinary tagged dependency with no `replace`.

## This is hand-written Go. It is not generated, and it is not protobuf.

**Read this before adding a file.** Mosaic has two published contract
repositories and they are built in opposite ways, which is a reasonable thing
to get wrong:

| Repository | Form | Source of truth |
|---|---|---|
| **`sdk`** (this one) | **hand-written Go** | the `.go` files in `contracts/platform/v1/` |
| **`sdui`** | **protobuf**, Go and TS generated | `proto/**/*.proto`, generated into `gen/` |

[ADR 0044](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0044-contracts-protobuf-workspace.md)
made the **SDUI and session** contracts protobuf. Its title names that scope,
and it does not extend here.

The reason is not historical accident. **This SDK's job is Go interfaces with
behaviour** — `Capability`, `ContentService`, the provider roles, `Telemetry` —
which a module *implements* in its own process. Protobuf describes messages and
RPC services; it cannot express an interface a third party satisfies in-process.
`sdui` is the opposite case: a wire format, consumed by four client languages,
where codegen is exactly right.

So: add a `.go` file beside `capability.go` and `provider.go`. Do not add a
`.proto`, do not add a `buf.yaml`, and do not generate anything. There are no
generated files here and no build step — `go build ./...` is the whole thing.

## Non-negotiable rules

- **No dependencies.** `go.mod` is a module line and a Go version, and that is
  load-bearing: a third party compiles against this contract and against
  nothing the Platform happened to choose. Adding a dependency here forces it
  on every module author and pins them to a version the Platform picked.
  This is why the telemetry surface (ADR 0059) declares its own interface
  rather than re-exporting OpenTelemetry.
- **Nothing here imports the Platform.** The dependency points one way. If a
  capability needs a private Platform import, the contracts are not ready to
  publish — that is the stop point, and it governs any change here.
- **No storage contracts, no transaction type, no identity or configuration
  models.** A capability calls application services, never stores (ADR 0012).
- **Apache-2.0**, unlike the Platform's AGPL. This is the permissive surface a
  third party compiles against. Files here carry no SPDX header — match the
  files already present rather than importing the Platform's convention.

## Versioning and release

Pre-1.0 on purpose. A change is a **minor** bump (`v0.13.0` → `v0.14.0`), tagged
and pushed, and the Platform's `require` is bumped to match:

```bash
git tag v0.14.0 && git push origin main && git push origin v0.14.0
```

For local cross-repo work, add `replace github.com/mosaic-media/sdk => ../sdk`
to the Platform's `go.mod` temporarily — then tag, push, bump, and remove the
`replace` before committing. A `replace` must never land in a commit.

Update the **Status** section of `README.md` in the same change: it is the
per-version changelog, and it is how anyone finds out what a tag contains.

## Everything runs in the container, nothing runs on the host

**Do not run `go build`, `go test`, `go vet` or `gofmt` directly on this
machine.** This repository's gates run inside its test container:

```bash
docker compose -f docker-compose.test.yml run --rm test
```

That runs gofmt, `go build ./...`, `go vet ./...` and `go test ./...` against
the Go version pinned in the compose file, which must stay equal to the one in
`go.mod`. Append `bash` for a shell in the same environment.

The reason is weakest here of all the repositories and still worth keeping,
because of what this module *is*. There is no hidden dependency to supply — no
database, no ffmpeg, no generator — and `go build ./...` really is the whole
thing. But **this is the surface a third party compiles against**, and the only
claim worth making about it is that it builds under a pinned toolchain, not
under whatever a particular machine happens to have installed. A contract that
compiles only where its author works is not a contract. Uniformity is also its
own argument: a rule with an exception in one repository is a rule nobody
applies reliably in the other six.

## Workflow

- Commit and push this repository **separately** from `platform`. It is its own
  git repository despite sitting beside the others on disk.
- **Commit author identity** must be `AdamNi-7080 <anicholls41@gmail.com>`.
- The test container green before pushing.
- Every exported type and function carries a doc comment that says *why*, not
  only what. This is a published contract read by people who cannot read the
  Platform's source; the comments are the documentation.

## The roadmap and the decision records

These rules are identical in every Mosaic repository. They exist because the
state of the build and the reasons behind it are the two things that rot fastest
and report nothing when they do — no build fails, no test goes red.

### The roadmap is maintained, not consulted

**`docs/roadmap.md` in [`architecture`](https://github.com/mosaic-media/architecture)
is the single record of where the build is.** Read it before starting work, and
**update it in the same session as the change that dates it** — not in a
follow-up, which does not happen.

- **A slice that lands is marked landed, with what was left out.** "Built" with
  no qualifier is a claim that the whole slice shipped; if part of it did not,
  say which part and why in the same sentence.
- **Implementation that departs from the plan is recorded where it departed.**
  The roadmap is derived from the code, not from the intention that preceded it,
  and the surprises are the most valuable thing in it.
- **Do not restate the roadmap here.** A second copy of "what is built" in a
  `CLAUDE.md` is how the first copy goes stale unnoticed. This file carries how
  to work in *this* repository; the roadmap carries what has been done across all
  of them.
- **A capability with no client path is not done — it is
  [owed](https://github.com/mosaic-media/architecture/blob/main/docs/unreachable-capability.md).**
  If you delete or fail to build a client path to a working service, add its row
  to that register in the same change.

### Decision records are append-only

An ADR is an account of what was decided and why, at a time. It is evidence, not
documentation, and its value is that it was not edited afterwards.

- **Never rewrite a record's body to match what was built.** Not to correct it,
  not to annotate it, not to add "as built, this differs". That pattern turns a
  record into a running commentary and destroys the thing it is for.
- **State changes in the `**Status:**` line, and nowhere else.** That is where a
  record says it is built, built in part (naming the part), or superseded —
  wholly ("Superseded by ADR N") or partly ("Partly superseded: X was reversed by
  ADR N; the rest stands").
- **A changed decision needs a new record that supersedes it.** If the code
  deliberately does something a record decided against, that is a decision and it
  is written down as one, with its own Context / Decision / Alternatives /
  Consequences. Both records then stand: the old one keeps its reasoning, the new
  one carries the change.
- **An unbuilt decision is not a superseded one.** "We have not done this yet"
  belongs in the Status line and the roadmap. Only a genuine reversal earns a new
  record.
- **Records live only in `architecture/docs/adr/`**, numbered sequentially in
  kebab-case. Adding one means adding it to `nav:` in `mkdocs.yml`, and
  `mkdocs build --strict` must pass.

**If the code and a record disagree, say so rather than quietly picking one.** An
honest "this is unresolved" is worth more than a plausible reconciliation that
reads as settled.
