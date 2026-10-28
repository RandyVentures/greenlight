package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditOutputNeverOverwritesInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.json")
	original := []byte("important input")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := auditOutput(path, func(w io.Writer) error { _, err := w.Write([]byte("report")); return err }); err == nil {
		t.Fatal("overwrote an existing file")
	}
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, original) {
		t.Fatal("input changed")
	}
}

func TestPurchaseCommandWritesParseableJSONBeforeGateFailure(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config.json")
	output := filepath.Join(root, "report.json")
	if err := os.WriteFile(config, []byte(`{"products":[{"id":"com.example.pro"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newPurchasesCommand()
	cmd.SetArgs([]string{config, "--format", "json", "--output", output, "--exit-code"})
	if err := cmd.Execute(); !errors.Is(err, ErrThreshold) {
		t.Fatalf("missing evidence should trip gate: %v", err)
	}
	b, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["kind"] != "purchases" {
		t.Fatalf("invalid JSON report: %s", b)
	}
}
