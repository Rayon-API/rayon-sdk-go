# Rayon Go SDK

Rayon manages current material for supported public sources. Send the source-page
URL, profile and exact request. No script collection or preparation is needed for
this header flow. Replace the reserved example URLs with your approved source and
endpoint. `{}` uses default profile values; supply measured values when needed.

Keep the API key server-side. Your integration owns the target HTTP client,
authentication, cookies and session. Rayon generates telemetry; your client sends
the target request.

[Quickstart](https://docs.rayonapi.com/quickstart) ·
[Material options](https://docs.rayonapi.com/material) ·
[API reference](https://docs.rayonapi.com/api-reference)

## Install

Go 1.23+, standard library only. Keep the `/v3` import path.

```sh
go get github.com/Rayon-API/rayon-sdk-go/v3@v3.0.1
```

## Generate

```go
package main

import (
    "context"
    "log"
    "os"
    rayon "github.com/Rayon-API/rayon-sdk-go/v3"
)

func main() {
    client, err := rayon.NewClient(os.Getenv("RAYON_API_KEY"))
    if err != nil { log.Fatal(err) }
    defer client.Close()
    result, err := client.Generate(context.Background(), rayon.ShapeRequest{
        SourceURL: "https://shop.example.com/account",
        Profile: rayon.Profile{},
        Request: rayon.TargetRequest{
            URL: "https://shop.example.com/api/check", Method: "POST",
            ContentType: rayon.String("application/json"), Body: rayon.String("{}"),
        },
    })
    if err != nil { log.Fatal(err) }
    // Apply result.Headers with your target client. Reuse client across calls.
}
```

All four request fields are required in JSON. Nil `ContentType` and `Body`
pointers encode null; `rayon.String("")` preserves an empty string. GET/HEAD
require a null body. Use `rayon.ValidateProfile` for offline profile validation.

Apply every returned header unchanged, including optional `a0`, to the exact
request. Keep application headers and session state in your target client.
Generate fresh telemetry for each request and inspect the target response to
check acceptance. Rayon returns no target cookies.

## Material and readiness

Generation accepts ready managed material or a ready private reference. Custom
scripts must go through preparation. Unsupported, expired or unprepared inputs
return an explicit error. Generation does not prepare material or retry automatically.

SDK 3.0.1 accepts a successful managed preparation fallback with `path: isolated`
and `fallback_reason: material_preparing`. These fields remain available on the
result so your application can report degraded operation. The SDK advertises this
capability automatically; generation request bodies stay unchanged. Older SDKs
remain native-only on a server configured for the capability migration. A fallback
requires fresh managed material and is never an automatic retry by the SDK.

## Custom material requires preparation

Preparation can take several minutes. Wait helpers return as soon as ready, with a
30-minute default limit. Timeouts and cancellation leave the job available to resume.

```go
ready, err := client.PrepareAndWait(ctx, explicitShape)
if err != nil { return err }
explicitShape.Scripts = nil
explicitShape.MaterialRef = ready.MaterialRef
result, err := client.Generate(ctx, explicitShape)
```

Submit complete sources and the intended context with the preparation helper. Wait for
`state: ready`, then use its `material_ref` for generation.
A queued job can already contain a reference; that does not establish readiness.
Readiness is scoped to supported inputs. Normal request inputs may vary only within
that documented scope; an unchanged program does not establish universal coverage.

Refresh custom material outside generation before the earlier of `ready_until` and
`material_expires_at`, or after a supported deployment change. Wait for the replacement
to become ready before switching references. Keep original sources on your side for
refresh. Expired references are never recovered by uploading scripts to a solve call.
For managed header calls, keep both material fields omitted on every request.
Form mode requires an explicit ready reference.

## Client behavior

Reuse one client for connection pooling. `Usage(ctx)` is free; numeric usage
values use `json.Number`. Default API URL is `https://api.rayonapi.com`, timeout
45 seconds. `WithBaseURL`, `WithTimeout` and context deadlines configure calls.
Errors expose safe billing and retry metadata. Check history before retrying
uncertain outcomes. Generation never retries or uploads sources on expiry. Refresh through preparation outside generation.
Automatic requests keep material omitted on every call.

See [client reference](docs/reference.md), [errors](https://docs.rayonapi.com/errors),
[release instructions](RELEASE.md), [security](SECURITY.md) and [license](LICENSE).
