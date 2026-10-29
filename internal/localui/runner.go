package localui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RandyVentures/greenlight/internal/asc"
	"github.com/RandyVentures/greenlight/internal/audit"
	"github.com/RandyVentures/greenlight/internal/buildsettings"
	"github.com/RandyVentures/greenlight/internal/checks"
	"github.com/RandyVentures/greenlight/internal/config"
	"github.com/RandyVentures/greenlight/internal/history"
	"github.com/RandyVentures/greenlight/internal/preflight"
	"github.com/RandyVentures/greenlight/internal/purchases"
	"github.com/RandyVentures/greenlight/internal/readiness"
	"github.com/RandyVentures/greenlight/internal/screenshots"
)

type Row struct {
	audit.Check
	Group string `json:"group"`
	Title string `json:"title"`
}
type Preview struct {
	File   string `json:"file"`
	Data   string `json:"data"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type Job struct {
	Scope          string        `json:"scope"`
	ID             string        `json:"id"`
	Status         string        `json:"status"`
	Steps          []Step        `json:"steps"`
	Rows           []Row         `json:"rows"`
	Images         []Preview     `json:"images"`
	Summary        audit.Summary `json:"summary"`
	Limitations    []string      `json:"limitations"`
	ElapsedMS      int64         `json:"elapsed_ms"`
	Started        time.Time     `json:"started"`
	ReleaseBinding string        `json:"release_binding,omitempty"`
	Ruleset        string        `json:"ruleset,omitempty"`
	ArtifactSHA256 string        `json:"artifact_sha256,omitempty"`
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	var selection Selection
	if !decode(w, r, &selection) {
		return
	}
	if err := selection.Validate(); err != nil {
		apiError(w, err, 400)
		return
	}
	// Resolve every supplied path before accepting work, not just the main folder.
	paths := []*string{&selection.Project}
	if selection.Readiness {
		for _, p := range []*string{&selection.Artifact, &selection.ReleaseMetadata, &selection.ReleaseEvidence} {
			if *p != "" {
				paths = append(paths, p)
			}
		}
	}
	if selection.Screenshots {
		paths = append(paths, &selection.ScreenshotPath)
	}
	if selection.Payments != "none" {
		for _, p := range []*string{&selection.StoreKitFile, &selection.ASCProductsFile, &selection.RevenueCatFile} {
			if *p != "" {
				paths = append(paths, p)
			}
		}
		for i := range selection.SourceFiles {
			paths = append(paths, &selection.SourceFiles[i])
		}
	}
	for _, p := range paths {
		path, err := s.resolve(*p)
		if err != nil {
			apiError(w, err, 400)
			return
		}
		*p = path
	}
	if info, err := os.Stat(selection.Project); err != nil || !info.IsDir() {
		apiError(w, fmt.Errorf("choose a project directory"), 400)
		return
	}
	steps, _ := Plan(selection)
	id, err := randomID()
	if err != nil {
		apiError(w, err, 500)
		return
	}
	s.mu.Lock()
	if s.active != "" {
		s.mu.Unlock()
		apiError(w, fmt.Errorf("an audit is already running in this workspace"), 409)
		return
	}
	// Reports live only in memory; retain at most eight per server session.
	if len(s.jobs) >= 8 {
		var oldest string
		var date time.Time
		for key, j := range s.jobs {
			if oldest == "" || j.Started.Before(date) {
				oldest = key
				date = j.Started
			}
		}
		delete(s.jobs, oldest)
	}
	scope := "Source checkout audit · no distribution artifact supplied. This does not inspect the published App Store binary."
	if selection.Readiness && selection.Artifact != "" {
		scope = "Local candidate audit · " + filepath.Base(selection.Artifact) + ". Current App Store approval and runtime behavior are separate evidence."
	}
	s.jobs[id] = &Job{Scope: scope, ID: id, Status: "running", Steps: steps, Rows: []Row{}, Images: []Preview{}, Started: time.Now(), Limitations: []string{"Automated checks cover supplied evidence, not Apple's approval decision.", "Source rules are heuristics. Runtime behavior requires testing the candidate build."}}
	var excluded []string
	if !selection.Screenshots {
		excluded = append(excluded, "screenshot audit")
	}
	if selection.Payments == "none" {
		excluded = append(excluded, "purchase catalog comparison")
	}
	if !selection.Localized {
		excluded = append(excluded, "localization coverage")
	}
	if !selection.Accounts {
		excluded = append(excluded, "account runtime review")
	}
	if !selection.Tracking {
		excluded = append(excluded, "tracking runtime review")
	}
	if !selection.LiveASC {
		excluded = append(excluded, "live App Store Connect")
	}
	if len(excluded) > 0 {
		s.jobs[id].Limitations = append(s.jobs[id].Limitations, "Not selected for this audit: "+strings.Join(excluded, ", ")+". These areas are excluded, not passed.")
	}
	s.active = id
	s.mu.Unlock()
	go s.run(id, selection)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	html := strings.HasSuffix(id, "/html")
	if html {
		id = strings.TrimSuffix(id, "/html")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		apiError(w, fmt.Errorf("report not found; rerun the audit if the local server restarted"), 404)
		return
	}
	if html {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		writeHTML(w, j)
		return
	}
	json.NewEncoder(w).Encode(j)
}

func (s *Server) appendReport(id, group string, report *audit.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	for _, c := range report.Checks {
		if c.ConfidenceReason == "" {
			c.ConfidenceReason = confidenceReason(c, group)
		}
		title := strings.ReplaceAll(c.ID, "-", " ")
		if strings.HasPrefix(c.ID, "history.") {
			title = "Previous issue: " + strings.SplitN(c.Evidence, " Prevention: ", 2)[0]
		}
		for _, gate := range readiness.Gates {
			if c.ID == "human."+gate.ID {
				title = gate.Title
				break
			}
		}
		if strings.HasPrefix(c.ID, "human.custom.") {
			title = "App-specific review: " + strings.TrimPrefix(c.ID, "human.custom.")
		}
		j.Rows = append(j.Rows, Row{Check: c, Group: group, Title: title})
	}
	j.Limitations = append(j.Limitations, report.Limitations...)
}

func (s *Server) run(id string, selection Selection) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()
	s.mu.Lock()
	steps := append([]Step(nil), s.jobs[id].Steps...)
	s.mu.Unlock()
	for i, step := range steps {
		s.mu.Lock()
		s.jobs[id].Steps[i].State = "running"
		s.mu.Unlock()
		var err error
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			switch step.ID {
			case "source":
				err = s.runSource(id, selection)
			case "screenshots":
				err = s.runScreenshots(ctx, id, selection)
			case "purchases":
				products := append([]purchases.Product(nil), selection.Products...)
				if selection.Payments != "revenuecat" {
					for i := range products {
						products[i].Entitlement = ""
					}
				}
				rcFile := ""
				if selection.Payments == "revenuecat" {
					rcFile = selection.RevenueCatFile
				}
				r, e := purchases.Compare(purchases.Config{Products: products, SourceFiles: selection.SourceFiles, StoreKitFile: selection.StoreKitFile, ASCFile: selection.ASCProductsFile, RevenueCatFile: rcFile}, s.root)
				err = e
				if e == nil {
					s.appendReport(id, step.ID, r)
				}
			case "localization":
				s.runLocalization(id, selection)
			case "accounts":
				s.manual(id, step.ID, "Account deletion & sign-in", "Exercise account creation, sign-in, and deletion in the candidate build. Source matches cannot prove that these flows work.")
			case "tracking":
				s.manual(id, step.ID, "Tracking consent & disclosures", "Verify consent appears before tracking begins and the privacy declarations match actual SDK behavior.")
			case "asc":
				err = s.runASC(ctx, id, selection)
			case "readiness":
				err = s.runReadiness(id, selection)
			case "history":
				records, e := s.history.List(selection.Project)
				err = e
				if e == nil {
					s.mu.Lock()
					var current []audit.Check
					for _, row := range s.jobs[id].Rows {
						current = append(current, row.Check)
					}
					s.mu.Unlock()
					s.appendReport(id, "history", history.Check(records, current))
				}
			}
		}
		if err != nil {
			s.appendReport(id, step.ID, &audit.Report{Checks: []audit.Check{{ID: step.ID + "-unverified", Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: err.Error(), Fix: "Check the selected inputs and rerun this audit."}}})
		}
		s.mu.Lock()
		s.jobs[id].Steps[i].State = "complete"
		if err != nil {
			s.jobs[id].Steps[i].State = "error"
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	report := audit.Report{}
	for _, row := range j.Rows {
		report.Checks = append(report.Checks, row.Check)
	}
	report.Compute()
	rank := map[string]int{"failed": 0, "suspected": 1, "unverified": 2, "passed": 3}
	sort.SliceStable(j.Rows, func(i, k int) bool { return rank[j.Rows[i].Status] < rank[j.Rows[k].Status] })
	j.Summary = report.Summary
	j.Status = "complete"
	j.ElapsedMS = time.Since(j.Started).Milliseconds()
	s.active = ""
}

func (s *Server) runSource(id string, selection Selection) error {
	r, err := preflight.RunForPlatform(selection.Project, "", "", false, selection.Platform)
	if err != nil {
		return err
	}
	report := &audit.Report{Limitations: []string{"Source findings are low-confidence heuristics, not confirmed defects. Generated output, dependency caches, and recognized test files are excluded. Inspect the compiled artifact and actual runtime before acting."}}
	if roots := buildsettings.AppleSources(selection.Project); len(roots) > 0 {
		report.Limitations = append(report.Limitations, "Source/privacy paths from explicit XcodeGen Apple targets: "+strings.Join(roots, ", ")+". This does not resolve arbitrary build scripts or inspect the shipped binary.")
	}
	for _, f := range r.Findings {
		severity := "warn"
		if f.Severity == "CRITICAL" || f.Severity == "HIGH" {
			severity = "block"
		}
		status, confidence, required := "suspected", "low", true
		if f.Severity == "INFO" {
			severity = "info"
			status = "unverified"
			confidence = "none"
			required = false
		}
		report.Checks = append(report.Checks, audit.Check{ID: f.Title, Status: status, Severity: severity, Confidence: confidence, Required: required, File: f.File, Evidence: f.Detail, Fix: f.Fix, Guideline: f.Doc})
	}
	if r.Incomplete {
		report.Checks = append(report.Checks, audit.Check{ID: "source-coverage", Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: "A source scanner did not complete; findings may be partial."})
	}
	if len(r.Findings) == 0 && !r.Incomplete {
		report.Checks = append(report.Checks, audit.Check{ID: "source-patterns", Status: "passed", Confidence: "medium", Required: true, Evidence: "No configured source-pattern findings in the selected directory."})
	}
	// Existing metadata helpers can return no findings for an unrecognized project.
	_, meta := preflight.CheckLocalMetadata(selection.Project)
	if meta.Source == "" && meta.AppName == "" && meta.BundleID == "" {
		report.Checks = append(report.Checks, audit.Check{ID: "app-metadata", Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: "No supported app configuration was identified in this directory. Confirm the project selection."})
	}
	s.appendReport(id, "source", report)
	return nil
}

func (s *Server) runScreenshots(ctx context.Context, id string, selection Selection) error {
	r, err := screenshots.Scan(ctx, selection.ScreenshotPath, screenshots.Options{DisplayType: selection.DisplayType, Language: selection.Language})
	if err != nil {
		return err
	}
	s.appendReport(id, "screenshots", &r.Report)
	var previews []Preview
	for _, im := range r.Images {
		if len(previews) >= 12 {
			break
		}
		data, err := thumbnail(im.File)
		if err != nil {
			continue
		}
		previews = append(previews, Preview{File: im.File, Data: data, Width: im.Width, Height: im.Height})
	}
	s.mu.Lock()
	s.jobs[id].Images = previews
	s.mu.Unlock()
	return nil
}

func thumbnail(path string) (string, error) {
	b, err := screenshots.ReadImage(path)
	if err != nil {
		return "", err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 50_000_000 {
		return "", fmt.Errorf("image dimensions outside preview limits")
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	bounds := im.Bounds()
	width := min(480, bounds.Dx())
	height := bounds.Dy() * width / bounds.Dx()
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.Set(x, y, im.At(bounds.Min.X+x*bounds.Dx()/width, bounds.Min.Y+y*bounds.Dy()/height))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 75}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func (s *Server) manual(id, group, title, evidence string) {
	s.appendReport(id, group, &audit.Report{Checks: []audit.Check{{ID: title, Status: "unverified", Severity: "info", Confidence: "none", Required: true, Evidence: evidence}}})
}

func (s *Server) runLocalization(id string, selection Selection) {
	if len(selection.Locales) == 0 {
		s.manual(id, "localization", "Expected locales", "Add the locales you intend to ship so screenshot coverage can be checked.")
		return
	}
	r := &audit.Report{}
	for _, locale := range selection.Locales {
		c := audit.Check{ID: "locale-screenshots", Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: locale + ": screenshot coverage not checked."}
		if selection.Screenshots {
			found := false
			walkErr := filepath.WalkDir(selection.ScreenshotPath, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if !entry.IsDir() && (ext == ".png" || ext == ".jpg" || ext == ".jpeg") {
					rel, _ := filepath.Rel(selection.ScreenshotPath, path)
					parts := strings.Split(filepath.ToSlash(rel), "/")
					if len(parts) > 1 && parts[0] == locale {
						found = true
					}
				}
				return nil
			})
			c.Confidence = "high"
			c.Status = "passed"
			c.Evidence = locale + ": screenshot files found in the matching locale folder."
			if !found {
				c.Status = "suspected"
				c.Evidence = locale + ": no screenshot files in the expected locale folder. Confirm fallback behavior or add assets."
			}
			if walkErr != nil {
				c.Status = "unverified"
				c.Confidence = "none"
				c.Evidence = locale + ": " + walkErr.Error()
			}
		}
		r.Checks = append(r.Checks, c)
	}
	s.appendReport(id, "localization", r)
	if !selection.Readiness {
		s.manual(id, "localization", "Translation review", "Verify translated metadata, screenshots, and purchase copy agree. Folder coverage does not verify translation accuracy.")
	}
}

func (s *Server) runASC(ctx context.Context, id string, selection Selection) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.AuthMethod != config.AuthMethodAPIKey {
		return fmt.Errorf("App Store Connect UI checks require saved API-key authentication; run greenlight auth setup")
	}
	client, err := asc.NewClient(cfg.KeyID, cfg.IssuerID, cfg.PrivateKeyPath)
	if err != nil {
		return err
	}
	runner := checks.NewRunner(client, false)
	runner.VersionID = selection.VersionID
	runner.Platform = "IOS"
	if selection.Platform == "macos" {
		runner.Platform = "MAC_OS"
	}
	r, err := runner.Run(ctx, selection.AppID, selection.Build, 2)
	if err != nil {
		return err
	}
	report := &audit.Report{Limitations: r.Limitations}
	for _, f := range r.Findings {
		status := "suspected"
		severity := "warn"
		if f.Severity == checks.SeverityBlock {
			status = "failed"
			severity = "block"
		}
		report.Checks = append(report.Checks, audit.Check{ID: f.Title, Status: status, Severity: severity, Confidence: f.Confidence, Required: true, Evidence: f.Detail, Fix: f.Fix})
	}
	for _, c := range r.Checks {
		if c.Status == "unverified" || c.Status == "passed" {
			report.Checks = append(report.Checks, audit.Check{ID: c.Name, Status: c.Status, Required: true, Evidence: c.Evidence, Confidence: "high"})
		}
	}
	s.appendReport(id, "asc", report)
	return nil
}

// Confidence describes the narrow check, never a rejection probability.
func confidenceReason(c audit.Check, group string) string {
	if c.Status == "unverified" {
		return "Insufficient evidence for this check. This is not evidence that the app is broken."
	}
	if group == "source" {
		return "Static source heuristic. Build settings, SDK configuration, and runtime behavior can change the interpretation; no defect is confirmed."
	}
	if group == "history" {
		return "Retest of a linked current check; not independent proof that the original issue recurred or was completely fixed."
	}
	if strings.HasPrefix(c.ID, "human.") {
		return "Developer-recorded, hash-bound attestation; the tool did not independently execute this test."
	}
	if group == "screenshots" && c.ID != "dimensions" && c.ID != "alpha-channel" {
		return "OCR text heuristic. Inspect the highlighted text and image; this is not an Apple review decision."
	}
	if c.Confidence == "high" {
		return "Direct inspection or comparison of the supplied inputs. Confidence applies only to this specific fact, not app approval or runtime behavior."
	}
	return "Evidence supports this limited check with context still needed. No calibrated probability is available."
}
