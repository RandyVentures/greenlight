package readiness

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RandyVentures/greenlight/internal/audit"
	"howett.net/plist"
)

func InspectArtifact(path string, expected ...Metadata) ([]audit.Check, error) {
	var names []string
	infos := map[string][]byte{}
	if strings.EqualFold(filepath.Ext(path), ".ipa") {
		z, err := zip.OpenReader(path)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		for _, f := range z.File {
			if f.FileInfo().IsDir() {
				continue
			}
			names = append(names, f.Name)
			if strings.HasSuffix(f.Name, "/Info.plist") && f.UncompressedSize64 <= 2<<20 {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				b, err := io.ReadAll(io.LimitReader(rc, 2<<20))
				rc.Close()
				if err != nil {
					return nil, err
				}
				infos[f.Name] = b
			}
		}
	} else {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("artifact must be .app, .xcarchive, or .ipa")
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext != ".app" && ext != ".xcarchive" {
			return nil, fmt.Errorf("select a .app, .xcarchive, or .ipa release artifact")
		}
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(path, p)
			names = append(names, filepath.ToSlash(rel))
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.Name() == "Info.plist" {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				if len(b) > 2<<20 {
					return fmt.Errorf("oversized Info.plist")
				}
				infos[filepath.ToSlash(rel)] = b
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(names)
	checks := []audit.Check{}
	flagged := false
	for _, name := range names {
		base := strings.ToLower(filepath.Base(name))
		if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".p8") || strings.HasSuffix(base, ".storekit") || base == "app-fixtures.json" {
			flagged = true
			checks = append(checks, audit.Check{ID: "release-packaging", Status: "failed", Severity: "block", Confidence: "high", Required: true, File: name, Evidence: "Development fixture or credential-shaped file is packaged in the candidate.", Fix: "Remove the file from release packaging and inspect the rebuilt artifact."})
		}
	}
	if !flagged {
		checks = append(checks, audit.Check{ID: "release-packaging", Status: "passed", Confidence: "high", Required: true, Evidence: "No configured development-fixture or credential-file names found. This is not a complete secret scan."})
	}
	mainInfos := []string{}
	for name := range infos {
		switch {
		case name == "Info.plist" && strings.HasSuffix(strings.ToLower(path), ".app"):
			mainInfos = append(mainInfos, name)
		case name == "Contents/Info.plist":
			mainInfos = append(mainInfos, name)
		case strings.HasPrefix(name, "Payload/") && strings.Count(name, "/") == 2 && strings.Contains(name, ".app/Info.plist"):
			mainInfos = append(mainInfos, name)
		case strings.HasPrefix(name, "Products/Applications/") && strings.Count(name, "/") == 3 && strings.Contains(name, ".app/Info.plist"):
			mainInfos = append(mainInfos, name)
		case strings.HasPrefix(name, "Products/Applications/") && strings.Count(name, "/") == 4 && strings.Contains(name, ".app/Contents/Info.plist"):
			mainInfos = append(mainInfos, name)
		}
	}
	if len(mainInfos) != 1 {
		return append(checks, audit.Check{ID: "release-identity", Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: "Could not identify exactly one main app Info.plist in this artifact."}), nil
	}
	var main map[string]any
	if _, err := plist.Unmarshal(infos[mainInfos[0]], &main); err != nil {
		return nil, err
	}
	identity := audit.Check{ID: "release-identity", Status: "passed", Confidence: "high", Required: true, Evidence: fmt.Sprintf("Candidate bundle %v, version %v, build %v.", main["CFBundleIdentifier"], main["CFBundleShortVersionString"], main["CFBundleVersion"])}
	for _, key := range []string{"CFBundleIdentifier", "CFBundleShortVersionString", "CFBundleVersion"} {
		if value, ok := main[key].(string); !ok || strings.TrimSpace(value) == "" {
			identity.Status = "failed"
			identity.Severity = "block"
			identity.Evidence = "Missing release identity field: " + key
		}
	}
	if len(expected) > 0 {
		for _, field := range []struct{ key, want string }{{"CFBundleIdentifier", expected[0].BundleID}, {"CFBundleShortVersionString", expected[0].Version}, {"CFBundleVersion", expected[0].Build}} {
			if field.want != "" && fmt.Sprint(main[field.key]) != field.want {
				identity.Status = "failed"
				identity.Severity = "block"
				identity.Evidence = "Candidate does not match expected " + field.key + ": " + field.want
			}
		}
	}
	checks = append(checks, identity)
	if platforms, ok := main["CFBundleSupportedPlatforms"].([]any); ok {
		for _, platform := range platforms {
			if strings.Contains(fmt.Sprint(platform), "Simulator") {
				checks = append(checks, audit.Check{ID: "release-platform", Status: "failed", Severity: "block", Confidence: "high", Required: true, Evidence: "Simulator bundle cannot be submitted as a device release."})
			}
		}
	}
	if !strings.EqualFold(filepath.Ext(path), ".ipa") {
		checks = append(checks, signing(componentPath(path, mainInfos[0]), expectedTeam(expected))...)
	}
	for _, name := range names {
		b, ok := infos[name]
		if !ok {
			continue
		}
		if !strings.HasSuffix(name, ".appex/Info.plist") && !strings.HasSuffix(name, ".appex/Contents/Info.plist") {
			continue
		}
		var ext map[string]any
		if _, err := plist.Unmarshal(b, &ext); err != nil {
			return nil, err
		}
		c := audit.Check{ID: "release-extension-version", Status: "passed", Confidence: "high", Required: true, File: name, Evidence: "Extension version/build matches the main candidate."}
		if fmt.Sprint(ext["CFBundleVersion"]) != fmt.Sprint(main["CFBundleVersion"]) || fmt.Sprint(ext["CFBundleShortVersionString"]) != fmt.Sprint(main["CFBundleShortVersionString"]) {
			c.Status = "failed"
			c.Severity = "block"
			c.Evidence = "Extension version/build differs from the main candidate."
		}
		checks = append(checks, c)
		if !strings.EqualFold(filepath.Ext(path), ".ipa") {
			checks = append(checks, signing(componentPath(path, name), expectedTeam(expected))...)
		}
	}
	checks = append(checks, audit.Check{ID: "release-signing", Status: "unverified", Severity: "info", Confidence: "none", Required: false, Evidence: "Signing, profile validity, entitlements, production capabilities, and Xcode validation require the distribution acceptance gate. File inspection does not prove distribution signing."})
	return checks, nil
}

func expectedTeam(values []Metadata) string {
	if len(values) > 0 {
		return values[0].TeamID
	}
	return ""
}
