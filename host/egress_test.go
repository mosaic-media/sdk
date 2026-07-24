package host

import (
	"net/http"
	"net/url"
	"testing"
)

// configureEgressProxy forces the default transport through the proxy for
// *every* host, including loopback — which is the case standard HTTP_PROXY
// handling deliberately excludes and the one that matters most.
func TestForcedProxyCoversLoopback(t *testing.T) {
	original, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Skip("default transport is not *http.Transport")
	}
	// Restore the transport's Proxy after the test, since it is a process
	// global.
	originalProxy := original.Proxy
	t.Cleanup(func() { original.Proxy = originalProxy })

	t.Setenv(EgressProxyEnv, "http://127.0.0.1:9999")
	configureEgressProxy()

	if original.Proxy == nil {
		t.Fatal("the default transport's Proxy was not set")
	}

	// A request to a loopback address must still resolve to the proxy — the
	// exclusion ProxyFromEnvironment applies is exactly what this overrides.
	loopbackReq, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:5432/", nil)
	got, err := original.Proxy(loopbackReq)
	if err != nil {
		t.Fatalf("proxy func errored: %v", err)
	}
	if got == nil {
		t.Fatal("a loopback request was not routed through the proxy — the SSRF gap is open")
	}
	if want := (&url.URL{Scheme: "http", Host: "127.0.0.1:9999"}); got.String() != want.String() {
		t.Errorf("proxy URL: got %s, want %s", got, want)
	}
}

// With no proxy configured, the transport is left alone.
func TestNoProxyEnvLeavesTheTransportAlone(t *testing.T) {
	original, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Skip("default transport is not *http.Transport")
	}
	originalProxy := original.Proxy
	t.Cleanup(func() { original.Proxy = originalProxy })

	// Distinguish "left alone" from "set to nil": install a sentinel and check
	// it survives.
	sentinel := func(*http.Request) (*url.URL, error) { return nil, nil }
	original.Proxy = sentinel

	t.Setenv(EgressProxyEnv, "")
	configureEgressProxy()

	// A nil env means no change; the sentinel must still be there. Comparing
	// funcs directly is not allowed, so check behaviour: the sentinel returns a
	// nil URL and no error for any request, which a forced-proxy func would not.
	got, err := original.Proxy(&http.Request{URL: &url.URL{Scheme: "http", Host: "example.com"}})
	if err != nil || got != nil {
		t.Error("configureEgressProxy changed the transport when no proxy was set")
	}
}
