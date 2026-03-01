package server

import (
	"fmt"
	"net/http"
)

// WriteEvent writes a single SSE event to the response.
func WriteEvent(w http.ResponseWriter, event, data string) error {
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
