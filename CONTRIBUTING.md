# Contributing

This is RandyVentures' free MIT-licensed fork of RevylAI/greenlight.
Open issues and pull requests against `RandyVentures/greenlight`.

For a new rule, include the official policy link, the date it was checked,
the evidence required, and cases that must remain unverified. Add synthetic
regression fixtures covering a real match and a plausible false positive.
Never commit customer code, credentials, real reviewer logins, private screenshots,
or purchase receipts.

```sh
gofmt -w .
go vet ./...
go test -race ./...
go build ./...
```

Report false positives with a rule name and minimal redacted reproduction.
Do not equate fixture pass rates with Apple approval accuracy. Prefer deterministic
checks and evidence over invented rejection percentages. Keep live ASC checks
read-only and missing coverage visible in machine-readable reports.

Preserve upstream's license and attribution. Broadly useful fixes can be proposed
upstream separately; contributing here does not automatically open a pull request
against upstream.
