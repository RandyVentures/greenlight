package readiness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	app := filepath.Join(root, "Candidate.app")
	os.Mkdir(app, 0o755)
	data := `<plist><dict><key>CFBundleIdentifier</key><string>com.example.candidate</string><key>CFBundleShortVersionString</key><string>1.0</string><key>CFBundleVersion</key><string>1</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, app
}

func TestChecklistIncludesPriorGatesAndDoesNotCertifySourceOnly(t *testing.T) {
	r, err := Scan(Options{Features: Features{Purchases: true, Subscriptions: true, Accounts: true, PublicContent: true, ThirdPartyContent: true, CloudSync: true, SafetySensitive: true}})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range r.Checks {
		if len(c.ID) > 6 && c.ID[:6] == "human." {
			count++
			if c.Status != "unverified" {
				t.Fatal("missing evidence passed")
			}
		}
	}
	if count != 15 || !r.Summary.Incomplete {
		t.Fatalf("missing original gates: %d %+v", count, r.Summary)
	}
}

func TestPackagedFixturesAndMismatchedExtensionAreBlockers(t *testing.T) {
	_, app := fixture(t)
	os.WriteFile(filepath.Join(app, "Local.storekit"), []byte("{}"), 0o644)
	ext := filepath.Join(app, "PlugIns", "Share.appex")
	os.MkdirAll(ext, 0o755)
	os.WriteFile(filepath.Join(ext, "Info.plist"), []byte(`<plist><dict><key>CFBundleVersion</key><string>2</string><key>CFBundleShortVersionString</key><string>1.0</string></dict></plist>`), 0o644)
	r, err := Scan(Options{Artifact: app})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range r.Checks {
		if c.Status == "failed" {
			got[c.ID] = true
		}
	}
	if !got["release-packaging"] || !got["release-extension-version"] {
		t.Fatalf("packaging gap not caught: %v", got)
	}
}

func TestAttestationsInvalidateOnArtifactInputAgeOrRulesetChange(t *testing.T) {
	root, app := fixture(t)
	input := filepath.Join(root, "listing.txt")
	os.WriteFile(input, []byte("Version one"), 0o644)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	opts := Options{Artifact: app, Inputs: []string{input}, Now: now}
	baseline, err := Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(root, "device.md")
	os.WriteFile(notes, []byte("Synthetic device evidence for tests, not real device acceptance"), 0o644)
	hash, _ := HashPath(notes)
	e := Evidence{Ruleset: Ruleset, Binding: baseline.Binding, Checks: map[string]Attestation{"device": {Status: "passed", Reviewer: "Test fixture", CheckedAt: now, Notes: "Synthetic", EvidenceFile: "device.md", EvidenceSHA256: hash}}}
	path := filepath.Join(root, "evidence.json")
	save := func() { b, _ := json.Marshal(e); os.WriteFile(path, b, 0o600) }
	save()
	opts.Evidence = path
	status := func() string {
		r, err := Scan(opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range r.Checks {
			if c.ID == "human.device" {
				return c.Status
			}
		}
		t.Fatal("missing gate")
		return ""
	}
	if status() != "passed" {
		t.Fatal("current bound evidence not accepted")
	}
	os.WriteFile(filepath.Join(app, "changed.bin"), []byte("new candidate"), 0o644)
	if status() != "unverified" {
		t.Fatal("old evidence accepted for new artifact")
	}
	os.Remove(filepath.Join(app, "changed.bin"))
	os.WriteFile(input, []byte("Version two"), 0o644)
	if status() != "unverified" {
		t.Fatal("old listing evidence accepted")
	}
	os.WriteFile(input, []byte("Version one"), 0o644)
	opts.Context = "changed UI feature selections"
	if status() != "unverified" {
		t.Fatal("changed selection context accepted old evidence")
	}
	opts.Context = ""
	a := e.Checks["device"]
	a.CheckedAt = now.Add(-8 * 24 * time.Hour)
	e.Checks["device"] = a
	save()
	if status() != "unverified" {
		t.Fatal("stale evidence accepted")
	}
	a.CheckedAt = now.Add(time.Hour)
	e.Checks["device"] = a
	save()
	if status() != "unverified" {
		t.Fatal("future-dated evidence accepted")
	}
	a.CheckedAt = now
	e.Checks["device"] = a
	e.Ruleset = "other"
	save()
	if status() != "unverified" {
		t.Fatal("wrong ruleset accepted")
	}
}

func TestMetadataRejectsMissingReviewNotesAndSubscriptionLinks(t *testing.T) {
	root, _ := fixture(t)
	path := filepath.Join(root, "metadata.json")
	os.WriteFile(path, []byte(`{"name":"Example","description":"Example app","privacy_url":"https://example.test/privacy","support_url":"https://example.test/support"}`), 0o644)
	checks, err := CheckMetadata(path, Features{Subscriptions: true})
	if err != nil {
		t.Fatal(err)
	}
	failed := map[string]bool{}
	for _, c := range checks {
		if c.Status == "failed" {
			failed[c.ID] = true
		}
	}
	if !failed["metadata-review-notes"] || !failed["metadata-terms-url"] {
		t.Fatalf("metadata gaps missed: %v", failed)
	}
}

func TestDistributionChecksRejectDebugDeviceProfilesAndExpiry(t *testing.T) {
	now := time.Now()
	checks := AssessSigning(map[string]any{"get-task-allow": true}, map[string]any{"ExpirationDate": now.Add(-time.Hour), "ProvisionedDevices": []string{"synthetic"}}, now)
	if checks[0].Status != "failed" || checks[1].Status != "failed" {
		t.Fatalf("development profile passed: %+v", checks)
	}
	checks = AssessSigning(map[string]any{}, nil, now)
	if checks[1].Status != "unverified" {
		t.Fatal("missing profile treated as validated")
	}
}

func TestExpectedIdentityAndCustomGate(t *testing.T) {
	root, app := fixture(t)
	metadata := filepath.Join(root, "metadata.json")
	if err := os.WriteFile(metadata, []byte(`{"bundle_id":"com.example.other","version":"2.0","additional_reviews":[{"id":"share-sheet","description":"Exercise the share extension on this candidate"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(Options{Artifact: app, Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	foundIdentity, foundCustom := false, false
	for _, c := range r.Checks {
		if c.ID == "release-identity" && c.Status == "failed" {
			foundIdentity = true
		}
		if c.ID == "human.custom.share-sheet" && c.Status == "unverified" && c.Required {
			foundCustom = true
		}
	}
	if !foundIdentity || !foundCustom {
		t.Fatalf("expected identity/custom gate not enforced: %+v", r.Checks)
	}
	if err := os.WriteFile(metadata, []byte(`{"additional_reviews":[{"id":"device","description":"One"},{"id":"device","description":"Two"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(Options{Metadata: metadata}); err == nil {
		t.Fatal("duplicate custom gates accepted")
	}
}
