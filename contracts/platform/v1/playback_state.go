package v1

import "time"

// Playback state (ADR 0046): where a viewer stopped, what they finished, and
// what they should be offered next.
//
// It is the one genuinely per-user thing in the content model. Content, its
// tree and its candidate releases are install-global, and even a resolved
// stream URL belongs to the bytes and the screen rather than to the viewer —
// but a position belongs to a person, and to nothing else.
//
// It is Platform-owned rather than module-owned, and the reason is the product
// rather than the layering. State written by a playback module would die when
// the module was swapped, be invisible to a local-file player, and be
// unreadable by anything that has no business resolving bytes and every
// business reading progress: an export writing an NFO `<watched>`, a request
// module deciding what to acquire next, a recommendation.

// PlaybackState is one viewer's position in one item.
type PlaybackState struct {
	// NodeID is the item being watched. State is keyed by node and not by
	// Part, because a viewer resumes *an episode* — not the 1080p release of an
	// episode they happened to start with. Watching half from one source and
	// finishing from another is one position, not two.
	NodeID NodeID
	// PartID is the release last played, recorded so a resume returns to the
	// same encode.
	//
	// That matters more than it sounds: two encodes of one film differ by
	// however much their intros differ, so resuming a different release lands
	// the viewer at the wrong moment. It is a hint rather than a requirement —
	// a device that cannot play that release falls back to one it can, and the
	// small drift is accepted knowingly.
	PartID PartID

	// Position is how far in the viewer had got.
	Position time.Duration
	// Duration is the item's length as the player reported it, 0 when unknown.
	// It comes from the client rather than the Part because the Part's own
	// duration is frequently absent and the player always knows.
	Duration time.Duration

	// Finished marks the item watched.
	Finished bool
	// FinishedExplicit records that a person said so, rather than a threshold
	// deciding it.
	//
	// The distinction is what stops the two mechanisms fighting. Deriving alone
	// gets credits wrong — someone who stops before them has not finished, and
	// someone who leaves them running has. Manual alone is a chore nobody does.
	// So a threshold decides by default and a person overrides, in either
	// direction, and their answer is never re-derived away.
	FinishedExplicit bool

	// UpdatedAt is when the position last moved. It orders the
	// continue-watching list, which is why it is the state's own timestamp
	// rather than a row-modified column.
	UpdatedAt time.Time
}

// InProgress reports whether this state belongs in a continue-watching list: it
// has started, and it has not finished.
//
// Position alone is not enough. A finished item keeps its position — that is
// what makes "finished" recoverable rather than destructive — so a list built
// from position would show everything anyone had ever completed.
func (s PlaybackState) InProgress() bool {
	return !s.Finished && s.Position > 0
}

// ResumeAt is the position to start from.
//
// A finished item resumes at the beginning rather than at its stored position,
// which is a deliberate departure from "resume where you stopped": the stored
// position of a finished film is thirty seconds from the end, and starting a
// rewatch there is never what anyone meant.
func (s PlaybackState) ResumeAt() time.Duration {
	if s.Finished {
		return 0
	}
	return s.Position
}

// RecordPlaybackProgressCommand reports where a viewer has got to.
//
// It is sent repeatedly during playback — on a slow cadence and at meaningful
// boundaries — so it is an upsert rather than a create, and the Platform
// coalesces bursts of it rather than writing once per second.
type RecordPlaybackProgressCommand struct {
	Caller Caller
	NodeID NodeID
	// PartID is the release being played, recorded alongside the position.
	PartID PartID
	// Position is how far in the player has reached.
	Position time.Duration
	// Duration is the item's length, 0 when the player does not know it yet.
	Duration time.Duration
}

// RecordPlaybackProgressResult carries the committed state, including whether
// this report crossed the completion threshold.
type RecordPlaybackProgressResult struct {
	State PlaybackState
}

// SetPlaybackFinishedCommand marks an item watched or unwatched explicitly.
//
// It is a separate command rather than a field on progress because it is a
// different act: progress is something a player observes, and this is something
// a person decides. Conflating them would make every position report capable of
// silently un-marking something.
type SetPlaybackFinishedCommand struct {
	Caller   Caller
	NodeID   NodeID
	Finished bool
}

// SetPlaybackFinishedResult carries the committed state.
type SetPlaybackFinishedResult struct {
	State PlaybackState
}

// GetPlaybackStateQuery reads one viewer's position in one item.
type GetPlaybackStateQuery struct {
	Caller Caller
	NodeID NodeID
}

// GetPlaybackStateResult carries the state, and whether there was one.
//
// Found distinguishes "never started" from "started and at zero", which the
// zero value cannot. A detail screen shows Play for the first and Restart for
// the second.
type GetPlaybackStateResult struct {
	State PlaybackState
	Found bool
}

// ListPlaybackStatesQuery reads a viewer's state for several items at once.
//
// It exists because the alternative is a query per row. A season with twenty
// episodes would otherwise ask twenty times to draw twenty watched marks, which
// is the shape that makes a screen slow in a way no single measurement finds.
type ListPlaybackStatesQuery struct {
	Caller  Caller
	NodeIDs []NodeID
}

// ListPlaybackStatesResult carries the states that exist, keyed by node. Nodes
// with no state are absent rather than present-and-zero.
type ListPlaybackStatesResult struct {
	States map[NodeID]PlaybackState
}

// ListInProgressQuery reads what a viewer has started and not finished, most
// recently touched first.
type ListInProgressQuery struct {
	Caller Caller
	// Limit caps the result, 0 for the service's own default. A
	// continue-watching rail is a rail, not an archive.
	Limit int
}

// InProgressItem pairs a position with the item it belongs to.
//
// The Node travels with the state because every caller needs both and fetching
// them separately is a query per row — the continue-watching rail's whole job
// is to render items, and a list of bare node ids would make it do the join a
// second time.
type InProgressItem struct {
	Node  Node
	State PlaybackState
}

// ListInProgressResult carries the continue-watching list in order.
type ListInProgressResult struct {
	Items []InProgressItem
}
