# Client behavior

Reuse one concurrency-safe client for connection pooling. Managed header calls
omit both material fields on every generation. Custom sources belong to
`Prepare`/`PrepareAndWait`, not `Generate`. Only a reference with current native
readiness for the supported context can generate.

`Prepare` returns a job; `WaitUntilReady` and `PrepareAndWait` return ready metadata.
A queued job can already contain a reference. Check ready state, validated readiness,
`ReadyUntil` and `MaterialExpiresAt`. Renew outside generation before the earlier
deadline and switch references only after the replacement is ready.

`ShapeRequest.Scripts` is retained for `PreparationRequest` compatibility, but
`Generate` rejects it before transport. Generation never retains a source cache,
starts preparation, uploads scripts, retries automatically or uses a solve-time preparation.
The server enforces readiness independently of the SDK.

Use `errors.As` with `*rayon.Error` for safe status, code, request ID, retry and
billing metadata. Check request history before resubmitting an uncertain outcome.

Default timeout is 45 seconds. Context cancellation/deadlines cover the call;
preparation waits have their separate bounded budget. `WithTimeout` and
`WithBaseURL` configure transport. HTTPS is required except loopback development.
Redirects are rejected. Responses remain bounded and strictly parsed.
