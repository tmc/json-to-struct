package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"os"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"golang.org/x/tools/txtar"
)

//go:embed templates.txt
var defaultTemplates string

// legacyGenerateFunc can be set by build tags to use legacy implementation
var legacyGenerateFunc func(input io.Reader, structName, pkgName string, cfg *generator) ([]byte, error)

type generator struct {
	PackageName string // package name to use in generated code
	TypeName    string // struct name to use in generated code

	OmitEmpty bool // use omitempty in json tags

	Template string // custom template to use instead of default

	fileTemplate *template.Template
	typeTemplate *template.Template
}

// FieldStat tracks statistics about a field across multiple JSON objects
type FieldStat struct {
	Name       string
	Types      map[string]int  // type name -> count
	TotalCount int             // how many times this field appeared
	IsArray    map[string]bool // type -> whether it was seen as array
	JsonName   string          // original JSON field name
	NestedObjs []any           // store nested objects for proper struct generation
}

// StructStats tracks field statistics for building consolidated struct
type StructStats struct {
	Fields     map[string]*FieldStat
	TotalLines int
	FieldOrder []string // Track order of first field encounter
}

func (g *generator) loadTemplates() error {
	var templateData string

	// Try to load from specified template file first
	if g.Template != "" {
		if data, err := os.ReadFile(g.Template); err == nil {
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
		g.fileTemplate = template.Must(template.New("file").Parse(fileTmpl))
	}
	if typeTmpl, ok := templates["type.tmpl"]; ok {
		g.typeTemplate = template.Must(template.New("type").Parse(typeTmpl))
	}

	return nil
}

// NewStructStats creates a new StructStats instance
func NewStructStats() *StructStats {
	return &StructStats{
		Fields:     make(map[string]*FieldStat),
		FieldOrder: make([]string, 0),
	}
}

// ProcessValue processes a single value and updates field statistics
func (s *StructStats) ProcessValue(key string, value any, g *generator) {
	fieldName := g.fmtFieldName(key)

	if s.Fields[fieldName] == nil {
		s.Fields[fieldName] = &FieldStat{
			Name:       fieldName,
			JsonName:   key,
			Types:      make(map[string]int),
			IsArray:    make(map[string]bool),
			NestedObjs: make([]any, 0),
		}
		// Track the order of first encounter
		s.FieldOrder = append(s.FieldOrder, fieldName)
	}

	field := s.Fields[fieldName]
	field.TotalCount++

	switch v := value.(type) {
	case []any:
		if len(v) > 0 {
			elementType := g.getGoType(v[0])
			field.Types[elementType]++
			field.IsArray[elementType] = true
			// Store nested objects from arrays
			if elementType == "struct" {
				field.NestedObjs = append(field.NestedObjs, v[0])
			}
		} else {
			field.Types["any"]++
			field.IsArray["any"] = true
		}
	case map[string]any:
		field.Types["struct"]++
		// Store the nested object for proper struct generation
		field.NestedObjs = append(field.NestedObjs, v)
	default:
		goType := g.getGoType(value)
		field.Types[goType]++
	}
}

// ProcessJSON processes a single JSON object
func (s *StructStats) ProcessJSON(data map[string]any, g *generator) {
	s.TotalLines++
	// Process keys in sorted order to ensure deterministic field ordering
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		s.ProcessValue(key, data[key], g)
	}
}

// getGoType returns the Go type name for a JSON value
func (g *generator) getGoType(value any) string {
	if value == nil {
		return "nil"
	}

	switch value.(type) {
	case bool:
		return "bool"
	case float64:
		return "float64"
	case string:
		return "string"
	case map[string]any:
		return "struct"
	case []any:
		return "[]any" // This will be refined by the caller
	default:
		return "any"
	}
}

// GetMostCommonType returns the most frequently seen type for a field
func (f *FieldStat) GetMostCommonType() string {
	var maxType string
	maxCount := 0
	hasNil := false

	for typeName, count := range f.Types {
		if typeName == "nil" {
			hasNil = true
		} else if count > maxCount {
			maxCount = count
			maxType = typeName
		}
	}

	// If we have both nil and non-nil values, make it a pointer type
	if hasNil && maxType != "" {
		return "*" + maxType
	}

	if maxType == "" {
		maxType = "any"
	}

	return maxType
}

