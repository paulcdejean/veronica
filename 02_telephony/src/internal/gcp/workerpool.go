package gcp

import (
	"context"
	"net/http"
)

// ScaleWorkerPool sets the pool's (projects/*/locations/*/workerPools/*)
// manual instance count. Cloud Run treats the count as runtime state — no
// new revision — which is what lets it track live calls: the webhook
// scales to one on dispatch, the driver scales to zero when the line goes
// quiet.
func ScaleWorkerPool(ctx context.Context, pool string, instances int) error {
	_, err := apiCall(ctx, http.MethodPatch,
		"https://run.googleapis.com/v2/"+pool+"?updateMask=scaling",
		map[string]any{"scaling": map[string]any{
			"scalingMode":         "MANUAL",
			"manualInstanceCount": instances,
		}})
	return err
}
