package buildsettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInjectionIsBoundToPlistAndAllConfigurations(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "App/Info.plist")
	write := func(yml string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "project.yml"), []byte(yml), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`targets:
  Main:
    info:
      path: App/Info.plist
    settings:
      base:
        GENERATE_INFOPLIST_FILE: YES
        INFOPLIST_KEY_CFBundleDisplayName: Example
        INFOPLIST_KEY_ITSAppUsesNonExemptEncryption: NO
  Other:
    info:
      path: Other/Info.plist
    settings:
      base:
        GENERATE_INFOPLIST_FILE: YES
        INFOPLIST_KEY_CFBundleDisplayName: Other
`)
	got := Injected(root, app)
	if got["CFBundleDisplayName"] != "Example" || got["ITSAppUsesNonExemptEncryption"] != "NO" {
		t.Fatalf("injection unresolved: %v", got)
	}
	if len(Injected(root, filepath.Join(root, "Unknown/Info.plist"))) != 0 {
		t.Fatal("unrelated target masked missing value")
	}
	write(`targets:
  Main:
    info:
      path: App/Info.plist
    settings:
      base:
        GENERATE_INFOPLIST_FILE: YES
        INFOPLIST_KEY_CFBundleDisplayName: Example
      configs:
        Release:
          INFOPLIST_KEY_CFBundleDisplayName: ""
`)
	if Injected(root, app)["CFBundleDisplayName"] != "" {
		t.Fatal("empty Release value was hidden by base config")
	}
}
func TestAppleSourceScopeIncludesSharedAndFallsBackWhenUnknown(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "project.yml")
	source := `targets:
  App:
    type: application
    platform: iOS
    sources:
      - path: App
      - path: Shared
  Tests:
    type: bundle.unit-test
    platform: iOS
    sources: [Tests]
`
	os.WriteFile(p, []byte(source), 0600)
	roots := AppleSources(root)
	if len(roots) != 2 || !InSources("Shared/Tracking.swift", roots) || InSources("web/src/page.tsx", roots) || InSources("Tests/Test.swift", roots) {
		t.Fatalf("wrong shipping target scope: %v", roots)
	}
	os.WriteFile(p, []byte(source+`  Unknown:
    type: application
    platform: iOS
    sources:
      - path: "${GENERATED}"
`), 0600)
	if roots := AppleSources(root); len(roots) != 0 {
		t.Fatalf("unknown sources silently omitted: %v", roots)
	}
}
