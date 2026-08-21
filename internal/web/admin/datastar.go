package admin

import (
	"html"
	"net/http"
	"strings"
)

// sseEvent is a single Datastar SSE event encoded as raw event-stream text.
type sseEvent struct {
	name string
	data []string // data lines (without the "data: " prefix)
}

// writeSSE streams one or more Datastar SSE events to the client. It sets the
// required text/event-stream headers and flushes so fragments arrive without
// buffering. This is the transport layer for every admin fragment update: the
// server stays the source of truth and Datastar only patches the DOM.
func writeSSE(w http.ResponseWriter, events ...sseEvent) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	for _, e := range events {
		w.Write([]byte("event: " + e.name + "\n"))
		for _, line := range e.data {
			w.Write([]byte("data: " + oneline(line) + "\n"))
		}
		w.Write([]byte("\n"))
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// patchElementsEvent builds a datastar-patch-elements event. When selector is
// empty the top-level element(s) in html are matched by their id (Datastar's
// default morph behaviour); otherwise selector targets an existing element.
// mode defaults to "outer" when empty.
func patchElementsEvent(mode, selector, html string) sseEvent {
	data := []string{}
	if selector != "" {
		data = append(data, "selector "+selector)
	}
	if mode != "" && mode != "outer" {
		data = append(data, "mode "+mode)
	}
	data = append(data, "elements "+html)
	return sseEvent{name: "datastar-patch-elements", data: data}
}

// toastEvent builds a datastar-patch-elements event that appends a toast to
// the shared #admin-toast-region. The element removes itself shortly after it
// is patched into the DOM (data-init runs on mount), so no client timer or
// signal bookkeeping is required.
func toastEvent(kind, message string) sseEvent {
	msg := html.EscapeString(message)
	toast := `<div class="toast toast-` + kind + `" role="status" data-init="setTimeout(() => el.remove(), 4500)">` +
		`<span class="toast-message">` + msg + `</span>` +
		`<button type="button" class="toast-close" aria-label="Dismiss" onclick="this.parentElement.remove()">&times;</button>` +
		`</div>`
	return patchElementsEvent("append", "#admin-toast-region", toast)
}

// oneline collapses newlines so a fragment fits on a single SSE data line.
func oneline(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}
