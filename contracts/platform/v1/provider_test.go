package v1_test

import (
	"context"
	"testing"

	v1 "github.com/mosaic-media/sdk/contracts/platform/v1"
)

// stubProvider is a module that fills all four provider roles, plus the base
// Capability. It exists to prove the provider surface holds together and is
// satisfiable from outside the package with no Platform types — the same
// guarantee capability_test.go makes for the base interface.
type stubProvider struct{}

func (stubProvider) Manifest() v1.Manifest {
	return v1.Manifest{
		ID: "stub", Version: "0.0.1", Name: "Stub",
		Provides: []v1.Role{v1.RoleMetadata, v1.RoleSearch, v1.RoleCatalog, v1.RoleStream},
	}
}

func (stubProvider) Import(_ context.Context, _ v1.ContentService, req v1.ImportRequest) (v1.ImportResult, error) {
	return v1.ImportResult{WorkID: v1.NodeID("work-" + req.Ref.NativeID)}, nil
}

func (stubProvider) Metadata(_ context.Context, req v1.MetadataRequest) (v1.ContentMetadata, error) {
	return v1.ContentMetadata{Ref: req.Ref, Title: "Blade Runner 2049", Year: 2017}, nil
}

func (stubProvider) Search(_ context.Context, req v1.SearchRequest) (v1.SearchResponse, error) {
	return v1.SearchResponse{Results: []v1.SearchResult{{
		Ref:   v1.ContentRef{Provider: "stub", NativeID: "tt1856101", NativeType: "movie", MediaType: v1.MediaMovie, ExternalScheme: "imdb", ExternalID: "tt1856101"},
		Title: req.Text,
	}}}, nil
}

func (stubProvider) Catalogs(_ context.Context, _ v1.CatalogsRequest) (v1.CatalogsResponse, error) {
	return v1.CatalogsResponse{Catalogs: []v1.Catalog{{ID: "top", NativeType: "movie", Name: "Popular"}}}, nil
}

func (stubProvider) CatalogItems(_ context.Context, req v1.CatalogItemsRequest) (v1.CatalogItemsResponse, error) {
	return v1.CatalogItemsResponse{Items: []v1.CatalogItem{{
		Ref:   v1.ContentRef{Provider: "stub", NativeID: "tt1254207", NativeType: req.NativeType, MediaType: v1.MediaMovie},
		Title: "Big Buck Bunny",
	}}}, nil
}

func (stubProvider) Streams(_ context.Context, _ v1.StreamRequest) (v1.StreamResponse, error) {
	return v1.StreamResponse{Streams: []v1.StreamLink{{
		Label:    "1080p",
		Location: v1.MediaLocation{Scheme: v1.RemoteLocation, Provider: "stub", Ref: "magnet:?xt=urn:btih:abc"},
	}}}, nil
}

// TestProviderRolesImplementableExternally checks that a single value satisfies
// the base Capability and every provider role, that Manifest.Provides carries
// the declared roles, and that each role returns its virtual type by value.
func TestProviderRolesImplementableExternally(t *testing.T) {
	var (
		cap  v1.Capability       = stubProvider{}
		meta v1.MetadataProvider = stubProvider{}
		srch v1.SearchProvider   = stubProvider{}
		cat  v1.CatalogProvider  = stubProvider{}
		strm v1.StreamProvider   = stubProvider{}
	)

	if got := cap.Manifest().Provides; len(got) != 4 {
		t.Fatalf("Manifest().Provides = %v, want 4 roles", got)
	}

	ctx := context.Background()
	if m, _ := meta.Metadata(ctx, v1.MetadataRequest{}); m.Year != 2017 {
		t.Fatalf("Metadata Year = %d, want 2017", m.Year)
	}
	if r, _ := srch.Search(ctx, v1.SearchRequest{Text: "blade"}); r.Results[0].Title != "blade" {
		t.Fatalf("Search echoed title = %q, want %q", r.Results[0].Title, "blade")
	}
	if c, _ := cat.Catalogs(ctx, v1.CatalogsRequest{}); c.Catalogs[0].ID != "top" {
		t.Fatalf("Catalogs[0].ID = %q, want %q", c.Catalogs[0].ID, "top")
	}
	if s, _ := strm.Streams(ctx, v1.StreamRequest{}); s.Streams[0].Location.Scheme != v1.RemoteLocation {
		t.Fatalf("Streams[0] scheme = %q, want %q", s.Streams[0].Location.Scheme, v1.RemoteLocation)
	}
}
