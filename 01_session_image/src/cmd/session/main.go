// Command session is Veronica's session driver: one Cloud Run job execution
// per phone call.
//
// The telephony layer's webhook function accepts the call with OpenAI and
// starts an execution of this job with CALL_ID and CALLER set. The driver
// attaches the call's sideband WebSocket (a control channel — the audio
// flows Twilio<->OpenAI directly), speaks the greeting, and then holds the
// socket for the life of the call. When the call ends OpenAI closes the
// socket and this process exits, which completes the job execution — the
// driver's lifetime is the call's.
//
// The work is split across the internal packages: realtime is the OpenAI
// sideband client, gcp is the Google Cloud plumbing, and session is the
// driver's actual flow. This command only turns the environment into a
// session.Config and runs it.
package main

import (
	"context"
	"errors"
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

	// SIGTERM means the execution is being cancelled from outside; exit
	// without hanging up — the call can survive losing its driver.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := session.Run(ctx, cfg); err != nil {
		log.Fatalf("session driver failed: %v", err)
	}
}

func configFromEnv() (session.Config, error) {
	cfg := session.Config{
		CallID:   os.Getenv("CALL_ID"),
		Caller:   os.Getenv("CALLER"),
		APIKey:   os.Getenv("OPENAI_API_KEY"),
		Greeting: os.Getenv("VOICE_GREETING"),
	}
	if cfg.CallID == "" || cfg.Caller == "" || cfg.APIKey == "" || cfg.Greeting == "" {
		return session.Config{}, errors.New("CALL_ID, CALLER, OPENAI_API_KEY and VOICE_GREETING must all be set")
	}
	return cfg, nil
}
