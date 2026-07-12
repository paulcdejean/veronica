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
// Environment: OPENAI_PROJECT_ID, VOICE_MODEL, VOICE_VOICE,
// VOICE_INSTRUCTIONS, SESSION_JOB (projects/*/locations/*/jobs/*), and
// OPENAI_API_KEY_SECRET / OPENAI_WEBHOOK_SECRET_SECRET naming the Secret
// Manager secrets whose latest versions are fetched at runtime — never
// through OpenTofu, so they stay out of variables and state.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

var (
	e164       = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)
	e164Digits = regexp.MustCompile(`\+[0-9]+`)
)

func init() {
	log.SetFlags(0) // Cloud Logging supplies timestamps.
	functions.HTTP("webhook", route)
}

func route(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/twiml":
		twiml(w)
	case r.Method == http.MethodPost && r.URL.Path == "/openai-webhook":
		openaiWebhook(w, r)
	default:
		http.NotFound(w, r)
	}
}

// answerOnBridge keeps the caller hearing ringing until OpenAI accepts, so
// a rejected caller gets a decline instead of a connect-then-hangup.
func twiml(w http.ResponseWriter) {
	sip := fmt.Sprintf("sip:%s@sip.api.openai.com;transport=tls", os.Getenv("OPENAI_PROJECT_ID"))
	var escaped strings.Builder
	xml.EscapeText(&escaped, []byte(sip))
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Response><Dial answerOnBridge="true"><Sip>%s</Sip></Dial></Response>`,
		escaped.String())
}

func openaiWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}

	webhookSecret, err := secretValue(ctx, os.Getenv("OPENAI_WEBHOOK_SECRET_SECRET"))
	if err != nil {
		log.Printf("webhook secret unavailable: %v", err)
		http.Error(w, "secret unavailable", http.StatusInternalServerError)
		return
	}
	if !verifySignature(r.Header, body, webhookSecret) {
		log.Print("webhook rejected: bad or missing signature")
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			CallID     string `json:"call_id"`
			SIPHeaders []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"sip_headers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil || event.Type != "realtime.call.incoming" {
		fmt.Fprint(w, "ignored")
		return
	}

	apiKey, err := secretValue(ctx, os.Getenv("OPENAI_API_KEY_SECRET"))
	if err != nil {
		log.Printf("api key unavailable: %v", err)
		http.Error(w, "secret unavailable", http.StatusInternalServerError)
		return
	}

	caller := ""
	for _, header := range event.Data.SIPHeaders {
		if strings.EqualFold(header.Name, "from") {
			caller = e164Digits.FindString(header.Value)
			break
		}
	}

	allowed, err := allowedNumbers(ctx)
	if err != nil {
		// Failing open would let anyone in; failing closed only costs a
		// legitimate caller a retry.
		log.Printf("allowlist unavailable, rejecting: %v", err)
		allowed = nil
	}
	if caller == "" || !allowed[caller] {
		log.Printf("call rejected: caller %q not in allowlist", caller)
		if err := callControl(ctx, apiKey, event.Data.CallID, "reject", map[string]any{"status_code": 603}); err != nil {
			log.Printf("reject failed: %v", err)
		}
		fmt.Fprint(w, "rejected")
		return
	}

	log.Printf("inbound call accepted from %s (%s)", caller, event.Data.CallID)
	err = callControl(ctx, apiKey, event.Data.CallID, "accept", map[string]any{
		"type":         "realtime",
		"model":        os.Getenv("VOICE_MODEL"),
		"instructions": os.Getenv("VOICE_INSTRUCTIONS"),
		"audio":        map[string]any{"output": map[string]any{"voice": os.Getenv("VOICE_VOICE")}},
	})
	if err != nil {
		log.Printf("accept failed: %v", err)
		http.Error(w, "accept failed", http.StatusInternalServerError)
		return
	}

	// The session driver greets and holds the call; if it fails to start,
	// the call still works (audio is Twilio<->OpenAI), just without a
	// greeting, tools, or mid-call allowlist enforcement.
	if err := startSession(ctx, event.Data.CallID, caller); err != nil {
		log.Printf("session driver failed to start for call %s: %v", event.Data.CallID, err)
	}
	fmt.Fprint(w, "accepted")
}

// verifySignature implements Standard Webhooks: HMAC-SHA256 over
// "<id>.<timestamp>.<body>" with the base64 secret after the whsec_ prefix;
// the signature header holds space-separated "v1,<base64>" candidates.
func verifySignature(header http.Header, body []byte, secret string) bool {
	id := header.Get("webhook-id")
	timestamp := header.Get("webhook-timestamp")
	signatures := header.Get("webhook-signature")
	if id == "" || timestamp == "" || signatures == "" || secret == "" {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if age := time.Since(time.Unix(seconds, 0)); age > 5*time.Minute || age < -5*time.Minute {
		return false
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%s.%s", id, timestamp, body)
	expected := mac.Sum(nil)
	for _, candidate := range strings.Fields(signatures) {
		version, signature, found := strings.Cut(candidate, ",")
		if !found || version != "v1" {
			continue
		}
		if given, err := base64.StdEncoding.DecodeString(signature); err == nil && hmac.Equal(given, expected) {
			return true
		}
	}
	return false
}

func callControl(ctx context.Context, apiKey, callID, action string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/realtime/calls/"+url.PathEscape(callID)+"/"+action,
		strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		detail, _ := io.ReadAll(response.Body)
		return fmt.Errorf("%s for call %s: %d %s", action, callID, response.StatusCode, detail)
	}
	return nil
}

// startSession runs one execution of the session-driver job with the call
// identity overridden in, tying the container's lifetime to this call.
func startSession(ctx context.Context, callID, caller string) error {
	job := os.Getenv("SESSION_JOB")
	if job == "" {
		return errors.New("SESSION_JOB is not set")
	}
	token, err := accessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		"overrides": map[string]any{
			"containerOverrides": []map[string]any{{
				"env": []map[string]string{
					{"name": "CALL_ID", "value": callID},
					{"name": "CALLER", "value": caller},
				},
			}},
		},
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://run.googleapis.com/v2/"+job+":run", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	detail, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		return fmt.Errorf("jobs.run: %d %s", response.StatusCode, detail)
	}
	var operation struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(detail, &operation)
	log.Printf("session driver started for call %s (%s)", callID, operation.Metadata.Name)
	return nil
}

// allowedNumbers reads the voice-contact-* entries from the project's
// common instance metadata, owned by the 00_contacts layer. Presence of an
// E.164 number authorizes the caller; empty or malformed values are
// skipped. Cloud Run's metadata server does not serve custom project
// metadata, hence the Compute API read.
func allowedNumbers(ctx context.Context) (map[string]bool, error) {
	project, err := metadataGet(ctx, "project/project-id")
	if err != nil {
		return nil, err
	}
	token, err := accessToken(ctx)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://compute.googleapis.com/compute/v1/projects/"+url.PathEscape(project), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("reading project metadata: %d %s", response.StatusCode, detail)
	}
	var projectInfo struct {
		CommonInstanceMetadata struct {
			Items []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"items"`
		} `json:"commonInstanceMetadata"`
	}
	if err := json.NewDecoder(response.Body).Decode(&projectInfo); err != nil {
		return nil, err
	}
	allowed := make(map[string]bool)
	for _, item := range projectInfo.CommonInstanceMetadata.Items {
		if !strings.HasPrefix(item.Key, "voice-contact-") {
			continue
		}
		number := strings.Join(strings.Fields(item.Value), "")
		if e164.MatchString(number) {
			allowed[number] = true
		}
	}
	return allowed, nil
}

