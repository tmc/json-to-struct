package main

import (
	"fmt"
	"sort"
	"strings"
)

type Fields []*Type

func (f Fields) String() string {
	result := []string{}
	for _, field := range f {
		result = append(result, field.String())
	}
	return strings.Join(result, "\n")
}

type Type struct {
	Name              string
	Repeated          bool
	Type              string
	Tags              map[string]string
	Children          Fields
	Config            *generator
	Stat              *FieldStat // Optional field statistics for comments
	ExtractedTypeName string     // If set, use this type name instead of inline struct
}

func (t *Type) GetType() string {
	// Use extracted type name if available
	if t.ExtractedTypeName != "" {
		if t.Repeated {
			return "[]" + t.ExtractedTypeName
		}
		return t.ExtractedTypeName
	}

	if t.Type == "nil" {
		t.Type = "any"
	}

	if t.Repeated {
		return "[]" + t.Type
	}
	return t.Type
}

func (t *Type) GetTags() string {
	if len(t.Tags) == 0 {
		return ""
	}

	keys := make([]string, 0, len(t.Tags))
	for key := range t.Tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		v := t.Tags[k]
		if k == "json" && t.Config.OmitEmpty {
			v += ",omitempty"
		}
		parts = append(parts, fmt.Sprintf(`%v:"%v"`, k, v))
	}
	return fmt.Sprintf("`%v`", strings.Join(parts, ","))
}

func (t *Type) GetStatComment() string {
	if t.Stat == nil || t.Config == nil || !t.Config.StatComments {
		return ""
	}

	// Build comment with field statistics
	comments := []string{}

	// Add occurrence count
	if t.Config.stats != nil && t.Config.stats.TotalLines > 0 {
		percentage := float64(t.Stat.TotalCount) * 100.0 / float64(t.Config.stats.TotalLines)
		comments = append(comments, fmt.Sprintf("seen in %.1f%% (%d/%d)",
			percentage, t.Stat.TotalCount, t.Config.stats.TotalLines))
	}

	// Add type distribution if multiple types seen
	if len(t.Stat.Types) > 1 {
		typeInfo := []string{}
		for typeName, count := range t.Stat.Types {
			typeInfo = append(typeInfo, fmt.Sprintf("%s:%d", typeName, count))
		}
		sort.Strings(typeInfo)
		comments = append(comments, "types: "+strings.Join(typeInfo, ", "))
	}

	// For numeric fields, show percentiles if they appear to be continuous
	if t.Type == "float64" && len(t.Stat.NumericVals) > 0 {
		// Check if values look like continuous data (not just small integers/enums)
		continuousData := false
		for _, v := range t.Stat.NumericVals {
			if v != float64(int(v)) || v < -100 || v > 100 {
				continuousData = true
				break
			}
		}

		if continuousData && len(t.Stat.NumericVals) > 2 {
			// Calculate percentiles
			sorted := make([]float64, len(t.Stat.NumericVals))
			copy(sorted, t.Stat.NumericVals)
			sort.Float64s(sorted)

			getPercentile := func(p float64) float64 {
				index := p * float64(len(sorted)-1)
				lower := int(index)
				upper := lower + 1
				if upper >= len(sorted) {
					return sorted[lower]
				}
				weight := index - float64(lower)
				return sorted[lower]*(1-weight) + sorted[upper]*weight
			}

			min := sorted[0]
			max := sorted[len(sorted)-1]

			// Format based on the range and values
			formatVal := func(v float64) string {
				if v == float64(int(v)) && v > -1000000 && v < 1000000 {
					return fmt.Sprintf("%.0f", v)
				}
				return fmt.Sprintf("%.2g", v)
			}

			if len(sorted) >= 10 {
				comments = append(comments, fmt.Sprintf("range: [%s, p25:%s, p50:%s, p75:%s, p90:%s, p99:%s, %s]",
					formatVal(min), formatVal(getPercentile(0.25)), formatVal(getPercentile(0.5)),
					formatVal(getPercentile(0.75)), formatVal(getPercentile(0.90)), formatVal(getPercentile(0.99)), formatVal(max)))
			} else {
				comments = append(comments, fmt.Sprintf("range: [%s, %s]", formatVal(min), formatVal(max)))
			}
		} else if len(t.Stat.Values) > 0 && len(t.Stat.Values) < 10 {
			// For enum-like numbers, show the values in order of appearance with percentages
			valueStrings := make([]string, 0, len(t.Stat.ValueOrder))
			for _, val := range t.Stat.ValueOrder {
				if count, exists := t.Stat.Values[val]; exists {
					percentage := float64(count) * 100.0 / float64(t.Stat.TotalCount)
					valueStrings = append(valueStrings, fmt.Sprintf("%s:%.1f%%", val, percentage))
				}
			}
			comments = append(comments, fmt.Sprintf("values: %s", strings.Join(valueStrings, ", ")))
		}
	} else if len(t.Stat.Values) > 0 && len(t.Stat.Values) < 10 && t.Type != "float64" {
		// For non-numeric fields with low cardinality, show in order of appearance with percentages
		valueStrings := make([]string, 0, len(t.Stat.ValueOrder))
		for _, val := range t.Stat.ValueOrder {
			if count, exists := t.Stat.Values[val]; exists {
				percentage := float64(count) * 100.0 / float64(t.Stat.TotalCount)
				displayVal := val
				if len(val) > 20 {
					// Truncate long values
					displayVal = fmt.Sprintf("%s...", val[:17])
				}
				valueStrings = append(valueStrings, fmt.Sprintf("%q:%.1f%%", displayVal, percentage))
			}
		}
		comments = append(comments, fmt.Sprintf("values: %s", strings.Join(valueStrings, ", ")))
	} else if len(t.Stat.Values) >= 10 {
		// Just show cardinality if too many unique values
		comments = append(comments, fmt.Sprintf("%d unique values", len(t.Stat.Values)))
	}

	if len(comments) > 0 {
		return " // " + strings.Join(comments, ", ")
	}
	return ""
}

