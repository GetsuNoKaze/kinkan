package trusttunnel

import "github.com/metacubex/http"

// Opt in only this TCP front. The patched HTTP server owns the experiment,
// randomness and environment switch; upstream HTTP also accepts this wrapper.
type kinkanPaddingHandler struct{ http.Handler }

func (kinkanPaddingHandler) KinkanTrustTunnelHTTP2Padding() bool { return true }

func kinkanWithPadding(handler http.Handler) http.Handler {
	return kinkanPaddingHandler{Handler: handler}
}