// secretCache holds resolved secrets for the life of the instance, the same
// freshness secret-backed env vars would have: a rotation lands on the next
// cold start.
var secretCache sync.Map

func secretValue(ctx context.Context, secretID string) (string, error) {
	if secretID == "" {
		return "", errors.New("secret id env var is not set")
	}
	if cached, ok := secretCache.Load(secretID); ok {
		return cached.(string), nil
	}
	project, err := metadataGet(ctx, "project/project-id")
	if err != nil {
		return "", err
	}
	token, err := accessToken(ctx)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://secretmanager.googleapis.com/v1/projects/%s/secrets/%s/versions/latest:access",
			url.PathEscape(project), url.PathEscape(secretID)), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("accessing secret %s: %d %s", secretID, response.StatusCode, detail)
	}
	var version struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(response.Body).Decode(&version); err != nil {
		return "", err
	}
	decoded, err := base64.StdEncoding.DecodeString(version.Payload.Data)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(decoded))
	secretCache.Store(secretID, value)
	return value, nil
}

func accessToken(ctx context.Context) (string, error) {
	body, err := metadataGetRaw(ctx, "instance/service-accounts/default/token")
	if err != nil {
		return "", err
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func metadataGet(ctx context.Context, path string) (string, error) {
	body, err := metadataGetRaw(ctx, path)
	return strings.TrimSpace(string(body)), err
}

func metadataGetRaw(ctx context.Context, path string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://metadata.google.internal/computeMetadata/v1/"+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Metadata-Flavor", "Google")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata server %s: %d", path, response.StatusCode)
	}
	return io.ReadAll(response.Body)
}