func (t *Type) String() string {
	if t.Config != nil && t.Config.typeTemplate != nil {
		return t.Config.renderTypeWithTemplate(t)
	}
	return t.Config.renderType(t)
}

func (t *Type) Merge(t2 *Type) error {
	if strings.Trim(t.Type, "*") != strings.Trim(t2.Type, "*") {
		if t.Type == "nil" {
			// When merging nil with a struct, copy the whole type (including children)
			if t2.Type == "struct" {
				t.Type = "*struct"
				t.Children = t2.Children
			} else {
				t.Type = fmt.Sprintf("*%s", strings.Trim(t2.Type, "*"))
			}
			return nil
		} else if t2.Type == "nil" {
			// When merging struct with nil, make it a pointer
			if t.Type == "struct" {
				t.Type = "*struct"
				// Children remain the same
			} else {
				t.Type = fmt.Sprintf("*%s", strings.Trim(t.Type, "*"))
			}
			return nil
		} else {
			t.Type = "any"
			return nil
		}
	}

	fields := map[string]*Type{}
	for _, typ := range t.Children {
		fields[typ.Name] = typ
	}
	for _, typ := range t2.Children {
		field, ok := fields[typ.Name]
		if !ok {
			t.Children = append(t.Children, typ)
			continue
		}
		if err := field.Merge(typ); err != nil {
			return fmt.Errorf("issue with '%v': %w", field.Name, err)
		}
	}

	return nil
}

// renderType renders the type as a Go struct definition.
func (g *generator) renderType(t *Type) string {
	return g.renderTypeWithKeyword(t, true)
}

