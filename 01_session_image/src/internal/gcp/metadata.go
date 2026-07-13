// Package gcp is the driver's Google Cloud plumbing: identity from the
// instance metadata server, and the Compute API read of the project's
// common instance metadata (the caller allowlist owned by 00_contacts).
//
// The webhook module (../../../02_telephony/src) carries its own copy of
// this plumbing: the two programs are separate Go modules, and each build
// ships only its own src/.
package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ProjectID reads the project this workload runs in from the metadata
// server.
func ProjectID(ctx context.Context) (string, error) {
	body, err := metadataGet(ctx, "project/project-id")
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(body)), nil
}

// ServiceAccountEmail reads the workload's own service account from the
// metadata server — for the driver this is also the pool's template
// identity, which scaling updates must name.
func ServiceAccountEmail(ctx context.Context) (string, error) {
	body, err := metadataGet(ctx, "instance/service-accounts/default/email")
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(body)), nil
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
