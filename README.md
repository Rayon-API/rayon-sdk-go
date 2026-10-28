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

Go 1.23+, standard library only. Keep the `/v2` import path.

```sh
go get github.com/Rayon-API/rayon-sdk-go/v2@v2.1.0
```

## Generate

```go
package main

import (
    "context"
    "log"
    "os"
    rayon "github.com/Rayon-API/rayon-sdk-go/v2"
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
    if result.FallbackReason != "" {
        log.Println("A fallback was used. Contact support with request ID:", result.RequestID)
    }
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

## Fallback reporting

A nonempty `FallbackReason` means a fallback was used. Contact support with
`RequestID` about the mapping issue. `FallbackMessage` provides safe guidance
when available. Do not infer fallback from timing alone.

Results retain optional `Path`, `MaterialReuse`, `DiscoveryState` and
`DiscoveryDurationMS`. Pointer fields distinguish absent values from false/zero.

## Optional material and preparation

For an explicit override, supply either `scripts` (complete ordered sources) or
`material_ref` (a private reference), never both. Omit the unused field; null and
empty values are invalid. Customer uploads and references remain private to the
API key. Automatic requests should keep both fields omitted on every call so
new public material can be selected.

Form mode still requires explicit material. Set `mode: "form"`, supply the exact
URL-encoded POST application body before telemetry insertion, and prepend the
returned `form_body` once. It contains telemetry only. Preserve duplicate fields
and encoding. See [form integration](https://docs.rayonapi.com/form-payload-integration).

Preparation is optional for explicit material. It checks the supplied context
before a workload; it is separate from managed selection and generation.
Usable validated mappings can be ready without every internal record being mapped.
Generated headers and form fields must still pass output validation.

```go
ready, err := client.PrepareAndWait(ctx, explicitShape)
if err != nil { return err }
explicitShape.Scripts = nil
explicitShape.MaterialRef = ready.MaterialRef
result, err := client.Generate(ctx, explicitShape)
```

`Prepare`, `GetPreparation`, `WaitUntilReady` and `PrepareAndWait` remain
available. Waits last at most 60 seconds or the caller's earlier context deadline.
Cancelling a wait leaves the server job running. Typed errors retain
`PreparationID` for resumption; terminal kinds are `preparation_failed` and
`preparation_expired`. Prepare current explicit context again after readiness
expiry or relevant input changes.

## Client behavior

Reuse one client for connection pooling. `Usage(ctx)` is free; numeric usage
values use `json.Number`. Default API URL is `https://api.rayonapi.com`, timeout
45 seconds. `WithBaseURL`, `WithTimeout` and context deadlines configure calls.
Errors expose safe billing and retry metadata. Check history before retrying
uncertain outcomes. Only confirmed uncharged material expiry receives one
recovery with retained explicit source. No other generation error is retried.
Automatic requests keep material omitted on every call.

See [client reference](docs/reference.md), [errors](https://docs.rayonapi.com/errors),
[release instructions](RELEASE.md), [security](SECURITY.md) and [license](LICENSE).
