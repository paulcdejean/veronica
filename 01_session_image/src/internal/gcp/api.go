package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// apiCall makes one authorized JSON request to a Google API, returning the
// response body; any non-2xx status becomes an error carrying the body.
func apiCall(ctx context.Context, method, url string, payload any) ([]byte, error) {
	token, err := AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	detail, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: %d %s", method, url, response.StatusCode, detail)
	}
	return detail, nil
}
