package host

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	goplugin "github.com/hashicorp/go-plugin"

	v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
)

// These tests run the real wire: a real gRPC connection, real protobuf
// serialization, and go-plugin's real broker for the callback direction. What
// they do not do is spawn a process — that is the probe module's job (platform#39's
// build order puts it after this).
//
// The distinction matters when reading a green result here: this proves the
// conversions, the dispatch and the bidirectional call graph. It does not prove
// the handshake or the Unix socket.

// ─── Test doubles ───────────────────────────────────────────────────────────

// stubCapability is a module. It fills one read role, which is deliberate:
// platform#39's step 1 is "a trivial in-repo module implementing one role", and a
// module that filled everything would not exercise the not-implemented path.
type stubCapability struct {
	manifest v1.Manifest

	// importFn lets a test decide what Import does, including failing.
	importFn func(ctx context.Context, svc v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error)
}

func (s *stubCapability) Manifest() v1.Manifest { return s.manifest }

func (s *stubCapability) Import(ctx context.Context, svc v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error) {
	return s.importFn(ctx, svc, req)
}

func (s *stubCapability) Search(_ context.Context, req v1.SearchRequest) (v1.SearchResponse, error) {
	return v1.SearchResponse{Results: []v1.SearchResult{{
		Ref:    v1.ContentRef{Provider: "stub", NativeID: req.Text, MediaType: v1.MediaMovie},
		Title:  "Result for " + req.Text,
		Year:   1982,
		Poster: "https://example.invalid/p.jpg",
	}}}, nil
}

// stubContent is the Platform's ContentService. Only the methods the tests
// exercise do anything; the rest satisfy the interface.
type stubContent struct {
	gotWork   v1.AddContentWorkCommand
	gotSearch v1.SearchContentQuery
	workErr   error
	callerIn  string
}

func (s *stubContent) AddContentWork(_ context.Context, cmd v1.AddContentWorkCommand) (v1.AddContentWorkResult, error) {
	s.gotWork = cmd
	s.callerIn = cmd.Caller.Session
	if s.workErr != nil {
		return v1.AddContentWorkResult{}, s.workErr
	}
	return v1.AddContentWorkResult{Work: v1.Node{
		ID:        "node-1",
		WorkID:    "node-1",
		Kind:      v1.NodeWork,
		MediaType: cmd.MediaType,
		Title:     cmd.Title,
		Status:    v1.NodeActive,
		// Echoed rather than fixed, so the assertion covers the return leg as
		// well as the outbound one: a converter that dropped Genres in only one
		// direction would otherwise pass.
		Genres:    cmd.Genres,
		CreatedAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
	}}, nil
}

