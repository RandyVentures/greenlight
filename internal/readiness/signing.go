package readiness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/RandyVentures/greenlight/internal/audit"
	"howett.net/plist"
)

func signing(path, expectedTeam string) []audit.Check {
	unknown := func(message string) []audit.Check {
		return []audit.Check{{ID: "release-signing", Status: "unverified", Severity: "info", Confidence: "none", Required: false, File: path, Evidence: message}}
	}
	if runtime.GOOS != "darwin" {
		return unknown("Signature/profile inspection needs macOS; distribution validation remains a human gate.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	diagnostic, verifyErr := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", path).CombinedOutput()
	if verifyErr != nil {
		if ctx.Err() != nil {
			return unknown("Local code-signature verification timed out.")
		}
		return []audit.Check{signatureFailure(path, diagnostic)}
	}
	b, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--display", "--entitlements", ":-", path).Output()
	if err != nil {
		return unknown("Cannot read signed entitlements; no signing readiness claim.")
	}
	var entitlements map[string]any
	if _, err := plist.Unmarshal(b, &entitlements); err != nil {
		return unknown("Signed entitlements were not a readable plist.")
	}
	var profile map[string]any
	for _, p := range []string{filepath.Join(path, "embedded.mobileprovision"), filepath.Join(path, "Contents", "embedded.provisionprofile")} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		b, err := exec.CommandContext(ctx, "/usr/bin/security", "cms", "-D", "-i", p).Output()
		if err != nil {
			return unknown("Cannot decode embedded provisioning profile.")
		}
		if _, err := plist.Unmarshal(b, &profile); err != nil {
			return unknown("Embedded provisioning profile is unreadable.")
		}
		break
	}
	checks := AssessSigning(entitlements, profile, time.Now())
	if expectedTeam != "" {
		c := audit.Check{ID: "release-team", Status: "passed", Confidence: "high", Required: true, Evidence: "Signed team matches configured expectation."}
		if entitlements["com.apple.developer.team-identifier"] != expectedTeam {
			c.Status = "failed"
			c.Severity = "block"
			c.Evidence = "Signed team does not match the expected team."
		}
		checks = append(checks, c)
	}
	for i := range checks {
		checks[i].File = path
	}
	return checks
}

func AssessSigning(entitlements, profile map[string]any, now time.Time) []audit.Check {
	checks := []audit.Check{{ID: "release-debug-entitlement", Status: "passed", Confidence: "high", Required: true, Evidence: "Signed entitlements do not enable get-task-allow."}}
	if entitlements["get-task-allow"] == true || entitlements["com.apple.security.get-task-allow"] == true {
		checks[0].Status = "failed"
		checks[0].Severity = "block"
		checks[0].Evidence = "Development debugging entitlement is enabled in the signed candidate."
	}
	c := audit.Check{ID: "release-profile", Status: "unverified", Confidence: "none", Required: false, Evidence: "No embedded profile could be inspected. Verify distribution signing and upload provenance in Xcode."}
	if profile != nil {
		c.Required = true
		c.Confidence = "high"
		c.Status = "passed"
		c.Evidence = "Profile has an unexpired date and no device/ad-hoc/enterprise indicators; Xcode validation remains required."
		expires, ok := profile["ExpirationDate"].(time.Time)
		if !ok {
			c.Status = "unverified"
			c.Confidence = "none"
			c.Evidence = "Provisioning profile expiry is missing or unsupported."
		} else if !expires.After(now) {
			c.Status = "failed"
			c.Severity = "block"
			c.Evidence = "Provisioning profile is expired."
		}
		_, devices := profile["ProvisionedDevices"]
		if devices || profile["ProvisionsAllDevices"] == true {
			c.Status = "failed"
			c.Severity = "block"
			c.Evidence = "Device-restricted, ad-hoc, or enterprise profile is not an App Store distribution profile."
		}
	}
	return append(checks, c)
}

func componentPath(root, info string) string {
	info = strings.TrimSuffix(info, "Contents/Info.plist")
	info = strings.TrimSuffix(info, "Info.plist")
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(info)))
}

// Trust availability is an environmental prerequisite, not evidence of corrupt
// app bytes. Keep it required/unverified so it cannot produce a false pass.
func signatureFailure(path string, diagnostic []byte) audit.Check {
	text := strings.ToLower(string(diagnostic))
	c := audit.Check{ID: "release-signature", Status: "failed", Severity: "block", Confidence: "high", Required: true, File: path, Evidence: "Local strict signature verification failed. This candidate is not distribution-validated."}
	for _, marker := range []string{"cssmerr_tp_not_trusted", "errsecinteractionnotallowed", "user interaction is not allowed", "operation not permitted"} {
		if strings.Contains(text, marker) {
			c.Status = "unverified"
			c.Severity = "warn"
			c.Confidence = "none"
			c.Evidence = "Local signing trust/keychain access could not establish verification. A restricted execution environment can cause this; no corrupt-signature claim is made."
			c.Fix = "Rerun signature verification with normal macOS trust/keychain access and inspect the candidate before changing signing."
			break
		}
	}
	return c
}
