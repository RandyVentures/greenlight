package screenshots

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RandyVentures/greenlight/internal/audit"
)

func fixturePNG(t *testing.T, path string, alpha bool) {
	t.Helper()
	var im image.Image = image.NewGray(image.Rect(0, 0, 750, 1334))
	if alpha {
		n := image.NewNRGBA(image.Rect(0, 0, 750, 1334))
		n.Set(1, 1, color.NRGBA{R: 255, A: 128})
		im = n
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
}

func TestOCRGroupsLinesAndKeepsBoxes(t *testing.T) {
	lines, err := ParseTSV([]byte("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n5\t1\t1\t1\t1\t1\t10\t20\t30\t15\t95\tFree\n5\t1\t1\t1\t1\t2\t50\t20\t20\t15\t92\ttrial\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Text != "Free trial" || lines[0].Confidence != 92 || lines[0].Box.Width != 60 {
		t.Fatalf("wrong OCR lines: %+v", lines)
	}
	checks := TextChecks("en-US/shot.png", lines)
	if checks[0].Status != "suspected" || len(checks[0].Regions) != 1 {
		t.Fatalf("price finding lost evidence: %+v", checks)
	}
}

func TestTextRulesAvoidSubstringFalsePositivesAndKeepLowConfidence(t *testing.T) {
	for _, c := range TextChecks("shot.png", []Line{{Text: "Freedom to plan your week", Confidence: 95}}) {
		if c.Status != "passed" {
			t.Fatalf("false positive: %+v", c)
		}
	}
	checks := TextChecks("shot.png", []Line{{Text: "$49.99", Confidence: 20}})
	if checks[0].Status != "suspected" || checks[0].Confidence != "low" || checks[1].Status != "unverified" {
		t.Fatalf("low confidence treated as a pass: %+v", checks)
	}
	for _, text := range []string{"R$ 9,99", "9,99 EUR", "Prueba gratis", "Essai gratuit", "Teste grátis"} {
		if TextChecks("shot.png", []Line{{Text: text, Confidence: 95}})[0].Status != "suspected" {
			t.Errorf("missed %q", text)
		}
	}
}

func TestAllLocalesAndHTMLAreOfflineAndEscaped(t *testing.T) {
	root := t.TempDir()
	for _, locale := range []string{"en-US", "es-MX"} {
		if err := os.Mkdir(filepath.Join(root, locale), 0o755); err != nil {
			t.Fatal(err)
		}
		fixturePNG(t, filepath.Join(root, locale, "shot.png"), false)
	}
	r, err := Scan(context.Background(), root, Options{DisplayType: "APP_IPHONE_47", Recognize: func(context.Context, string) ([]Line, error) {
		return []Line{{Text: `Free trial <script>alert(1)</script>`, Confidence: 95, Box: audit.Box{Left: 10, Top: 20, Width: 100, Height: 40}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Images) != 2 || !r.Gate() {
		t.Fatalf("locale finding missing: %+v", r)
	}
	var b bytes.Buffer
	if err := r.WriteHTML(&b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{"data:image/png;base64,", "&lt;script&gt;", "class=\"box\"", "en-US", "es-MX"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	if strings.Contains(html, "<script>") {
		t.Fatal("unescaped OCR in HTML")
	}
}

func TestMissingOCRAndAlphaNeverPass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alpha.png")
	fixturePNG(t, path, true)
	r, err := Scan(context.Background(), path, Options{DisplayType: "APP_IPHONE_47", Recognize: func(context.Context, string) ([]Line, error) { return nil, fmt.Errorf("tesseract unavailable") }})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Summary.Incomplete || r.Summary.Failed != 1 || !r.Gate() {
		t.Fatalf("false clean result: %+v", r.Summary)
	}
	for _, c := range r.Checks {
		if c.ID == "alpha-channel" && c.Status != "failed" {
			t.Fatal("alpha missed")
		}
	}
}

func TestBadImagesEmptyOCRAndMissingDisplayType(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "broken.png"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixturePNG(t, filepath.Join(root, "valid.png"), false)
	r, err := Scan(context.Background(), root, Options{Recognize: func(context.Context, string) ([]Line, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Summary.Incomplete || r.Summary.Unverified != 3 {
		t.Fatalf("missing evidence passed: %+v", r)
	}
	var html bytes.Buffer
	if err := r.WriteHTML(&html); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "broken.png") || !strings.Contains(html.String(), "Image cannot be decoded") {
		t.Fatal("HTML omitted unreadable image evidence")
	}
}

func TestAcceptedDimensionsAndUnknownDisplayType(t *testing.T) {
	for _, size := range [][2]int{{1290, 2796}, {1320, 2868}, {2868, 1320}} {
		if valid, known := ValidSize("APP_IPHONE_67", size[0], size[1]); !valid || !known {
			t.Fatalf("accepted size rejected: %v", size)
		}
	}
	if valid, known := ValidSize("NEW_APPLE_DEVICE", 1, 1); valid || known {
		t.Fatal("unknown type treated as checked")
	}
	if valid, _ := ValidSize("APP_DESKTOP", 800, 1280); valid {
		t.Fatal("Mac portrait accepted")
	}
}
