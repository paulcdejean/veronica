package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// AcceptCall answers realtime.call.incoming by attaching a session
// configuration ({"type": "realtime", "model": ..., "instructions": ...})
// to the call; OpenAI then bridges the audio.
func AcceptCall(ctx context.Context, apiKey, callID string, session map[string]any) error {
	return callControl(ctx, apiKey, callID, "accept", session)
}

// RejectCall declines the call with a SIP status code (603 is a plain
// decline).
func RejectCall(ctx context.Context, apiKey, callID string, statusCode int) error {
	return callControl(ctx, apiKey, callID, "reject", map[string]any{"status_code": statusCode})
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
