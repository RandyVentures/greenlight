package readiness

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/RandyVentures/greenlight/internal/audit"
)

type Metadata struct {
	AdditionalReviews []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	} `json:"additional_reviews"`
	BundleID    string `json:"bundle_id"`
	Version     string `json:"version"`
	Build       string `json:"build"`
	TeamID      string `json:"team_id"`
	Name        string `json:"name"`
	Subtitle    string `json:"subtitle"`
	Description string `json:"description"`
	Keywords    string `json:"keywords"`
	PrivacyURL  string `json:"privacy_url"`
	SupportURL  string `json:"support_url"`
	TermsURL    string `json:"terms_url"`
	ReviewNotes string `json:"review_notes"`
}

func CheckMetadata(path string, features Features) ([]audit.Check, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Metadata
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	var checks []audit.Check
	for _, field := range []struct {
		name, value string
		limit       int
		required    bool
	}{{"name", m.Name, 30, true}, {"subtitle", m.Subtitle, 30, false}, {"description", m.Description, 4000, true}, {"review-notes", m.ReviewNotes, 4000, true}} {
		c := audit.Check{ID: "metadata-" + field.name, Status: "passed", Confidence: "high", Required: true, File: path, Evidence: fmt.Sprintf("%s: %d characters.", field.name, utf8.RuneCountInString(field.value))}
		if field.required && strings.TrimSpace(field.value) == "" || utf8.RuneCountInString(field.value) > field.limit {
			c.Status = "failed"
			c.Severity = "block"
			c.Evidence = "Missing or overlong " + field.name
		}
		checks = append(checks, c)
	}
	for _, link := range []struct {
		name, value string
		required    bool
	}{{"privacy", m.PrivacyURL, true}, {"support", m.SupportURL, true}, {"terms", m.TermsURL, features.Subscriptions}} {
		if !link.required && link.value == "" {
			continue
		}
		u, err := url.Parse(link.value)
		c := audit.Check{ID: "metadata-" + link.name + "-url", Status: "passed", Confidence: "high", Required: true, Evidence: link.name + " URL is HTTPS; reachability and page content remain unverified."}
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			c.Status = "failed"
			c.Severity = "block"
			c.Evidence = "Missing or invalid HTTPS " + link.name + " URL."
		}
		checks = append(checks, c)
	}
	if len([]byte(m.Keywords)) > 100 {
		checks = append(checks, audit.Check{ID: "metadata-keywords", Status: "suspected", Severity: "warn", Confidence: "medium", Required: true, Evidence: "Keywords exceed the inherited checklist's conservative 100 UTF-8-byte budget; verify Apple's current field validation."})
	}
	return checks, nil
}
