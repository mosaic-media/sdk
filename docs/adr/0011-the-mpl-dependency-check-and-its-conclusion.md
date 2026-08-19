# The MPL-2.0 dependency, checked rather than assumed

**Status:** Accepted, and it is an analysis rather than a change — nothing is
built or altered by it. Discharges the obligation
[sdk#7](0007-go-plugin-as-the-extension-harness.md) states and leaves undone: the
licence check "should be done and recorded rather than assumed", and it was
recorded nowhere. Governed by
[architecture#1](https://github.com/mosaic-media/architecture/blob/main/docs/adr/0001-licensing.md).

**This is not legal advice.** Its value is that the question stops being
invisible, and that whoever asks it next starts from the reading rather than from
the assumption.

## Context

`github.com/hashicorp/go-plugin` is MPL-2.0 and is required by `host/go.mod`. It
sits in `sdk/host`, which is Apache-2.0, and reaches the Platform, which is
AGPL-3.0-only with a linking exception.

sdk#7 adopted it and explicitly declined to assume the combination was fine. The
temptation is to note that this pairing is widespread in the Go ecosystem and move
on, which is the assumption that record refused.

## Decision

**The dependency stands, and the reading it stands on is written down here.**

**MPL-2.0 is file-level copyleft.** Its obligations attach to the files that
carry the licence — "Covered Software" — and not to a larger work that combines
with them. This is the substantive difference from GPL-family licences and it is
the whole reason the combination is ordinary rather than exceptional.

**§3.3 is the operative clause**, not the secondary-licence route. MPL-2.0 permits
distributing a Larger Work under other terms provided the Covered Software's own
files stay under MPL and their source stays available. The secondary-licence
mechanism — the Exhibit B notice that lets MPL files be taken under GPL/LGPL/AGPL
terms — **does not apply here**, because go-plugin does not carry that notice. It
is worth naming the route that does not apply, because it is the one somebody
reaching for "is this AGPL-compatible?" will find first and conclude the wrong
thing from.

**What Mosaic therefore owes, and already does:**

- **Do not modify go-plugin's files.** Mosaic does not; it is a module
  requirement, resolved from the proxy, never vendored and never patched.
- **Keep the source of those files available.** It is public, and `go.mod` and
  `go.sum` pin the exact version, so "which source" is answered by the build
  rather than by anybody's memory.
- **Do not strip notices.** Nothing here rewrites third-party files.

**The obligation travels with the binary, not with this module.** `sdk` and
`sdk/host` distribute no MPL bytes — a consumer resolves them independently. It is
the linked Platform binary that carries go-plugin's compiled code, so the
availability obligation is discharged where binaries are published, and the
version it must point at is the one in the build graph.

## Alternatives considered

**Treat the check as blocking and get it reviewed before release.** *Rejected as
the default,* and it is the right escalation if Mosaic is ever distributed
commercially or a licensor raises a question. A written reading is not a cleared
one, and this record does not pretend otherwise.

**Drop go-plugin and hand-roll the harness.** *Rejected:* sdk#7 weighed that on
effort and correctness and chose adoption; a licence reading that comes out fine
is not a reason to reopen it.

**Relicense `sdk/host`.** *Rejected:* nothing requires it. File-level copyleft
does not reach the surrounding module, which is the finding above.

## Consequences

**The one question a lawyer would answer differently from this record** is whether
AGPL §13's corresponding-source obligation, offered over a network, is fully
satisfied for the MPL portion by pinning and public availability rather than by
shipping an archive. The reading here is that it is, because the MPL files are not
AGPL-licensed and their own licence sets the standard for their availability. That
is the sentence to take to somebody qualified if it ever matters.

**A second MPL dependency does not get this record for free.** The reading is
about file-level copyleft in general, but "do not modify, keep source available,
keep notices" has to be true of each one, and a vendored or patched MPL dependency
would land in a different place entirely.

**sdk#7's outstanding item is discharged**, which its Status line should say; its
two Open questions are answered in
[platform#103](https://github.com/mosaic-media/platform/blob/main/docs/adr/0103-module-output-is-telemetry-and-containment-stays-one-mechanism.md),
where their mechanisms live.
