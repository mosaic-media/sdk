package v1_test

import (
	"testing"

	v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
)

// TestModuleVersionOfSomethingNotLinked — asking about a module this binary was
// not built with must not invent an answer. Returning a plausible number for an
// absent module is exactly the failure mode a hand-maintained constant had.
func TestModuleVersionOfSomethingNotLinked(t *testing.T) {
	if got := v1.ModuleVersion("github.com/example/not-linked"); got != v1.DevVersion {
		t.Errorf("ModuleVersion(absent) = %q, want %q", got, v1.DevVersion)
	}
}

// TestModuleVersionUnderTestReportsDevel — the SDK's own tests build it as the
// main module, so there is no released version to report and it must say so
// rather than guessing.
func TestModuleVersionUnderTestReportsDevel(t *testing.T) {
	if got := v1.ModuleVersion("github.com/mosaic-media/sdk"); got != v1.DevVersion {
		t.Errorf("ModuleVersion(self, under test) = %q, want %q — a local build is not a release", got, v1.DevVersion)
	}
}

// TestDevVersionIsConspicuous guards the choice of sentinel. It has to be
// obviously not-a-version at a glance in a boot log, because the whole point is
// that "which version is running" stops being answerable by a plausible-looking
// number that nobody updated.
func TestDevVersionIsConspicuous(t *testing.T) {
	if v1.DevVersion == "" || v1.DevVersion[0] >= '0' && v1.DevVersion[0] <= '9' {
		t.Errorf("DevVersion = %q, which reads like a real version", v1.DevVersion)
	}
}
