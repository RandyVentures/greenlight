package screenshots

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RandyVentures/greenlight/internal/audit"
)

const maxImageBytes = 30 << 20

type Line struct {
	Text       string    `json:"text"`
	Confidence float64   `json:"confidence"`
	Box        audit.Box `json:"box"`
}

type Image struct {
	File   string `json:"file"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Lines  []Line `json:"lines"`
}

type Result struct {
	audit.Report
	Images []Image `json:"images"`
}

type Recognizer func(context.Context, string) ([]Line, error)

type Options struct {
	DisplayType string
	Language    string
	Recognize   Recognizer // injectable for deterministic, offline tests
}

func Scan(ctx context.Context, root string, opts Options) (*Result, error) {
	if opts.Language == "" {
		opts.Language = "eng"
	}
	if opts.Recognize == nil {
		opts.Recognize = func(ctx context.Context, path string) ([]Line, error) { return Tesseract(ctx, path, opts.Language) }
	}
	r := &Result{Report: audit.Report{Kind: "screenshots", Checks: []audit.Check{}, Limitations: []string{
		"OCR findings are suggestions for human review; they do not predict Apple's decision.",
		"Text rules cover currency symbols and selected English, Spanish, French, German, and Portuguese phrases; OCR language packs do not translate rules.",
		"No visual feature verification, privacy-data detection, or semantic comparison of translations is performed.",
	}}, Images: []Image{}}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			return nil
		}
		data, err := ReadImage(path)
		if err != nil {
			r.Checks = append(r.Checks, unverified("image-readable", path, err.Error()))
			return nil
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 50_000_000 {
			r.Checks = append(r.Checks, unverified("image-readable", path, "Image cannot be decoded safely (limit: 50 million pixels)."))
			return nil
		}
		im := Image{File: path, Width: cfg.Width, Height: cfg.Height, Lines: []Line{}}
		c := audit.Check{ID: "dimensions", Status: "unverified", Severity: "info", Confidence: "none", Required: true, File: path, Guideline: Specifications, Evidence: fmt.Sprintf("%dx%d; specify --display-type to validate an intended device size.", cfg.Width, cfg.Height)}
		if opts.DisplayType != "" {
			valid, known := ValidSize(opts.DisplayType, cfg.Width, cfg.Height)
			if known {
				c.Status = "passed"
				c.Confidence = "high"
				c.Evidence = fmt.Sprintf("%dx%d for %s", cfg.Width, cfg.Height, opts.DisplayType)
				if !valid {
					c.Status = "failed"
					c.Severity = "block"
					c.Fix = "Export an accepted size for the intended display type."
				}
			} else {
				c.Evidence = "Unknown display type: " + opts.DisplayType + ". Verify against Apple's current specifications."
			}
		}
		r.Checks = append(r.Checks, c)
		c = audit.Check{ID: "alpha-channel", Status: "passed", Severity: "block", Confidence: "high", Required: true, File: path, Evidence: "No PNG alpha channel/transparency declaration found.", Guideline: Specifications}
		if pngAlpha(data) {
			c.Status = "failed"
			c.Evidence = "PNG declares an alpha channel or transparency chunk."
			c.Fix = "Flatten the screenshot and export without an alpha channel."
		}
		r.Checks = append(r.Checks, c)
		lines, err := opts.Recognize(ctx, path)
		if err != nil {
			r.Checks = append(r.Checks, unverified("ocr", path, err.Error()))
		} else if len(lines) == 0 {
			r.Checks = append(r.Checks, unverified("ocr", path, "OCR found no text; text policy checks cannot be verified."))
		} else {
			im.Lines = lines
			r.Checks = append(r.Checks, TextChecks(path, lines)...)
		}
		r.Images = append(r.Images, im)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(r.Images) == 0 && len(r.Checks) == 0 {
		return nil, fmt.Errorf("no PNG or JPEG screenshots found in %s", root)
	}
	r.Compute()
	return r, nil
}

func unverified(id, file, evidence string) audit.Check {
	return audit.Check{ID: id, File: file, Status: "unverified", Severity: "warn", Confidence: "none", Required: true, Evidence: evidence}
}

var textRules = []struct {
	id, guideline, fix string
	pattern            *regexp.Regexp
}{
	{"pricing-copy", "2.3.7", "Remove pricing from screenshot metadata; review the highlighted text in context.", regexp.MustCompile(`(?i)([$€£¥₹₩₽]\s*[0-9]|[0-9][0-9.,]*\s*(USD|EUR|GBP|BRL|MXN|CAD|AUD)\b|\b(free trial|gratis|gratuit|gratuito|grátis|kostenlos|free)\b)`)},
	{"placeholder-copy", "2.1", "Replace unfinished or placeholder copy with final content.", regexp.MustCompile(`(?i)\b(lorem ipsum|placeholder|coming soon|under construction|todo|tbd)\b`)},
	{"platform-reference", "2.3.10", "Confirm whether this platform reference is permitted for your app's specific functionality.", regexp.MustCompile(`(?i)\b(android|google play|play store|windows phone)\b`)},
}

// TextChecks keeps all matched lines/boxes, rather than treating an OCR phrase
// as a confirmed policy violation. Low-confidence OCR remains visible.
func TextChecks(file string, lines []Line) []audit.Check {
	var checks []audit.Check
	low := false
	for _, l := range lines {
		if l.Confidence < 60 {
			low = true
		}
	}
	for _, rule := range textRules {
		c := audit.Check{ID: rule.id, Status: "passed", Severity: "warn", Confidence: "medium", Required: true, File: file, Evidence: "No matching text in the recognized lines.", Guideline: "https://developer.apple.com/app-store/review/guidelines/", Fix: rule.fix}
		var matches []string
		for _, line := range lines {
			if rule.pattern.MatchString(line.Text) {
				matches = append(matches, line.Text)
				c.Regions = append(c.Regions, line.Box)
				if line.Confidence < 60 {
					c.Confidence = "low"
				}
			}
		}
		if len(matches) > 0 {
			c.Status = "suspected"
			c.Evidence = strings.Join(matches, " | ")
		} else {
			c.Fix = ""
			if low {
				c.Status = "unverified"
				c.Confidence = "low"
				c.Evidence = "Low-confidence OCR prevents a reliable negative text check."
			}
		}
		checks = append(checks, c)
	}
	return checks
}

func ReadImage(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxImageBytes {
		return nil, fmt.Errorf("image exceeds 30 MiB")
	}
	return b, nil
}

func pngAlpha(b []byte) bool {
	if len(b) < 26 || !bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return false
	}
	if b[25] == 4 || b[25] == 6 {
		return true
	}
	for i := 8; i+12 <= len(b); {
		n := int(binary.BigEndian.Uint32(b[i : i+4]))
		if n > len(b)-i-12 {
			return false
		}
		if string(b[i+4:i+8]) == "tRNS" {
			return true
		}
		i += n + 12
	}
	return false
}

func Tesseract(ctx context.Context, path, language string) ([]Line, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if strings.HasPrefix(language, "-") {
		return nil, fmt.Errorf("invalid OCR language")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "tesseract", abs, "stdout", "-l", language, "tsv")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("local Tesseract OCR failed (install tesseract and requested language packs): %w; %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseTSV(out)
}

func ParseTSV(data []byte) ([]Line, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(header) < 12 || header[0] != "level" || header[11] != "text" {
		return nil, fmt.Errorf("unexpected Tesseract TSV header")
	}
	var lines []Line
	lastKey := ""
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) < 12 {
			return nil, fmt.Errorf("malformed Tesseract TSV row")
		}
		if row[0] != "5" || strings.TrimSpace(row[11]) == "" {
			continue
		}
		nums := make([]int, 4)
		for i := range nums {
			v, e := strconv.Atoi(row[6+i])
			if e != nil || v < 0 {
				return nil, fmt.Errorf("invalid OCR coordinates")
			}
			nums[i] = v
		}
		conf, e := strconv.ParseFloat(row[10], 64)
		if e != nil || math.IsNaN(conf) || math.IsInf(conf, 0) || conf < 0 || conf > 100 {
			return nil, fmt.Errorf("invalid OCR confidence")
		}
		key := strings.Join(row[1:5], "/")
		box := audit.Box{Left: nums[0], Top: nums[1], Width: nums[2], Height: nums[3]}
		if len(lines) == 0 || key != lastKey {
			lines = append(lines, Line{Text: row[11], Confidence: conf, Box: box})
		} else {
			l := &lines[len(lines)-1]
			l.Text += " " + row[11]
			if conf < l.Confidence {
				l.Confidence = conf
			}
			right := max(l.Box.Left+l.Box.Width, box.Left+box.Width)
			bottom := max(l.Box.Top+l.Box.Height, box.Top+box.Height)
			l.Box.Left = min(l.Box.Left, box.Left)
			l.Box.Top = min(l.Box.Top, box.Top)
			l.Box.Width = right - l.Box.Left
			l.Box.Height = bottom - l.Box.Top
		}
		lastKey = key
	}
	return lines, nil
}
