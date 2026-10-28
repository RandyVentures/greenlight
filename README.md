# Greenlight — RandyVentures fork

Free, open-source tools for finding App Store submission problems before you submit.
Local screenshot OCR, purchase catalog comparison, and checks against the exact
App Store version and attached build you intend to submit.

Based on [RevylAI/greenlight](https://github.com/RevylAI/greenlight). Original
copyright and MIT license are preserved. This fork is maintained by RandyVentures;
it is not affiliated with Apple and does not predict or guarantee approval.

## Install

With Go 1.24 or newer:

```sh
go install github.com/RandyVentures/greenlight/cmd/greenlight@main
```

Or build this fork:

```sh
git clone https://github.com/RandyVentures/greenlight.git
cd greenlight
make build
./build/greenlight --help
```

Screenshot recognition uses [Tesseract](https://github.com/tesseract-ocr/tesseract)
installed locally. Use `brew install tesseract` on macOS or
`sudo apt-get install tesseract-ocr` on Ubuntu. Install additional language packs
for screenshots in other languages. Other commands do not require OCR.

The upstream Homebrew formula installs upstream Greenlight, **not this fork**.
There are no fork-specific binary releases yet; build from source or use Go.

## What's added

### Screenshot auditing

```sh
greenlight screenshots ./screenshots/en-US --display-type APP_IPHONE_67 --format html --output screenshot-report.html
greenlight screenshots ./screenshots/es-MX --display-type APP_IPHONE_67 --ocr-language spa --format json --exit-code
```

Recursively reads PNG/JPEG files, checks dimensions and PNG alpha declarations,
and uses local OCR to flag pricing/free-trial copy, placeholders, and references
to other mobile platforms. HTML reports embed images and highlight matching text.
No screenshot upload, AI account, or API key is needed.

Use one device display type per invocation; keep different sizes in separate
folders. Without `--display-type`, dimensions are explicitly **unverified**.
Unknown display types, unreadable files, missing OCR, and low-confidence negative
text checks never count as clean results. Text flags require human review,
including legitimate in-app content that happens to match a phrase.

[Screenshot documentation and limitations](docs/screenshots.md)

### Purchase configuration consistency

```sh
greenlight purchases examples/purchases/config.json --format json
greenlight purchases ./purchase-audit.json --exit-code
```

Compares expected product IDs against quoted literals in source, Xcode `.storekit`
files, supplied ASC product snapshots, and a documented normalized RevenueCat
product/entitlement snapshot. Extra legacy products are allowed.

This command runs offline. It does **not** fetch live catalogs, execute purchases,
verify restore/refund behavior, or prove IAPs are attached to a submission.
Those behaviors remain **unverified**, even when every catalog matches.

[Purchase snapshot format and example](docs/purchases.md)

### Exact App Store submission checks

```sh
greenlight auth setup
greenlight scan --app-id YOUR_APP_ID --platform IOS --version-id YOUR_VERSION_RESOURCE_ID --build 8 --format json --exit-code
```

Uses read-only ASC API requests. All version-dependent checks use the selected
version. Build processing and encryption checks follow **that version's attached
build**, never the newest upload. `--build` asserts an expected attached build
number; it does not change Apple's selection.

Without `--version-id`, the scanner selects a version only if exactly one editable
version matches. Ambiguous versions require explicit selection. Versions,
localizations, screenshot sets, and screenshots are paginated. Screenshot checks
inspect every returned localization and verify asset delivery state.

API failures mark coverage incomplete in terminal, JSON, and JUnit reports;
`--exit-code` fails on blockers or incomplete coverage. Required device coverage,
locale fallback, privacy declarations, and functional QA still require review.

`scan` implements tiers 1–2 (metadata and text rules); binary inspection is a
separate `preflight --ipa` operation. Unknown display types are unverified rather
than rejected by an outdated size table.

## Reading results

The new local auditors report each check as:

| Status | Meaning |
| --- | --- |
| `passed` | Supplied evidence satisfied this particular check. |
| `failed` | A concrete mismatch or invalid input was found. |
| `suspected` | A heuristic matched; inspect the evidence before changing anything. |
| `unverified` | Evidence is missing, unreadable, unsupported, or outside scope. |

Reports include confidence, evidence, policy links where applicable, and coverage
counts. Coverage measures checks evaluated; it is **not** a rejection probability.
`--exit-code` on local auditors fails for failed/suspected findings or missing
**required** evidence. Optional out-of-scope runtime checks do not fail a catalog
comparison. Without `--exit-code`, findings are reported without failing the command.

Report files must be new paths; auditors refuse to overwrite existing files.
Keep private screenshots and account snapshots out of public Git history.

## Existing scanners

The fork retains upstream's source, privacy, binary, and Android scanners:

```sh
greenlight preflight . --ipa build.ipa --format json --exit-code
greenlight codescan .
greenlight privacy .
greenlight ipa build.ipa
greenlight playscan . --apk app-release.apk --exit-code
greenlight guidelines search privacy
```

Static rules are heuristics, including inherited rules. A source match cannot
prove a flow works. Platform/payment rules can depend on region and app category;
review current official policy before applying a suggested fix.

The inherited `verify` command integrates with the external Revyl cloud service
and requires its CLI/account. It is optional and **not required** for this fork's
additions. We have not validated that external service in this fork.

## CI and contributing

```yaml
- uses: actions/checkout@v4
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
- run: go build -o build/greenlight ./cmd/greenlight
- run: ./build/greenlight purchases examples/purchases/config.json --exit-code
```

Run `go test -race ./...`, `go vet ./...`, and `gofmt -l .` before contributing.
Tests use synthetic fixtures and mocked read-only Apple API responses. They do
not establish agreement with Apple's real review decisions. See
[CONTRIBUTING.md](CONTRIBUTING.md) for rules and false-positive reports.

## License and attribution

[MIT](LICENSE). Greenlight was created by Revyl and its contributors.
RandyVentures maintains this fork's additions. No paid tier, subscription,
or hosted service is required for the screenshot, purchase, or ASC audit tools.
