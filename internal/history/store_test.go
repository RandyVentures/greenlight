package history

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RandyVentures/greenlight/internal/audit"
)

func TestPrivatePersistenceAndProjectIsolation(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	other := t.TempDir()
	s, _ := New(root)
	r, err := s.Add(project, Record{Origin: "internal_preflight", Summary: "Synthetic screenshot finding", RootCause: "Fixture copy", Prevention: "Inspect current screenshot OCR", RuleID: "pricing-copy"})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := New(root)
	records, err := second.List(project)
	if err != nil || len(records) != 1 || records[0].ID != r.ID {
		t.Fatalf("history did not survive reopening: %+v %v", records, err)
	}
	path, _ := s.path(project)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("history permissions: %v", info.Mode())
	}
	if rel, err := filepath.Rel(project, path); err == nil && rel != ".." && len(rel) > 0 && rel[0] != '.' {
		t.Fatal("history written into project")
	}
	if records, err := second.List(other); err != nil || len(records) != 0 {
		t.Fatal("history leaked across projects")
	}
	if err := s.Resolve(project, r.ID, ""); err == nil {
		t.Fatal("resolution without a fix accepted")
	}
	if err := s.Resolve(project, r.ID, "Removed the sample price caption"); err != nil {
		t.Fatal(err)
	}
	records, _ = second.List(project)
	if records[0].Status != "resolved" || records[0].Resolution == "" {
		t.Fatal("resolution was not persisted")
	}
}

func TestResolvedRecordsAlwaysRetestAndMissingChecksStayUnknown(t *testing.T) {
	records := []Record{{ID: "one", Status: "resolved", Summary: "Prior finding", Prevention: "Check screenshots", RuleID: "pricing-copy"}}
	for _, tc := range []struct {
		current []audit.Check
		want    string
	}{
		{nil, "unverified"},
		{[]audit.Check{{ID: "pricing-copy", Status: "passed"}}, "passed"},
		{[]audit.Check{{ID: "pricing-copy", Status: "passed"}, {ID: "pricing-copy", Status: "suspected"}}, "suspected"},
		{[]audit.Check{{ID: "pricing-copy", Status: "unverified"}}, "unverified"},
		{[]audit.Check{{ID: "pricing-copy", Status: "UNKNOWN"}}, "unverified"},
	} {
		r := Check(records, tc.current)
		if r.Checks[0].Status != tc.want {
			t.Fatalf("got %s want %s", r.Checks[0].Status, tc.want)
		}
	}
	records[0].RuleID = ""
	if Check(records, nil).Checks[0].Status != "unverified" {
		t.Fatal("manual reminder claimed as passed")
	}
}

func TestCorruptOrLockedHistoryIsNotSilentlyOverwritten(t *testing.T) {
	s, _ := New(t.TempDir())
	project := t.TempDir()
	path, _ := s.path(project)
	if err := os.WriteFile(path, []byte("broken JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(project, Record{Origin: "apple_review", Summary: "Test", Prevention: "Retest"}); err == nil {
		t.Fatal("corrupt history overwritten")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "broken JSON" {
		t.Fatal("corrupt history lost")
	}
	os.Remove(path)
	os.WriteFile(path+".lock", nil, 0o600)
	if _, err := s.Add(project, Record{Origin: "apple_review", Summary: "Test", Prevention: "Retest"}); err == nil {
		t.Fatal("concurrent writer lock ignored")
	}
}
