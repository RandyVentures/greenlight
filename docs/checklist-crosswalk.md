# Prior checklist → Greenlight crosswalk

This comparison uses the reusable Python app-store preflight and its 15 human
acceptance gates previously used across our apps. It also incorporates release
lessons about public screenshot copy and first-IAP submission membership. The
public implementation contains generic checks and synthetic examples; it does
not include private app configurations, reviewer credentials, or rejection logs.

“Existing” below means Greenlight before the readiness/history addition, including
this fork's screenshot, purchase-catalog, and exact-version ASC improvements.
It does not mean every feature was present in upstream Greenlight.

| Prior requirement | Existing coverage | Added coverage / remaining boundary |
| --- | --- | --- |
| Source patterns, permissions, privacy manifests | Existing source/privacy scanners | Retained; human privacy gate reconciles shipped behavior, SDKs, and published labels. |
| Public asset dimensions, alpha, pricing/trial text | Fork's local image checks and OCR | Human artwork gate adds visual correctness, paid disclosures, rights, and separation from reviewer-only evidence. OCR cannot establish complete visual compliance. |
| Product IDs, StoreKit, ASC/RevenueCat catalogs | Fork's offline purchase comparison | Human purchase/subscription gates cover real sandbox flows, restore, cancellation, pending states, expiry, refunds, and offers. |
| Exact app/version/build and localized listing | Fork's optional live ASC scan | Retained. Listing gate covers all intended locales and current claims; submission gate covers actual package membership and first IAPs. |
| Candidate identity and extension version/build | Existing IPA rules cover some binary facts; no acceptance checklist in UI | New `.app`/`.xcarchive`/`.ipa` inspection compares main and extension versions and optional expected bundle/version/build. |
| Distribution signatures, profile expiry, debug entitlement, team | Existing binary inspection is partial | New macOS inspection of directory candidates checks strict signature, readable entitlements, profile expiry/type, and optional expected team. IPA signing and Xcode validation remain human acceptance. |
| Exclude development fixtures and credential files | No release-packaging check in the UI | Candidate filename checks for `.storekit`, `.env*`, `.p8`, and `app-fixtures.json`. This is not a full secret or fixture-content scan. |
| Local metadata / reviewer notes / HTTPS links / keywords | Optional live ASC metadata; source metadata patterns | New local JSON checks required fields and lengths, URL syntax, and inherited conservative 100 UTF-8-byte keyword budget. URL reachability, page content, and all locale reconciliation remain human review. |
| Physical-device, upgrade, offline, permissions, persistence, crash/performance QA | Static checks cannot establish these | Required human device gate with candidate-bound, dated evidence. |
| Reviewer access, age/export/category/storefronts, selected build | Some optional ASC facts | Required submission gate. This tool never submits, creates reviewer accounts, or asserts first-IAP membership from catalog presence. |
| Agreements, tax/banking, release mode | Not established by source/catalog checks | Required business gate. |
| Login/deletion and privacy accuracy | Static patterns + unverified manual reminders | Required account/privacy gates when applicable; attestations do not execute these flows. |
| Moderation, content/service rights, production sync, safety | No complete acceptance coverage | Conditional human moderation, rights, sync, and safety gates. |
| App-specific acceptance requirements | No structured extension to human checklist | `additional_reviews` in metadata creates required custom gates, also linkable from rejection history. |
| Evidence tied to exact candidate/input/config/ruleset | No reusable human evidence binding | SHA-256 binding, reviewer, timestamp, evidence-file hash; expires after seven days. Changed bytes, inputs, selections, or readiness ruleset invalidate it. |
| Prior rejection prevention | No persistent project log | Private local history; open **and resolved** records are checked against current rules. Missing/unknown/skipped evidence never passes. |

## All 15 inherited acceptance gates

Always included: `scope`, `device`, `privacy`, `artwork`, `listing`, `submission`,
`archive`, and `business`. Feature-dependent: `purchases`, `subscriptions`,
`accounts`, `moderation`, `rights`, `sync`, and `safety`.

Custom gates supplement these; they cannot remove a required built-in gate or
turn an automated failure into a pass. Report IDs are `human.<gate>`.

## Deliberate limits

The earlier tool had its own app/version-scoped, age-limited ASC snapshot format.
The fork's live ASC checks and normalized purchase snapshots do **not** implement
that entire provenance format. Refresh local snapshots yourself; a matching file
hash does not establish freshness or correct account ownership. Runtime tests,
private service permissions, business status, and review-package membership still
need actual evidence. No historical rejection data was imported automatically.

Neither the old checklist nor this port guarantees Apple approval. Readiness rules
are bundled code; they do not automatically synchronize with Apple's latest policy.
See [readiness evidence](release-readiness.md), [history](rejection-history.md), and
[guideline maintenance](guideline-maintenance.md).
