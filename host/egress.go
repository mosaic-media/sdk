package host

import (
	"net/http"
	"net/url"
	"os"
)

// EgressProxyEnv is the environment variable the Platform sets to the address of
// the forward proxy a module's egress must go through (ADR 0064). The Platform
// sets it when it spawns the module; a module never sets it.
const EgressProxyEnv = "MOSAIC_EGRESS_PROXY"

// configureEgressProxy wires the process's default HTTP transport through the
// Platform's egress proxy, and is why ADR 0064 can call sdk/host's client
// "pre-wired to the proxy" rather than merely env-configured.
//
// The standard HTTP_PROXY/HTTPS_PROXY variables are not enough on their own, and
// the gap is exactly the target that matters most. Go's ProxyFromEnvironment
// hardcodes a bypass for "localhost" and loopback addresses — so a module using
// an ordinary client would reach 127.0.0.1 (the Platform's own PostgreSQL, say)
// *directly*, around the proxy and around its deny list. Setting the transport's
// Proxy to a function that always returns the proxy URL has no such exception:
// every request, loopback included, goes through the proxy, where the deny list
// decides.
//
// It mutates http.DefaultTransport in place. That is safe here because it runs
// at startup before any request is in flight, and it is the whole point: a
// module builds an ordinary `&http.Client{}` with a nil Transport, which reads
// http.DefaultTransport at request time — so mutating the default is what covers
// a module that never thought about the proxy at all. A module that builds a
// fully custom Transport with an explicit nil Proxy can still bypass this; that
// residual gap is what OS-level network denial (ADR 0064's layer 3) closes, and
// this does not claim to.
func configureEgressProxy() {
	addr := os.Getenv(EgressProxyEnv)
	if addr == "" {
		return
	}
	u, err := url.Parse(addr)
	if err != nil {
		return
	}
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}
	t.Proxy = func(*http.Request) (*url.URL, error) { return u, nil }
}
