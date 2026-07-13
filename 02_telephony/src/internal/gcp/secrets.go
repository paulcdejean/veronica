package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// secretCache holds resolved secrets for the life of the instance, the same
// freshness secret-backed env vars would have: a rotation lands on the next
// cold start.
var secretCache sync.Map

// SecretValue fetches the latest version of a Secret Manager secret in this
// project, by secret name.
func SecretValue(ctx context.Context, secretID string) (string, error) {
	if secretID == "" {
		return "", errors.New("secret id env var is not set")
	}
	if cached, ok := secretCache.Load(secretID); ok {
		return cached.(string), nil
	}
	project, err := ProjectID(ctx)
	if err != nil {
		return "", err
	}
	token, err := AccessToken(ctx)
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
