package screenshots

import (
	"encoding/base64"
	"html/template"
	"io"
	"net/http"

	"github.com/RandyVentures/greenlight/internal/audit"
)

// WriteHTML embeds images and highlights suspect OCR lines. The report is
// self-contained: no script, remote fonts, tracking, or network requests.
func (r *Result) WriteHTML(w io.Writer) error {
	type region struct{ Left, Top, Width, Height float64 }
	type item struct {
		Image
		URL     template.URL
		Regions []region
		Checks  []audit.Check
	}
	view := struct {
		Summary     audit.Summary
		Limitations []string
		Items       []item
		Other       []audit.Check
	}{Summary: r.Summary, Limitations: r.Limitations}
	for _, im := range r.Images {
		b, err := ReadImage(im.File)
		if err != nil {
			return err
		}
		it := item{Image: im, URL: template.URL("data:" + http.DetectContentType(b) + ";base64," + base64.StdEncoding.EncodeToString(b))}
		for _, c := range r.Checks {
			if c.File == im.File {
				it.Checks = append(it.Checks, c)
				for _, box := range c.Regions {
					it.Regions = append(it.Regions, region{float64(box.Left) * 100 / float64(im.Width), float64(box.Top) * 100 / float64(im.Height), float64(box.Width) * 100 / float64(im.Width), float64(box.Height) * 100 / float64(im.Height)})
				}
			}
		}
		view.Items = append(view.Items, it)
	}
	for _, c := range r.Checks {
		found := false
		for _, im := range r.Images {
			if im.File == c.File {
				found = true
				break
			}
		}
		if !found {
			view.Other = append(view.Other, c)
		}
	}
	return htmlReport.Execute(w, view)
}

var htmlReport = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'">
<title>Greenlight screenshot audit</title><style>
body{font:16px system-ui,sans-serif;margin:2rem auto;padding:0 1rem;max-width:1000px;background:#f7f7f5;color:#17232b}article{display:grid;grid-template-columns:minmax(180px,320px) 1fr;gap:2rem;border-top:1px solid #cdd3d4;padding:2rem 0}.image{position:relative;align-self:start}.image img{display:block;width:100%;height:auto}.box{position:absolute;outline:3px solid #e04721;background:#ffb02033;pointer-events:none}li{margin-bottom:1rem;overflow-wrap:anywhere}small{color:#4e5961}@media(max-width:600px){article{grid-template-columns:1fr}}
</style><h1>Screenshot audit</h1><p><strong>{{.Summary.Verdict}}</strong> · {{.Summary.Checked}}/{{.Summary.Applicable}} applicable checks evaluated</p><p>Flags need human review. This report does not predict App Review approval.</p>
{{range .Items}}<article><div class="image"><img src="{{.URL}}" alt="Screenshot being audited">{{range .Regions}}<span class="box" style="left:{{.Left}}%;top:{{.Top}}%;width:{{.Width}}%;height:{{.Height}}%"></span>{{end}}</div><div><h2>{{.File}}</h2><small>{{.Width}} × {{.Height}}</small><ul>{{range .Checks}}<li><strong>{{.Status}} · {{.ID}}</strong><br>{{.Evidence}}{{if .Fix}}<br>Fix: {{.Fix}}{{end}}{{if .Guideline}}<br><a href="{{.Guideline}}">Policy source</a>{{end}}</li>{{end}}</ul></div></article>{{end}}
{{if .Other}}<h2>Files without an image preview</h2><ul>{{range .Other}}<li><strong>{{.Status}} · {{.File}}</strong>: {{.Evidence}}</li>{{end}}</ul>{{end}}
<h2>Scope</h2><ul>{{range .Limitations}}<li>{{.}}</li>{{end}}</ul></html>`))
