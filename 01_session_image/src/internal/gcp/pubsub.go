package gcp

import (
	"context"
	"encoding/json"
	"net/http"
)

// Message is one pulled Pub/Sub message.
type Message struct {
	AckID string
	Data  []byte
}

// Pull asks the subscription (projects/*/subscriptions/*) for one message,
// returning ok=false when none is immediately available.
func Pull(ctx context.Context, subscription string) (Message, bool, error) {
	body, err := apiCall(ctx, http.MethodPost,
		"https://pubsub.googleapis.com/v1/"+subscription+":pull",
		map[string]any{"maxMessages": 1})
	if err != nil {
		return Message{}, false, err
	}
	var result struct {
		ReceivedMessages []struct {
			AckID   string `json:"ackId"`
			Message struct {
				Data []byte `json:"data"` // base64 in the JSON, decoded here
			} `json:"message"`
		} `json:"receivedMessages"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Message{}, false, err
	}
	if len(result.ReceivedMessages) == 0 {
		return Message{}, false, nil
	}
	received := result.ReceivedMessages[0]
	return Message{AckID: received.AckID, Data: received.Message.Data}, true, nil
}

// Ack removes the message from the subscription for good.
func Ack(ctx context.Context, subscription, ackID string) error {
	_, err := apiCall(ctx, http.MethodPost,
		"https://pubsub.googleapis.com/v1/"+subscription+":acknowledge",
		map[string]any{"ackIds": []string{ackID}})
	return err
}

// Nack makes the message immediately available for redelivery.
func Nack(ctx context.Context, subscription, ackID string) error {
	_, err := apiCall(ctx, http.MethodPost,
		"https://pubsub.googleapis.com/v1/"+subscription+":modifyAckDeadline",
		map[string]any{"ackIds": []string{ackID}, "ackDeadlineSeconds": 0})
	return err
}
