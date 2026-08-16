package v1

// The content read queries. Like the commands, each authenticates its Caller
// and passes policy before reading; unlike them, a query opens no
// transaction.

// SearchContentQuery finds content by title, media type and kind. Every
// filter is optional; the empty query returns the first page of everything.
// This is "do I already have this?" — the read a capability makes before it
// sources anything.
type SearchContentQuery struct {
	Caller Caller
	// Title matches case-insensitively anywhere in the title.
	Title     string
	MediaType MediaType
	Kind      NodeKind
	// AttributesContain narrows to nodes whose Attributes document contains
	// this JSON document. Empty means no filter.
	//
	// # Why containment, and not a typed filter
	//
	// A node's Attributes are module-owned opaque JSON: the Platform stores them
	// and never interprets them (platform#9 assigns their correctness to the
	// writing capability). A typed filter would require the Platform to learn
	// what a module put there, which is the coupling the whole arrangement
	// exists to avoid. Containment is the one question that can be asked without
	// understanding the answer — does this document contain this shape — so it
	// is the only filter that respects the ownership.
	//
	// That makes it the counterpart of FindContentByExternalID, which has always
	// worked this way over the neighbouring external-ids document.
	//
	// # What a caller should expect
	//
	//	// works a module recorded as available on a given service
	//	AttributesContain: []byte(`{"watch":{"providers":["Netflix"]}}`)
	//
	// Matching is structural and nested: a document containing the key with a
	// superset of the listed array entries matches. It must be a valid JSON
	// document; anything else is an InvalidArgument rather than an empty result,
	// because a filter that silently matched nothing would read as "you own none
	// of these" rather than as a mistake.
	//
	// It is a storage-engine obligation, not a Postgres detail: any
	// StorageAdapter implementation must support containment over both JSON
	// documents. The Platform's own does it with an indexed jsonb operator.
	//
	// A module can see what another module wrote. That is deliberate and
	// matches the rest of the read surface — providers are resolvable across
	// modules by design (sdk#2) — but it means a module should treat its
	// attribute keys as a published shape rather than a private one.
	AttributesContain []byte
	// Genres narrows to nodes carrying every genre listed. Empty means no
	// filter.
	//
	// Conjunctive rather than disjunctive because that is what a facet control
	// means when two chips are lit: "crime and comedy", not "either". One
	// chip — the common case — reads the same under both rules, so the choice
	// only shows up when a user has expressed a narrowing and the union would
	// have widened it instead.
	//
	// Matching is on the stored strings, which are the source's own words
	// (Node.Genres). The Platform canonicalises their form the way it does
	// every open vocabulary (platform#11) and does not reconcile their meaning, so
	// "Sci-Fi" and "Science Fiction" are two genres here because they are two
	// genres in the sources.
	Genres []string
	// Limit is clamped to a Platform maximum and defaults when zero or
	// negative, so a search cannot ask for an unbounded scan.
	Limit int
}

// SearchContentResult carries the matching nodes.
type SearchContentResult struct {
	Nodes []Node
}

// FindContentByExternalIDQuery resolves content by a provider's own
// identifier — the strong form of "do I already have this", not depending on
// titles matching.
type FindContentByExternalIDQuery struct {
	Caller Caller
	Scheme string
	Value  string
}

// FindContentByExternalIDResult carries the matches. It is a list because an
// anime and its source manga can share a provider reference and remain two
// Works (platform#9).
type FindContentByExternalIDResult struct {
	Nodes []Node
}

// GetContentNodeQuery reads a single node, optionally with its direct
// children — one level, not a subtree, since variable depth means a caller
// descends deliberately (platform#9).
type GetContentNodeQuery struct {
	Caller       Caller
	NodeID       NodeID
	WithChildren bool
}

// GetContentNodeResult carries the node and, when asked, its direct children
// in order. Children is nil unless requested and empty for a childless node.
type GetContentNodeResult struct {
	Node     Node
	Children []Node
}

// ListContentPartsQuery reads the playable parts of an item node.
//
// It is the read side of AttachContentPart (platform#25): without it a
// capability cannot see what it has itself created, and a re-import has no way
// to know which releases are already stored before adding the ones that are
// not.
type ListContentPartsQuery struct {
	Caller Caller
	// NodeID is the item whose parts to read. A work or container has none of
	// its own.
	NodeID NodeID
}

// ListContentPartsResult carries the node's parts in natural order, so the
// segments of a multi-disc edition come back in sequence and a candidate set
// comes back in the order its source ranked it.
type ListContentPartsResult struct {
	Parts []Part
}
