// Package purchases compares explicit product IDs against local evidence.
// It never signs into a billing service or initiates a transaction.
package purchases

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/RandyVentures/greenlight/internal/audit"
)

type Product struct {
	ID          string `json:"id"`
	Entitlement string `json:"entitlement,omitempty"`
}

type Config struct {
	Products       []Product `json:"products"`
	SourceFiles    []string  `json:"source_files"`
	StoreKitFile   string    `json:"storekit_file,omitempty"`
	ASCFile        string    `json:"app_store_connect_file,omitempty"`
	RevenueCatFile string    `json:"revenuecat_file,omitempty"`
}

type catalogProduct struct {
	state        string
	entitlements []string
}

func readJSON(path string, dest any, strict bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) > 10<<20 {
		return fmt.Errorf("JSON exceeds 10 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if strict {
		d.DisallowUnknownFields()
	}
	if err := d.Decode(dest); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON document")
	}
	return nil
}

func Scan(configPath string) (*audit.Report, error) {
	var cfg Config
	if err := readJSON(configPath, &cfg, true); err != nil {
		return nil, fmt.Errorf("purchase config: %w", err)
	}
	return Compare(cfg, filepath.Dir(configPath))
}

// Compare runs the same checks from in-memory inputs (used by the local UI).
// Relative evidence paths are resolved against base; no files are written.
func Compare(cfg Config, base string) (*audit.Report, error) {
	if len(cfg.Products) == 0 {
		return nil, fmt.Errorf("configure at least one expected product")
	}
	seen := map[string]bool{}
	for _, p := range cfg.Products {
		if strings.TrimSpace(p.ID) == "" || p.ID != strings.TrimSpace(p.ID) || seen[p.ID] {
			return nil, fmt.Errorf("empty, duplicate, or whitespace-padded product ID: %q", p.ID)
		}
		seen[p.ID] = true
	}
	resolve := func(path string) string {
		if path == "" || filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(base, path)
	}
	r := &audit.Report{Kind: "purchases", Checks: []audit.Check{}, Limitations: []string{
		"Source checks find exact quoted literals; they do not prove the product is used in the shipping build. Dynamically assembled IDs may not be found.",
		"ASC and RevenueCat checks use supplied snapshots, whose currency and completeness must be verified by the caller. No services are contacted.",
		"Extra catalog products are allowed to preserve legacy subscriptions. Only configured expected products are compared.",
	}}
	var source strings.Builder
	var sourceErrors []string
	for _, file := range cfg.SourceFiles {
		b, err := os.ReadFile(resolve(file))
		if err != nil {
			sourceErrors = append(sourceErrors, file+": "+err.Error())
			continue
		}
		source.Write(b)
		source.WriteByte('\n')
	}
	for _, p := range cfg.Products {
		c := audit.Check{ID: "source-product-id", Status: "passed", Severity: "warn", Confidence: "high", Required: true, Evidence: p.ID + ": exact quoted literal found in configured source files.", File: strings.Join(cfg.SourceFiles, ", ")}
		literal := regexp.MustCompile("[\"'`]" + regexp.QuoteMeta(p.ID) + "[\"'`]")
		if len(cfg.SourceFiles) == 0 || len(sourceErrors) > 0 {
			c.Status = "unverified"
			c.Confidence = "none"
			c.Evidence = "Source coverage incomplete: " + strings.Join(sourceErrors, "; ")
			if len(cfg.SourceFiles) == 0 {
				c.Evidence = "No source_files configured for " + p.ID
			}
		} else if !literal.MatchString(source.String()) {
			c.Status = "suspected"
			c.Confidence = "medium"
			c.Evidence = p.ID + ": no exact quoted literal found."
			c.Fix = "Check for a product ID typo or confirm how dynamically configured IDs are loaded."
		}
		r.Checks = append(r.Checks, c)
	}
	loaders := []struct {
		name, path string
		required   bool
		load       func(string) (map[string]catalogProduct, error)
	}{
		{"storekit", resolve(cfg.StoreKitFile), cfg.StoreKitFile != "", loadStoreKit},
		{"app-store-connect", resolve(cfg.ASCFile), true, loadASC},
		{"revenuecat", resolve(cfg.RevenueCatFile), needsEntitlement(cfg.Products), loadRevenueCat},
	}
	for _, loader := range loaders {
		if loader.name == "revenuecat" && !loader.required && loader.path == "" {
			continue
		}
		var catalog map[string]catalogProduct
		var err error
		if loader.path != "" {
			catalog, err = loader.load(loader.path)
		}
		for _, p := range cfg.Products {
			c := audit.Check{ID: loader.name + "-product-id", Status: "passed", Severity: "block", Confidence: "high", Required: loader.required, File: loader.path, Evidence: p.ID + ": present in supplied catalog."}
			if loader.path == "" || err != nil {
				c.Status = "unverified"
				c.Confidence = "none"
				c.Evidence = p.ID + ": catalog not supplied."
				if err != nil {
					c.Evidence = p.ID + ": " + err.Error()
				}
				r.Checks = append(r.Checks, c)
				continue
			}
			product, ok := catalog[p.ID]
			if !ok {
				c.Status = "failed"
				c.Evidence = p.ID + ": missing from supplied catalog."
				c.Fix = "Correct the ID or supply a complete catalog for the intended app/environment."
			}
			r.Checks = append(r.Checks, c)
			if !ok {
				continue
			}
			if loader.name == "app-store-connect" {
				state := audit.Check{ID: "asc-product-state", Status: "passed", Severity: "warn", Confidence: "high", Required: true, File: loader.path, Evidence: p.ID + ": " + product.state}
				switch product.state {
				case "APPROVED":
				case "READY_TO_SUBMIT", "WAITING_FOR_REVIEW", "IN_REVIEW", "PENDING_BINARY_APPROVAL":
					state.Status = "suspected"
					state.Evidence += "; configuration exists, but approval/attachment to this submission is unverified."
					state.Fix = "Confirm the product is included in the intended submission and available for reviewer testing."
				case "MISSING_METADATA", "DEVELOPER_ACTION_NEEDED", "REJECTED", "DEVELOPER_REMOVED_FROM_SALE", "REMOVED_FROM_SALE":
					state.Status = "failed"
					state.Severity = "block"
					state.Fix = "Resolve the product's metadata, review, or sale status."
				default:
					state.Status = "unverified"
					state.Confidence = "none"
					state.Evidence = p.ID + ": missing or unrecognized ASC product state: " + product.state
				}
				r.Checks = append(r.Checks, state)
			}
			if loader.name == "revenuecat" && p.Entitlement != "" {
				c.ID = "revenuecat-entitlement"
				c.Evidence = p.ID + ": mapped to " + p.Entitlement
				c.Status = "failed"
				for _, e := range product.entitlements {
					if e == p.Entitlement {
						c.Status = "passed"
					}
				}
				if c.Status == "failed" {
					c.Evidence = p.ID + ": expected entitlement " + p.Entitlement + " is missing."
					c.Fix = "Correct the product-to-entitlement mapping in the intended RevenueCat app, then refresh the snapshot."
				}
				r.Checks = append(r.Checks, c)
			}
		}
	}
	for _, id := range []string{"sandbox-purchase", "restore-purchases", "refund-revocation", "submission-product-attachment"} {
		r.Checks = append(r.Checks, audit.Check{ID: id, Status: "unverified", Severity: "info", Confidence: "none", Required: false, Evidence: "Catalog comparison does not exercise or verify this behavior."})
	}
	r.Compute()
	return r, nil
}

