//go:build legacy

package main

import (
	"bytes"
	_ "embed"
	"os"
	"text/template"

	"golang.org/x/tools/txtar"
)

var (
	fileTemplate *template.Template
	typeTemplate *template.Template
)

func init() {
	loadTemplates("")
}

func loadTemplates(templatePath string) {
	var templateData string

	// Try to load from specified template file first
	if templatePath != "" {
		if data, err := os.ReadFile(templatePath); err == nil {
			templateData = string(data)
		}
	}

	// Fallback to embedded templates if external file not found or not specified
	if templateData == "" {
		templateData = defaultTemplates
	}

	// Parse the template data (either external or embedded)
	archive := txtar.Parse([]byte(templateData))
	templates := make(map[string]string)
	for _, file := range archive.Files {
		templates[file.Name] = string(file.Data)
	}

	if fileTmpl, ok := templates["file.tmpl"]; ok {
		fileTemplate = template.Must(template.New("file").Parse(fileTmpl))
	}
	if typeTmpl, ok := templates["type.tmpl"]; ok {
		typeTemplate = template.Must(template.New("type").Parse(typeTmpl))
	}
}

func (t *Type) templateString() string {
	var buf bytes.Buffer
	template := typeTemplate
	if t.Config != nil && t.Config.typeTemplate != nil {
		template = t.Config.typeTemplate
	}
	if err := template.Execute(&buf, t); err != nil {
		panic(err)
	}
	return buf.String()
}

func renderFile(packageName, content string, cfg *generator) string {
	data := struct {
		Package string
		Imports []string
		Content string
	}{
		Package: packageName,
		Imports: nil, // No imports needed for now
		Content: content,
	}

	var buf bytes.Buffer
	template := fileTemplate
	if cfg != nil && cfg.fileTemplate != nil {
		template = cfg.fileTemplate
	}
	if err := template.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String()
}