func (s *stubContent) AddContentChild(context.Context, v1.AddContentChildCommand) (v1.AddContentChildResult, error) {
	return v1.AddContentChildResult{}, nil
}
func (s *stubContent) AttachContentPart(context.Context, v1.AttachContentPartCommand) (v1.AttachContentPartResult, error) {
	return v1.AttachContentPartResult{}, nil
}
func (s *stubContent) SetContentArtwork(context.Context, v1.SetContentArtworkCommand) (v1.SetContentArtworkResult, error) {
	return v1.SetContentArtworkResult{}, nil
}
func (s *stubContent) RelateContent(context.Context, v1.RelateContentCommand) (v1.RelateContentResult, error) {
	return v1.RelateContentResult{}, nil
}
func (s *stubContent) BindContentSource(context.Context, v1.BindContentSourceCommand) (v1.BindContentSourceResult, error) {
	return v1.BindContentSourceResult{}, nil
}
func (s *stubContent) ResolveContentBinding(context.Context, v1.ResolveContentBindingCommand) (v1.ResolveContentBindingResult, error) {
	return v1.ResolveContentBindingResult{}, nil
}
func (s *stubContent) SearchContent(_ context.Context, q v1.SearchContentQuery) (v1.SearchContentResult, error) {
	s.gotSearch = q
	return v1.SearchContentResult{}, nil
}
func (s *stubContent) FindContentByExternalID(context.Context, v1.FindContentByExternalIDQuery) (v1.FindContentByExternalIDResult, error) {
	return v1.FindContentByExternalIDResult{}, nil
}
func (s *stubContent) GetContentNode(context.Context, v1.GetContentNodeQuery) (v1.GetContentNodeResult, error) {
	return v1.GetContentNodeResult{}, nil
}
func (s *stubContent) ListContentParts(context.Context, v1.ListContentPartsQuery) (v1.ListContentPartsResult, error) {
	return v1.ListContentPartsResult{}, nil
}
func (s *stubContent) RecordPlaybackProgress(context.Context, v1.RecordPlaybackProgressCommand) (v1.RecordPlaybackProgressResult, error) {
	return v1.RecordPlaybackProgressResult{}, nil
}
func (s *stubContent) SetPlaybackFinished(context.Context, v1.SetPlaybackFinishedCommand) (v1.SetPlaybackFinishedResult, error) {
	return v1.SetPlaybackFinishedResult{}, nil
}
func (s *stubContent) GetPlaybackState(context.Context, v1.GetPlaybackStateQuery) (v1.GetPlaybackStateResult, error) {
	return v1.GetPlaybackStateResult{}, nil
}
func (s *stubContent) ListPlaybackStates(context.Context, v1.ListPlaybackStatesQuery) (v1.ListPlaybackStatesResult, error) {
	return v1.ListPlaybackStatesResult{}, nil
}
func (s *stubContent) ListInProgress(context.Context, v1.ListInProgressQuery) (v1.ListInProgressResult, error) {
	return v1.ListInProgressResult{}, nil
}

var _ v1.ContentService = (*stubContent)(nil)

