package server

import (
	"context"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/settings"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/version"
)

type reportECU struct {
	Desc     ecu.Description
	Items    []ecu.InfoItem
	Codes    []dtc.DTC
	CodesErr string
	Values   []reportValue
	Monitors []ecu.Monitor
	MIL      bool
}

type reportValue struct {
	Name, Value, Unit string
}

var reportLabels = map[string]map[string]string{
	"en": {"title": "Diagnostic report", "vehicle": "Connection", "adapter": "Adapter", "date": "Date",
		"unit": "Control unit", "ident": "Identification", "codes": "Trouble codes", "none": "No trouble codes stored.",
		"values": "Live values at the time of the report", "readiness": "Readiness monitors", "ready": "complete",
		"notready": "not complete", "code": "Code", "desc": "Description", "status": "Status", "detail": "Detail",
		"footer": "Created with Universal Diagnostic Tool", "print": "Print / save as PDF", "protocol": "Protocol"},
	"ru": {"title": "Отчёт о диагностике", "vehicle": "Подключение", "adapter": "Адаптер", "date": "Дата",
		"unit": "Блок управления", "ident": "Идентификация", "codes": "Коды неисправностей", "none": "Кодов неисправностей нет.",
		"values": "Параметры на момент отчёта", "readiness": "Мониторы готовности", "ready": "завершён",
		"notready": "не завершён", "code": "Код", "desc": "Описание", "status": "Статус", "detail": "Уточнение",
		"footer": "Создано программой Universal Diagnostic Tool", "print": "Печать / сохранить в PDF", "protocol": "Протокол"},
}

const reportHTML = `<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.L.title}} - {{.Date}}</title>
<style>
body{font:14px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;margin:24px auto;max-width:960px;padding:0 16px;color:#1d2330;background:#fff}
h1{font-size:24px;margin:0 0 4px}h2{font-size:18px;margin:28px 0 8px;border-bottom:2px solid #1d2330;padding-bottom:4px}
h3{font-size:15px;margin:18px 0 6px}table{border-collapse:collapse;width:100%;margin:6px 0 12px}
td,th{border:1px solid #c9ced8;padding:5px 8px;text-align:left;vertical-align:top}th{background:#eef1f6}
.muted{color:#5a6475}.code{font-family:ui-monospace,Consolas,monospace;font-weight:600;white-space:nowrap}
.flag{display:inline-block;border:1px solid #9aa3b2;border-radius:4px;padding:0 5px;margin:1px;font-size:12px}
.bad{color:#b42318}.ok{color:#1a7f37}@media print{.noprint{display:none}body{margin:0}}
</style></head><body>
<p class="noprint"><button onclick="window.print()">{{.L.print}}</button></p>
<h1>{{.L.title}}</h1>
<p class="muted">{{.L.date}}: {{.Date}} · {{.L.adapter}}: {{.Adapter}} · {{.Version}}</p>
{{range .ECUs}}
<h2>{{$.L.unit}}: {{.Desc.Name}}</h2>
<p class="muted">{{$.L.protocol}}: {{.Desc.Protocol}}{{if .Desc.Address}} · {{.Desc.Address}}{{end}}</p>
{{if .Items}}<h3>{{$.L.ident}}</h3><table>{{range .Items}}<tr><th>{{tr .Name}}</th><td>{{.Value}}</td></tr>{{end}}</table>{{end}}
<h3>{{$.L.codes}}</h3>
{{if .CodesErr}}<p class="bad">{{.CodesErr}}</p>{{else if not .Codes}}<p class="ok">{{$.L.none}}</p>{{else}}
<table><tr><th>{{$.L.code}}</th><th>{{$.L.desc}}</th><th>{{$.L.status}}</th></tr>
{{range .Codes}}<tr><td class="code">{{.Code}}</td><td>{{tr .Desc}}{{if .Detail}}<br><span class="muted">{{tr .Detail}}</span>{{end}}</td>
<td>{{range .Flags}}<span class="flag">{{flag .}}</span>{{end}}</td></tr>{{end}}</table>{{end}}
{{if .Monitors}}<h3>{{$.L.readiness}}</h3><table>{{range .Monitors}}<tr><td>{{tr .Name}}</td><td class="{{if .Complete}}ok{{else}}bad{{end}}">{{if .Complete}}{{$.L.ready}}{{else}}{{$.L.notready}}{{end}}</td></tr>{{end}}</table>{{end}}
{{if .Values}}<h3>{{$.L.values}}</h3><table>{{range .Values}}<tr><td>{{.Name}}</td><td>{{.Value}} {{.Unit}}</td></tr>{{end}}</table>{{end}}
{{end}}
<p class="muted">{{.L.footer}} {{.Version}}</p>
</body></html>`

var reportTmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"tr":   func(t i18n.Text) string { return "" },
	"flag": func(string) string { return "" },
}).Parse(reportHTML))

func (s *Server) apiReport(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang != "ru" {
		lang = "en"
	}
	sess := s.mgr.session()
	if sess == nil {
		writeError(w, http.StatusConflict, errNotConnected)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var ecus []reportECU
	for _, e := range sess.ECUs() {
		h := e.Handler()
		re := reportECU{Desc: h.Describe()}
		re.Items, _ = h.Identify(ctx)
		if codes, err := h.ReadDTC(ctx); err != nil {
			re.CodesErr = err.Error()
		} else {
			re.Codes = codes
		}
		if rr, ok := h.(ecu.ReadinessReader); ok {
			re.MIL, _, re.Monitors, _ = rr.Readiness(ctx)
		}
		params := h.Params()
		var ids []string
		index := ecu.ParamIndex(params)
		for _, p := range params {
			if p.Kind == "num" && len(ids) < 24 {
				ids = append(ids, p.ID)
			}
		}
		if len(ids) > 0 {
			if vals, err := h.Read(ctx, ids); err == nil {
				for _, v := range vals {
					if !v.OK {
						continue
					}
					p := index[v.ID]
					val := v.Text
					if val == "" || p.Kind == "num" {
						val = formatNumber(v.V, p.Digits)
					}
					re.Values = append(re.Values, reportValue{Name: p.Name.Get(lang), Value: val, Unit: p.Unit})
				}
			}
		}
		ecus = append(ecus, re)
	}
	t, err := reportTmpl.Clone()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	t.Funcs(template.FuncMap{
		"tr":   func(t i18n.Text) string { return t.Get(lang) },
		"flag": func(f string) string { return dtc.FlagNames[f].Get(lang) },
	})
	data := map[string]any{"Lang": lang, "L": reportLabels[lang], "Date": time.Now().Format("2006-01-02 15:04"),
		"Adapter": sess.Adapter, "Version": version.Name + " " + version.Version, "ECUs": ecus}
	path := filepath.Join(settings.Sub("reports"), "report-"+time.Now().Format("20060102-150405")+".html")
	if f, err := os.Create(path); err == nil {
		_ = t.Execute(f, data)
		f.Close()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
	_ = t.Execute(w, data)
}

func formatNumber(v float64, digits int) string {
	if digits < 0 {
		digits = 0
	}
	return trimZeros(strconvFormat(v, digits))
}