// renderInlineStruct renders a struct type inline (for nested anonymous structs)
func (g *generator) renderInlineStruct(t *Type, depth int) string {
	indent := strings.Repeat("\t", depth)

	// Handle pointer to struct
	if t.Type == "*struct" {
		if len(t.Children) == 0 {
			return "*struct{}"
		}
		// Build the pointer struct with proper indentation
		var result strings.Builder
		result.WriteString("*struct {\n")
		for _, child := range t.Children {
			result.WriteString(indent + "\t")
			result.WriteString(child.Name)
			result.WriteString(" ")

			if child.Type == "struct" && len(child.Children) > 0 {
				// Recursively render nested struct
				result.WriteString(g.renderInlineStruct(child, depth+1))
			} else if child.Type == "*struct" && len(child.Children) > 0 {
				// Recursively render nested pointer struct
				result.WriteString(g.renderInlineStruct(child, depth+1))
			} else if child.Type == "struct" && len(child.Children) == 0 {
				// Empty struct
				result.WriteString("struct{}")
			} else if child.Type == "*struct" && len(child.Children) == 0 {
				// Empty pointer struct
				result.WriteString("*struct{}")
			} else {
				// Simple type
				if child.Type == "nil" {
					child.Type = "any"
				}
				typeStr := child.Type
				if child.Repeated {
					typeStr = "[]" + typeStr
				}
				if child.ExtractedTypeName != "" {
					typeStr = child.ExtractedTypeName
					if child.Repeated {
						typeStr = "[]" + child.ExtractedTypeName
					}
				}
				result.WriteString(typeStr)
			}

			if tags := child.GetTags(); tags != "" {
				result.WriteString(" ")
				result.WriteString(tags)
			}
			// Add stat comments if enabled
			if g.StatComments {
				if comment := child.GetStatComment(); comment != "" {
					result.WriteString(comment)
				}
			}
			result.WriteString("\n")
		}
		result.WriteString(indent + "}")
		return result.String()
	}

	if t.Type != "struct" {
		// Not a struct, just return the type
		if t.Repeated {
			return "[]" + t.Type
		}
		return t.Type
	}

	if len(t.Children) == 0 {
		// Empty struct
		if t.Repeated {
			return "[]struct{}"
		}
		return "struct{}"
	}

	// Build the struct with proper indentation
	var result strings.Builder
	if t.Repeated {
		result.WriteString("[]")
	}
	result.WriteString("struct {\n")

	for _, child := range t.Children {
		result.WriteString(indent + "\t")
		result.WriteString(child.Name)
		result.WriteString(" ")

		if child.Type == "struct" && len(child.Children) > 0 {
			// Recursively render nested struct
			result.WriteString(g.renderInlineStruct(child, depth+1))
		} else if child.Type == "struct" && len(child.Children) == 0 {
			// Empty struct
			result.WriteString("struct{}")
		} else {
			// Simple type - don't call GetType to avoid infinite recursion
			if child.Type == "nil" {
				child.Type = "any"
			}
			typeStr := child.Type
			if child.Repeated {
				typeStr = "[]" + typeStr
			}
			if child.ExtractedTypeName != "" {
				typeStr = child.ExtractedTypeName
				if child.Repeated {
					typeStr = "[]" + child.ExtractedTypeName
				}
			}
			result.WriteString(typeStr)
		}

		if tags := child.GetTags(); tags != "" {
			result.WriteString(" ")
			result.WriteString(tags)
		}
		// Add stat comments if enabled
		if g.StatComments {
			if comment := child.GetStatComment(); comment != "" {
				result.WriteString(comment)
			}
		}
		result.WriteString("\n")
	}

	result.WriteString(indent + "}")
	return result.String()
}

// renderTypeWithKeyword renders the type, optionally including the 'type' keyword
func (g *generator) renderTypeWithKeyword(t *Type, includeTypeKeyword bool) string {
	// If this is using an extracted type, don't render children
	if t.ExtractedTypeName != "" {
		if includeTypeKeyword {
			return fmt.Sprintf("type %s %s%s", t.Name, t.GetType(), t.GetTags())
		}
		return fmt.Sprintf("%s %s%s", t.Name, t.GetType(), t.GetTags())
	}

	// Check if this is a struct with no children
	if t.Type == "struct" && len(t.Children) == 0 {
		// Empty struct needs braces
		if includeTypeKeyword {
			return fmt.Sprintf("type %s struct {}%s", t.Name, t.GetTags())
		}
		return fmt.Sprintf("%s struct {}%s", t.Name, t.GetTags())
	}

	if len(t.Children) == 0 {
		// Non-struct types (like string, int, etc.)
		if includeTypeKeyword {
			return fmt.Sprintf("type %s %s%s", t.Name, t.GetType(), t.GetTags())
		}
		return fmt.Sprintf("%s %s%s", t.Name, t.GetType(), t.GetTags())
	}

	result := []string{}
	if includeTypeKeyword {
		result = append(result, fmt.Sprintf("type %s %s {", t.Name, t.GetType()))
	} else {
		result = append(result, fmt.Sprintf("%s %s {", t.Name, t.GetType()))
	}

	for _, child := range t.Children {
		result = append(result, fmt.Sprintf("    %s", g.renderTypeWithKeyword(child, false)))
	}
	result = append(result, fmt.Sprintf("}%s", t.GetTags()))
	return strings.Join(result, "\n")
}
