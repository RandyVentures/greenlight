// Package history keeps developer-supplied rejection records outside app repos.
package history

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/RandyVentures/greenlight/internal/audit"
)

type Record struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Status     string    `json:"status"`
	Origin     string    `json:"origin"`
	Guideline  string    `json:"guideline"`
	Summary    string    `json:"summary"`
	RootCause  string    `json:"root_cause"`
	Prevention string    `json:"prevention"`
	RuleID     string    `json:"rule_id"`
	Resolution string    `json:"resolution"`
	Version    string    `json:"version"`
	Build      string    `json:"build"`
	RejectedAt string    `json:"rejected_at"`
}
type document struct {
	Version int      `json:"schema_version"`
	Records []Record `json:"records"`
}
type Store struct {
	Root string
	mu   sync.Mutex
}

func New(root string) (*Store, error) {
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(dir, "greenlight", "rejection-history")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{Root: abs}, nil
}

func (s *Store) path(project string) (string, error) {
	abs, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(s.Root, hex.EncodeToString(sum[:])+".json"), nil
}

func (s *Store) load(path string) ([]Record, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("history must be a regular private file")
	}
	if info.Size() > 4<<20 {
		return nil, fmt.Errorf("history exceeds 4 MiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc document
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if doc.Version != 1 {
		return nil, fmt.Errorf("unsupported history schema")
	}
	if doc.Records == nil {
		doc.Records = []Record{}
	}
	return doc.Records, nil
}
func (s *Store) List(project string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(project)
	if err != nil {
		return nil, err
	}
	return s.load(path)
}

func validate(r Record) error {
	if len(r.Version) > 100 || len(r.Build) > 100 {
		return fmt.Errorf("version/build is too long")
	}
	if r.RejectedAt != "" {
		if _, err := time.Parse("2006-01-02", r.RejectedAt); err != nil {
			return fmt.Errorf("review date must use YYYY-MM-DD")
		}
	}
	if strings.TrimSpace(r.Summary) == "" || strings.TrimSpace(r.Prevention) == "" {
		return fmt.Errorf("summary and prevention steps are required")
	}
	if r.Origin != "apple_review" && r.Origin != "internal_preflight" {
		return fmt.Errorf("choose Apple review or internal preflight as the source")
	}
	if len(r.Summary) > 1000 || len(r.RootCause) > 8000 || len(r.Prevention) > 8000 || len(r.Resolution) > 8000 || len(r.Guideline) > 100 || len(r.RuleID) > 200 {
		return fmt.Errorf("rejection record is too long")
	}
	return nil
}

func (s *Store) write(project string, change func([]Record) ([]Record, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(project)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("history is locked by another writer; retry after it finishes: %w", err)
	}
	lock.Close()
	defer os.Remove(path + ".lock")
	records, err := s.load(path)
	if err != nil {
		return err
	}
	records, err = change(records)
	if err != nil {
		return err
	}
	if len(records) > 1000 {
		return fmt.Errorf("history limit is 1000 records per project")
	}
	b, err := json.MarshalIndent(document{Version: 1, Records: records}, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > 4<<20 {
		return fmt.Errorf("history exceeds the 4 MiB storage limit")
	}
	tmp, err := os.CreateTemp(s.Root, ".history-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func (s *Store) Add(project string, r Record) (Record, error) {
	if err := validate(r); err != nil {
		return Record{}, err
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return Record{}, err
	}
	r.ID = hex.EncodeToString(b)
	r.Status = "open"
	r.Resolution = ""
	r.CreatedAt = time.Now().UTC()
	r.UpdatedAt = r.CreatedAt
	err := s.write(project, func(records []Record) ([]Record, error) { return append([]Record{r}, records...), nil })
	return r, err
}
func (s *Store) Resolve(project, id, resolution string) error {
	if strings.TrimSpace(resolution) == "" || len(resolution) > 8000 {
		return fmt.Errorf("record the fix before resolving a rejection")
	}
	return s.write(project, func(records []Record) ([]Record, error) {
		for i := range records {
			if records[i].ID == id {
				records[i].Status = "resolved"
				records[i].Resolution = resolution
				records[i].UpdatedAt = time.Now().UTC()
				return records, nil
			}
		}
		return nil, fmt.Errorf("history record not found")
	})
}

// Check always revisits resolved records too. A resolution is history, not proof
// that a later build is safe. Only current matched checks can pass a retest.
func Check(records []Record, current []audit.Check) *audit.Report {
	r := &audit.Report{Kind: "rejection-history", Checks: []audit.Check{}, Limitations: []string{"Rejection history is entered by the developer; records are not imported or authenticated by Apple.", "A resolved record never suppresses retesting. Matching a broad rule is not proof that every aspect of the original rejection is fixed."}}
	for _, record := range records {
		c := audit.Check{ID: "history." + record.ID, Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: record.Summary + " Prevention: " + record.Prevention, Fix: "Retest this issue on the current candidate. Recorded status: " + record.Status + "."}
		if record.RuleID != "" {
			matches := []audit.Check{}
			for _, check := range current {
				if check.ID == record.RuleID {
					matches = append(matches, check)
				}
			}
			if len(matches) > 0 {
				c.Status = "passed"
				c.Confidence = "medium"
				c.Evidence += " Current check: " + record.RuleID + "."
				for _, m := range matches {
					if m.Status == "failed" || m.Status == "suspected" {
						c.Status = "suspected"
						if m.Confidence == "low" || m.Confidence == "none" || m.Confidence == "" {
							c.Confidence = "low"
						} else {
							c.Confidence = "medium"
						}
						c.Evidence += " A matching current check needs attention."
						break
					}
					if m.Status == "passed" && (m.Confidence == "low" || m.Confidence == "none" || m.Confidence == "") && c.Confidence != "none" {
						c.Confidence = "low"
					}
					if m.Status != "passed" {
						c.Status = "unverified"
						c.Confidence = "none"
					}
				}
				if c.Status == "passed" {
					c.Fix = "Current matching checks passed; manually confirm they cover the original rejection."
				}
			} else {
				c.Evidence += " Linked check was not evaluated in this audit: " + record.RuleID + "."
			}
		}
		r.Checks = append(r.Checks, c)
	}
	r.Compute()
	return r
}