func (g *generator) generate(output io.Writer, input io.Reader) error {
	// Check if legacy implementation is available and use it
	if legacyGenerateFunc != nil {
		b, err := legacyGenerateFunc(input, g.TypeName, g.PackageName, g)
		if err != nil {
			return err
		}
		_, err = output.Write(b)
		return err
	}

	// New multi-line implementation
	stats := NewStructStats()

	// Read all input
	inputBytes, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	inputStr := strings.TrimSpace(string(inputBytes))
	if inputStr == "" {
		return fmt.Errorf("no input provided")
	}

	// Try to parse as different JSON structures
	var iresult any
	if err := json.Unmarshal(inputBytes, &iresult); err != nil {
		return fmt.Errorf("error parsing JSON: %w", err)
	}

	switch result := iresult.(type) {
	case map[string]any:
		// Single JSON object
		stats.ProcessJSON(result, g)
	case []any:
		// Array of objects - process each one
		for _, item := range result {
			if obj, ok := item.(map[string]any); ok {
				stats.ProcessJSON(obj, g)
			}
		}
	case []map[string]any:
		// Array of maps - process each one
		for _, obj := range result {
			stats.ProcessJSON(obj, g)
		}
	default:
		return fmt.Errorf("unsupported JSON structure: %T", iresult)
	}

	if stats.TotalLines == 0 {
		return fmt.Errorf("no valid JSON objects found")
	}

	// Generate the struct definition
	typ := g.buildTypeFromStats(stats)
	src := g.renderFile(typ.String())

	formatted, err := format.Source([]byte(src))
	if err != nil {
		return fmt.Errorf("error formatting generated code: %w", err)
	}

	_, err = output.Write(formatted)
	return err
}

// buildTypeFromStats creates a Type from accumulated statistics
func (g *generator) buildTypeFromStats(stats *StructStats) *Type {
	result := &Type{
		Name:   g.TypeName,
		Type:   "struct",
		Config: g,
	}

	// Convert field stats to Type children
	var children []*Type

	// Use different ordering strategies based on whether this is a single object or merged objects
	var fieldNames []string
	if stats.TotalLines == 1 {
		// Single object: use alphabetical order by JSON key (like legacy generateFieldTypes)
		// Collect all JSON keys deterministically by iterating over sorted field names
		fieldKeys := make([]string, 0, len(stats.Fields))
		for fieldName := range stats.Fields {
			fieldKeys = append(fieldKeys, fieldName)
		}
		sort.Strings(fieldKeys)

		jsonKeys := make([]string, 0, len(stats.Fields))
		for _, fieldName := range fieldKeys {
			jsonKeys = append(jsonKeys, stats.Fields[fieldName].JsonName)
		}
		sort.Strings(jsonKeys)

		// Convert JSON keys to field names in sorted order
		for _, jsonKey := range jsonKeys {
			fieldName := g.fmtFieldName(jsonKey)
			fieldNames = append(fieldNames, fieldName)
		}
	} else {
		// Multiple objects: use encounter order (like legacy Type.Merge)
		fieldNames = stats.FieldOrder
	}

	for _, fieldName := range fieldNames {
		stat := stats.Fields[fieldName]
		child := &Type{
			Name:   stat.Name,
			Config: g,
		}

		// Determine the most common type
		mostCommonType := stat.GetMostCommonType()

		// Check if it's an array type
		isArray := false
		for typeName, isArr := range stat.IsArray {
			if stat.Types[typeName] > 0 && isArr {
				isArray = true
				child.Type = typeName
				break
			}
		}

		if !isArray {
			child.Type = mostCommonType
		}

		child.Repeated = isArray

		// For struct types, create proper nested structures by merging all nested objects
		if child.Type == "struct" && len(stat.NestedObjs) > 0 {
			child.Type = "struct"
			// Merge all nested objects like the legacy implementation does
			child.Children = g.mergeNestedObjects(stat.NestedObjs, child.Name)
		}

		// Set JSON tags if field name differs from JSON name
		if stat.Name != stat.JsonName {
			child.Tags = map[string]string{"json": stat.JsonName}
		}

		// Legacy implementation doesn't use pointer types for optional fields
		// It just relies on json:",omitempty" tags

		children = append(children, child)
	}

	result.Children = children
	return result
}

