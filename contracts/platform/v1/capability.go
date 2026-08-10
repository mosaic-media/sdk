package v1

import "context"

// Capability is what an optional Module implements so the Platform can invoke
// it. sdk#1 always reserved a capability and registration surface for the
// SDK; platform#12 populated only the content services, and this is the first
// addition of the capability side.
//
// The dependency direction is the whole point: a Module depends only on this
// SDK, and the Platform holds the Capability, registers it and routes to it.
// The Module never imports the Platform. A Capability sources content from
// somewhere the Platform does not know about — a provider, an addon, a
// scanner — and reflects it into the object graph through ContentService,
// acting as the Caller it is handed (platform#13). It owns no schema (platform#8):
// everything it does to the graph goes through the published services.
//
// Capability is the base every module implements: identity plus the one write
// verb, Import. The read side — search, catalogs, metadata, streams — is the
// optional provider roles in provider.go, which a module additionally
// implements and declares in Manifest.Provides (sdk#2). The Platform
// discovers a module's roles by type-asserting the Capability to each provider
// interface.
type Capability interface {
	// Manifest is the Module's self-declaration — the stable identity the
	// Platform registers it under and routes imports to, and the roles it
	// declares.
	Manifest() Manifest

	// Import materialises one virtual content item — named by the request's
	// Ref, which a read role (search, catalog) produced — into the object graph
	// through svc, acting as the request's Caller throughout (platform#18: import
	// is the one crossing from the virtual plane to the library). It returns
	// what it did so the invoker (or a test) can see the shape without
	// re-reading the graph. Errors carry a Platform error category, since every
	// service call it makes does.
	Import(ctx context.Context, svc ContentService, req ImportRequest) (ImportResult, error)
}

// ImportRequest is what the Platform hands a capability for one invocation. It
// is a struct rather than a parameter list so the surface can grow — the
// module system adds to what a module receives (settings now; more as the
// system matures) without breaking the interface each time.
type ImportRequest struct {
	// Caller is the principal the capability acts as (platform#13); it forwards
	// this to every service call.
	Caller Caller
	// Ref names the virtual content item to materialise — the handle a read
	// role produced (platform#18). It replaced a free-form query string in v0.4.0:
	// import is no longer "parse an id from a string" but "materialise this
	// result". The module reads Ref.NativeID/NativeType to source the detail.
	Ref ContentRef
	// Settings is the module's user-managed configuration document — the
	// opaque JSON a user set for this module through the Platform (an addon
	// list, an API key). The Platform stores and hands it back without
	// interpreting it; the module owns its meaning. It is an empty object
	// ({}) when the user has set nothing.
	Settings []byte
}

// Manifest is a Module's declaration of identity and the roles it fills. It
// grows as the module system needs it (the permissions a module declares, the
// media types it sources); those are named future additions, not omissions to
// fix now.
type Manifest struct {
	// ID is the stable key the Platform registers the capability under and a
	// caller names to invoke it. It must be unique across registered modules.
	ID string
	// Version is the module's own version, carried for diagnostics and future
	// compatibility checks. It is the module's version, not the SDK's.
	Version string
	// Name is a human-readable label for the module.
	Name string
	// Description is one sentence about what this module IS, in the author's
	// words — "AIOStreams aggregates many sources behind one instance". It is
	// shown to a user deciding whether to install it, beside the capabilities
	// the Platform derives from Provides.
	//
	// Only the author can write it. The Platform can say what a module DOES
	// (the roles are right here), and deliberately does not keep prose about
	// other people's modules: that table would go stale the day a third party
	// published one. Optional — a module that says nothing is described by its
	// capabilities alone.
	Description string
	// Provides declares the provider roles this module fills (sdk#2). The
	// Platform checks at composition that each declared role is backed by the
	// matching provider interface in provider.go, then resolves providers by
	// role at runtime. Empty means the module only imports. This is the first
	// real growth of the manifest shape, and what the media_types registry
	// (platform#11) has waited on.
	Provides []Role
}

// ImportResult reports what an Import did. Containers, Items and Parts count
// the nodes and parts the import created; AlreadyKnown is true when the
// content was already present, in which case the import was a no-op that
// returned the existing work's id in WorkID.
type ImportResult struct {
	WorkID       NodeID
	AlreadyKnown bool
	Containers   int
	Items        int
	Parts        int
}
