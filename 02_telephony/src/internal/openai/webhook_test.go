package openai

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestEventCaller(t *testing.T) {
	body := []byte(`{
		"type": "realtime.call.incoming",
		"data": {
			"call_id": "rtc_123",
			"sip_headers": [
				{"name": "Via", "value": "SIP/2.0/TLS x.example.com"},
				{"name": "From", "value": "\"Paul\" <sip:+15125551234@x.example.com>;tag=abc"}
			]
		}
	}`)
	event, err := ParseEvent(body)
	if err != nil {
		t.Fatalf("ParseEvent() error: %v", err)
	}
	if event.Type != "realtime.call.incoming" {
		t.Errorf("Type = %q", event.Type)
	}
	if event.Data.CallID != "rtc_123" {
		t.Errorf("Data.CallID = %q", event.Data.CallID)
	}
	if got := event.Caller(); got != "+15125551234" {
		t.Errorf("Caller() = %q, want +15125551234", got)
	}
}

func TestEventCallerMissing(t *testing.T) {
	event, err := ParseEvent([]byte(`{"type": "realtime.call.incoming", "data": {"call_id": "rtc_123"}}`))
	if err != nil {
		t.Fatalf("ParseEvent() error: %v", err)
	}
	if got := event.Caller(); got != "" {
		t.Errorf("Caller() = %q, want empty", got)
	}
}

// sign produces the Standard Webhooks v1 signature the way OpenAI does:
// HMAC-SHA256 over "<id>.<timestamp>.<body>".
func sign(key []byte, id, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%s.%s", id, timestamp, body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	body := []byte(`{"type":"realtime.call.incoming"}`)
	now := strconv.FormatInt(time.Now().Unix(), 10)
	stale := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)

	header := func(id, timestamp, signatures string) http.Header {
		h := http.Header{}
		h.Set("webhook-id", id)
		h.Set("webhook-timestamp", timestamp)
		h.Set("webhook-signature", signatures)
		return h
	}

	cases := []struct {
		name   string
		header http.Header
		body   []byte
		secret string
		want   bool
	}{
		{"valid", header("msg_1", now, "v1,"+sign(key, "msg_1", now, body)), body, secret, true},
		{"valid among candidates", header("msg_1", now, "v1,AAAA v1,"+sign(key, "msg_1", now, body)), body, secret, true},
		{"tampered body", header("msg_1", now, "v1,"+sign(key, "msg_1", now, body)), []byte(`{}`), secret, false},
		{"wrong id", header("msg_2", now, "v1,"+sign(key, "msg_1", now, body)), body, secret, false},
		{"wrong secret", header("msg_1", now, "v1,"+sign([]byte("another-key-entirely-32-bytes!!!"), "msg_1", now, body)), body, secret, false},
		{"stale timestamp", header("msg_1", stale, "v1,"+sign(key, "msg_1", stale, body)), body, secret, false},
		{"unknown version", header("msg_1", now, "v2,"+sign(key, "msg_1", now, body)), body, secret, false},
		{"missing headers", http.Header{}, body, secret, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VerifySignature(c.header, c.body, c.secret); got != c.want {
				t.Errorf("VerifySignature() = %v, want %v", got, c.want)
			}
		})
	}
}
