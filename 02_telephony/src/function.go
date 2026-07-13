// Package webhook is Veronica's voice front door, a Cloud Run function.
//
// The call path is: Twilio answers the phone number and fetches /twiml,
// which rings the call through to OpenAI over SIP. OpenAI then asks us
// whether to take the call by POSTing a signed realtime.call.incoming
// webhook to /openai-webhook; we check the caller against the
// voice-contact-* project metadata (owned by 00_contacts) and reject
// anyone unknown. A known caller is dispatched, not accepted: we publish
// the call to the handoff topic and scale the session-driver worker pool
// from zero, and the driver — on the line for the call's first moment and
// its last — is what picks up. Audio flows Twilio<->OpenAI directly; this
// function never touches media and is done in under a second.
//
// The work is split across the internal packages: handler serves the two
// routes, openai verifies the webhook signature and rejects calls over
// REST, and gcp is the Google Cloud plumbing (secrets, allowlist, the
// handoff topic, the pool's scaling). This file only registers the handler
// — the Functions Framework requires the registration to happen in the
// module's root package.
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
