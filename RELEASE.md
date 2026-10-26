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


Version 2.0.0 supports the available additive preparation API at
`/v1/materials/prepare` and `/v1/materials/preparations/{preparation_id}`. Merge
this postactivation release copy and tag/publish only after the maintainer records
backend and ingress GO. Ready requires validated complete output and Rust
publication; partial output is not ready, and ready preparations execute only
in Rust. The 30-day migration schedule is announced separately, with no fixed
cutoff set here. This release does not enable mandatory prepared-only/Rust-only
generation. Verify explicit
preparation, strict response metadata, terminal failures, bounded context-aware
polling and transient GET retries. Build a clean external consumer using the
`/v2` import path after publication. Local tests do not deploy the API or publish
a version.
