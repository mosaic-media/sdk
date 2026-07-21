// Package v1 is the Platform's published contract surface — the packages a
// Module compiles against, and the only Platform code it may import
// (ADR 0008, ADR 0016).
//
// It holds the content models (Node, Part, Relation, SourceBinding and their
// vocabularies), the command, query and result types of the content
// application services, the service interface those methods satisfy, and the
// opaque Caller a capability forwards from its invocation context (ADR 0017).
//
// It deliberately does not hold the store contracts (NodeStore, Tx,
// StorageAdapter) or the identity and configuration models: those are the
// Platform's plumbing and stay under internal/. A capability calls
// application services, never stores.
//
// This package lives in the standalone SDK module
// (github.com/mosaic-media/sdk), extracted from the Platform repository
// so a Module depends on it exactly as any third party would (ADR 0008). The
// Platform consumes it as an ordinary module dependency; nothing here imports
// the Platform.
package v1
