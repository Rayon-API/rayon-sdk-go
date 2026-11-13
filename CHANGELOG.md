# Changelog

## v3.0.1 (unreleased)

- Accept managed preparation fallback responses and preserve `path` and `fallback_reason`.
- Advertise fallback support automatically without changing generation request bodies.
- Reject unknown or inconsistent fallback metadata and never replay generation automatically.

## 3.0.0

- Require preparation for custom material; use ready references for generation.
- Keep managed generation available without material fields.
- Allow preparation waits up to 30 minutes and retain job IDs for resuming a wait.
- Remove generation-time source uploads and automatic material replacement.

## 3.0.0-rc.2

- Allow preparation waits up to 30 minutes by default; explicit timeouts and cancellation remain supported.
- Keep per-request deadlines bounded and retain job IDs for resuming a wait.

## Preparation-required release candidate

- Generation accepts ready managed material or a ready private reference; raw sources belong to preparation.
- No generation source-upload cache, automatic expiry recovery, foreground learning or solve-time preparation.
- Existing preparation/wait helpers and supported-context checks remain; refresh outside generation.
- Breaking candidate: core SDK major 3, Playwright major 2. Not published or deployed.


## 2.1.0 — managed material

- Allow omitted material for managed header generation; preserve explicit overrides and preparation.
- Keep automatic selection current across repeated requests and material changes.
- Expose safe fallback and operation metadata. Keep existing retry, privacy and output validation.

## 2.0.0 (explicit material preparation)

- Add explicit preparation, status polling, bounded context-aware waits, safe ready metadata and resumable errors for the available additive preparation API.
- Document complete-output validation and validated generation for ready preparations; keep mandatory prepared-only cutover disabled, with the 30-day migration schedule announced separately.
- Move the module/import path to `github.com/Rayon-API/rayon-sdk-go/v2`.
- Keep generation signatures, result frames and confirmed unbilled source-miss recovery unchanged.

## 1.0.1 — 2026-10-23

- Clarify generation failure and incomplete-result messages with conditional script-loader and support guidance.
- Explain generated header and form validation failures without assigning fault to supplied material.

## 1.0.0 (unreleased)

- Add the standard-library Go client for header/form Shape generation and usage.
- Preserve exact request bodies and explicit JSON nulls with validated browser profiles.
- Add per-client bounded material caching, concurrent upload coalescing and one confirmed uncharged material-expiry recovery.
- Add strict bounded JSON, sanitized error catalog guidance and context cancellation/deadlines.
