// Package handler serves the function's two routes: /twiml, which hands
// the inbound Twilio call to OpenAI over SIP, and /openai-webhook, which
// answers OpenAI's should-I-take-this-call webhook — verify the signature,
// check the caller against the allowlist, accept or reject, and start the
// session driver.
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/paulcdejean/veronica/webhook/internal/gcp"
	"github.com/paulcdejean/veronica/webhook/internal/openai"
)

// Config is everything the handler takes from its deployment, gathered in
// one place. The *Secret fields are Secret Manager secret names, never
// values — the values exist only as secret versions added with gcloud.
// The persona lives with the session driver, which is what accepts calls.
type Config struct {
	// OpenAIProjectID is the user part of the SIP URI the call is handed
	// to — an id, not a credential.
	OpenAIProjectID string

	// CallTopic (projects/*/topics/*) carries the handoff to the driver;
	// SessionWorkerPool (projects/*/locations/*/workerPools/*) is the
	// driver's pool, scaled up when a call is dispatched.
	CallTopic         string
	SessionWorkerPool string

	APIKeySecret        string
	WebhookSecretSecret string
}

// ConfigFromEnv reads the deployment's environment. Missing values surface
// as errors when first used rather than at startup, matching how the
// secret values themselves may not exist until first rotated in.
func ConfigFromEnv() Config {
	return Config{
		OpenAIProjectID:     os.Getenv("OPENAI_PROJECT_ID"),
		CallTopic:           os.Getenv("CALL_TOPIC"),
		SessionWorkerPool:   os.Getenv("SESSION_WORKER_POOL"),
		APIKeySecret:        os.Getenv("OPENAI_API_KEY_SECRET"),
		WebhookSecretSecret: os.Getenv("OPENAI_WEBHOOK_SECRET_SECRET"),
	}
}

// Handler routes the function's requests.
type Handler struct {
	cfg Config
}

func New(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/twiml":
		h.twiml(w)
	case r.Method == http.MethodPost && r.URL.Path == "/openai-webhook":
		h.openaiWebhook(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) openaiWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}

	webhookSecret, err := gcp.SecretValue(ctx, h.cfg.WebhookSecretSecret)
	if err != nil {
		log.Printf("webhook secret unavailable: %v", err)
		http.Error(w, "secret unavailable", http.StatusInternalServerError)
		return
	}
	if !openai.VerifySignature(r.Header, body, webhookSecret) {
		log.Print("webhook rejected: bad or missing signature")
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	event, err := openai.ParseEvent(body)
	if err != nil || event.Type != "realtime.call.incoming" {
		fmt.Fprint(w, "ignored")
		return
	}

	apiKey, err := gcp.SecretValue(ctx, h.cfg.APIKeySecret)
	if err != nil {
		log.Printf("api key unavailable: %v", err)
		http.Error(w, "secret unavailable", http.StatusInternalServerError)
		return
	}

	caller := event.Caller()
	allowed, err := gcp.AllowedNumbers(ctx)
	if err != nil {
		// Failing open would let anyone in; failing closed only costs a
		// legitimate caller a retry.
		log.Printf("allowlist unavailable, rejecting: %v", err)
		allowed = nil
	}
	if caller == "" || !allowed[caller] {
		log.Printf("call rejected: caller %q not in allowlist", caller)
		if err := openai.RejectCall(ctx, apiKey, event.Data.CallID, 603); err != nil {
			log.Printf("reject failed: %v", err)
		}
		fmt.Fprint(w, "rejected")
		return
	}

	// The driver is what picks up: publish the call and scale the pool from
	// zero, and the starting instance pulls the message, accepts, and holds
	// the line — attached from the call's first moment, so nothing escapes
	// the transcript. Until then the caller just hears ringing, and a
	// failure here means OpenAI retries the whole (idempotent) webhook.
	handoff, err := json.Marshal(map[string]string{
		"call_id": event.Data.CallID,
		"caller":  caller,
	})
	if err != nil {
		http.Error(w, "handoff failed", http.StatusInternalServerError)
		return
	}
	if err := gcp.Publish(ctx, h.cfg.CallTopic, handoff); err != nil {
		log.Printf("publishing call %s: %v", event.Data.CallID, err)
		http.Error(w, "handoff failed", http.StatusInternalServerError)
		return
	}
	if err := gcp.ScaleWorkerPool(ctx, h.cfg.SessionWorkerPool, 1); err != nil {
		log.Printf("scaling the session pool for call %s: %v", event.Data.CallID, err)
		http.Error(w, "handoff failed", http.StatusInternalServerError)
		return
	}
	log.Printf("call %s from %s dispatched to the session driver", event.Data.CallID, caller)
	fmt.Fprint(w, "dispatched")
}
