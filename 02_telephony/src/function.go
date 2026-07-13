// Package webhook is Veronica's voice front door, a Cloud Run function.
//
// The call path is: Twilio answers the phone number and fetches /twiml,
// which hands the call to OpenAI over SIP. OpenAI then asks us whether to
// take the call by POSTing a signed realtime.call.incoming webhook to
// /openai-webhook; we check the caller against the voice-contact-* project
// metadata (owned by 00_contacts) and either accept the call with
// Veronica's persona or reject it. On accept we start one execution of the
// session-driver job, which attaches the call's WebSocket, greets, and
// holds it until the call ends. Audio flows Twilio<->OpenAI directly — this
// function never touches media and is done in under a second.
//
// The work is split across the internal packages: handler serves the two
// routes, openai verifies the webhook signature and controls the call over
// REST, and gcp is the Google Cloud plumbing (secrets, allowlist, job
// executions). This file only registers the handler — the Functions
// Framework requires the registration to happen in the module's root
// package.
package webhook

import (
	"log"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"

	"github.com/paulcdejean/veronica/webhook/internal/handler"
)

func init() {
	log.SetFlags(0) // Cloud Logging supplies timestamps.
	functions.HTTP("webhook", handler.New(handler.ConfigFromEnv()).ServeHTTP)
}
