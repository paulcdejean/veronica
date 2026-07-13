package handler

import (
	"strings"
	"testing"
)

func TestTwimlBody(t *testing.T) {
	want := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Response><Dial answerOnBridge="true">` +
		`<Sip>sip:proj_123@sip.api.openai.com;transport=tls</Sip>` +
		`</Dial></Response>`
	if got := twimlBody("proj_123"); got != want {
		t.Errorf("twimlBody() = %s, want %s", got, want)
	}
}

func TestTwimlBodyEscapes(t *testing.T) {
	got := twimlBody("a<b&c")
	want := `<Sip>sip:a&lt;b&amp;c@sip.api.openai.com;transport=tls</Sip>`
	if !strings.Contains(got, want) {
		t.Errorf("twimlBody() = %s, want it to contain %s", got, want)
	}
}
