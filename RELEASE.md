# Releasing the Go module

Module: `github.com/Rayon-API/rayon-sdk-go/v2`. Package: `rayon`. Go 1.23 or newer.

Before release, run from a clean reviewed checkout:

```sh
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
go list -m all
```

Review the tracked file list for secrets, captures and customer material. Tests use synthetic loopback servers and make no billable API calls. Check the API contract, shared profile fixtures and embedded error catalog against the canonical service before changing them.

A maintainer must create the public GitHub repository at the exact module path and push the reviewed source. Tag the reviewed v2 module commit `v2.0.0`; later versions follow Go semantic versioning. Push tags only after explicit release authorization. Go consumes source directly from the repository and version tags; there is no binary upload, registry token or automatic publish workflow. Confirm the version resolves from a clean external consumer after publication. Never move or replace a published tag. Major versions from v2 onward require the corresponding module path suffix.

Local testing and this repository's CI do not create commits, tags, repositories or releases.


Version 2.0.0 is coordinated with backend preparation API deployment. Confirm
both preparation routes are deployed before tagging/publishing. Verify explicit
preparation, strict response metadata, terminal failures, bounded context-aware
polling and transient GET retries. Build a clean external consumer using the
`/v2` import path after publication. Local tests do not deploy the API or publish
a version.
