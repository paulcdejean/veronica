// Package handler serves the function's two routes: /twiml, which hands
// the inbound Twilio call to OpenAI over SIP, and /openai-webhook, which
// answers OpenAI's should-I-take-this-call webhook — verify the signature,
// check the caller against the allowlist, accept or reject, and start the
// session driver.
package handler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/paulcdejean/veronica/webhook/internal/gcp"
	"github.com/paulcdejean/veronica/webhook/internal/openai"
)

// Config is everything the handler takes from its deployment, gathered in
// one place. The persona fields come from the tofu workspace configuration;
// the *Secret fields are Secret Manager secret names, never values — the
// values exist only as secret versions added with gcloud.
type Config struct {
	// OpenAIProjectID is the user part of the SIP URI the call is handed
	// to — an id, not a credential.
	OpenAIProjectID string

	// Model, Voice and Instructions configure the Realtime session on
	// accept; Instructions is Veronica's entire system prompt.
	Model        string
	Voice        string
	Instructions string

	// SessionJob is the fully qualified Cloud Run job
	// (projects/*/locations/*/jobs/*) run once per accepted call.
	SessionJob string

	APIKeySecret        string
	WebhookSecretSecret string
}

// ConfigFromEnv reads the deployment's environment. Missing values surface
// as errors when first used rather than at startup, matching how the
// secret values themselves may not exist until first rotated in.
func ConfigFromEnv() Config {
	return Config{
		OpenAIProjectID:     os.Getenv("OPENAI_PROJECT_ID"),
		Model:               os.Getenv("VOICE_MODEL"),
		Voice:               os.Getenv("VOICE_VOICE"),
		Instructions:        os.Getenv("VOICE_INSTRUCTIONS"),
		SessionJob:          os.Getenv("SESSION_JOB"),
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

	log.Printf("inbound call accepted from %s (%s)", caller, event.Data.CallID)
	err = openai.AcceptCall(ctx, apiKey, event.Data.CallID, map[string]any{
		"type":         "realtime",
		"model":        h.cfg.Model,
		"instructions": h.cfg.Instructions,
		"audio":        map[string]any{"output": map[string]any{"voice": h.cfg.Voice}},
	})
	if err != nil {
		log.Printf("accept failed: %v", err)
		http.Error(w, "accept failed", http.StatusInternalServerError)
		return
	}

	// The session driver greets and holds the call; if it fails to start,
	// the call still works (audio is Twilio<->OpenAI), just without a
	// greeting, tools, or mid-call allowlist enforcement.
	execution, err := gcp.RunJob(ctx, h.cfg.SessionJob, map[string]string{
		"CALL_ID": event.Data.CallID,
		"CALLER":  caller,
	})
	if err != nil {
		log.Printf("session driver failed to start for call %s: %v", event.Data.CallID, err)
	} else {
		log.Printf("session driver started for call %s (%s)", event.Data.CallID, execution)
	}
	fmt.Fprint(w, "accepted")
}
