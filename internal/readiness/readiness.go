// Package readiness carries the portfolio release checklist into Greenlight.
// Human evidence is tied to a candidate and inputs; it is an attestation, not
// proof independently established by this program.
package readiness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/RandyVentures/greenlight/internal/audit"
)

const Ruleset = "readiness-2026-10-29.1"

type Features struct {
	Purchases         bool `json:"purchases"`
	Subscriptions     bool `json:"subscriptions"`
	Accounts          bool `json:"accounts"`
	PublicContent     bool `json:"public_content"`
	ThirdPartyContent bool `json:"third_party_content"`
	CloudSync         bool `json:"cloud_sync"`
	SafetySensitive   bool `json:"safety_sensitive"`
}

type Options struct {
	Artifact string
	Metadata string
	Evidence string
	Inputs   []string
	Features Features
	Now      time.Time
	Context  string
}

type Result struct {
	audit.Report
	Ruleset        string `json:"ruleset"`
	Binding        string `json:"binding"`
	ArtifactSHA256 string `json:"artifact_sha256,omitempty"`
}

type Gate struct{ ID, Title, Description, Feature string }

var Gates = []Gate{
	{"scope", "Rules & applicability", "Review current official rules, feature declarations, and any app-specific requirements.", ""},
	{"device", "Device & upgrade acceptance", "Test this candidate on supported physical devices: install, upgrade, offline and denied permissions, persistence, crashes, and advertised flows.", ""},
	{"privacy", "Privacy accuracy", "Compare shipped SDK/data behavior, permission prompts, tracking consent timing, manifests, privacy policy, and published App Privacy answers.", ""},
	{"artwork", "Public artwork review", "Visually inspect public screenshots and creative assets for current UI, readable crops, pricing/trial copy, paid-feature disclosures, and rights. Reviewer-only IAP evidence is separate.", ""},
	{"listing", "Listing reconciliation", "Compare every intended locale's listing, screenshots, support/privacy/terms content, and advertised features with the actual candidate.", ""},
	{"submission", "Review package & access", "Verify reviewer access and notes, category, age/export answers, territories, selected build, and membership of the app and any first IAPs in the intended submission.", ""},
	{"archive", "Distribution validation", "Validate the distribution archive in Xcode and confirm signing, extension versions, capabilities, current SDK requirements, and upload provenance.", ""},
	{"business", "Business readiness", "Check applicable agreements, tax/banking status, storefront availability, and intended release mode in the developer account.", ""},
	{"purchases", "Purchase lifecycle", "Exercise each real sandbox product, cancellation, pending states, restore, entitlement persistence, pricing/availability, and purchase review attachments.", "purchases"},
	{"subscriptions", "Subscription lifecycle & terms", "Check recurring value, renewal, expiry, refunds, offer eligibility, billing period, price, and terms/privacy disclosures.", "subscriptions"},
	{"accounts", "Reviewer login & deletion", "Verify reviewer login, sign-in behavior, and deletion of accounts/data on this candidate.", "accounts"},
	{"moderation", "Public content safeguards", "Exercise moderation, reporting, blocking, and support/contact paths for public user content.", "public_content"},
	{"rights", "Third-party content & services", "Record the rights or permissions for shipped content and service integrations; working imports alone do not establish permission.", "third_party_content"},
	{"sync", "Production sync & recovery", "Test production cloud configuration, two-device data/media sync, conflicts, deletion persistence, and recovery.", "cloud_sync"},
	{"safety", "Safety claims & flows", "Review safety-sensitive claims, limitations, reminders, and actual user flows for this app.", "safety_sensitive"},
}

func Applies(g Gate, f Features) bool {
	switch g.Feature {
	case "":
		return true
	case "purchases":
		return f.Purchases || f.Subscriptions
	case "subscriptions":
		return f.Subscriptions
	case "accounts":
		return f.Accounts
	case "public_content":
		return f.PublicContent
	case "third_party_content":
		return f.ThirdPartyContent
	case "cloud_sync":
		return f.CloudSync
	case "safety_sensitive":
		return f.SafetySensitive
	}
	return false
}

// HashPath includes sorted relative file names and content. Symlinks are hashed
// as link text and never followed, so a bundle cannot pull outside files in.
func HashPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("select an actual file/directory, not a symlink")
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("select a regular file or directory")
		}
		return hashFile(path)
	}
	var entries []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			entries = append(entries, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	h := sha256.New()
	for _, p := range entries {
		rel, _ := filepath.Rel(path, p)
		i, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		value := ""
		if i.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return "", err
			}
			value = "link:" + target
		} else if i.Mode().IsRegular() {
			value, err = hashFile(p)
			if err != nil {
				return "", err
			}
		} else {
			return "", fmt.Errorf("unsupported artifact file: %s", rel)
		}
		entry, _ := json.Marshal([]string{filepath.ToSlash(rel), value})
		h.Write(entry)
		h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type Attestation struct {
	Status         string    `json:"status"`
	Reviewer       string    `json:"reviewer"`
	CheckedAt      time.Time `json:"checked_at"`
	Notes          string    `json:"notes"`
	EvidenceFile   string    `json:"evidence_file"`
	EvidenceSHA256 string    `json:"evidence_sha256"`
}
type Evidence struct {
	Ruleset string                 `json:"ruleset"`
	Binding string                 `json:"binding"`
	Checks  map[string]Attestation `json:"checks"`
}

