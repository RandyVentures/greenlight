# Release acceptance checklist

The readiness layer carries our prior reusable release checklist into Greenlight.
It combines candidate and local metadata inspection with 15 feature-dependent
human acceptance gates. [Comparison with prior coverage](checklist-crosswalk.md).

## In the UI

Leave **Release acceptance checklist** enabled. Select the candidate `.app`,
`.xcarchive`, or `.ipa`, optional release metadata, and optional human evidence
JSON. Enable subscriptions, public user content, third-party content/services,
cloud sync, or safety-sensitive behavior under **Additional release features**.
Purchases and accounts use the main feature controls.

Run once to obtain the **Release evidence binding** in results or JSON. Missing
artifacts and human evidence stay unverified. This does not prevent a useful
source-only audit; it prevents source checks from claiming release acceptance.

## CLI

```sh
greenlight readiness --artifact /path/to/Release.xcarchive \
  --metadata /path/to/release-metadata.json \
  --purchases --subscriptions --accounts \
  --input /path/to/screenshots --input /path/to/catalog.json \
  --format json --output /path/to/readiness-first.json

# After performing and recording the actual acceptance work:
greenlight readiness --artifact /path/to/Release.xcarchive \
  --metadata /path/to/release-metadata.json \
  --purchases --subscriptions --accounts \
  --input /path/to/screenshots --input /path/to/catalog.json \
  --evidence /path/to/acceptance.json --exit-code
```

Use exactly the same selections and input ordering for both CLI runs. UI bindings
also include UI selection context; generate and reuse evidence through the same
interface. A UI binding is not interchangeable with a CLI binding.

Optional CLI flags: `--public-content`, `--third-party-content`, `--cloud-sync`,
`--safety-sensitive`. Report output paths must be new files. `--exit-code` fails
on failed/suspected checks or missing required evidence.

## Metadata and custom requirements

Copy [the synthetic metadata example](../examples/readiness/metadata.json).
`bundle_id`, `version`, `build`, and `team_id` optionally assert expected identity.
Name, description, and review notes must be present and within configured limits;
subtitle is optional. Privacy/support URLs require HTTPS; subscriptions also require
terms. URL checks validate syntax, not reachability or legal sufficiency. Metadata
is one local listing record; the human listing gate covers every intended locale.

Add `additional_reviews` for requirements specific to your app:

```json
{"additional_reviews":[
  {"id":"share-sheet","description":"Exercise the share extension on the current candidate."}
]}
```

Custom IDs must be unique lowercase letters/digits/underscores/hyphens, start with
a letter or digit, and contain at most 80 characters (maximum 100 custom gates).
This creates report ID `human.custom.share-sheet` and evidence key
`custom.share-sheet`. Changing these definitions changes the binding.

## Recording actual human evidence

Copy [the evidence template](../examples/readiness/evidence.template.json). Its
placeholder values intentionally cannot pass. Each applicable gate needs:

- `status`: `passed` or `failed`, based on work actually done.
- `reviewer`, nonempty `notes`, and `checked_at` in RFC 3339 UTC or offset time.
- `evidence_file` and its SHA-256 (`evidence_sha256`). Relative paths resolve from
  the evidence JSON's directory. Use a file with concrete test notes/results.
- The report's current `ruleset` and `binding` at the top level.

For an ordinary evidence file, obtain the hash with `shasum -a 256 device-notes.md`
on macOS or `sha256sum device-notes.md` on Linux. Do not hash the evidence JSON
itself. Keep notes outside the artifact and selected input directories to avoid
circular bindings. The UI requires evidence files to remain inside its workspace.

Built-in evidence keys: `scope`, `device`, `privacy`, `artwork`, `listing`,
`submission`, `archive`, `business`, `purchases`, `subscriptions`, `accounts`,
`moderation`, `rights`, `sync`, `safety`. Only applicable gates are included.
Report IDs prepend `human.`; evidence keys do not.

Evidence is accepted only for the current artifact/input/configuration/ruleset
binding, with an unchanged evidence-file hash and timestamp no more than seven
days old and not in the future. Artifact hashing includes sorted file paths and
contents; symlinks are hashed as link text without following them. Selected inputs
are hashed too. Evidence is self-reported acceptance, not independently verified
execution, an Apple signature, or proof of approval. A human pass cannot suppress
an automated failure.

## Candidate inspection limits

Directory candidates on macOS use local read-only `codesign` and `security`
inspection. Missing/unreadable profiles remain unverified; profile shape alone
does not establish distribution trust. IPA contents are read without extraction;
signing for IPA files and non-macOS hosts remains a human gate. Simulator platform
markers, version mismatches, and configured development/credential filenames are
reported. Xcode validation, actual uploaded-build provenance, physical-device QA,
purchase flows, and review-package membership must still be checked by a person.

The binding covers local evidence and configuration, not changing remote ASC
state. Fetch current account information separately when needed. Greenlight does
not download new Apple policy rules during this process.
