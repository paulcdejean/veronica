package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
)

// RunJob starts one execution of a Cloud Run job
// (projects/*/locations/*/jobs/*) with env overridden into the job's
// container, and returns the execution operation's name. Starting an
// execution needs roles/run.developer on the job — invoker alone cannot
// pass overrides.
func RunJob(ctx context.Context, job string, env map[string]string) (string, error) {
	if job == "" {
		return "", errors.New("job name is empty")
	}
	token, err := AccessToken(ctx)
	if err != nil {
		return "", err
	}
	overrides := make([]map[string]string, 0, len(env))
	for _, name := range slices.Sorted(maps.Keys(env)) {
		overrides = append(overrides, map[string]string{"name": name, "value": env[name]})
	}
	body, err := json.Marshal(map[string]any{
		"overrides": map[string]any{
			"containerOverrides": []map[string]any{{"env": overrides}},
		},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://run.googleapis.com/v2/"+job+":run", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	detail, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		return "", fmt.Errorf("jobs.run: %d %s", response.StatusCode, detail)
	}
	var operation struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(detail, &operation)
	return operation.Metadata.Name, nil
}
