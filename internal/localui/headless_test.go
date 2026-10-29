package localui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestHeadlessAndUIShareRowsSummaryAndEvidenceBinding(t *testing.T) {
	s := testServer(t)
	selection := Selection{Project: ".", Platform: "ios", Payments: "none", Readiness: true}
	direct, err := RunAudit(context.Background(), s.root, selection, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, s, "POST", "/api/run", selection)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var start map[string]string
	json.Unmarshal(w.Body.Bytes(), &start)
	var ui Job
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		json.Unmarshal(request(t, s, "GET", "/api/jobs/"+start["id"], nil).Body.Bytes(), &ui)
		if ui.Status == "complete" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ui.Status != "complete" {
		t.Fatal("UI audit did not finish")
	}
	normalize := func(rows []Row) []Row {
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID+rows[i].File < rows[j].ID+rows[j].File })
		return rows
	}
	if !reflect.DeepEqual(direct.Summary, ui.Summary) || !reflect.DeepEqual(normalize(direct.Rows), normalize(ui.Rows)) || direct.ReleaseBinding != ui.ReleaseBinding {
		t.Fatal("UI/headless evidence drift")
	}
}
func TestHeadlessRespectsWorkspaceBoundaryAndDoesNotCreateHistory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	state := filepath.Join(t.TempDir(), "not-created")
	selection := Selection{Project: outside, Platform: "ios", Payments: "none"}
	if _, err := RunAudit(context.Background(), root, selection, state); err == nil {
		t.Fatal("headless path escaped workspace")
	}
	selection.Project = "."
	selection.History = true
	if _, err := RunAudit(context.Background(), root, selection, state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("read-only audit created history storage")
	}
}
