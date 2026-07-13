# Cloudflare Containers migration

Cloudflare Containers is the first option that cleanly fits the whole
architecture. The recommended design is one named Cloudflare Container per
OpenAI `call_id`, with SIP left completely untouched.

```text
Twilio ═══════ SIP audio ═══════ OpenAI
                                  │ webhook
                                  ▼
                         Cloudflare Worker
                                  │ call_id
                                  ▼
                         Container(call_id)
                                  └── sideband WSS ──► OpenAI
```

## Why this fits

The concurrency objection exposes a second defect in the current design: the
webhook always scales the pool to exactly `1` in
[`handler.go`](02_telephony/src/internal/handler/handler.go), while
[`session.go`](01_session_image/src/internal/session/session.go) drives calls
serially. A second simultaneous call therefore waits behind the first and
times out.

Cloudflare's explicit-ID model fixes that naturally:

- Zero calls means zero running containers.
- Two calls produce two containers because their `call_id` values differ.
- Each container exits independently when its call ends.
- `max_instances = 10`, for example, is only a safety ceiling, not ten warm
  instances.

That is Cloudflare's currently documented scaling mechanism: create or address
instances using unique IDs and stop them individually. Its
[scaling documentation](https://developers.cloudflare.com/containers/platform-details/scaling-and-routing/)
specifically describes this model for short-lived, explicitly controlled
workloads.

## Proposed architecture

1. Restore the Cloudflare Worker front door and contacts KV that already
   existed in commit `baa4d77`. The later migration commit says it was retired
   specifically because a Worker could not hold the sideband WebSocket for the
   whole call.
2. Have the Worker verify the OpenAI signature, check KV, then call
   `getByName(call_id)` and start the corresponding container.
3. Adapt the existing Go driver into a one-call HTTP service. It accepts the
   OpenAI call, attaches the existing sideband client, reports `ready` to the
   Worker, and continues in the background.
4. When the sideband closes, have the Go process exit, immediately stopping
   that call's container. Set `sleepAfter = "1h"` only as a safety fuse; the
   default ten minutes would otherwise kill a long call.
5. Keep the mid-call allowlist check by letting the container query KV through
   an internal Worker binding. Cloudflare explicitly supports
   [connecting Containers to Worker bindings](https://developers.cloudflare.com/containers/platform-details/workers-connections/).
6. After proving the new path, remove the GCP function, load balancer, Pub/Sub,
   worker pool, Artifact Registry, and associated IAM. The stable hostname can
   remain unchanged.

The public request path and call-lifetime runtime belong together on
Cloudflare. Keeping the GCP webhook in front would preserve the approximately
$18/month load balancer and require an authentication bridge back to GCP for
mid-call allowlist checks.

## Startup behavior

The timing claim is promising but not guaranteed. Cloudflare says cold starts
"can often" be in the 1–3 second range depending on image size and startup
work; that is not a p99 SLA.
[Containers became generally available on April 13, 2026](https://developers.cloudflare.com/changelog/post/2026-04-13-containers-sandbox-ga/),
and the
[lifecycle documentation](https://developers.cloudflare.com/containers/platform-details/architecture/)
explains the pre-fetched-image startup model.

The existing static, distroless Go image in
[`Dockerfile`](01_session_image/src/Dockerfile) is nearly ideal for this. The
driver should begin listening immediately and do no call-specific work before
its readiness port is available. The Worker should wait until the container
has accepted the call and attached the sideband before returning success from
the webhook.

## Economics

The economics work without keeping spare capacity running:

- Workers Paid has a $5/month minimum.
- A `lite` container has 256 MiB of memory and 2 GB of disk.
- Included usage provides exactly 100 `lite` instance-hours per month, or
  about 109 calls at the current 55-minute cap.
- Beyond the included usage, memory plus disk costs approximately `$0.0025`
  per 55-minute call.
- Fully consuming the available 1/16 vCPU throughout such a call would add
  about `$0.0041`; the mostly idle sideband driver should use substantially
  less active CPU.
- Sideband JSON traffic is tiny because audio remains SIP-direct.

These figures come from
[Cloudflare's Container pricing](https://developers.cloudflare.com/containers/pricing/).
Workers, Durable Objects, and logs are metered separately, but this workload
should remain inside their paid-plan allowances at the expected volume. The
existing approximately $18/month GCP load balancer disappears, so the base
economics improve rather than merely shifting compute providers.

## Lifecycle and recovery

The `sleepAfter` timer is driven by incoming container activity, not by the
outbound OpenAI WebSocket. It must therefore be set beyond the 55-minute
session cap. Normal cleanup should not wait for that timer: the Go process
should exit as soon as its call finishes so billing stops with the call.

Cloudflare does not guarantee that an individual container will remain on the
same host for any particular duration. Its
[FAQ says hosts restart irregularly](https://developers.cloudflare.com/containers/faq/).
That does not kill the SIP call, but it can temporarily lose tools, monitoring,
transcription, and allowlist enforcement.

The hardened design should store the active call assignment in the
container's Durable Object. If the process receives a platform `SIGTERM`, a
replacement instance can read the stored `call_id` and reattach. OpenAI
documents attaching a sideband WebSocket to an already accepted call using
its
[`call_id`](https://developers.openai.com/api/docs/guides/realtime-sip#monitor-call-events).
Actual reconnection behavior should still be tested because the documentation
does not explicitly promise every reconnect edge case.

Planned deployments should configure a rollout grace period longer than the
session cap so active calls can finish before their containers become eligible
for replacement.

## Canary acceptance criteria

Before dismantling GCP, deploy a narrow canary and verify:

1. Cold-start p99 from webhook receipt to OpenAI accept is under 10 seconds,
   leaving useful margin inside the observed 15–20 second ring budget.
2. Two simultaneous real calls each receive a distinct container and are
   accepted independently.
3. A full 55-minute sideband connection remains healthy and its container
   stops promptly afterward.
4. A forced rollout or host restart during a call leaves the SIP call alive
   and successfully reattaches its sideband.
5. Duplicate webhook deliveries for the same `call_id` are idempotent and
   route to the same logical container.
6. A failed or over-budget cold start produces a deliberate rejection or
   retryable webhook failure rather than leaving the caller ringing until an
   opaque server error.

If those tests pass, this migration directly fixes both 0→1 latency and 0→N
concurrency, preserves the simple SIP media path, and removes most of the
current infrastructure rather than adding another layer.
