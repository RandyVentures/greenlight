---
name: greenlight
description: Audit App Store submission evidence using Greenlight, including screenshots, purchase catalogs, source/build checks, and read-only App Store Connect checks. Use for requested submission-readiness or rejection-risk reviews.
---

# Greenlight submission audit

Use RandyVentures' free fork to find submission problems and report evidence.
A clean scan does not establish App Review approval or working purchase flows.

## Install and choose the relevant checks

If the fork is missing, install with:

```sh
go install github.com/RandyVentures/greenlight/cmd/greenlight@main
```

The upstream Homebrew formula installs upstream rather than this fork.

- Source/build inspection: `greenlight preflight . --ipa build.ipa --format json`.
  Omit `--ipa` when no artifact is available and disclose that coverage gap.
- Screenshots: `greenlight screenshots ./screenshots --display-type APP_IPHONE_67 --format json`.
  Use the actual intended display type, one device-size folder per run. OCR needs
  local Tesseract and the relevant `--ocr-language` packs. Suspect text requires
  human review; missing/low-confidence evidence remains unverified.
- Purchases: `greenlight purchases ./purchase-audit.json --format json`.
  Read [the configuration and snapshot formats](https://github.com/RandyVentures/greenlight/blob/main/docs/purchases.md)
  before preparing inputs. Compare existing evidence; do not invent catalogs or
  entitlements to make a report pass. Omitted local StoreKit evidence is optional;
  missing ASC/source evidence or required RevenueCat mappings is incomplete.
- Live ASC: `greenlight scan --app-id ID --version-id VERSION_RESOURCE_ID --build BUILD_NUMBER --format json`.
  Use known identifiers and existing credentials. This command only reads ASC.
  It follows the selected version's attached build. Without an explicit version,
  only a single matching editable version can be selected; ambiguity needs resolution.

Choose checks that match the user's request and available evidence. Do not
install tooling, create credentials, modify code, submit, publish, purchase, or
invoke a cloud test service unless authorized by the actual task. The inherited
Revyl `verify` integration is optional and does not run as part of these additions.

## Triage and verify

Prioritize concrete blockers, then heuristic warnings. Inspect the cited evidence
and current official policy before applying fixes; platform/payment rules have
exceptions. Apply fixes only within the user's requested scope. Re-run affected
checks after changes, and stop when actionable issues are resolved or the remaining
checks require evidence unavailable to the current task. Never loop merely to
obtain a particular verdict string.

Report failed, suspected, and unverified checks separately. Preserve missing
coverage rather than suppressing it. Purchase catalog matches do not test sandbox
purchase, restore, refunds/revocation, or product attachment to a submission.
Do not invent approval percentages or claim runtime verification from static scans.

JSON has no banner; `--exit-code` enables CI gating. `screenshots` and `purchases`
fail the gate on failed/suspected findings or missing required evidence. `scan`
fails on blockers or incomplete checks. Report paths must not already exist.

Original Greenlight: [RevylAI/greenlight](https://github.com/RevylAI/greenlight), MIT.
Fork and additions: [RandyVentures/greenlight](https://github.com/RandyVentures/greenlight).
