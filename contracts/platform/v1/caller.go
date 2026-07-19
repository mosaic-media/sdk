package v1

// Caller identifies who a command or query acts as (ADR 0017).
//
// A capability does not originate authority. The Platform invokes the
// capability within a context carrying a principal, and the capability
// forwards that principal — this value — to every service it calls. It is
// opaque in intent: a capability receives one and passes it on rather than
// inspecting or constructing it.
//
// It carries no more authority than the session it references. The Platform
// validates that session on every call, so a forged or stale reference
// authenticates as nothing. For the reference capability the principal is the
// invoking user; a system principal for background work is a future addition
// to this type, not a change to the commands that carry it.
type Caller struct {
	// Session is the reference to the acting user's session, as the Platform
	// handed it to the capability.
	Session string
}

// CallerFromSession builds a Caller from a session reference. The Platform
// uses it to mint the principal it hands a capability; a capability normally
// forwards a Caller it was given rather than building one.
func CallerFromSession(session string) Caller {
	return Caller{Session: session}
}
