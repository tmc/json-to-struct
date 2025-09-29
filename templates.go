package main

import (
	"bytes"
	"text/template"
)

var structTemplate = template.Must(template.New("struct").Parse(`type {{.Name}} {{.GetType}} {
{{range .Children}}	{{.Name}} {{if eq .Type "struct"}}{{if .Repeated}}[]{{end}}struct {
{{range .Children}}		{{.Name}} {{.GetType}} {{.GetTags}}
{{end}}	}{{else}}{{.GetType}}{{end}} {{.GetTags}}
{{end}}} {{.GetTags}}`))

var fieldTemplate = template.Must(template.New("field").Parse(`type {{.Name}} {{.GetType}} {{.GetTags}}`))

func (t *Type) templateString() string {
	if t.Type != "struct" {
		var buf bytes.Buffer
		if err := fieldTemplate.Execute(&buf, t); err != nil {
			panic(err)
		}
		return buf.String()
	}

	var buf bytes.Buffer
	if err := structTemplate.Execute(&buf, t); err != nil {
		panic(err)
	}
	return buf.String()
}