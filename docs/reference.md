# Client behavior

Reuse a client for connection pooling and material caching. It retains up to 128 source bundles and 16 MiB, refreshes within five seconds of expiry, and coalesces simultaneous first uploads. Every caller still generates its own fresh result. Cache state is isolated per client and cleared on close or eviction. Keep your own source copy. Failed uploads fail queued calls; canceling a queued caller does not cancel its owner's upload.

The only automatic retry is one HTTP 422 `material_expired` response explicitly marked `billing_status: "not_charged"`, when original source is still retained. Other outcomes are never retried by the SDK. Use `errors.As` with `*rayon.Error` for sanitized `Kind`, `StatusCode`, `RequestID`, `Code`, `Reason`, `Action`, `BillingStatus` and `RetryAfter`. Raw server messages and transport errors are excluded. Check request history before resending ambiguous outcomes.

Default timeout is 45 seconds, covering material queueing and recovery. Context cancellation/deadlines apply to the entire call. Configure `rayon.WithTimeout` or `rayon.WithBaseURL`; HTTPS is required except loopback development servers. Redirects are rejected. The client requests identity encoding and bounds JSON responses to 256 KiB, rejecting compressed responses and duplicate keys.