func needsEntitlement(products []Product) bool {
	for _, p := range products {
		if p.Entitlement != "" {
			return true
		}
	}
	return false
}

func addProduct(c map[string]catalogProduct, id string, p catalogProduct) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("catalog product has no product ID")
	}
	if _, ok := c[id]; ok {
		return fmt.Errorf("duplicate catalog product: %s", id)
	}
	c[id] = p
	return nil
}

func loadStoreKit(path string) (map[string]catalogProduct, error) {
	type entry struct {
		ID string `json:"productID"`
	}
	var doc struct {
		Products []entry `json:"products"`
		Groups   []struct {
			Subscriptions []entry `json:"subscriptions"`
		} `json:"subscriptionGroups"`
	}
	if err := readJSON(path, &doc, false); err != nil {
		return nil, err
	}
	if doc.Products == nil && doc.Groups == nil {
		return nil, fmt.Errorf("not a StoreKit configuration")
	}
	c := map[string]catalogProduct{}
	entries := doc.Products
	for _, g := range doc.Groups {
		entries = append(entries, g.Subscriptions...)
	}
	for _, e := range entries {
		if err := addProduct(c, e.ID, catalogProduct{}); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func loadASC(path string) (map[string]catalogProduct, error) {
	var doc struct {
		Data []struct {
			Attributes struct {
				ID       string `json:"productId"`
				State    string `json:"state"`
				IAPState string `json:"inAppPurchaseState"`
			} `json:"attributes"`
		} `json:"data"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}
	if err := readJSON(path, &doc, false); err != nil {
		return nil, err
	}
	if doc.Data == nil {
		return nil, fmt.Errorf("expected ASC data array containing IAPs and/or subscriptions")
	}
	if doc.Links.Next != "" {
		return nil, fmt.Errorf("ASC snapshot has more pages; merge all pages and clear links.next before comparing")
	}
	c := map[string]catalogProduct{}
	for _, e := range doc.Data {
		state := e.Attributes.State
		if state == "" {
			state = e.Attributes.IAPState
		}
		if err := addProduct(c, e.Attributes.ID, catalogProduct{state: state}); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func loadRevenueCat(path string) (map[string]catalogProduct, error) {
	// Explicit normalized format, not a subscriber export or an undocumented
	// claim to support every RevenueCat API version. See docs/purchases.md.
	var doc struct {
		Version  int `json:"schema_version"`
		Products []struct {
			ID           string   `json:"id"`
			Entitlements []string `json:"entitlements"`
		} `json:"products"`
	}
	if err := readJSON(path, &doc, true); err != nil {
		return nil, err
	}
	if doc.Version != 1 || doc.Products == nil {
		return nil, fmt.Errorf("expected RevenueCat catalog snapshot schema_version 1")
	}
	c := map[string]catalogProduct{}
	for _, p := range doc.Products {
		if err := addProduct(c, p.ID, catalogProduct{entitlements: p.Entitlements}); err != nil {
			return nil, err
		}
	}
	return c, nil
}
