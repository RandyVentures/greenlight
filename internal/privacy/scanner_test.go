package privacy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Required Reason API detection must match real call sites that pass arguments —
// stat(path, &st) / statfs(&buf) — not only the argument-less stat() form, which
// essentially never appears in real source. The old `stat\(\)` patterns silently
// missed both categories.
func TestRequiredReasonDetectsRealCallSites(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Disk.swift", "func freeSpace() {\n  var b = statbuf\n  statfs(\"/\", &b)\n}\n")
	writeFile(t, dir, "Files.swift", "func when(path: String) {\n  var s = statbuf\n  stat(path, &s)\n}\n")

	res, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	want := map[string]bool{"Disk Space": false, "File Timestamp": false}
	for _, api := range res.DetectedAPIs {
		if _, ok := want[api]; ok {
			want[api] = true
		}
	}
	for api, found := range want {
		if !found {
			t.Errorf("expected %q detected from a real call site; DetectedAPIs=%v", api, res.DetectedAPIs)
		}
	}
}

func TestCustomStatHelperIsNotAFileTimestampAPI(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "View.swift", `func stat(_ emoji: String, _ value: String, _ label: String) {}
 stat("icon", "1", "cheers")`)
	r, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, api := range r.DetectedAPIs {
		if api == "File Timestamp" {
			t.Fatal("custom Swift helper reported as filesystem API")
		}
	}
	writeFile(t, dir, "View.swift", `func stat(_ emoji: String, _ value: String, _ label: String) {}
 stat(path, &buffer)
 Darwin.stat(path, &buffer)`)
	r, err = Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, api := range r.DetectedAPIs {
		if api == "File Timestamp" {
			found = true
		}
	}
	if !found {
		t.Fatal("qualified native API hidden by same-file helper")
	}
}
func TestSDKNamesDoNotMatchProseOrGeneratedOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Page.tsx", `const title="Community Guidelines"; const note="Google account loads profile; analytics is not configured";`)
	os.Mkdir(filepath.Join(dir, "out"), 0755)
	writeFile(t, filepath.Join(dir, "out"), "bundle.js", `GADMobileAds.sharedInstance()`)
	r, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TrackingSDKs) != 0 {
		t.Fatalf("prose/generated output claimed SDKs: %v", r.TrackingSDKs)
	}
	writeFile(t, dir, "Ads.swift", "import GoogleMobileAds\nGADMobileAds.sharedInstance()")
	r, err = Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TrackingSDKs) != 1 {
		t.Fatal("real SDK reference missed")
	}
	for _, f := range r.Findings {
		if f.Guideline == "5.1.2" && (f.Severity == "CRITICAL" || !strings.Contains(f.Detail, "alone does not establish")) {
			t.Fatalf("SDK presence overclaimed tracking: %+v", f)
		}
	}
}

func TestWrapperAttributionSettersDoNotProveSDKIntegration(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "RevenueWrapper.js", `class Purchases {setAppsflyerID(id) {} setMixpanelDistinctID(id) {}}`)
	r, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TrackingSDKs) != 0 {
		t.Fatalf("wrapper setter mistaken for installed SDK: %v", r.TrackingSDKs)
	}
	writeFile(t, root, "Analytics.swift", "import AppsFlyerLib\nimport Mixpanel")
	r, err = Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TrackingSDKs) != 2 {
		t.Fatalf("real SDK imports missed: %v", r.TrackingSDKs)
	}
}
func TestExplicitApplePrivacyScopeExcludesCopiedAndroidAssets(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "android/app/assets"), 0755)
	writeFile(t, filepath.Join(root, "android/app/assets"), "SDK.js", "import Mixpanel from 'mixpanel';")
	r, err := ScanForPlatform(root, "ios")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TrackingSDKs) != 0 {
		t.Fatal("Android SDK included in Apple scope")
	}
	writeFile(t, root, "App.swift", `let setting = UserDefaults.standard.string(forKey: "app.preference")`)
	r, err = ScanForPlatform(root, "ios")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range r.Findings {
		if strings.Contains(f.Title, "User Defaults") {
			found = true
		}
	}
	if !found {
		t.Fatal("actual app-owned defaults privacy gap was suppressed")
	}
}
