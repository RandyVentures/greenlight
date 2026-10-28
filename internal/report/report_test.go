package report

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/RandyVentures/greenlight/internal/checks"
)

func TestIncompleteCoverageIsVisibleInEveryOutput(t *testing.T) {
	r := &checks.Results{Incomplete: true, Checks: []checks.CheckResult{{Name: "screenshots", Status: "unverified", Evidence: "API unavailable"}}}
	r.ComputeSummary()
	rep := New(r, time.Second)
	var b bytes.Buffer
	if err := rep.WriteJSON(&b); err != nil {
		t.Fatal(err)
	}
	var doc checks.Results
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Summary.Passed {
		t.Fatal("JSON passed incomplete scan")
	}
	b.Reset()
	if err := rep.WriteJUnit(&b); err != nil {
		t.Fatal(err)
	}
	var suites junitTestSuites
	if err := xml.Unmarshal(b.Bytes(), &suites); err != nil {
		t.Fatal(err)
	}
	if suites.Suites[0].Failures != 1 {
		t.Fatal("JUnit passed incomplete scan")
	}
	b.Reset()
	if err := rep.WriteTerminal(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "INCOMPLETE") {
		t.Fatal("terminal hid incomplete status")
	}
}
