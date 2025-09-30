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

// FormatError is returned when generated code fails to format
type FormatError struct {
	OriginalError error
	Source        string // The unformatted source code
	LineNum       int
	Column        int
}

func (e *FormatError) Error() string {
	return fmt.Sprintf("formatting error at line %d:%d: %v", e.LineNum, e.Column, e.OriginalError)
}

func (e *FormatError) Unwrap() error {
	return e.OriginalError
}

//go:embed templates.txt
var defaultTemplates string

// legacyGenerateFunc can be set by build tags to use legacy implementation
var legacyGenerateFunc func(input io.Reader, structName, pkgName string, cfg *generator) ([]byte, error)

type generator struct {
	PackageName string // package name to use in generated code
	TypeName    string // struct name to use in generated code

	OmitEmpty      bool   // use omitempty in json tags
	StatComments   bool   // add field statistics as comments
	Stream         bool   // show progressive output with terminal clearing
	ExtractStructs bool   // extract repeated structs to reduce duplication
	UpdateInterval int    // milliseconds between stream updates
	FieldOrder     string // field ordering strategy: common-first, rare-first, alphabetical

	Template string // custom template to use instead of default

	fileTemplate *template.Template
	typeTemplate *template.Template

	// Statistics gathered during parsing
	stats *StructStats

	// Extracted struct definitions
	extractedTypes map[string]*Type

	// Cache for fmtFieldName to avoid repeated expensive string operations
	fieldNameCache map[string]string
}

// FieldStat tracks statistics about a field across multiple JSON objects
type FieldStat struct {
	Name        string
	Types       map[string]int  // type name -> count
	TotalCount  int             // how many times this field appeared
	IsArray     map[string]bool // type -> whether it was seen as array
	JsonName    string          // original JSON field name
	NestedObjs  []any           // store nested objects for proper struct generation
	Values      map[string]int  // for string/number fields, track unique values and their counts
	NumericVals []float64       // for numeric fields, track all values for percentile calculation
	ValueOrder  []string        // track order of first appearance for values
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

	// Choose template based on stat-comments flag
	var typeTemplateKey string
	if g.StatComments {
		// Try to use stat-comments version first
		if _, ok := templates["type-with-stats.tmpl"]; ok {
			typeTemplateKey = "type-with-stats.tmpl"
		} else {
			typeTemplateKey = "type.tmpl"
		}
	} else {
		typeTemplateKey = "type.tmpl"
	}

	if fileTmpl, ok := templates["file.tmpl"]; ok {
		g.fileTemplate = template.Must(template.New("file").Parse(fileTmpl))
	}
	if typeTmpl, ok := templates[typeTemplateKey]; ok {
		g.typeTemplate = template.Must(template.New("type").Funcs(template.FuncMap{
			"GetStatComment": func(t *Type) string {
				return t.GetStatComment()
			},
			"RenderInlineStruct": func(t *Type, depth int) string {
				return g.renderInlineStruct(t, depth)
			},
		}).Parse(typeTmpl))
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
			Values:     make(map[string]int),
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
	case string:
		field.Types["string"]++
		// Track string values for cardinality
		if len(field.Values) < 100 { // Limit tracking to avoid memory issues
			if _, exists := field.Values[v]; !exists {
				field.ValueOrder = append(field.ValueOrder, v)
			}
			field.Values[v]++
		}
	case float64:
		field.Types["float64"]++
		// Track all numeric values for statistics
		if field.NumericVals == nil {
			field.NumericVals = make([]float64, 0)
		}
		field.NumericVals = append(field.NumericVals, v)

		// Track numeric values if they look like enums (small integers)
		if v == float64(int(v)) && v >= -100 && v <= 100 {
			valStr := fmt.Sprintf("%d", int(v))
			if _, exists := field.Values[valStr]; !exists {
				field.ValueOrder = append(field.ValueOrder, valStr)
			}
			field.Values[valStr]++
		}
	case bool:
		field.Types["bool"]++
		valStr := fmt.Sprintf("%v", v)
		if _, exists := field.Values[valStr]; !exists {
			field.ValueOrder = append(field.ValueOrder, valStr)
		}
		field.Values[valStr]++
	case nil:
		field.Types["nil"]++
	default:
		goType := g.getGoType(value)
		field.Types[goType]++
	}
}

