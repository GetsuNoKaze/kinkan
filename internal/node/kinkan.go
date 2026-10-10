package node

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/component/authevent"

	"mikan/internal/nodeapi"
	"mikan/internal/scannerlog"
)

// Kinkan's part of the engine. Mikan's files call into it with one line each, so that
// merges from upstream meet as few of the fork's lines as possible.

// kinkanEngine is embedded in Engine.
type kinkanEngine struct {
	site           *siteServer // the website the node shows (kinkan_site.go), under e.mu
	scanners       *scannerlog.Journal
	scannerEvents  chan authevent.Event
	scannerDropped atomic.Int64
}

// kinkanApply runs at the end of Apply, still under e.mu: a state applied without an
// error brings the node's site in line with it.
func (e *Engine) kinkanApply(st nodeapi.DesiredState, res *nodeapi.ApplyResult, err *error) {
	if *err == nil {
		res.SiteMissing = e.applySite(st)
	}
}

// kinkanHealth adds what the node's site is to its health.
func (e *Engine) kinkanHealth(h *nodeapi.Health) {
	e.mu.Lock()
	h.Site = e.siteStatus()
	e.mu.Unlock()
}

// ownSite says whether a REALITY dest is the node's own website on loopback.
func (e *Engine) ownSite(host, port string) bool {
	e.mu.Lock()
	own := sitePort(e.applied)
	e.mu.Unlock()
	return own > 0 && (host == "127.0.0.1" || host == "localhost") && port == strconv.Itoa(own)
}

// persistScanners saves the scanner journal along with the counters.
func (e *Engine) persistScanners() {
	if e.scanners == nil {
		return
	}
	if err := e.scanners.Persist(time.Now()); err != nil {
		e.log.Warn("persist scanner journal", "err", err)
	}
}

// kinkanHandlers are the fork's Node API endpoints.
func kinkanHandlers(mux *http.ServeMux, e *Engine, log *slog.Logger) {
	mux.HandleFunc("GET /v1/scanners", func(w http.ResponseWriter, r *http.Request) {
		s, err := e.Scanners(r.Context())
		if err != nil {
			fail(w, log, err)
			return
		}
		writeJSON(w, http.StatusOK, s)
	})
	mux.HandleFunc("PUT /v1/site/{hash}", siteHandler(e, log))
}
