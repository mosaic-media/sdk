package v1

import (
	"runtime/debug"
	"strings"
)

// DevVersion is what ModuleVersion reports when a module was not resolved from
// a released version — a local build, a `replace`, a workspace, or the module's
// own tests.
//
// It is deliberately conspicuous. "Which version is actually running" is the
// question, and a local build answering with a plausible-looking number is how
// the drift this exists to prevent gets started again.
const DevVersion = "(devel)"

// ModuleVersion reports the version of modulePath as it was actually linked
// into the running binary.
//
// Use it rather than a hand-maintained constant in the Manifest. A number a
// human has to remember to change is a number that will be wrong, and it is
// wrong in the least visible way possible: the Platform logs a version at boot,
// and a stale one looks exactly like a correct one.
//
// Reading it from the build graph makes the answer structurally true, and
// answers a question a constant cannot: whether the binary is running a
// released module or a local checkout — the case where a container resolves a
// module from the proxy while its source is being edited beside it.
//
// Usage, from a module's Manifest:
//
//	Version: v1.ModuleVersion("github.com/mosaic-media/module-stremio-addons")
func ModuleVersion(modulePath string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return DevVersion
	}
	// The module under its own tests is the main module, not a dependency.
	if info.Main.Path == modulePath {
		return normaliseVersion(info.Main.Version)
	}
	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		// A replaced or workspaced module is a local build whatever version the
		// require line claims, and saying so is the point.
		if dep.Replace != nil {
			return DevVersion
		}
		return normaliseVersion(dep.Version)
	}
	return DevVersion
}

// normaliseVersion strips the leading "v" so a Manifest reads "0.12.0" like a
// version rather than "v0.12.0" like a tag, and maps the empty and placeholder
// forms onto DevVersion.
func normaliseVersion(v string) string {
	if v == "" || v == DevVersion || v == "devel" {
		return DevVersion
	}
	return strings.TrimPrefix(v, "v")
}
