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
// Works (ADR 0013).
type FindContentByExternalIDResult struct {
	Nodes []Node
}

// GetContentNodeQuery reads a single node, optionally with its direct
// children — one level, not a subtree, since variable depth means a caller
// descends deliberately (ADR 0013).
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
