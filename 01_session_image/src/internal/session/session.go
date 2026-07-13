// Package session is the driver's flow for one live phone call: attach the
// call's sideband socket, speak the greeting, then hold the line — logging
// the session's events and re-checking the caller against the allowlist —
// until the call ends. Tool handling (function_call events, the Cassandra
// bridge) will grow here.
package session

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/paulcdejean/veronica/session/internal/gcp"
	"github.com/paulcdejean/veronica/session/internal/realtime"
)

// The SIP answer and Twilio's bridging take a beat to connect the caller's
// audio path after accept; a greeting spoken too early reaches the caller
// clipped ("...ronica speaking"). The job's own startup already adds a few
// seconds, so this is a small extra margin.
const greetingSettle = 1 * time.Second

// Realtime sessions cap at 60 minutes; leave on our own terms before then.
const sessionCap = 55 * time.Minute

// How often to re-check the caller against the allowlist mid-call.
const allowlistInterval = 1 * time.Minute

// Config is the identity of the one call this process drives.
type Config struct {
	CallID   string
	Caller   string
	APIKey   string
	Greeting string
}

// Run drives the call until it ends, the context is cancelled, or the
// session cap is reached.
func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithTimeout(ctx, sessionCap)
	defer cancel()

	project, err := gcp.ProjectID(ctx)
	if err != nil {
		return fmt.Errorf("reading project id: %w", err)
	}

	conn, err := realtime.Attach(ctx, cfg.CallID, cfg.APIKey)
	if err != nil {
		return fmt.Errorf("attaching to call %s: %w", cfg.CallID, err)
	}
	defer conn.Close()
	log.Printf("attached to call %s from %s", cfg.CallID, cfg.Caller)

	// Attaching does not replay session.created, so nudge the server into
	// echoing session.updated (a no-op update) to get the effective
	// instructions into the logs.
	if err := conn.Send(ctx, map[string]any{
		"type":    "session.update",
		"session": map[string]any{"type": "realtime"},
	}); err != nil {
		return fmt.Errorf("sending session probe: %w", err)
	}

	select {
	case <-time.After(greetingSettle):
	case <-ctx.Done():
		return nil
	}

	// The model only speaks when a response is created, so nudge the
	// greeting out rather than waiting for the caller's "hello?". The
	// token cap keeps the greeting from growing a monologue; output audio
	// runs ~20 tokens per spoken second, so 100 is ~5 seconds of speech.
	if err := conn.Send(ctx, map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"instructions":      fmt.Sprintf("Say exactly this and nothing more: %q", cfg.Greeting),
			"max_output_tokens": 100,
		},
	}); err != nil {
		return fmt.Errorf("sending greeting: %w", err)
	}

	events := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		defer close(events)
		for {
			data, err := conn.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
			events <- data
		}
	}()

	allowlist := time.NewTicker(allowlistInterval)
	defer allowlist.Stop()

	for {
		select {
		case data, ok := <-events:
			if !ok {
				err := <-readErr
				if ctx.Err() != nil {
					// Cancelled or hit the session cap; the deferred
					// close drops the socket, the call survives unless
					// OpenAI ends it at the 60-minute mark anyway.
					log.Printf("detaching: %v", context.Cause(ctx))
					return nil
				}
				// The server closing the socket is how a finished call
				// looks from here.
				log.Printf("call %s ended (%v)", cfg.CallID, err)
				return nil
			}
			handleEvent(data)

		case <-allowlist.C:
			allowed, err := gcp.AllowedNumbers(ctx, project)
			if err != nil {
				// A transient metadata read failure must not end the call.
				log.Printf("allowlist re-check failed, keeping the call: %v", err)
				continue
			}
			if !allowed[cfg.Caller] {
				log.Printf("caller %s removed from allowlist, hanging up call %s", cfg.Caller, cfg.CallID)
				if err := realtime.Hangup(cfg.APIKey, cfg.CallID); err != nil {
					log.Printf("hangup for call %s: %v", cfg.CallID, err)
				}
				return nil
			}
		}
	}
}

// handleEvent logs the events worth seeing; tool handling (function_call
// events, the Cassandra bridge) will grow here.
func handleEvent(data []byte) {
	event, err := realtime.ParseEvent(data)
	if err != nil {
		return
	}
	switch event.Type {
	case "session.created", "session.updated":
		// The echoed config shows exactly which instructions the live call
		// runs on — the whole system prompt, nothing hidden.
		log.Printf("%s: model=%s instructions=%q", event.Type, event.Session.Model, event.Session.Instructions)
	case "error":
		log.Printf("realtime error: %s", event.Error)
	case "response.done":
		log.Printf("response done: status=%s details=%s usage=%s",
			event.Response.Status, event.Response.StatusDetails, event.Response.Usage)
	}
}