func Scan(opts Options) (*Result, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	r := &Result{Ruleset: Ruleset, Report: audit.Report{Kind: "release-readiness", Checks: []audit.Check{}, Limitations: []string{"Human attestations are self-reported evidence, not independently verified tests or Apple approval.", "A changed artifact, selected input, feature selection, or readiness ruleset invalidates prior attestations. Evidence expires after seven days."}}}
	parts := map[string]string{"ruleset": Ruleset, "context": opts.Context}
	features, _ := json.Marshal(opts.Features)
	parts["features"] = string(features)
	complete := opts.Artifact != ""
	if opts.Artifact != "" {
		hash, err := HashPath(opts.Artifact)
		if err != nil {
			return nil, err
		}
		r.ArtifactSHA256 = hash
		parts["artifact"] = hash
	}
	inputs := append([]string(nil), opts.Inputs...)
	if opts.Metadata != "" {
		inputs = append(inputs, opts.Metadata)
	}
	for i, path := range inputs {
		if path == "" {
			continue
		}
		hash, err := HashPath(path)
		if err != nil {
			return nil, err
		}
		parts[fmt.Sprintf("input:%d:%s", i, filepath.Clean(path))] = hash
	}
	b, _ := json.Marshal(parts)
	sum := sha256.Sum256(b)
	r.Binding = hex.EncodeToString(sum[:])
	if opts.Artifact == "" {
		r.Checks = append(r.Checks, audit.Check{ID: "release-artifact", Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: "No release artifact supplied; source checks cannot establish distribution readiness."})
	} else {
		var expected Metadata
		if opts.Metadata != "" {
			b, e := os.ReadFile(opts.Metadata)
			if e != nil {
				return nil, e
			}
			if e = json.Unmarshal(b, &expected); e != nil {
				return nil, e
			}
		}
		checks, err := InspectArtifact(opts.Artifact, expected)
		if err != nil {
			return nil, err
		}
		r.Checks = append(r.Checks, checks...)
	}
	if opts.Metadata != "" {
		checks, err := CheckMetadata(opts.Metadata, opts.Features)
		if err != nil {
			return nil, err
		}
		r.Checks = append(r.Checks, checks...)
	} else {
		r.Checks = append(r.Checks, audit.Check{ID: "release-metadata", Status: "unverified", Severity: "info", Confidence: "none", Required: false, Evidence: "No local release metadata supplied; listing, review notes, and policy links remain unverified."})
	}
	var evidence Evidence
	var evidenceErr error
	if opts.Evidence != "" {
		data, err := os.ReadFile(opts.Evidence)
		if err != nil {
			evidenceErr = err
		} else {
			evidenceErr = json.Unmarshal(data, &evidence)
		}
	}
	gates := append([]Gate(nil), Gates...)
	if opts.Metadata != "" {
		var m Metadata
		b, err := os.ReadFile(opts.Metadata)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		if len(m.AdditionalReviews) > 100 {
			return nil, fmt.Errorf("too many custom review gates")
		}
		for _, review := range m.AdditionalReviews {
			if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`).MatchString(review.ID) || seen[review.ID] || strings.TrimSpace(review.Description) == "" {
				return nil, fmt.Errorf("custom gates require a unique lowercase ID and description")
			}
			seen[review.ID] = true
			gates = append(gates, Gate{ID: "custom." + review.ID, Title: review.ID, Description: review.Description})
		}
	}
	for _, g := range gates {
		if !Applies(g, opts.Features) {
			continue
		}
		c := audit.Check{ID: "human." + g.ID, Status: "unverified", Severity: "info", Confidence: "none", Required: true, Evidence: g.Description, Guideline: "https://developer.apple.com/app-store/review/guidelines/"}
		a, ok := evidence.Checks[g.ID]
		valid := complete && evidenceErr == nil && evidence.Ruleset == Ruleset && evidence.Binding == r.Binding && ok && (a.Status == "passed" || a.Status == "failed") && strings.TrimSpace(a.Reviewer) != "" && strings.TrimSpace(a.Notes) != "" && !a.CheckedAt.IsZero() && !a.CheckedAt.After(opts.Now) && opts.Now.Sub(a.CheckedAt) <= 7*24*time.Hour && a.EvidenceFile != ""
		if valid {
			path := a.EvidenceFile
			if !filepath.IsAbs(path) {
				path = filepath.Join(filepath.Dir(opts.Evidence), path)
			}
			hash, err := HashPath(path)
			valid = err == nil && hash == a.EvidenceSHA256
		}
		if valid {
			c.Status = a.Status
			c.Confidence = "medium"
			c.Evidence = "Recorded attestation by " + a.Reviewer + " at " + a.CheckedAt.Format(time.RFC3339) + ": " + a.Notes
			if c.Status == "failed" {
				c.Severity = "block"
			}
		} else if opts.Evidence != "" {
			c.Evidence += " Supplied attestation is absent, stale, unreadable, or not bound to these inputs."
		}
		r.Checks = append(r.Checks, c)
	}
	if evidenceErr != nil {
		r.Limitations = append(r.Limitations, "Attestation input could not be read: "+evidenceErr.Error())
	}
	r.Compute()
	return r, nil
}