// ProcessJSON processes a single JSON object
func (s *StructStats) ProcessJSON(data map[string]any, g *generator) {
	s.TotalLines++
	// Process all fields - ordering will be handled when building the final type
	for key, value := range data {
		s.ProcessValue(key, value, g)
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

	// Use streaming mode if requested
	if g.Stream {
		return g.generateStream(output, input)
	}

	// New multi-line implementation
	stats := NewStructStats()
	g.stats = stats

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
		// Not valid JSON, try NDJSON (newline-delimited JSON)
		lines := strings.Split(inputStr, "\n")
		hasValidJSON := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err == nil {
				stats.ProcessJSON(obj, g)
				hasValidJSON = true
			}
		}
		if !hasValidJSON {
			return fmt.Errorf("error parsing JSON: %w", err)
		}
	} else {
		// Successfully parsed as regular JSON
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
	}

	if stats.TotalLines == 0 {
		return fmt.Errorf("no valid JSON objects found")
	}

	// Generate the struct definition
	typ := g.buildTypeFromStats(stats)

	// Extract repeated structs if requested
	if g.ExtractStructs {
		g.extractRepeatedStructs(typ)
	}

	// Build the complete output with extracted types
	var src string
	if g.ExtractStructs && len(g.extractedTypes) > 0 {
		// Render extracted types first, then main type
		var parts []string

		// Sort extracted type names for deterministic output
		var names []string
		for name := range g.extractedTypes {
			names = append(names, name)
		}
		sort.Strings(names)

		// Add extracted types
		for _, name := range names {
			parts = append(parts, g.extractedTypes[name].String())
		}

		// Add main type
		parts = append(parts, typ.String())

		src = g.renderFile(strings.Join(parts, "\n\n"))
	} else {
		src = g.renderFile(typ.String())
	}

	formatted, err := format.Source([]byte(src))
	if err != nil {
		// Write the unformatted source to output anyway so user can see what was generated
		output.Write([]byte(src))

		// Parse go/format error which is like "61:17: expected '{', found `json:"result,omitempty"`"
		var lineNum, colNum int
		fmt.Sscanf(err.Error(), "%d:%d:", &lineNum, &colNum)

		// Return a FormatError with all the info
		// The error will be printed to stderr but we still wrote the output
		return &FormatError{
			OriginalError: err,
			Source:        src,
			LineNum:       lineNum,
			Column:        colNum,
		}
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

	// Sort fields by occurrence count (most common first), then alphabetically for determinism
	type fieldInfo struct {
		name     string
		jsonName string
		count    int
		order    int // first encounter order
	}

	fields := make([]fieldInfo, 0, len(stats.Fields))
	orderMap := make(map[string]int)
	for i, name := range stats.FieldOrder {
		orderMap[name] = i
	}

	for fieldName, stat := range stats.Fields {
		fields = append(fields, fieldInfo{
			name:     fieldName,
			jsonName: stat.JsonName,
			count:    stat.TotalCount,
			order:    orderMap[fieldName],
		})
	}

	// Sort based on configured field ordering strategy
	switch g.FieldOrder {
	case "encounter":
		// Use encounter order (no sorting by count)
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].order < fields[j].order
		})
	case "rare-first":
		// Rare fields first (ascending count), then by encounter order
		sort.Slice(fields, func(i, j int) bool {
			if fields[i].count != fields[j].count {
				return fields[i].count < fields[j].count // Lower count first
			}
			return fields[i].order < fields[j].order
		})
	case "common-first":
		// Common fields first (descending count), then by encounter order
		sort.Slice(fields, func(i, j int) bool {
			if fields[i].count != fields[j].count {
				return fields[i].count > fields[j].count // Higher count first
			}
			return fields[i].order < fields[j].order
		})
	default: // "alphabetical" or unspecified
		// Alphabetical by JSON key name (legacy default)
		sort.Slice(fields, func(i, j int) bool {
			return strings.ToLower(fields[i].jsonName) < strings.ToLower(fields[j].jsonName)
		})
	}

	fieldNames := make([]string, len(fields))
	for i, f := range fields {
		fieldNames[i] = f.name
	}

	for _, fieldName := range fieldNames {
		stat := stats.Fields[fieldName]
		child := &Type{
			Name:   stat.Name,
			Config: g,
			Stat:   stat, // Add statistics for comment generation
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
	// Initialize cache if needed
	if g.fieldNameCache == nil {
		g.fieldNameCache = make(map[string]string)
	}

	// Check cache first
	if cached, ok := g.fieldNameCache[s]; ok {
		return cached
	}

	// Compute the formatted name efficiently
	parts := strings.Split(s, "_")
	for i := range parts {
		// Replace deprecated strings.Title with efficient implementation
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
		}
	}
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		lastLower := strings.ToLower(last)
		if uppercaseFixups[lastLower] {
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
	result := string(runes)

	// Cache the result
	g.fieldNameCache[s] = result
	return result
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

// mergeNestedObjects merges multiple nested objects into a single Type structure with statistics
func (g *generator) mergeNestedObjects(nestedObjs []any, name string) []*Type {
	if len(nestedObjs) == 0 {
		return nil
	}

	// Create a nested stats collector
	nestedStats := NewStructStats()

	// Process all nested objects to gather statistics
	for _, obj := range nestedObjs {
		if objMap, ok := obj.(map[string]any); ok {
			nestedStats.ProcessJSON(objMap, g)
		}
	}

	// Build the type from the statistics (recursive)
	nestedType := g.buildTypeFromStats(nestedStats)
	return nestedType.Children
}

// renderTypeWithTemplate renders the type using the configured template
func (g *generator) renderTypeWithTemplate(t *Type) string {
	var buf bytes.Buffer
	if err := g.typeTemplate.Execute(&buf, t); err != nil {
		panic(err)
	}
	return buf.String()
}
