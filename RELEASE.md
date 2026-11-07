# Releasing the Go module

Module: `github.com/Rayon-API/rayon-sdk-go/v3`. Package: `rayon`. Go 1.23 or newer.

Before release, run from a clean reviewed checkout:

```sh
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
go list -m all
```

Review the tracked file list for secrets, captures and customer material. Tests use synthetic loopback servers and make no billable API calls. Check the API contract, shared profile fixtures and embedded error catalog against the canonical service before changing them.

A maintainer must create the public GitHub repository at the exact module path and push the reviewed source. Tag the reviewed v3 module commit `v3.0.0-rc.1`; later versions follow Go semantic versioning. Push tags only after explicit release authorization. Go consumes source directly from the repository and version tags; there is no binary upload, registry token or automatic publish workflow. Confirm the version resolves from a clean external consumer after publication. Never move or replace a published tag. Major versions from v2 onward require the corresponding module path suffix.

Local testing and this repository's CI do not create commits, tags, repositories or releases.

## Managed-material release gate

Candidate: 3.0.0-rc.1. Verify managed header generation, required custom preparation and ready references,
materialless-preparation rejection, safe fallback metadata and all existing
preparation/cancellation tests. Check package contents and a clean consumer.
Publish only after the maintainer approves the API contract and release report.
Keep Go's `/v3` import path. Publish core JS before the Playwright adapter, then
examples and docs. Local builds do not publish packages or deploy services.

## Preparation-required release candidate

This branch is not published or deployed. Generation rejects raw sources and
never uploads sources, starts learning or retries on expiry. Custom material
must reach ready before its reference is used. Make compatible preparation available, distribute reviewed SDKs, migrate clients,
then enforce the new solve contract. Publish core SDK major 3 before Playwright major 2.
Keep all account/context/readiness and billing checks. Do not move published tags.
Refresh the Playwright registry lock entry after core JS is published; the new
core version cannot resolve from npm before that publication.
