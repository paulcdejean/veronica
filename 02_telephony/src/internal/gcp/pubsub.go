package gcp

import (
	"context"
	"net/http"
)

// Publish sends one message to the topic (projects/*/topics/*).
func Publish(ctx context.Context, topic string, data []byte) error {
	_, err := apiCall(ctx, http.MethodPost,
		"https://pubsub.googleapis.com/v1/"+topic+":publish",
		map[string]any{"messages": []map[string]any{{"data": data}}})
	return err
}
