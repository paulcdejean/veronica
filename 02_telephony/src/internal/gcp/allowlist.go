package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)

type metadataItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AllowedNumbers reads the voice-contact-* entries from the project's
// common instance metadata, owned by the 00_contacts layer. Cloud Run's own
// metadata server does not serve custom project metadata, hence the Compute
// API read.
func AllowedNumbers(ctx context.Context) (map[string]bool, error) {
	project, err := ProjectID(ctx)
	if err != nil {
		return nil, err
	}
	token, err := AccessToken(ctx)
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
		detail, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("reading project metadata: %d %s", response.StatusCode, detail)
	}
	var projectInfo struct {
		CommonInstanceMetadata struct {
			Items []metadataItem `json:"items"`
		} `json:"commonInstanceMetadata"`
	}
	if err := json.NewDecoder(response.Body).Decode(&projectInfo); err != nil {
		return nil, err
	}
	return parseAllowlist(projectInfo.CommonInstanceMetadata.Items), nil
}

// parseAllowlist keeps the voice-contact-* values that hold an E.164
// number; presence of a number is what authorizes a caller, so empty or
// malformed values are skipped. Whitespace inside a number is tolerated —
// the values are typed by hand into the Cloud console.
func parseAllowlist(items []metadataItem) map[string]bool {
	allowed := make(map[string]bool)
	for _, item := range items {
		if !strings.HasPrefix(item.Key, "voice-contact-") {
			continue
		}
		number := strings.Join(strings.Fields(item.Value), "")
		if e164.MatchString(number) {
			allowed[number] = true
		}
	}
	return allowed
}
