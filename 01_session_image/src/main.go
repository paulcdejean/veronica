// Veronica's session driver: one Cloud Run job execution per phone call.
//
// The telephony layer's webhook function accepts the call with OpenAI and
// starts an execution of this job with CALL_ID and CALLER set. This process
// attaches the call's sideband WebSocket (a control channel — the audio
// flows Twilio<->OpenAI directly), speaks the greeting, and then holds the
// socket for the life of the call: it re-checks the caller against the
// allowlist and hangs up if they have been removed, and it is where tool
// calls (function_call events) will be handled when Veronica grows tools.
// When the call ends OpenAI closes the socket and this process exits, which
// completes the job execution — the driver's lifetime is the call's.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

// The SIP answer and Twilio's bridging take a beat to connect the caller's
// audio path after accept; a greeting spoken too early reaches the caller
// clipped ("...ronica speaking"). The job's own startup already adds a few
// seconds, so this is a small extra margin.
const greetingSettle = 1 * time.Second

// Realtime sessions cap at 60 minutes; leave on our own terms before then.
const sessionCap = 55 * time.Minute

// How often to re-check the caller against the allowlist mid-call.
const allowlistInterval = 1 * time.Minute

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)

func main() {
	log.SetFlags(0) // Cloud Logging supplies timestamps.
	if err := run(); err != nil {
		log.Fatalf("session driver failed: %v", err)
	}
}

func run() error {
	callID := os.Getenv("CALL_ID")
	caller := os.Getenv("CALLER")
	apiKey := os.Getenv("OPENAI_API_KEY")
	greeting := os.Getenv("VOICE_GREETING")
	if callID == "" || caller == "" || apiKey == "" || greeting == "" {
		return errors.New("CALL_ID, CALLER, OPENAI_API_KEY and VOICE_GREETING must all be set")
	}

	// SIGTERM means the execution is being cancelled from outside; exit
	// without hanging up — the call can survive losing its driver.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, sessionCap)
	defer cancel()

	project, err := metadataGet(ctx, "project/project-id")
	if err != nil {
		return fmt.Errorf("reading project id: %w", err)
	}

	conn, _, err := websocket.Dial(ctx,
		"wss://api.openai.com/v1/realtime?call_id="+url.QueryEscape(callID),
		&websocket.DialOptions{HTTPHeader: http.Header{
			"Authorization": {"Bearer " + apiKey},
		}},
	)
	if err != nil {
		return fmt.Errorf("attaching to call %s: %w", callID, err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	log.Printf("attached to call %s from %s", callID, caller)

	// Attaching does not replay session.created, so nudge the server into
	// echoing session.updated (a no-op update) to get the effective
	// instructions into the logs.
	if err := send(ctx, conn, map[string]any{
		"type":    "session.update",
		"session": map[string]any{"type": "realtime"},
	}); err != nil {
		return fmt.Errorf("sending session probe: %w", err)
	}

	select {
	case <-time.After(greetingSettle):
	case <-ctx.Done():
		return nil
	}

	// The model only speaks when a response is created, so nudge the
	// greeting out rather than waiting for the caller's "hello?". The
	// token cap keeps the greeting from growing a monologue; output audio
	// runs ~20 tokens per spoken second, so 100 is ~5 seconds of speech.
	if err := send(ctx, conn, map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"instructions":      fmt.Sprintf("Say exactly this and nothing more: %q", greeting),
			"max_output_tokens": 100,
		},
	}); err != nil {
		return fmt.Errorf("sending greeting: %w", err)
	}

	events := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		defer close(events)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
			events <- data
		}
	}()

	allowlist := time.NewTicker(allowlistInterval)
	defer allowlist.Stop()

	for {
		select {
		case data, ok := <-events:
			if !ok {
				err := <-readErr
				if ctx.Err() != nil {
					// Cancelled or hit the session cap; the deferred
					// close drops the socket, the call survives unless
					// OpenAI ends it at the 60-minute mark anyway.
					log.Printf("detaching: %v", context.Cause(ctx))
					return nil
				}
				// The server closing the socket is how a finished call
				// looks from here.
				log.Printf("call %s ended (%v)", callID, err)
				return nil
			}
			handleEvent(data)

		case <-allowlist.C:
			allowed, err := allowedNumbers(ctx, project)
			if err != nil {
				// A transient metadata read failure must not end the call.
				log.Printf("allowlist re-check failed, keeping the call: %v", err)
				continue
			}
			if !allowed[caller] {
				log.Printf("caller %s removed from allowlist, hanging up call %s", caller, callID)
				hangup(apiKey, callID)
				return nil
			}
		}
	}
}

// handleEvent logs the events worth seeing; tool handling (function_call
// events, the Cassandra bridge) will grow here.
func handleEvent(data []byte) {
	var event struct {
		Type    string          `json:"type"`
		Error   json.RawMessage `json:"error"`
		Session struct {
			Model        string `json:"model"`
			Instructions string `json:"instructions"`
		} `json:"session"`
		Response struct {
			Status        string          `json:"status"`
			StatusDetails json.RawMessage `json:"status_details"`
			Usage         json.RawMessage `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	switch event.Type {
	case "session.created", "session.updated":
		// The echoed config shows exactly which instructions the live call
		// runs on — the whole system prompt, nothing hidden.
		log.Printf("%s: model=%s instructions=%q", event.Type, event.Session.Model, event.Session.Instructions)
	case "error":
		log.Printf("realtime error: %s", event.Error)
	case "response.done":
		log.Printf("response done: status=%s details=%s usage=%s",
			event.Response.Status, event.Response.StatusDetails, event.Response.Usage)
	}
}

func send(ctx context.Context, conn *websocket.Conn, message map[string]any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// hangup ends the call via the REST control endpoint. Best effort on a
// fresh context: it runs when the session is already being torn down.
func hangup(apiKey, callID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/realtime/calls/"+url.PathEscape(callID)+"/hangup", nil)
	if err != nil {
		log.Printf("hangup request for call %s: %v", callID, err)
		return
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Printf("hangup for call %s: %v", callID, err)
		return
	}
	defer response.Body.Close()
	// 404 just means the call already ended on its own.
	if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		log.Printf("hangup for call %s: %d %s", callID, response.StatusCode, body)
	}
}

// allowedNumbers reads the voice-contact-* entries from the project's
// common instance metadata — the same allowlist the webhook checked at
// accept time, owned by the 00_contacts layer. Presence of an E.164 number
// authorizes the caller; empty or malformed values are skipped.
func allowedNumbers(ctx context.Context, project string) (map[string]bool, error) {
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
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("reading project metadata: %d %s", response.StatusCode, body)
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
	return string(bytes.TrimSpace(body)), err
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
