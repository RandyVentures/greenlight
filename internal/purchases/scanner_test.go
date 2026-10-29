package purchases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/RandyVentures/greenlight/internal/audit"
)

func writeFixture(t *testing.T, root, name, body string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func purchaseFixture(t *testing.T) (string, Config) {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "Products.swift", `let product = "com.example.pro"`)
	writeFixture(t, root, "Local.storekit", `{"products":[{"productID":"com.example.pro"}],"subscriptionGroups":[{"subscriptions":[{"productID":"com.example.legacy"}]}]}`)
	writeFixture(t, root, "asc.json", `{"data":[{"attributes":{"productId":"com.example.pro","state":"APPROVED"}}]}`)
	writeFixture(t, root, "rc.json", `{"schema_version":1,"products":[{"id":"com.example.pro","entitlements":["pro"]}]}`)
	return root, Config{Products: []Product{{ID: "com.example.pro", Entitlement: "pro"}}, SourceFiles: []string{"Products.swift"}, StoreKitFile: "Local.storekit", ASCFile: "asc.json", RevenueCatFile: "rc.json"}
}

func scanFixture(t *testing.T, root string, cfg Config) *audit.Report {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := writeFixture(t, root, "config.json", string(b))
	r, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMatchingCatalogsDoNotClaimRuntimeVerification(t *testing.T) {
	root, cfg := purchaseFixture(t)
	r := scanFixture(t, root, cfg)
	if r.Gate() || r.Summary.Unverified != 4 {
		t.Fatalf("unexpected report: %+v", r)
	}
	for _, c := range r.Checks {
		if c.ID == "sandbox-purchase" && (c.Status != "unverified" || c.Required) {
			t.Fatalf("runtime overclaim: %+v", c)
		}
	}
}

func TestMissingCatalogAndEntitlementAreActionable(t *testing.T) {
	root, cfg := purchaseFixture(t)
	writeFixture(t, root, "rc.json", `{"schema_version":1,"products":[{"id":"com.example.pro","entitlements":["wrong"]}]}`)
	r := scanFixture(t, root, cfg)
	if r.Summary.Failed != 1 || !r.Gate() {
		t.Fatalf("entitlement mismatch passed: %+v", r)
	}
	cfg.ASCFile = ""
	r = scanFixture(t, root, cfg)
	if !r.Summary.Incomplete {
		t.Fatal("missing ASC evidence passed")
	}
}

func TestPartialOrMalformedCatalogIsUnverified(t *testing.T) {
	for _, body := range []string{
		`{"data":[],"links":{"next":"https://api.appstoreconnect.apple.com/v1/more"}}`,
		`{"data":[{"attributes":{"productId":"com.example.pro"}},{"attributes":{"productId":"com.example.pro"}}]}`,
		`{"data":[{"attributes":{"state":"APPROVED"}}]}`,
		`{"not_a_catalog":[]}`,
	} {
		root, cfg := purchaseFixture(t)
		writeFixture(t, root, "asc.json", body)
		r := scanFixture(t, root, cfg)
		if !r.Summary.Incomplete {
			t.Fatalf("bad catalog passed: %s", body)
		}
	}
}

func TestProductStatesAndLiteralBoundaries(t *testing.T) {
	root, cfg := purchaseFixture(t)
	writeFixture(t, root, "Products.swift", `let product = "com.example.pro.annual"`)
	writeFixture(t, root, "asc.json", `{"data":[{"attributes":{"productId":"com.example.pro","inAppPurchaseState":"READY_TO_SUBMIT"}}]}`)
	r := scanFixture(t, root, cfg)
	if r.Summary.Suspected != 1 || !r.Summary.Incomplete {
		t.Fatalf("missing source must stay unverified while review-pending catalog remains suspected: %+v", r)
	}
	writeFixture(t, root, "asc.json", `{"data":[{"attributes":{"productId":"com.example.pro","state":"REJECTED"}}]}`)
	r = scanFixture(t, root, cfg)
	if r.Summary.Failed != 1 {
		t.Fatal("rejected IAP passed")
	}
}

func TestConfigTyposAndMissingSourceCannotPass(t *testing.T) {
	root, cfg := purchaseFixture(t)
	path := writeFixture(t, root, "bad.json", `{"products":[{"id":"x"}],"soruce_files":["Products.swift"]}`)
	if _, err := Scan(path); err == nil {
		t.Fatal("config typo silently ignored")
	}
	cfg.SourceFiles = append(cfg.SourceFiles, "missing.swift")
	if !scanFixture(t, root, cfg).Summary.Incomplete {
		t.Fatal("source read failure passed")
	}
	cfg.Products = append(cfg.Products, cfg.Products[0])
	b, _ := json.Marshal(cfg)
	writeFixture(t, root, "bad.json", string(b))
	if _, err := Scan(path); err == nil {
		t.Fatal("duplicate expected products accepted")
	}
}

func TestDynamicOfferingIDsAreUnknownNotBrokenPurchases(t *testing.T) {
	root, cfg := purchaseFixture(t)
	writeFixture(t, root, "Products.swift", `let product = offerings.current?.monthly?.storeProduct`)
	r := scanFixture(t, root, cfg)
	for _, c := range r.Checks {
		if c.ID == "source-product-id" && (c.Status != "unverified" || c.Confidence != "none") {
			t.Fatalf("runtime offering treated as defect: %+v", c)
		}
	}
	if r.Summary.Failed != 0 || r.Summary.Suspected != 0 || !r.Summary.Incomplete {
		t.Fatalf("unexpected dynamic catalog interpretation: %+v", r.Summary)
	}
}
