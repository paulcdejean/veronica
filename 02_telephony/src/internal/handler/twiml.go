package handler

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// twiml tells Twilio to ring the call through to OpenAI over SIP.
// answerOnBridge keeps the caller hearing ringing until the session driver
// accepts, so a rejected caller gets a decline instead of a
// connect-then-hangup; the timeout leaves room for the driver's container
// to start before Twilio gives up on the ringing.
func (h *Handler) twiml(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprint(w, twimlBody(h.cfg.OpenAIProjectID))
}

func twimlBody(openaiProjectID string) string {
	sip := fmt.Sprintf("sip:%s@sip.api.openai.com;transport=tls", openaiProjectID)
	var escaped strings.Builder
	xml.EscapeText(&escaped, []byte(sip))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<Response><Dial answerOnBridge="true" timeout="60"><Sip>%s</Sip></Dial></Response>`,
		escaped.String())
}
