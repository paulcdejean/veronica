// Package session is the driver's flow: pull the next call from the
// handoff subscription, pick it up (the accept is here, not in the
// webhook, so the driver is attached from the call's first moment), hold
// the line — greeting, logging the session's events, re-checking the
// caller against the allowlist — until the call ends, and scale the pool
// away when nothing is left to drive. Tool handling (function_call events,
// the Cassandra bridge) will grow here.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/paulcdejean/veronica/session/internal/gcp"
	"github.com/paulcdejean/veronica/session/internal/realtime"
)

// The SIP answer and Twilio's bridging take a beat to connect the caller's
// audio path after accept; a greeting spoken too early reaches the caller
// clipped ("...ronica speaking").
const greetingSettle = 1 * time.Second

// Realtime sessions cap at 60 minutes; leave on our own terms before then.
const sessionCap = 55 * time.Minute

// How often to re-check the caller against the allowlist mid-call.
const allowlistInterval = 1 * time.Minute

// The webhook publishes the call before scaling the pool up, so the
// message is normally waiting when this process boots; the retries cover
// redelivery lag after a crash.
const (
	pullAttempts   = 3
	pullRetryDelay = 2 * time.Second
)

// Config is the driver's deployment: the persona it answers with and the
// handoff plumbing it serves.
type Config struct {
	APIKey       string
	Greeting     string
	Model        string
	Voice        string
	Instructions string
	// Subscription (projects/*/subscriptions/*) delivers the calls the
	// webhook dispatches; WorkerPool (projects/*/locations/*/workerPools/*)
	// is this pool's own name, scaled to zero when the line goes quiet.
	Subscription string
	WorkerPool   string
}

// call is the handoff message the webhook publishes.
type call struct {
	ID     string `json:"call_id"`
	Caller string `json:"caller"`
}

// Serve drives dispatched calls until the subscription runs dry, then
// scales the pool — and with it this process — down to zero. A cancelled
// context (SIGTERM: the scale-down landing, or an external hand on the
// pool) exits without touching the count.
func Serve(ctx context.Context, cfg Config) error {
	project, err := gcp.ProjectID(ctx)
	if err != nil {
		return fmt.Errorf("reading project id: %w", err)
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		message, ok, err := nextMessage(ctx, cfg.Subscription)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !ok {
			scaleDown(cfg)
			return nil
		}

		var c call
		if err := json.Unmarshal(message.Data, &c); err != nil || c.ID == "" || c.Caller == "" {
			// A malformed handoff can never become a call; drop it.
			log.Printf("dropping malformed call message %q", message.Data)
			ack(ctx, cfg.Subscription, message)
			continue
		}

		if err := drive(ctx, cfg, project, c, message); err != nil {
			return err
		}
	}
}

// nextMessage pulls the pending call, retrying briefly so a redelivery
// that lags the process restart is not mistaken for an empty line.
func nextMessage(ctx context.Context, subscription string) (gcp.Message, bool, error) {
	for attempt := 1; ; attempt++ {
		message, ok, err := gcp.Pull(ctx, subscription)
		if err != nil || ok || attempt == pullAttempts {
			return message, ok, err
		}
		select {
		case <-time.After(pullRetryDelay):
		case <-ctx.Done():
			return gcp.Message{}, false, ctx.Err()
		}
	}
}

