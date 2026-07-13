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
//
// identity is the pool's service account. The update mask keeps it from
// being changed, but it must still ride in the body: Cloud Run validates
// the caller's actAs against the *request's* template service account,
// and an absent one resolves to the project's compute default SA (denied,
// observed live 2026-07-13) instead of the pool's own.
func ScaleWorkerPool(ctx context.Context, pool, identity string, instances int) error {
	_, err := apiCall(ctx, http.MethodPatch,
		"https://run.googleapis.com/v2/"+pool+"?updateMask=scaling.manualInstanceCount",
		map[string]any{
			"template": map[string]any{"serviceAccount": identity},
			"scaling": map[string]any{
				"manualInstanceCount": instances,
			},
		})
	return err
}