// connect wires both sides over a real gRPC connection with a live broker.
func connect(t *testing.T, impl v1.Capability, content v1.ContentService, categoryOf CategoryFunc) v1.Capability {
	t.Helper()

	p := &Plugin{Impl: impl, Content: content, CategoryOf: categoryOf}
	client, server := goplugin.TestPluginGRPCConn(t, false, map[string]goplugin.Plugin{
		PluginName: p,
	})
	// Stop the server before closing the client, which is the opposite of the
	// obvious order and is why: `client.Close()` sends go-plugin's Shutdown RPC,
	// whose handler calls `GRPCServer.Stop()` on a gRPC handler goroutine while
	// this goroutine calls it too. Both write `GRPCServer.broker`, unsynchronised
	// — a data race inside go-plugin that `-race` catches intermittently on any
	// test using this helper, not only a new one. Stopping first means the
	// Shutdown RPC finds no server to reach and the handler never runs.
	t.Cleanup(func() {
		server.Stop()
		_ = client.Close()
	})

	raw, err := client.Dispense(PluginName)
	if err != nil {
		t.Fatalf("dispense: %v", err)
	}
	cap, ok := raw.(v1.Capability)
	if !ok {
		t.Fatalf("dispensed %T, which does not implement v1.Capability", raw)
	}
	return cap
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestManifestCrossesTheBoundary(t *testing.T) {
	want := v1.Manifest{
		ID:       "stub",
		Version:  "v1.2.3",
		Name:     "Stub Module",
		Provides: []v1.Role{v1.RoleSearch},
	}
	c := connect(t, &stubCapability{manifest: want}, &stubContent{}, nil)

	got := c.Manifest()
	if got.ID != want.ID || got.Version != want.Version || got.Name != want.Name {
		t.Errorf("manifest identity: got %+v, want %+v", got, want)
	}
	if len(got.Provides) != 1 || got.Provides[0] != v1.RoleSearch {
		t.Errorf("provides: got %v, want %v", got.Provides, want.Provides)
	}
}

// The registry holds a v1.Capability and cannot tell a proxy from a local
// struct — the property platform#39 is arranged around.
func TestProxySatisfiesTheCapabilityInterface(t *testing.T) {
	c := connect(t, &stubCapability{manifest: v1.Manifest{ID: "stub"}}, &stubContent{}, nil)
	if _, ok := c.(v1.Capability); !ok {
		t.Fatal("proxy does not satisfy v1.Capability")
	}
	// And every role, unconditionally — which is exactly why Roles() reads the
	// manifest instead of type-asserting.
	if _, ok := c.(v1.SearchProvider); !ok {
		t.Fatal("proxy does not satisfy v1.SearchProvider")
	}
}

func TestRolesComeFromTheManifestNotTypeAssertion(t *testing.T) {
	// The stub fills only Search. A type assertion for a role it does not fill
	// still succeeds against the proxy, so the manifest is the only honest
	// answer.
	c := connect(t, &stubCapability{
		manifest: v1.Manifest{ID: "stub", Provides: []v1.Role{v1.RoleSearch}},
	}, &stubContent{}, nil)

	if _, ok := c.(v1.PlaybackProvider); !ok {
		t.Fatal("precondition: the proxy is expected to satisfy every role interface")
	}
	roles := Roles(c)
	if len(roles) != 1 || roles[0] != v1.RoleSearch {
		t.Fatalf("Roles: got %v, want [search]", roles)
	}
}

// The bidirectional case: the Platform calls Import, and the module calls back
// into ContentService over the broker, within that invocation.
func TestImportCallsBackIntoContentService(t *testing.T) {
	content := &stubContent{}
	impl := &stubCapability{
		manifest: v1.Manifest{ID: "stub"},
		importFn: func(ctx context.Context, svc v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error) {
			out, err := svc.AddContentWork(ctx, v1.AddContentWorkCommand{
				Caller:    req.Caller,
				MediaType: req.Ref.MediaType,
				Title:     "Blade Runner",
			})
			if err != nil {
				return v1.ImportResult{}, err
			}
			return v1.ImportResult{WorkID: out.Work.ID, Items: 1}, nil
		},
	}
	c := connect(t, impl, content, nil)

	got, err := c.Import(context.Background(), nil, v1.ImportRequest{
		Caller:   v1.CallerFromSession("handle-abc"),
		Ref:      v1.ContentRef{Provider: "stub", NativeID: "tt0083658", MediaType: v1.MediaMovie},
		Settings: []byte(`{"key":"value"}`),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got.WorkID != "node-1" || got.Items != 1 {
		t.Errorf("result: got %+v, want WorkID=node-1 Items=1", got)
	}

	// The callback arrived, carrying the Caller handle the Platform minted.
	if content.callerIn != "handle-abc" {
		t.Errorf("caller did not survive the callback: got %q, want %q", content.callerIn, "handle-abc")
	}
	if content.gotWork.Title != "Blade Runner" {
		t.Errorf("callback payload: got title %q", content.gotWork.Title)
	}
	if content.gotWork.MediaType != v1.MediaMovie {
		t.Errorf("media type did not survive: got %q", content.gotWork.MediaType)
	}
}

func TestSettingsCrossUninterpreted(t *testing.T) {
	var gotSettings []byte
	impl := &stubCapability{
		manifest: v1.Manifest{ID: "stub"},
		importFn: func(_ context.Context, _ v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error) {
			gotSettings = req.Settings
			return v1.ImportResult{}, nil
		},
	}
	c := connect(t, impl, &stubContent{}, nil)

	// platform#17 stores module settings as opaque JSON, and platform#39 notes this
	// crossing unchanged is a small vindication of that.
	want := `{"addons":["https://example.invalid/manifest.json"],"nested":{"n":1}}`
	if _, err := c.Import(context.Background(), nil, v1.ImportRequest{
		Caller:   v1.CallerFromSession("h"),
		Settings: []byte(want),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if string(gotSettings) != want {
		t.Errorf("settings: got %q, want %q", gotSettings, want)
	}
}

// A module's error reaches the Platform with its message intact and no
// category — exactly as in process, where CategoryOf maps an uncategorised
// error to Internal.
func TestModuleErrorCrossesWithoutACategory(t *testing.T) {
	impl := &stubCapability{
		manifest: v1.Manifest{ID: "stub"},
		importFn: func(context.Context, v1.ContentService, v1.ImportRequest) (v1.ImportResult, error) {
			return v1.ImportResult{}, errors.New("upstream returned 502")
		},
	}
	c := connect(t, impl, &stubContent{}, nil)

	_, err := c.Import(context.Background(), nil, v1.ImportRequest{Caller: v1.CallerFromSession("h")})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); got == "" {
		t.Fatal("error message was lost")
	}
	if cat := CategoryOfWireError(err); cat != "" {
		t.Errorf("a module error should carry no category, got %q", cat)
	}
}

// A Platform error reaching the module keeps its category, supplied by the
// injected CategoryFunc since this package cannot read the Platform's own
// vocabulary.
func TestPlatformErrorCategorySurvivesTheCallback(t *testing.T) {
	content := &stubContent{workErr: errors.New("node already exists")}
	var seen error
	impl := &stubCapability{
		manifest: v1.Manifest{ID: "stub"},
		importFn: func(ctx context.Context, svc v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error) {
			_, err := svc.AddContentWork(ctx, v1.AddContentWorkCommand{Caller: req.Caller})
			seen = err
			return v1.ImportResult{}, nil
		},
	}
	categoryOf := func(error) string { return "conflict" }
	c := connect(t, impl, content, categoryOf)

	if _, err := c.Import(context.Background(), nil, v1.ImportRequest{Caller: v1.CallerFromSession("h")}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if seen == nil {
		t.Fatal("the module saw no error from the callback")
	}
	if got := CategoryOfWireError(seen); got != "conflict" {
		t.Errorf("category: got %q, want conflict", got)
	}
	if seen.Error() != "node already exists" {
		t.Errorf("message: got %q", seen.Error())
	}
}

func TestSearchRoleRoundTrips(t *testing.T) {
	c := connect(t, &stubCapability{
		manifest: v1.Manifest{ID: "stub", Provides: []v1.Role{v1.RoleSearch}},
	}, &stubContent{}, nil)

	sp, ok := c.(v1.SearchProvider)
	if !ok {
		t.Fatal("proxy is not a SearchProvider")
	}
	out, err := sp.Search(context.Background(), v1.SearchRequest{
		Caller: v1.CallerFromSession("h"),
		Text:   "blade runner",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results: got %d, want 1", len(out.Results))
	}
	got := out.Results[0]
	if got.Title != "Result for blade runner" || got.Year != 1982 {
		t.Errorf("result: got %+v", got)
	}
	if got.Ref.MediaType != v1.MediaMovie || got.Ref.NativeID != "blade runner" {
		t.Errorf("ref did not survive: got %+v", got.Ref)
	}
}

// stubRicher fills the two roles that carry the fields SDK v0.23.0 and v0.24.0
// added. It exists for TestLaterSDKFieldsCrossTheBoundary and returns values
// that are all non-zero, because a zero is what a dropped field looks like.
type stubRicher struct {
	stubCapability
	// gotFilters is what CatalogItems was asked with, so the request leg is
	// asserted too. Every field before this one travelled module-to-Platform;
	// a filter selection travels the other way, and a converter can drop one
	// direction while carrying the other.
	gotFilters map[string]string
}

func (s *stubRicher) Metadata(context.Context, v1.MetadataRequest) (v1.ContentMetadata, error) {
	return v1.ContentMetadata{
		Ref:   v1.ContentRef{Provider: "stub", NativeID: "tt0083658", MediaType: v1.MediaMovie},
		Title: "Blade Runner",
		Cast:  []v1.Person{{Name: "Harrison Ford", Role: "Rick Deckard"}},
		Crew:  []v1.Person{{Name: "Ridley Scott", Role: "Director"}},
		Episodes: []v1.EpisodePreview{{
			Season: 1, Episode: 1, Title: "Pilot", RuntimeMinutes: 51,
		}},
	}, nil
}

func (s *stubRicher) Catalogs(context.Context, v1.CatalogsRequest) (v1.CatalogsResponse, error) {
	return v1.CatalogsResponse{Catalogs: []v1.Catalog{{
		ID: "popular", NativeType: "movie", Name: "Popular",
		Filters: []v1.CatalogFilter{{
			Name: "genre", Label: "Genre",
			Options: []v1.CatalogFilterOption{{Value: "28", Label: "Action"}},
		}},
	}}}, nil
}

func (s *stubRicher) CatalogItems(_ context.Context, req v1.CatalogItemsRequest) (v1.CatalogItemsResponse, error) {
	s.gotFilters = req.Filters
	return v1.CatalogItemsResponse{
		Items:   []v1.CatalogItem{{Ref: v1.ContentRef{Provider: "stub", NativeID: "x"}}},
		HasMore: true,
	}, nil
}

// TestLaterSDKFieldsCrossTheBoundary pins the three fields that were added to
// the SDK with no wire representation to carry them.
//
// ContentMetadata.Crew, EpisodePreview.RuntimeMinutes and
// CatalogItemsResponse.HasMore compile fine against a module.proto that has no
// fields for them, and the converters then drop them: an out-of-process module
// populates one and the Platform receives a zero, with nothing failing, because
// a dropped field is indistinguishable from a source that did not supply one.
// Which is why this asserts on the far side of a real gRPC round trip rather
// than on the converters directly.
//
// The rule this encodes: a field added to the SDK's virtual-content DTOs needs
// a module.proto field and a line in each direction of the converter, and this
// test is where forgetting shows up.
func TestLaterSDKFieldsCrossTheBoundary(t *testing.T) {
	impl := &stubRicher{stubCapability: stubCapability{
		manifest: v1.Manifest{
			ID:       "stub",
			Provides: []v1.Role{v1.RoleMetadata, v1.RoleCatalog},
		},
	}}
	c := connect(t, impl, &stubContent{}, nil)

	mp, ok := c.(v1.MetadataProvider)
	if !ok {
		t.Fatal("proxy is not a MetadataProvider")
	}
	meta, err := mp.Metadata(context.Background(), v1.MetadataRequest{
		Caller: v1.CallerFromSession("h"),
	})
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}

	if len(meta.Crew) != 1 {
		t.Fatalf("Crew did not cross: got %d entries, want 1", len(meta.Crew))
	}
	if meta.Crew[0].Name != "Ridley Scott" || meta.Crew[0].Role != "Director" {
		t.Errorf("Crew garbled: got %+v", meta.Crew[0])
	}
	// Cast alongside it, so a converter that merged the two rather than keeping
	// them separate fails here instead of reading as success.
	if len(meta.Cast) != 1 || meta.Cast[0].Name != "Harrison Ford" {
		t.Errorf("Cast did not survive beside Crew: got %+v", meta.Cast)
	}
	if len(meta.Episodes) != 1 {
		t.Fatalf("Episodes: got %d, want 1", len(meta.Episodes))
	}
	if meta.Episodes[0].RuntimeMinutes != 51 {
		t.Errorf("RuntimeMinutes did not cross: got %d, want 51",
			meta.Episodes[0].RuntimeMinutes)
	}

	cp, ok := c.(v1.CatalogProvider)
	if !ok {
		t.Fatal("proxy is not a CatalogProvider")
	}
	items, err := cp.CatalogItems(context.Background(), v1.CatalogItemsRequest{
		Caller:  v1.CallerFromSession("h"),
		Filters: map[string]string{"genre": "28"},
	})
	if err != nil {
		t.Fatalf("catalog items: %v", err)
	}
	if !items.HasMore {
		t.Error("HasMore did not cross: got false, want true")
	}

	// The filter selection, on the outbound leg. This is the first field on
	// this request that a caller sets rather than a provider answers, so it is
	// the first that a converter could drop in the direction the others do not
	// travel in.
	if got := impl.gotFilters["genre"]; got != "28" {
		t.Errorf("CatalogItemsRequest.Filters did not cross: got %q, want %q", got, "28")
	}

	// And the filter declaration, on the way back.
	cats, err := cp.Catalogs(context.Background(), v1.CatalogsRequest{Caller: v1.CallerFromSession("h")})
	if err != nil {
		t.Fatalf("catalogs: %v", err)
	}
	if len(cats.Catalogs) != 1 {
		t.Fatalf("catalogs: got %d, want 1", len(cats.Catalogs))
	}
	filters := cats.Catalogs[0].Filters
	if len(filters) != 1 || filters[0].Name != "genre" || filters[0].Label != "Genre" {
		t.Fatalf("Catalog.Filters did not cross: got %+v", filters)
	}
	if len(filters[0].Options) != 1 || filters[0].Options[0].Value != "28" || filters[0].Options[0].Label != "Action" {
		t.Errorf("CatalogFilter.Options did not cross: got %+v", filters[0].Options)
	}
}

// TestGenresCrossTheContentBoundary pins the library half of M2 slice 4 to the
// same rule, on the other service: the writes a module makes back into the
// Platform.
//
// A genre that does not survive this trip is the worst shape the failure has.
// Nothing errors, the import succeeds, the node is written, and the facet the
// whole slice exists for silently offers fewer titles than the library holds —
// an omission from a filtered list, which is the one kind of wrong answer a
// user cannot see.
func TestGenresCrossTheContentBoundary(t *testing.T) {
	content := &stubContent{}
	var committed v1.Node
	impl := &stubCapability{
		manifest: v1.Manifest{ID: "stub"},
		importFn: func(ctx context.Context, svc v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error) {
			out, err := svc.AddContentWork(ctx, v1.AddContentWorkCommand{
				Caller:    req.Caller,
				MediaType: req.Ref.MediaType,
				Title:     "Blade Runner",
				Genres:    []string{"Science Fiction", "Thriller"},
			})
			if err != nil {
				return v1.ImportResult{}, err
			}
			committed = out.Work
			if _, err := svc.SearchContent(ctx, v1.SearchContentQuery{
				Caller: req.Caller, Genres: []string{"Science Fiction"},
			}); err != nil {
				return v1.ImportResult{}, err
			}
			return v1.ImportResult{WorkID: out.Work.ID, Items: 1}, nil
		},
	}
	c := connect(t, impl, content, nil)

	if _, err := c.Import(context.Background(), nil, v1.ImportRequest{
		Caller: v1.CallerFromSession("h"),
		Ref:    v1.ContentRef{Provider: "stub", NativeID: "tt0083658", MediaType: v1.MediaMovie},
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	if got := content.gotWork.Genres; len(got) != 2 || got[0] != "Science Fiction" || got[1] != "Thriller" {
		t.Errorf("AddContentWorkCommand.Genres did not cross: got %q", got)
	}
	// The return leg: Node.Genres, read back on the committed work.
	if got := committed.Genres; len(got) != 2 || got[0] != "Science Fiction" {
		t.Errorf("Node.Genres did not cross back: got %q", got)
	}
	if got := content.gotSearch.Genres; len(got) != 1 || got[0] != "Science Fiction" {
		t.Errorf("SearchContentQuery.Genres did not cross: got %q", got)
	}
}

// stubQuietCatalog fills RoleCatalog and never mentions HasMore — a provider
// written before the field existed.
type stubQuietCatalog struct{ stubCapability }

func (s *stubQuietCatalog) Catalogs(context.Context, v1.CatalogsRequest) (v1.CatalogsResponse, error) {
	return v1.CatalogsResponse{}, nil
}

func (s *stubQuietCatalog) CatalogItems(context.Context, v1.CatalogItemsRequest) (v1.CatalogItemsResponse, error) {
	return v1.CatalogItemsResponse{
		Items: []v1.CatalogItem{{Ref: v1.ContentRef{Provider: "stub", NativeID: "x"}}},
	}, nil
}

// A provider that says nothing about HasMore must still read as "last page" on
// the far side. That is the compatibility claim the SDK's doc comment makes —
// the zero value is the old behaviour — and it is worth a test because the
// field is a bool: a converter that inverted it would pass the test above.
func TestHasMoreDefaultsToLastPage(t *testing.T) {
	c := connect(t, &stubQuietCatalog{stubCapability{
		manifest: v1.Manifest{ID: "stub", Provides: []v1.Role{v1.RoleCatalog}},
	}}, &stubContent{}, nil)

	cp, ok := c.(v1.CatalogProvider)
	if !ok {
		t.Fatal("proxy is not a CatalogProvider")
	}
	out, err := cp.CatalogItems(context.Background(), v1.CatalogItemsRequest{
		Caller: v1.CallerFromSession("h"),
	})
	if err != nil {
		t.Fatalf("catalog items: %v", err)
	}
	if out.HasMore {
		t.Error("HasMore: got true from a provider that never set it")
	}
}

// stubSource fills the two source roles SDK v0.26.0 grew fields on. Like
// stubRicher it answers with values that are all non-zero, because a zero is
// what a dropped field looks like — and it records the SubtitlesRequest it was
// handed, because two of the five fields travel Platform-to-module and only the
// module can report that they arrived.
// The mutex is not ceremony. The request is recorded on the gRPC server's
// goroutine and read on the test's, and the RPC's return is not an edge the race
// detector recognises across go-plugin's in-memory connection — unguarded, this
// trips `-race` intermittently rather than never.
type stubSource struct {
	stubCapability

	mu           sync.Mutex
	gotSubtitles v1.SubtitlesRequest
}

func (s *stubSource) subtitlesRequest() v1.SubtitlesRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gotSubtitles
}

func (s *stubSource) Streams(_ context.Context, _ v1.StreamRequest) (v1.StreamResponse, error) {
	return v1.StreamResponse{Streams: []v1.StreamLink{{
		Label:      "Torrentio",
		Title:      "Blade.Runner.1982.2160p.UHD.BluRay.x265.DTS-HD.MA.5.1",
		Quality:    "2160p",
		SizeBytes:  21_474_836_480,
		Seeders:    412,
		Location:   v1.MediaLocation{Scheme: v1.RemoteLocation, Provider: "stub", Ref: "magnet:?xt=stub"},
		Container:  "mkv",
		VideoCodec: "hevc",
		AudioCodec: "dts",
	}}}, nil
}

func (s *stubSource) Subtitles(_ context.Context, req v1.SubtitlesRequest) (v1.SubtitlesResponse, error) {
	s.mu.Lock()
	s.gotSubtitles = req
	s.mu.Unlock()
	return v1.SubtitlesResponse{Subtitles: []v1.Subtitle{{
		Language: "eng",
		URL:      "https://example.invalid/s01e02.srt",
		ID:       "sub-1",
	}}}, nil
}

// TestStreamAndSubtitleFieldsCrossTheBoundary pins SDK v0.26.0's five fields to
// the rule TestLaterSDKFieldsCrossTheBoundary encodes, on the two source roles
// that carry them.
//
// StreamLink.Container, .VideoCodec and .AudioCodec, and SubtitlesRequest.Season
// and .Episode, compile perfectly against a module.proto with no fields to hold
// them, and the converters then drop them with no error anywhere: a module fills
// a codec and the Platform receives an empty string, indistinguishable from a
// source that did not parse one. Which is why this asserts on the far side of a
// real gRPC round trip rather than on the converters.
//
// The two directions are not the same test. The three codec fields travel
// module-to-Platform and are asserted on what the caller receives; the two
// subtitle coordinates travel Platform-to-module and can only be asserted by the
// module reporting what it was handed. A converter that dropped one direction
// while carrying the other would pass half of this.
func TestStreamAndSubtitleFieldsCrossTheBoundary(t *testing.T) {
	impl := &stubSource{stubCapability: stubCapability{
		manifest: v1.Manifest{
			ID:       "stub",
			Provides: []v1.Role{v1.RoleStream, v1.RoleSubtitles},
		},
	}}
	c := connect(t, impl, &stubContent{}, nil)

	sp, ok := c.(v1.StreamProvider)
	if !ok {
		t.Fatal("proxy is not a StreamProvider")
	}
	streams, err := sp.Streams(context.Background(), v1.StreamRequest{
		Caller: v1.CallerFromSession("h"), Season: 1, Episode: 2,
	})
	if err != nil {
		t.Fatalf("streams: %v", err)
	}
	if len(streams.Streams) != 1 {
		t.Fatalf("streams: got %d, want 1", len(streams.Streams))
	}
	link := streams.Streams[0]

	if link.Container != "mkv" {
		t.Errorf("Container did not cross: got %q, want %q", link.Container, "mkv")
	}
	if link.VideoCodec != "hevc" {
		t.Errorf("VideoCodec did not cross: got %q, want %q", link.VideoCodec, "hevc")
	}
	if link.AudioCodec != "dts" {
		t.Errorf("AudioCodec did not cross: got %q, want %q", link.AudioCodec, "dts")
	}
	// The fields that already crossed, asserted beside the new ones: three
	// strings appended to one message is exactly the shape a converter garbles
	// by pairing the wrong field with the wrong value, and that reads as success
	// unless something pins the originals too.
	if link.Label != "Torrentio" || link.Quality != "2160p" || link.Seeders != 412 {
		t.Errorf("existing StreamLink fields did not survive beside the new ones: got %+v", link)
	}
	if link.SizeBytes != 21_474_836_480 {
		t.Errorf("SizeBytes: got %d", link.SizeBytes)
	}
	if link.Location.Scheme != v1.RemoteLocation || link.Location.Ref != "magnet:?xt=stub" {
		t.Errorf("Location did not survive: got %+v", link.Location)
	}

	sub, ok := c.(v1.SubtitlesProvider)
	if !ok {
		t.Fatal("proxy is not a SubtitlesProvider")
	}
	subs, err := sub.Subtitles(context.Background(), v1.SubtitlesRequest{
		Caller:  v1.CallerFromSession("h"),
		Ref:     v1.ContentRef{ExternalScheme: "imdb", ExternalID: "tt0903747", MediaType: v1.MediaTVSeries},
		Season:  1,
		Episode: 2,
	})
	if err != nil {
		t.Fatalf("subtitles: %v", err)
	}

	// The outbound leg. Nothing the caller can see reports this, so the module
	// has to: a request that arrives with two zeroes is precisely what the
	// provider used to be given, and it resolves subtitles for the wrong episode
	// without erroring.
	gotReq := impl.subtitlesRequest()
	if got := gotReq.Season; got != 1 {
		t.Errorf("SubtitlesRequest.Season did not cross: got %d, want 1", got)
	}
	if got := gotReq.Episode; got != 2 {
		t.Errorf("SubtitlesRequest.Episode did not cross: got %d, want 2", got)
	}
	// Beside them, the ref they qualify — coordinates that arrived without the
	// identity they belong to would address nothing.
	if got := gotReq.Ref.ExternalID; got != "tt0903747" {
		t.Errorf("SubtitlesRequest.Ref did not survive beside the coordinates: got %q", got)
	}

	// And the return leg of the same call, so the role is proven whole rather
	// than only in the direction the new fields travel.
	if len(subs.Subtitles) != 1 {
		t.Fatalf("subtitles: got %d, want 1", len(subs.Subtitles))
	}
	if subs.Subtitles[0].Language != "eng" || subs.Subtitles[0].ID != "sub-1" {
		t.Errorf("Subtitle did not survive: got %+v", subs.Subtitles[0])
	}
}

// A role the module does not fill is refused rather than silently returning
// nothing, so a Platform bug shows up as an error instead of an empty result.
func TestUnfilledRoleIsRefused(t *testing.T) {
	c := connect(t, &stubCapability{manifest: v1.Manifest{ID: "stub"}}, &stubContent{}, nil)

	ap, ok := c.(v1.ArtworkProvider)
	if !ok {
		t.Fatal("proxy is not an ArtworkProvider")
	}
	if _, err := ap.Artwork(context.Background(), v1.ArtworkRequest{
		Caller: v1.CallerFromSession("h"),
	}); err == nil {
		t.Fatal("expected an error for a role the module does not fill")
	}
}