// drive picks up one call and holds it until it ends. Returning an error
// exits the process; the pool is still scaled up, so Cloud Run restarts it
// and the next attempt resumes from whatever the message's ack state says.
func drive(ctx context.Context, cfg Config, project string, c call, message gcp.Message) error {
	ctx, cancel := context.WithTimeout(ctx, sessionCap)
	defer cancel()

	// The accept is what answers the phone: it gives the Realtime session
	// Veronica's persona and bridges the audio. Until it runs the caller
	// hears ringing, and once it runs the driver is already here — nothing,
	// greeting included, will escape the transcript when transcripts land.
	err := realtime.Accept(ctx, cfg.APIKey, c.ID, map[string]any{
		"type":         "realtime",
		"model":        cfg.Model,
		"instructions": cfg.Instructions,
		"audio":        map[string]any{"output": map[string]any{"voice": cfg.Voice}},
	})
	if errors.Is(err, realtime.ErrCallNotFound) {
		// The caller hung up, or Twilio stopped ringing, while we started.
		log.Printf("call %s from %s already gone, dropping it", c.ID, c.Caller)
		ack(ctx, cfg.Subscription, message)
		return nil
	}
	if err != nil {
		// Put the call back for the restart that follows this return.
		nack(ctx, cfg.Subscription, message)
		return fmt.Errorf("accepting call %s: %w", c.ID, err)
	}
	ack(ctx, cfg.Subscription, message)
	log.Printf("picked up call %s from %s", c.ID, c.Caller)

	// From here a failure exits with the message already acked: the restart
	// finds an empty subscription and scales down, and the call goes on
	// driverless — degraded (no greeting, no mid-call enforcement, no
	// tools) but alive.
	conn, err := realtime.Attach(ctx, c.ID, cfg.APIKey)
	if err != nil {
		return fmt.Errorf("attaching to call %s: %w", c.ID, err)
	}
	defer conn.Close()
	log.Printf("attached to call %s", c.ID)

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

	return hold(ctx, cfg, conn, project, c)
}

// hold keeps the line until the call ends, the context is cancelled, or
// the caller loses their allowlist entry.
func hold(ctx context.Context, cfg Config, conn *realtime.Conn, project string, c call) error {
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
					// Cancelled or hit the session cap; dropping the socket
					// detaches, the call survives unless OpenAI ends it at
					// the 60-minute mark anyway.
					log.Printf("detaching: %v", context.Cause(ctx))
					return nil
				}
				// The server closing the socket is how a finished call
				// looks from here.
				log.Printf("call %s ended (%v)", c.ID, err)
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
			if !allowed[c.Caller] {
				log.Printf("caller %s removed from allowlist, hanging up call %s", c.Caller, c.ID)
				if err := realtime.Hangup(cfg.APIKey, c.ID); err != nil {
					log.Printf("hangup for call %s: %v", c.ID, err)
				}
				return nil
			}
		}
	}
}

// scaleDown ends the pool's provisioning once the subscription runs dry. A
// call dispatched between the last empty pull and the count reaching zero
// would strand, so check once more afterwards and hand the pool to a fresh
// instance if something arrived. Teardown runs on its own context: the
// serve context may already be gone, and the PATCH to zero SIGTERMs this
// very process.
func scaleDown(cfg Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// The driver runs as the pool's identity, which scaling must name.
	identity, err := gcp.ServiceAccountEmail(ctx)
	if err != nil {
		log.Printf("reading own identity for scale-down: %v", err)
		return
	}
	if err := gcp.ScaleWorkerPool(ctx, cfg.WorkerPool, identity, 0); err != nil {
		log.Printf("scaling the pool down: %v", err)
		return
	}
	message, ok, err := gcp.Pull(ctx, cfg.Subscription)
	if err != nil || !ok {
		log.Print("no calls pending, pool scaled to zero")
		return
	}
	nack(ctx, cfg.Subscription, message)
	if err := gcp.ScaleWorkerPool(ctx, cfg.WorkerPool, identity, 1); err != nil {
		log.Printf("scaling the pool back up for a late call: %v", err)
		return
	}
	log.Print("call arrived during scale-down, handing the pool to a fresh instance")
}

// ack and nack are best effort: a failed ack means a redelivery that the
// accept's 404 path drops, never a lost or doubled call.
func ack(ctx context.Context, subscription string, message gcp.Message) {
	if err := gcp.Ack(ctx, subscription, message.AckID); err != nil {
		log.Printf("ack failed: %v", err)
	}
}

func nack(ctx context.Context, subscription string, message gcp.Message) {
	if err := gcp.Nack(ctx, subscription, message.AckID); err != nil {
		log.Printf("nack failed: %v", err)
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
