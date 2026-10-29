package localui

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RandyVentures/greenlight/internal/audit"
	"github.com/RandyVentures/greenlight/internal/purchases"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func request(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:9999"+path, &buf)
	r.Header.Set("Authorization", "Bearer "+s.token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:9999")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestLocalSessionRequiredAndCrossOriginRejected(t *testing.T) {
	s := testServer(t)
	for _, headers := range []map[string]string{
		{},
		{"Authorization": "Bearer " + s.token, "Origin": "https://untrusted.example"},
		{"Authorization": "Bearer " + s.token, "Sec-Fetch-Site": "cross-site"},
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:9999/api/info", nil)
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("accepted unauthorized request: %v / %d", headers, w.Code)
		}
	}
	s.host = "127.0.0.1:9999"
	r := httptest.NewRequest("GET", "http://rebinding.example/api/info", nil)
	r.Header.Set("Authorization", "Bearer "+s.token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("untrusted Host accepted")
	}
	if w := request(t, s, "GET", "/api/info", nil); w.Code != 200 {
		t.Fatalf("valid local session rejected: %s", w.Body)
	}
}

func TestWorkspaceBoundaryIncludesSymlinks(t *testing.T) {
	s := testServer(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.swift"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.root, "escape")); err != nil {
		t.Skip(err)
	}
	for _, path := range []string{"..", outside, "escape", filepath.Join("escape", "secret.swift")} {
		if _, err := s.resolve(path); err == nil {
			t.Errorf("escaped workspace: %q", path)
		}
	}
	w := request(t, s, "POST", "/api/run", Selection{Project: ".", Platform: "ios", Payments: "storekit", Products: []purchases.Product{{ID: "pro"}}, SourceFiles: []string{filepath.Join(outside, "secret.swift")}})
	if w.Code != 400 {
		t.Fatalf("outside source accepted: %d", w.Code)
	}
}

func TestBrowseDoesNotExposeHiddenOrSymlinkedEntries(t *testing.T) {
	s := testServer(t)
	for _, name := range []string{"visible.swift", ".env"} {
		if err := os.WriteFile(filepath.Join(s.root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(s.root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := request(t, s, "GET", "/api/browse?path=.", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "visible.swift") || strings.Contains(w.Body.String(), ".env") || strings.Contains(w.Body.String(), "node_modules") {
		t.Fatalf("unexpected listing: %s", w.Body)
	}
}

func TestSelectionDrivesPlanAndDisabledPurchasesIgnoreStaleRows(t *testing.T) {
	base := Selection{Platform: "ios", Payments: "none", Products: []purchases.Product{{ID: ""}}}
	if err := base.Validate(); err != nil {
		t.Fatalf("disabled purchase inputs blocked source audit: %v", err)
	}
	plan, err := Plan(base)
	if err != nil || len(plan) != 1 {
		t.Fatalf("base plan: %+v %v", plan, err)
	}
	base.Payments = "revenuecat"
	base.Screenshots = true
	base.Localized = true
	base.Accounts = true
	base.Tracking = true
	plan, err = Plan(base)
	if err != nil || len(plan) != 6 {
		t.Fatalf("tailored plan: %+v %v", plan, err)
	}
	base.ScreenshotPath = "shots"
	base.Products = []purchases.Product{{ID: "pro"}}
	if err := base.Validate(); err == nil {
		t.Fatal("RevenueCat expected entitlement missing but accepted")
	}
}

func TestAuditJobReportsMissingEvidenceAndManualFlows(t *testing.T) {
	s := testServer(t)
	if err := os.WriteFile(filepath.Join(s.root, "Info.plist"), []byte(`<plist><dict><key>CFBundleDisplayName</key><string>Example</string><key>CFBundleIdentifier</key><string>com.example.app</string><key>CFBundleShortVersionString</key><string>1.0</string></dict></plist>`), 0o644); err != nil {
		t.Fatal(err)
	}
	w := request(t, s, "POST", "/api/run", Selection{Project: ".", Platform: "ios", Payments: "storekit", Products: []purchases.Product{{ID: "pro"}}, Accounts: true})
	if w.Code != 202 {
		t.Fatalf("job rejected: %s", w.Body)
	}
	var started map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	var job Job
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res := request(t, s, "GET", "/api/jobs/"+started["id"], nil)
		if err := json.Unmarshal(res.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.Status == "complete" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Status != "complete" || !job.Summary.Incomplete {
		t.Fatalf("job did not preserve missing evidence: %+v", job)
	}
	foundManual := false
	for _, row := range job.Rows {
		if row.ID == "app-metadata" {
			t.Fatal("native app metadata was incorrectly marked missing")
		}
		if row.Group == "accounts" {
			foundManual = true
			if row.Status != "unverified" {
				t.Fatal("manual flow claimed as tested")
			}
		}
	}
	if !foundManual {
		t.Fatal("account flow omitted")
	}
	if len(job.Images) != 0 {
		t.Fatal("unexpected screenshot processing")
	}
	if strings.Contains(w.Body.String(), s.token) {
		t.Fatal("session token included in report")
	}
}

func TestBusyWorkspaceAndBadJSONDoNotLaunchWork(t *testing.T) {
	s := testServer(t)
	s.active = "already-running"
	w := request(t, s, "POST", "/api/run", Selection{Project: ".", Platform: "ios", Payments: "none"})
	if w.Code != 409 {
		t.Fatalf("concurrent audit accepted: %d", w.Code)
	}
	w = request(t, s, "POST", "/api/plan", map[string]any{"platform": "ios", "payments": "none", "unexpected": true})
	if w.Code != 400 {
		t.Fatal("unknown request fields accepted")
	}
}

func TestEmbeddedUIIsAvailableWithoutNodeOrNetwork(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/", "/app.js", "/app.css"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("missing embedded asset %s: %d", path, w.Code)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing local-only resource policy")
		}
	}
}

func TestCompressedAssetsAndEscapedHTMLExport(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest("GET", "/app.js", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("compressed asset not served")
	}
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	plain, err := io.ReadAll(reader)
	if err != nil || len(plain) < 1000 {
		t.Fatal("invalid compressed bundle")
	}
	s.jobs["fixture"] = &Job{ID: "fixture", Status: "complete", Rows: []Row{{Check: audit.Check{ID: "x", Status: "suspected", Evidence: `<script>alert('x')</script>`}, Title: "Test"}}}
	w = request(t, s, "GET", "/api/jobs/fixture/html", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatalf("unsafe or invalid HTML export: %s", w.Body)
	}
}
