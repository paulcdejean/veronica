// Package gcp is the function's Google Cloud plumbing: identity from the
// instance metadata server, secret values from Secret Manager, the caller
// allowlist from the project's common instance metadata, and Cloud Run job
// executions.
//
// The session-driver module (../../../01_session_image/src) carries its own
// copy of the metadata and allowlist plumbing: the two programs are
// separate Go modules, and each build ships only its own src/.
package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ProjectID reads the project this workload runs in from the metadata
// server.
func ProjectID(ctx context.Context) (string, error) {
	body, err := metadataGet(ctx, "project/project-id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// AccessToken returns an OAuth token for the workload's service account.
func AccessToken(ctx context.Context) (string, error) {
	body, err := metadataGet(ctx, "instance/service-accounts/default/token")
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

func metadataGet(ctx context.Context, path string) ([]byte, error) {
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
