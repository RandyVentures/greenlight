package checks

import (
	"context"
	"fmt"
	"os"

	"github.com/RandyVentures/greenlight/internal/asc"
)

// Check is an individual compliance check function.
type Check func(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error

// Runner orchestrates all checks across tiers.
type Runner struct {
	client    *asc.Client
	verbose   bool
	checks    map[Tier][]namedCheck
	VersionID string
	Platform  string
}

type namedCheck struct {
	name string
	fn   Check
}

func NewRunner(client *asc.Client, verbose bool) *Runner {
	r := &Runner{
		client:  client,
		verbose: verbose,
		checks:  make(map[Tier][]namedCheck),
	}
	r.registerChecks()
	return r
}

func (r *Runner) registerChecks() {
	// Tier 1: Metadata & completeness (API-based)
	r.register(TierMetadata, "App exists & accessible", checkAppExists)
	r.register(TierMetadata, "App name length", checkAppNameLength)
	r.register(TierMetadata, "Version prepared", checkVersionPrepared)
	r.register(TierMetadata, "Metadata completeness", checkMetadataCompleteness)
	r.register(TierMetadata, "Screenshots uploaded", checkScreenshots)
	r.register(TierMetadata, "Screenshot dimensions", checkScreenshotDimensions)
	r.register(TierMetadata, "Build processed", checkBuildProcessed)
	r.register(TierMetadata, "Age rating declared", checkAgeRating)
	r.register(TierMetadata, "Encryption compliance", checkEncryption)
	r.register(TierMetadata, "Territory availability", checkTerritoryAvailability)
	r.register(TierMetadata, "Pricing consistency", checkPricingConsistency)

	// Tier 2: Content analysis
	r.register(TierContent, "Platform references", checkPlatformReferences)
	r.register(TierContent, "Placeholder content", checkPlaceholderContent)
	r.register(TierContent, "URL reachability", checkURLReachability)
	r.register(TierContent, "TestFlight external testing", checkTestFlightExternal)
}

func (r *Runner) register(tier Tier, name string, fn Check) {
	r.checks[tier] = append(r.checks[tier], namedCheck{name: name, fn: fn})
}

// Run executes all checks up to the specified max tier.
func (r *Runner) Run(ctx context.Context, appID, buildNum string, maxTier int) (*Results, error) {
	if maxTier < 1 || maxTier > 2 {
		return nil, fmt.Errorf("scan supports tiers 1-2; use preflight --ipa for binary inspection")
	}
	client, version, err := r.client.ForSubmission(appID, r.VersionID, r.Platform)
	if err != nil {
		return nil, err
	}
	results := &Results{
		AppID:     appID,
		VersionID: version.ID, Version: version.Attributes.VersionString, Platform: version.Attributes.Platform,
		Findings: []Finding{}, Checks: []CheckResult{},
		Limitations: []string{"Metadata checks do not exercise the app, purchase/restore/refund flows, or guarantee App Review approval.", "Required device coverage and locale fallback need manual confirmation. Unknown display types are unverified."},
	}
	build, buildErr := client.GetVersionBuild(version.ID)
	if buildErr == nil && build != nil {
		results.BuildID = build.ID
		results.BuildNumber = build.Attributes.Version
	}
	if buildNum != "" {
		if buildErr != nil {
			return nil, buildErr
		}
		if build == nil || build.Attributes.Version != buildNum {
			actual := "none"
			if build != nil {
				actual = build.Attributes.Version
			}
			results.Findings = append(results.Findings, Finding{Tier: TierMetadata, Severity: SeverityBlock, Confidence: "high", Title: "Attached build does not match --build", Detail: fmt.Sprintf("Expected %s; selected version %s has build %s", buildNum, version.ID, actual), Fix: "Select the intended build before submitting."})
		}
	}

	for tier := TierMetadata; int(tier) <= maxTier; tier++ {
		checks, ok := r.checks[tier]
		if !ok {
			continue
		}

		for _, check := range checks {
			if r.verbose {
				fmt.Fprintf(os.Stderr, "  [tier %d] running: %s\n", tier, check.name)
			}

			if err := ctx.Err(); err != nil {
				return nil, err
			}
			before := len(results.Findings)
			if err := check.fn(ctx, client, appID, &results.Findings); err != nil {
				results.Incomplete = true
				results.Checks = append(results.Checks, CheckResult{Name: check.name, Status: "unverified", Evidence: err.Error()})
				if r.verbose {
					fmt.Fprintf(os.Stderr, "  [tier %d] error in %s: %v\n", tier, check.name, err)
				}
				// Non-fatal: record as a finding rather than aborting
				results.Findings = append(results.Findings, Finding{
					Tier:       tier,
					Severity:   SeverityWarn,
					Title:      fmt.Sprintf("Check '%s' failed to run", check.name),
					Detail:     err.Error(),
					Confidence: "none",
				})
			} else {
				status := "passed"
				if len(results.Findings) > before {
					status = "findings"
				}
				results.Checks = append(results.Checks, CheckResult{Name: check.name, Status: status})
			}
		}
	}

	results.ComputeSummary()
	return results, nil
}
