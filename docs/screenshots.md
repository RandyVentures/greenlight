# Screenshot auditor

```sh
greenlight screenshots examples/screenshots --display-type APP_IPHONE_47 --format html --output /tmp/screenshot-report.html
```

The example image is synthetic, contains deliberate pricing/trial copy, and is
not a screenshot from a customer app. Its report should need attention.

## Inputs and dependencies

- PNG, JPG, JPEG files; a single file or a recursive directory. Symlinks are skipped.
- Local Tesseract on PATH, with requested language packs installed.
- `--ocr-language eng` by default; `eng+spa` recognizes both languages where installed.
- `--display-type` is an ASC identifier such as `APP_IPHONE_67`, `APP_IPHONE_65`,
  `APP_IPHONE_47`, `APP_IPAD_PRO_3GEN_129`, or `APP_DESKTOP`. See
  `internal/screenshots/dimensions.go` for the currently supported subset.
- Each file is limited to 30 MiB / 50 million pixels; OCR has a 30-second per-image timeout.

## Evidence and limitations

Dimension and alpha-channel checks are mechanical. Text checks are heuristic;
they preserve recognized text, confidence, and line coordinates. The HTML report
embeds images and uses those coordinates for highlights. It contains no scripts,
remote images, fonts, or tracking.

No recognized text makes OCR unverified. OCR words below 60 confidence make
negative pattern checks unverified. A low-confidence match remains visible as a
low-confidence suspected finding. OCR can miss text even when confidence is high;
always inspect screenshots before submitting.

Current rules recognize currency-symbol prices, selected currency codes, and
selected English, Spanish, French, German, and Portuguese free/trial terms.
Installing an OCR language pack does not add translated rules. Currency amounts
inside a finance app, for example, may be legitimate app content: a match does not
establish a violation. There is no automatic comparison of translations' meaning,
detection of exposed personal data, or feature verification.

Mixed device sizes should be audited separately with their intended display types.
The scanner does not infer which device sizes your app must submit, inspect the
live store, modify images, or upload anything. New report paths are required to
prevent accidentally overwriting source assets.

## Rule sources

Rules and the accepted-size subset were reviewed on **2026-10-28**:

- [Apple screenshot specifications](https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications/)
- [App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/):
  §2.1 (completeness), §2.3.7 (metadata/pricing), §2.3.10 (other platforms).

The dimensions table is dated evidence. Unsupported types are unverified; future
Apple policy changes need a rule update and a regression fixture.
