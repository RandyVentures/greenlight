# Confidence, evidence, and validation

Confidence is the strength of evidence for a **specific check**, not a probability
of Apple rejection. The labels are an ordinal rubric, not statistically calibrated
percentages. Severity describes potential impact if the finding is applicable;
it does not increase confidence. Informational source reminders are optional
unverified checks, not suspected defects. UI rows show both status and confidence, with an
explanation when expanded. HTML/JSON exports preserve the explanation.

| Confidence | Interpretation |
| --- | --- |
| High | Direct measurement or comparison of supplied inputs, such as image dimensions or catalog membership. The inputs can still be stale or from the wrong candidate/account. |
| Medium | Bounded evidence requiring context, such as recognized screenshot text, a recorded human attestation, or a linked history retest. It does not establish the entire user flow. |
| Low | Static source heuristic or ambiguous evidence; inspect build configuration and actual shipped/runtime behavior before treating it as a defect. |
| Insufficient evidence (`none`) | The check cannot reach a conclusion. Missing evidence is not evidence of a broken app. |

A high-confidence OCR positive is still a text match requiring contextual review,
not an authenticated Apple decision. A successful source-literal match proves only
that text exists in the selected file; it does not prove the product is used.
Missing literals remain unverified because products may come from SDK offerings
or server configuration. Concrete supplied-catalog mismatches remain actionable.

## Source scope and false-positive controls

- The local UI honors the selected Apple platform. General `preflight` retains
  automatic cross-platform scanning; an iOS UI run excludes Google Play policy
  and copied Android source/assets.
- Common scratch/generated/dependency output (`.tmp`, `out`, `.next`, `.build`, etc.) and recognized
  test files/directories and build/lint configs are excluded from source/privacy heuristics.
  Runtime Capacitor/Expo app configs remain in scope. This is not
  a complete build-graph resolver. Inspect final candidate artifacts separately.
- When a project has explicit shipping Apple target source paths in XcodeGen,
  source/privacy scanning uses those paths. Unknown source syntax falls back to a
  broader scan; build scripts, implicit sources, and final packaging still need
  artifact review. Separate marketing websites are not native app metadata.
- SDK rules use specific SDK symbols/module names rather than broad prose matches.
  An analytics/advertising SDK reference alone does not establish tracking or ATT.
  Attribution setter names in purchase SDK wrappers do not prove that the named
  advertising SDK is installed. Real SDK symbol/import cases remain tested.
- A same-file Swift helper named `stat` is distinguished from native calls; explicit
  Darwin calls and ordinary output-buffer calls remain detectable. Arbitrary
  aliases, wrappers, and overloads still need manual/build-level inspection.
- Consistent XcodeGen `info.properties` and generated `INFOPLIST_KEY_*` values are
  considered only for the matching Info.plist and configurations. Values in another
  target, or empty/conflicting configurations, cannot satisfy the check.
  Other Xcode build-system overrides remain outside this static resolver.
- Apple's `CFBundleName` fallback is recognized when `CFBundleDisplayName` is absent.

A source checkout may be newer than the approved App Store release. Reports label
whether a distribution artifact was supplied. Approval of one build does not
validate all later working-tree changes; a warning in development source does not
prove the published app has a defect. Record the actual version/build and evidence
source before drawing release conclusions.

## Reproducible validation

```sh
go test -race ./...
go vet ./...
cd internal/localui/web
npm ci
npm run format:check
npm test
npm run build
```

Regression fixtures cover custom-vs-native API calls, SDK prose vs real imports,
generated/test exclusions, platform selection, per-plist/config injection, bundle
name fallback, dynamic product IDs, real catalog mismatches, missing evidence,
OCR confidence, and resolved-history retests. Positive and negative cases protect
against both false positives and suppressing a genuine finding. These local tests
do not measure accuracy over all real App Store reviews or establish a calibrated
rejection probability. Hosted CI success must be checked separately.

## Official sources reviewed for these corrections

- [Xcode information property list values](https://developer.apple.com/documentation/bundleresources/managing-your-app-s-information-property-list)
- [CFBundleName and display-name fallback](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundlename)
- [Required reason API categories](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)
- [Apple's tracking definition and ATT](https://developer.apple.com/app-store/user-privacy-and-data-use/)

Review date: 2026-10-28. This scoped review does not certify every inherited rule
or automatically keep them current; see [guideline maintenance](guideline-maintenance.md).

## Second-stack validation

A separate approved React/Capacitor application was checked using a saved,
checksum-matched submitted IPA, its archive, selected public-asset allowlist,
and dated purchase snapshots. This exposed additional scratch-result, copied
Android-asset, tooling-config, and SDK-wrapper false positives. Generic fixtures
cover these corrections while retaining genuine app-owned required-reason API
warnings. Platform wording behind runtime guards remains a low-confidence
context check; the tool does not silently certify every branch as unreachable.

An approved application can still merit a privacy-declaration review. A manifest
inside an SDK bundle is evidence about that SDK; do not automatically use it to
clear every API call in app-owned source. Review the app target, compiled artifact,
and Apple's [required-reason API guidance](https://developer.apple.com/documentation/technotes/tn3183-adding-required-reason-api-entries-to-your-privacy-manifest).
No rule is waived because an application was approved, and a retained warning is
not a prediction that Apple will reject the next update. Two applications and
synthetic regression fixtures do not constitute a statistically calibrated benchmark.
