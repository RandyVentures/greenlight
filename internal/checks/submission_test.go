package checks

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RandyVentures/greenlight/internal/asc"
)

type apiTransport func(*http.Request) (*http.Response, error)

func (f apiTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func submissionRunner(t *testing.T, pages map[string]string, checks ...namedCheck) *Runner {
	t.Helper()
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = apiTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatalf("mutation attempted: %s", r.Method)
		}
		b, ok := pages[r.URL.RequestURI()]
		if !ok {
			return nil, fmt.Errorf("fixture has no route %s", r.URL.RequestURI())
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b)), Header: http.Header{}}, nil
	})
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture.p8")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := asc.NewClient("fixture", "fixture", path)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(client, false)
	r.checks = map[Tier][]namedCheck{TierMetadata: checks}
	return r
}

func fixturePages() map[string]string {
	return map[string]string{
		"/v1/apps/a/appStoreVersions?limit=200": `{"data":[{"id":"v","attributes":{"versionString":"1.0","platform":"IOS","appStoreState":"PREPARE_FOR_SUBMISSION"}}]}`,
		"/v1/appStoreVersions/v/build":          `{"data":{"id":"attached","attributes":{"version":"8","processingState":"VALID","usesNonExemptEncryption":false}}}`,
	}
}

func TestAttachedBuildAssertionAndEncryptionUseSameBuild(t *testing.T) {
	pages := fixturePages()
	r := submissionRunner(t, pages, namedCheck{"build", checkBuildProcessed}, namedCheck{"encryption", checkEncryption})
	res, err := r.Run(context.Background(), "a", "9", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Blocks != 1 || res.BuildID != "attached" || res.BuildNumber != "8" {
		t.Fatalf("wrong selected build: %+v", res)
	}
}

func TestMissingAttachedBuildIsBlockerEvenWithUploadedBuilds(t *testing.T) {
	pages := fixturePages()
	pages["/v1/appStoreVersions/v/build"] = `{"data":null}`
	r := submissionRunner(t, pages, namedCheck{"build", checkBuildProcessed})
	res, err := r.Run(context.Background(), "a", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Blocks != 1 || res.Summary.Passed {
		t.Fatalf("unattached build passed: %+v", res)
	}
}

func TestScreenshotDimensionsCoverPaginatedSecondLocale(t *testing.T) {
	pages := fixturePages()
	pages["/v1/appStoreVersions/v/appStoreVersionLocalizations?limit=200"] = `{"data":[{"id":"en","attributes":{"locale":"en-US"}}],"links":{"next":"https://api.appstoreconnect.apple.com/v1/more-locales"}}`
	pages["/v1/more-locales"] = `{"data":[{"id":"es","attributes":{"locale":"es-MX"}}]}`
	pages["/v1/appStoreVersionLocalizations/en/appScreenshotSets?limit=200"] = `{"data":[{"id":"en-set","attributes":{"screenshotDisplayType":"APP_IPHONE_67"}}]}`
	pages["/v1/appStoreVersionLocalizations/es/appScreenshotSets?limit=200"] = `{"data":[{"id":"es-set","attributes":{"screenshotDisplayType":"APP_IPHONE_67"}}]}`
	pages["/v1/appScreenshotSets/en-set/appScreenshots?limit=200"] = `{"data":[{"id":"good","attributes":{"fileName":"good.png","imageAsset":{"width":1320,"height":2868}}}]}`
	pages["/v1/appScreenshotSets/es-set/appScreenshots?limit=200"] = `{"data":[{"id":"bad","attributes":{"fileName":"bad.png","imageAsset":{"width":1,"height":1}}}]}`
	r := submissionRunner(t, pages, namedCheck{"dimensions", checkScreenshotDimensions})
	res, err := r.Run(context.Background(), "a", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Blocks != 1 || !strings.Contains(res.Findings[0].Detail, "es-MX") {
		t.Fatalf("second locale missed: %+v", res)
	}
}

func TestTransportErrorsCannotProducePassedSummary(t *testing.T) {
	r := submissionRunner(t, fixturePages(), namedCheck{"screenshots", checkScreenshotDimensions})
	res, err := r.Run(context.Background(), "a", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Incomplete || res.Summary.Passed || res.Checks[0].Status != "unverified" {
		t.Fatalf("partial scan passed: %+v", res)
	}
}

func TestAgeRatingUsesMatchingAppInfoAndPreservesUnknown(t *testing.T) {
	pages := fixturePages()
	pages["/v1/apps/a/appInfos?limit=200"] = `{"data":[{"id":"live","attributes":{"appStoreState":"READY_FOR_SALE","appStoreAgeRating":"4_PLUS"}},{"id":"draft","attributes":{"appStoreState":"PREPARE_FOR_SUBMISSION","appStoreAgeRating":"9_PLUS"}}]}`
	r := submissionRunner(t, pages, namedCheck{"age-rating", checkAgeRating})
	res, err := r.Run(context.Background(), "a", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Summary.Passed {
		t.Fatalf("matching rating failed: %+v", res)
	}
	pages["/v1/apps/a/appInfos?limit=200"] = `{"data":[{"id":"live","attributes":{"appStoreState":"READY_FOR_SALE","appStoreAgeRating":"4_PLUS"}}]}`
	res, err = r.Run(context.Background(), "a", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Incomplete || res.Summary.Passed {
		t.Fatal("unrelated live app info hid unknown draft age rating")
	}
}
