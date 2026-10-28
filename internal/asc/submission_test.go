package asc

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureClient(t *testing.T, pages map[string]string) *Client {
	t.Helper()
	return &Client{token: "fixture-only", tokenExp: time.Now().Add(time.Hour), httpClient: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatalf("unexpected mutation: %s", r.Method)
		}
		body, ok := pages[r.URL.RequestURI()]
		if !ok {
			t.Fatalf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}}
}

func TestSubmissionSelectionIgnoresUploadAndVersionOrder(t *testing.T) {
	c := fixtureClient(t, map[string]string{
		"/v1/apps/a/appStoreVersions?limit=200": `{"data":[{"id":"live","attributes":{"platform":"IOS","appStoreState":"READY_FOR_SALE"}}],"links":{"next":"https://api.appstoreconnect.apple.com/v1/page2"}}`,
		"/v1/page2":                             `{"data":[{"id":"draft","attributes":{"platform":"IOS","versionString":"2.0","appStoreState":"PREPARE_FOR_SUBMISSION"}}]}`,
		"/v1/appStoreVersions/draft/build":      `{"data":{"id":"older-attached","attributes":{"version":"8","processingState":"VALID"}}}`,
	})
	scoped, v, err := c.ForSubmission("a", "", "IOS")
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "draft" {
		t.Fatalf("selected %s", v.ID)
	}
	build, err := scoped.GetVersionBuild(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if build.Attributes.Version != "8" {
		t.Fatalf("wrong build: %+v", build)
	}
	versions, err := scoped.GetAppStoreVersions("a")
	if err != nil || len(versions) != 1 || versions[0].ID != "draft" {
		t.Fatalf("scope lost: %+v %v", versions, err)
	}
	if _, err := scoped.GetAppStoreVersions("another-app"); err == nil {
		t.Fatal("cross-app scope accepted")
	}
}

func TestSubmissionSelectionRequiresDisambiguation(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/apps/a/appStoreVersions?limit=200": `{"data":[{"id":"ios","attributes":{"platform":"IOS","appStoreState":"REJECTED"}},{"id":"mac","attributes":{"platform":"MAC_OS","appStoreState":"METADATA_REJECTED"}}]}`})
	if _, _, err := c.ForSubmission("a", "", ""); err == nil {
		t.Fatal("ambiguous versions accepted")
	}
	_, v, err := c.ForSubmission("a", "mac", "")
	if err != nil || v.ID != "mac" {
		t.Fatalf("explicit version failed: %+v %v", v, err)
	}
	if _, _, err := c.ForSubmission("a", "not-this-app", ""); err == nil {
		t.Fatal("unowned version accepted")
	}
}

func TestPaginationNeverForwardsCredentialsToAnotherHost(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/v1/apps/a/appStoreVersions?limit=200": `{"data":[],"links":{"next":"https://example.org/stolen"}}`})
	if _, err := c.GetAppStoreVersions("a"); err == nil {
		t.Fatal("untrusted pagination accepted")
	}
}

func TestPaginationCycleAndNullBuild(t *testing.T) {
	c := fixtureClient(t, map[string]string{
		"/v1/apps/a/appStoreVersions?limit=200": `{"data":[],"links":{"next":"https://api.appstoreconnect.apple.com/v1/cycle"}}`,
		"/v1/cycle":                             `{"data":[],"links":{"next":"https://api.appstoreconnect.apple.com/v1/cycle"}}`,
		"/v1/appStoreVersions/v/build":          `{"data":null}`,
	})
	if _, err := c.GetAppStoreVersions("a"); err == nil {
		t.Fatal("pagination cycle accepted")
	}
	if b, err := c.GetVersionBuild("v"); err != nil || b != nil {
		t.Fatalf("null attachment: %+v %v", b, err)
	}
}