// renderFile renders the complete Go file with package and type definition
func (g *generator) renderFile(content string) string {
	if g.fileTemplate != nil {
		data := struct {
			Package string
			Imports []string
			Content string
		}{
			Package: g.PackageName,
			Imports: nil, // No imports needed for basic struct types
			Content: content,
		}

		var buf bytes.Buffer
		if err := g.fileTemplate.Execute(&buf, data); err != nil {
			// Fallback to simple format
			return fmt.Sprintf("package %s\n\n%s", g.PackageName, content)
		}
		return buf.String()
	}

	// Default format
	return fmt.Sprintf("package %s\n\n%s", g.PackageName, content)
}

var uppercaseFixups = map[string]bool{"id": true, "url": true}

// fmtFieldName formats a JSON field name as a Go struct field name
func (g *generator) fmtFieldName(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if uppercaseFixups[strings.ToLower(last)] {
			parts[len(parts)-1] = strings.ToUpper(last)
		}
	}
	assembled := strings.Join(parts, "")
	runes := []rune(assembled)
	for i, c := range runes {
		ok := unicode.IsLetter(c) || unicode.IsDigit(c)
		if i == 0 {
			ok = unicode.IsLetter(c)
		}
		if !ok {
			runes[i] = '_'
		}
	}
	return string(runes)
}

// generateFieldTypesFromMap creates Type structures from a map, similar to legacy generateFieldTypes
func (g *generator) generateFieldTypesFromMap(obj map[string]any) []*Type {
	result := []*Type{}

	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		typ := g.generateTypeFromValue(key, obj[key])
		typ.Name = g.fmtFieldName(key)
		// if we need to rewrite the field name we need to record the json field in a tag.
		if typ.Name != key {
			typ.Tags = map[string]string{"json": key}
		}
		result = append(result, typ)
	}
	return result
}

// generateTypeFromValue creates a Type from a value, similar to legacy generateType
func (g *generator) generateTypeFromValue(name string, value any) *Type {
	result := &Type{Name: name, Config: g}
	switch v := value.(type) {
	case []any:
		result.Repeated = true
		if len(v) > 0 {
			// For now, handle arrays of basic types
			t := g.generateTypeFromValue("", v[0])
			result.Type = t.Type
			result.Children = t.Children
		} else {
			result.Type = "any"
		}
	case map[string]any:
		result.Type = "struct"
		result.Children = g.generateFieldTypesFromMap(v)
	default:
		if value == nil {
			result.Type = "nil"
		} else {
			result.Type = g.getGoType(value)
		}
	}
	return result
}

// mergeNestedObjects merges multiple nested objects into a single Type structure
func (g *generator) mergeNestedObjects(nestedObjs []any, name string) []*Type {
	if len(nestedObjs) == 0 {
		return nil
	}

	// Create a type from the first object
	var baseType *Type
	if firstMap, ok := nestedObjs[0].(map[string]any); ok {
		baseType = g.generateTypeFromValue(name, firstMap)
	} else {
		return nil
	}

	// Merge with remaining objects
	for i := 1; i < len(nestedObjs); i++ {
		if objMap, ok := nestedObjs[i].(map[string]any); ok {
			nextType := g.generateTypeFromValue(name, objMap)
			if err := baseType.Merge(nextType); err != nil {
				// If merge fails, continue with what we have
				continue
			}
		}
	}

	return baseType.Children
}

// renderTypeWithTemplate renders the type using the configured template
func (g *generator) renderTypeWithTemplate(t *Type) string {
	var buf bytes.Buffer
	if err := g.typeTemplate.Execute(&buf, t); err != nil {
		panic(err)
	}
	return buf.String()
}
