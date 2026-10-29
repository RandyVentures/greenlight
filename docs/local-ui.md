# Local React workspace

[Architecture and diagrams](architecture.md) · [Guideline freshness](guideline-maintenance.md)

```sh
greenlight ui /path/to/your/app
# Or choose projects underneath a common parent:
greenlight ui /path/to/projects
```

The command opens your browser to a generated loopback URL. `--no-open` prints
the URL without opening a browser; `--port 8080` chooses a fixed loopback port
instead of an available one. Ctrl+C stops the server. Do not expose it through
a tunnel or reverse proxy; this is a local developer tool, not a hosted service.

## Choose the app and evidence

The workspace argument defines the file browser's boundary. Select a project
folder inside it, then choose iOS/iPadOS or macOS. Evidence paths are relative to
the **opened workspace**, not the selected project subfolder. Absolute paths
inside that workspace are also accepted. Use the folder buttons to avoid typing
paths. To work outside the current boundary, restart with the intended workspace.

Select StoreKit or RevenueCat to add product ID comparison and the relevant
catalog inputs. Enter expected product IDs; RevenueCat also asks for entitlement
identifiers. For a mixed catalog, leave the entitlement blank on consumables that
do not grant one; at least one expected entitlement is required in RevenueCat mode.
Catalogs use the same [purchase snapshot formats](purchases.md) as the
CLI. Missing catalogs or source evidence remain unverified. Extra catalog products
are allowed so legacy subscriptions do not need to be removed.

Screenshot OCR uses the local Tesseract executable and installed language packs.
Select one intended display size per run. See [screenshot limitations](screenshots.md).
For localization, use directories named after the requested locale codes directly
under the screenshot folder, e.g. `screenshots/en-US/` and `screenshots/es-MX/`.
Missing folders are flagged for review because store fallback may be intentional.
Folder coverage does not verify translation accuracy.

## What the checklist means

Core source/privacy checks always run; feature declarations add checks rather
than suppressing code patterns that might reveal an undeclared feature.

- **Automated:** source/configuration inspection, screenshot OCR, purchase catalogs.
- **Automated + manual:** requested locale folders plus translation review.
- **Human acceptance:** device, privacy, submission, business, and conditional feature gates require current bound evidence; missing evidence stays unverified.
- **Past rejections:** retest linked current rules and preserve prevention reminders.
- **Online · opt-in:** read the exact ASC version/build using a saved API key.

Selecting “No in-app purchases” excludes purchase catalog comparison; it does not
report purchases as tested or passed. Manual checks without valid bound human
evidence stay unverified, and required missing evidence makes coverage incomplete. Source-pattern findings are heuristic
and need review. Concrete catalog/dimension mismatches appear as blockers.

App Store Connect checks make outbound requests only after their toggle is enabled
and an audit is started. They may also check URLs in your store metadata. Configure
the saved API key with `greenlight auth setup`; the key never enters the browser.
The local UI cannot create credentials, change store records, or submit an app.

## Release evidence and rejection history

Leave **Release acceptance checklist** enabled to include the prior 15-gate
checklist. Select a `.app`, `.xcarchive`, or `.ipa` artifact, local release metadata,
and optional acceptance evidence. Additional feature toggles select conditional
gates. The report includes the binding needed to record actual human acceptance;
changed candidates, inputs, or selections invalidate it. See [readiness inputs,
custom gates, and evidence](release-readiness.md).

Open **Rejection history** to save an actual Apple rejection or an internal finding,
its cause, fix, and prevention steps. Link a repeat rule or custom acceptance gate.
**Recheck past rejections** adds current retests, even for resolved records. Skipped
or missing linked evidence remains unverified. History persists outside app repos
by default; [storage and CLI details](rejection-history.md).

## Results and export

Progress shows actual check areas, not an estimated completion percentage. Results
separate blockers, suspected issues, passed checks, and unverified evidence.
Expand a finding for its evidence, suggested next step, confidence, and policy
source. Up to 12 local screenshot thumbnails are included; recognized text regions
are highlighted in the interactive view.

Select HTML or JSON and export the report. HTML embeds evidence thumbnails and
needs no server or remote resources to view. JSON includes machine-readable check
statuses. Reports may contain private code paths, recognized text, and screenshots;
review them before sharing. Exported HTML shows the server's local timezone in its
timestamp. Neither format predicts Apple's review outcome.

One audit runs at a time. The latest eight reports remain in process memory until
the server exits. Inputs and reports are not saved to application storage. Explicitly
saved rejection records persist in the OS configuration directory; history findings
in exports include summaries and prevention steps. A random local session credential is stored in the current browser tab's session
storage so reloading continues to work. The printed URL puts that credential in
the fragment; the app removes it from the address bar after loading.

## Development

Normal Go builds use committed production assets and require **no Node installation**.
To change the React source, use Node 22 and npm:

```sh
cd internal/localui/web
npm ci
npm test
npm run build
cd ../../..
go test -race ./...
go build -o build/greenlight ./cmd/greenlight
./build/greenlight ui .
```

React and React DOM are the only frontend runtime packages. esbuild generates the
minified JS/CSS and precompressed assets under `internal/localui/web/static/`, which
Go embeds. Commit the rebuilt assets with source changes. CI rebuilds them and
checks for drift. There is no UI framework, router, database, telemetry, or CDN.

The server uses a random session token, same-origin request checks, Host validation,
and a workspace path boundary including symlink resolution for selected paths.
The file browser hides dotfiles, dependency directories, and symlinks. Tests cover
unauthorized requests, path escapes, missing evidence, report escaping, and jobs.

The included example is synthetic and is available when the opened workspace
contains this repository's `examples/ui-demo` directory. It performs no purchase,
login, account deletion, or online ASC operation.

The UI uses the installed version's bundled checks. It does not automatically
download Apple's latest policy or update scanner rules. Enabling ASC fetches app
data, not new guidelines; see [guideline maintenance](guideline-maintenance.md).
