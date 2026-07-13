// Package openai talks to the OpenAI platform: verifying its Standard
// Webhooks signatures, reading the incoming-call event, and controlling
// Realtime calls over the REST endpoints.
package openai

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var e164Digits = regexp.MustCompile(`\+[0-9]+`)

// Event is a Realtime webhook event, carrying only the fields the function
// reads.
type Event struct {
	Type string `json:"type"`
	Data struct {
		CallID     string `json:"call_id"`
		SIPHeaders []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"sip_headers"`
	} `json:"data"`
}

// ParseEvent decodes one webhook event body.
func ParseEvent(body []byte) (Event, error) {
	var event Event
	err := json.Unmarshal(body, &event)
	return event, err
}

// Caller extracts the caller's number from the SIP From header — the first
// +digits run in a value like `"Paul" <sip:+15125551234@x.com>;tag=abc` —
// or "" when there is none.
func (e Event) Caller() string {
	for _, header := range e.Data.SIPHeaders {
		if strings.EqualFold(header.Name, "from") {
			return e164Digits.FindString(header.Value)
		}
	}
	return ""
}

// VerifySignature implements Standard Webhooks: HMAC-SHA256 over
// "<id>.<timestamp>.<body>" with the base64 secret after the whsec_ prefix;
// the signature header holds space-separated "v1,<base64>" candidates. A
// timestamp more than five minutes off rejects the delivery (replay
// window).
func VerifySignature(header http.Header, body []byte, secret string) bool {
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
