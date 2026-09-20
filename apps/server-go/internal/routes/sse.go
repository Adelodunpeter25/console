// SSE helpers built on fasthttp's StreamWriter: fiber.Ctx must not be
// written from goroutines after the handler returns, so the stream body
// writer is the only valid output handle.
package routes

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

type sseStream struct {
	w *bufio.Writer
}

func (s *sseStream) Send(event, data string) error {
	_, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, data)
	if err != nil {
		return err
	}
	return s.w.Flush()
}

// streamSSE sets SSE headers and runs fn inside the response stream writer;
// fn must exit when Send reports an error (client gone).
func streamSSE(c *fiber.Ctx, fn func(*sseStream)) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		fn(&sseStream{w: w})
	})
	return nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
