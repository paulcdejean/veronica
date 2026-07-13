// Command session is Veronica's session driver: a Cloud Run worker pool
// instance that exists only while a phone call does.
//
// The telephony layer's webhook function screens each incoming call,
// publishes {call_id, caller} to the handoff subscription, and scales this
// pool from zero to one. This process boots in seconds, pulls the call,
// and picks it up: the accept (which stops the caller's ringing) and the
// sideband WebSocket attach both happen here, so the driver is on the line
// for the call's entire life — the audio itself flows Twilio<->OpenAI
// directly. When the call ends and no other is pending, the driver scales
// the pool back to zero, ending its own provisioning.
//
// The work is split across the internal packages: realtime is the OpenAI
// client, gcp is the Google Cloud plumbing, and session is the driver's
// actual flow. This command only turns the environment into a
// session.Config and serves it.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/paulcdejean/veronica/session/internal/session"
)

func main() {
	log.SetFlags(0) // Cloud Logging supplies timestamps.

	cfg, err := configFromEnv()
	if err != nil {
		log.Fatalf("session driver failed: %v", err)
	}

	// SIGTERM means the pool is scaling down — normally by this process's
	// own hand after the line went quiet. Exit without hanging up; a call
	// can survive losing its driver.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := session.Serve(ctx, cfg); err != nil {
		log.Fatalf("session driver failed: %v", err)
	}
}

func configFromEnv() (session.Config, error) {
	cfg := session.Config{
		APIKey:       os.Getenv("OPENAI_API_KEY"),
		Greeting:     os.Getenv("VOICE_GREETING"),
		Model:        os.Getenv("VOICE_MODEL"),
		Voice:        os.Getenv("VOICE_VOICE"),
		Instructions: os.Getenv("VOICE_INSTRUCTIONS"),
		Subscription: os.Getenv("CALL_SUBSCRIPTION"),
		WorkerPool:   os.Getenv("WORKER_POOL"),
	}
	for name, value := range map[string]string{
		"OPENAI_API_KEY":     cfg.APIKey,
		"VOICE_GREETING":     cfg.Greeting,
		"VOICE_MODEL":        cfg.Model,
		"VOICE_VOICE":        cfg.Voice,
		"VOICE_INSTRUCTIONS": cfg.Instructions,
		"CALL_SUBSCRIPTION":  cfg.Subscription,
		"WORKER_POOL":        cfg.WorkerPool,
	} {
		if value == "" {
			return session.Config{}, fmt.Errorf("%s must be set", name)
		}
	}
	return cfg, nil
}
