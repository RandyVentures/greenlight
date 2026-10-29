package localui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/RandyVentures/greenlight/internal/history"
	"github.com/RandyVentures/greenlight/internal/readiness"
)

func (s *Server) runReadiness(id string, selection Selection) error {
	inputs := []string{}
	if selection.Screenshots {
		inputs = append(inputs, selection.ScreenshotPath)
	}
	if selection.Payments != "none" {
		inputs = append(inputs, selection.SourceFiles...)
		for _, p := range []string{selection.StoreKitFile, selection.ASCProductsFile} {
			if p != "" {
				inputs = append(inputs, p)
			}
		}
		if selection.Payments == "revenuecat" && selection.RevenueCatFile != "" {
			inputs = append(inputs, selection.RevenueCatFile)
		}
	}
	// The browser cannot use an evidence JSON to hash arbitrary outside files.
	if selection.ReleaseEvidence != "" {
		b, err := os.ReadFile(selection.ReleaseEvidence)
		if err != nil {
			return err
		}
		var evidence readiness.Evidence
		if err := json.Unmarshal(b, &evidence); err != nil {
			return err
		}
		for _, a := range evidence.Checks {
			if a.EvidenceFile != "" {
				p := a.EvidenceFile
				if !filepath.IsAbs(p) {
					p = filepath.Join(filepath.Dir(selection.ReleaseEvidence), p)
				}
				if _, err := s.resolve(p); err != nil {
					return fmt.Errorf("attestation evidence must be inside the workspace: %w", err)
				}
			}
		}
	}
	bindingSelection := selection
	bindingSelection.ReleaseEvidence = ""
	bindingSelection.History = false
	contextData, _ := json.Marshal(bindingSelection)
	r, err := readiness.Scan(readiness.Options{Context: string(contextData), Artifact: selection.Artifact, Metadata: selection.ReleaseMetadata, Evidence: selection.ReleaseEvidence, Inputs: inputs, Features: readiness.Features{Purchases: selection.Payments != "none", Subscriptions: selection.Subscriptions, Accounts: selection.Accounts, PublicContent: selection.PublicContent, ThirdPartyContent: selection.ThirdPartyContent, CloudSync: selection.CloudSync, SafetySensitive: selection.SafetySensitive}})
	if err != nil {
		return err
	}
	s.appendReport(id, "readiness", &r.Report)
	s.mu.Lock()
	s.jobs[id].ReleaseBinding = r.Binding
	s.jobs[id].Ruleset = r.Ruleset
	s.jobs[id].ArtifactSHA256 = r.ArtifactSHA256
	s.mu.Unlock()
	return nil
}

func (s *Server) historyAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		project, err := s.resolve(r.URL.Query().Get("project"))
		if err != nil {
			apiError(w, err, 400)
			return
		}
		records, err := s.history.List(project)
		if err != nil {
			apiError(w, err, 500)
			return
		}
		json.NewEncoder(w).Encode(records)
		return
	}
	var request struct {
		Project    string         `json:"project"`
		Action     string         `json:"action"`
		Record     history.Record `json:"record"`
		ID         string         `json:"id"`
		Resolution string         `json:"resolution"`
	}
	if !decode(w, r, &request) {
		return
	}
	project, err := s.resolve(request.Project)
	if err != nil {
		apiError(w, err, 400)
		return
	}
	switch request.Action {
	case "add":
		_, err = s.history.Add(project, request.Record)
	case "resolve":
		err = s.history.Resolve(project, request.ID, request.Resolution)
	default:
		err = fmt.Errorf("unsupported history action")
	}
	if err != nil {
		apiError(w, err, 400)
		return
	}
	records, err := s.history.List(project)
	if err != nil {
		apiError(w, err, 500)
		return
	}
	json.NewEncoder(w).Encode(records)
}
