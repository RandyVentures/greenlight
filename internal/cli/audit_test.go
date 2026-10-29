package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func agentFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "greenlight.audit.json")
	cfg := defaultAuditConfig()
	cfg.Selection.History = false
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "Info.plist"), []byte(`<plist><dict><key>CFBundleName</key><string>Synthetic</string><key>CFBundleIdentifier</key><string>com.example.synthetic</string><key>ITSAppUsesNonExemptEncryption</key><false/></dict></plist>`), 0600)
	os.WriteFile(filepath.Join(root, "PrivacyInfo.xcprivacy"), []byte(`<plist><dict><key>NSPrivacyTracking</key><false/></dict></plist>`), 0600)
	return root, path
}
func TestHeadlessJSONHasNoBannerAndMissingEvidenceTripsGate(t *testing.T) {
	_, path := agentFixture(t)
	cmd := newAuditCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", path})
	err := cmd.Execute()
	if !errors.Is(err, ErrThreshold) || ExitCode(err) != 1 {
		t.Fatalf("missing required evidence did not gate: %v", err)
	}
	var doc auditEnvelope
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not pure JSON: %v", err)
	}
	if doc.SchemaVersion != 1 || doc.Kind != "greenlight-audit" || doc.Job == nil || !doc.Summary.Incomplete {
		t.Fatalf("invalid report: %+v", doc)
	}
	for _, r := range doc.Rows {
		if r.ConfidenceReason == "" {
			t.Fatal("confidence explanation lost in headless output")
		}
	}
}
func TestAgentConfigIsRelativeToConfigLocationAndRejectsTypos(t *testing.T) {
	root, path := agentFixture(t)
	cfg, err := loadAuditConfig(path)
	if err != nil || cfg.Workspace != root {
		t.Fatalf("workspace not config-relative: %+v %v", cfg, err)
	}
	for _, body := range []string{
		`{"schema_version":99,"selection":{"platform":"ios"}}`,
		`{"schema_version":1,"selection":{"platform":"ios","paymetns":"none"}}`,
		`{"schema_version":1,"selection":null}`,
		`{"schema_version":1,"selection":{"platform":"ios"}} {}`,
	} {
		os.WriteFile(path, []byte(body), 0600)
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"--config", path})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil || ExitCode(err) != 2 {
			t.Fatalf("bad config did not exit 2: %s %v", body, err)
		}
	}
}
func TestPlanDoesNotRunOrContactConfiguredASC(t *testing.T) {
	_, path := agentFixture(t)
	cfg, _ := loadAuditConfig(path)
	cfg.Workspace = "."
	cfg.Selection.LiveASC = true
	cfg.Selection.AppID = "synthetic"
	cfg.Selection.VersionID = "synthetic-version"
	b, _ := json.Marshal(cfg)
	os.WriteFile(path, b, 0600)
	cmd := newAuditCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--config", path, "--plan"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var doc auditEnvelope
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Job != nil || len(doc.Plan) == 0 || doc.Kind != "greenlight-audit-plan" {
		t.Fatalf("unexpected plan: %+v", doc)
	}
	cmd = newAuditCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", path})
	if err := cmd.Execute(); err == nil || ExitCode(err) != 2 || out.Len() != 0 {
		t.Fatal("configured network was not blocked without --allow-online")
	}
}
func TestExecutionFailureProducesReportAndExitTwoEvenWithoutGate(t *testing.T) {
	_, path := agentFixture(t)
	cmd := newAuditCommand()
	cmd.SetContext(func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", path, "--exit-code=false"})
	if err := cmd.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatalf("execution failure not distinct: %v", err)
	}
	var doc auditEnvelope
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Job == nil || !doc.Job.HasExecutionErrors() {
		t.Fatal("failed steps missing from partial report")
	}
}
func TestInitAndReportsNeverOverwriteFiles(t *testing.T) {
	_, path := agentFixture(t)
	original, _ := os.ReadFile(path)
	cmd := newAuditCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init", path})
	if err := cmd.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatal("init overwrote existing config")
	}
	cmd = newAuditCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", path, "--output", path})
	if err := cmd.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatal("report overwrote input")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(original, after) {
		t.Fatal("input modified")
	}
}
