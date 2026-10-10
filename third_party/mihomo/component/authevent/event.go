// Package authevent exposes bounded metadata about rejected authentication to
// embedders. Credentials, URLs, headers and user payloads are never included.
package authevent

import "sync/atomic"

type Event struct{ Protocol, Local, Remote, Reason, Method string }
type handler struct{ fn func(Event) }

var sink atomic.Pointer[handler]

// SetHandler installs a callback. It must return immediately; embedders should
// enqueue events in a bounded queue rather than do I/O on handshake goroutines.
func SetHandler(fn func(Event)) {
	if fn == nil {
		sink.Store(nil)
	} else {
		sink.Store(&handler{fn})
	}
}
func Emit(e Event) {
	if h := sink.Load(); h != nil {
		h.fn(e)
	}
}
