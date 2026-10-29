// Package localui serves the embedded React workspace on loopback only.
package localui

import (
	"fmt"
	"strings"

	"github.com/RandyVentures/greenlight/internal/purchases"
)

type Selection struct {
	Project           string              `json:"project"`
	Platform          string              `json:"platform"`
	Payments          string              `json:"payments"`
	Accounts          bool                `json:"accounts"`
	Tracking          bool                `json:"tracking"`
	Localized         bool                `json:"localized"`
	Locales           []string            `json:"locales"`
	Screenshots       bool                `json:"screenshots"`
	ScreenshotPath    string              `json:"screenshot_path"`
	DisplayType       string              `json:"display_type"`
	Language          string              `json:"language"`
	Products          []purchases.Product `json:"products"`
	SourceFiles       []string            `json:"source_files"`
	StoreKitFile      string              `json:"storekit_file"`
	ASCProductsFile   string              `json:"asc_products_file"`
	RevenueCatFile    string              `json:"revenuecat_file"`
	LiveASC           bool                `json:"live_asc"`
	AppID             string              `json:"app_id"`
	VersionID         string              `json:"version_id"`
	Build             string              `json:"build"`
	Readiness         bool                `json:"readiness"`
	Artifact          string              `json:"artifact"`
	ReleaseMetadata   string              `json:"release_metadata"`
	ReleaseEvidence   string              `json:"release_evidence"`
	Subscriptions     bool                `json:"subscriptions"`
	PublicContent     bool                `json:"public_content"`
	ThirdPartyContent bool                `json:"third_party_content"`
	CloudSync         bool                `json:"cloud_sync"`
	SafetySensitive   bool                `json:"safety_sensitive"`
	History           bool                `json:"history"`
}

type Step struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Mode   string `json:"mode"`
	State  string `json:"state"`
}

func Plan(s Selection) ([]Step, error) {
	if s.Platform != "ios" && s.Platform != "macos" {
		return nil, fmt.Errorf("choose iOS or macOS")
	}
	if s.Payments != "none" && s.Payments != "storekit" && s.Payments != "revenuecat" {
		return nil, fmt.Errorf("choose a supported purchase provider")
	}
	if len(s.Products) > 100 || len(s.SourceFiles) > 100 || len(s.Locales) > 100 {
		return nil, fmt.Errorf("too many products, source files, or locales")
	}
	steps := []Step{{ID: "source", Label: "Source & privacy", Detail: "Metadata, permission declarations, privacy manifests, and code patterns.", Mode: "automatic", State: "queued"}}
	if s.Screenshots {
		steps = append(steps, Step{ID: "screenshots", Label: "Screenshots", Detail: "Dimensions, transparency, and local OCR for suspect copy.", Mode: "automatic", State: "queued"})
	}
	if s.Payments != "none" {
		steps = append(steps, Step{ID: "purchases", Label: "Purchase configuration", Detail: "Product IDs, StoreKit, and supplied store catalogs.", Mode: "automatic", State: "queued"})
	}
	if s.Localized {
		steps = append(steps, Step{ID: "localization", Label: "Localization coverage", Detail: "Requested locale folders and manual translation review.", Mode: "mixed", State: "queued"})
	}
	if s.Accounts && !s.Readiness {
		steps = append(steps, Step{ID: "accounts", Label: "Account flows", Detail: "Manual verification of sign-in and account deletion.", Mode: "manual", State: "queued"})
	}
	if s.Tracking && !s.Readiness {
		steps = append(steps, Step{ID: "tracking", Label: "Tracking consent", Detail: "Manual verification of consent timing and disclosures.", Mode: "manual", State: "queued"})
	}
	if s.LiveASC {
		steps = append(steps, Step{ID: "asc", Label: "App Store Connect", Detail: "Read the selected version and attached build using saved API credentials.", Mode: "online", State: "queued"})
	}
	if s.Readiness {
		steps = append(steps, Step{ID: "readiness", Label: "Release acceptance", Detail: "Candidate packaging, listing evidence, and build-bound human acceptance gates.", Mode: "mixed", State: "queued"})
	}
	if s.History {
		steps = append(steps, Step{ID: "history", Label: "Past rejections", Detail: "Retest linked checks and keep manual prevention reminders visible.", Mode: "mixed", State: "queued"})
	}
	return steps, nil
}

func (s Selection) Validate() error {
	if _, err := Plan(s); err != nil {
		return err
	}
	if s.Screenshots && strings.TrimSpace(s.ScreenshotPath) == "" {
		return fmt.Errorf("choose a screenshot folder or file")
	}
	if s.Payments != "none" && len(s.Products) == 0 {
		return fmt.Errorf("add at least one expected product ID")
	}
	if s.Payments != "none" {
		for _, p := range s.Products {
			if strings.TrimSpace(p.ID) == "" {
				return fmt.Errorf("enter a product ID for each row")
			}
			if s.Payments == "revenuecat" && strings.TrimSpace(p.Entitlement) == "" {
				return fmt.Errorf("enter an expected entitlement for each RevenueCat product")
			}
		}
	}
	if s.LiveASC && (s.AppID == "" || s.VersionID == "") {
		return fmt.Errorf("enter the App Store app ID and version resource ID")
	}
	return nil
}
