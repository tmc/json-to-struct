package main

import (
	"crypto/md5"
	"fmt"
	"sort"
	"strings"
)

// extractRepeatedStructs identifies and extracts repeated struct patterns
func (g *generator) extractRepeatedStructs(root *Type) {
	if !g.ExtractStructs {
		return
	}

	g.extractedTypes = make(map[string]*Type)

	// Build a map of struct signatures to track duplicates
	structMap := make(map[string][]*Type)
	g.collectStructSignatures(root, structMap)

	// Extract structs that appear multiple times, or nullable structs (to avoid *struct without braces)
	for signature, types := range structMap {
		shouldExtract := len(types) > 1 || strings.HasSuffix(signature, ":nullable")

		if shouldExtract {
			// This struct appears multiple times or is nullable, extract it
			extracted := g.createExtractedType(types[0], signature)
			if extracted != nil {
				g.extractedTypes[extracted.Name] = extracted

				// Replace all occurrences with references
				for _, t := range types {
					// For nullable structs, the extracted type is a pointer
					if t.Type == "*struct" {
						t.ExtractedTypeName = "*" + extracted.Name
						t.Type = "*" + extracted.Name
					} else {
						t.ExtractedTypeName = extracted.Name
						t.Type = extracted.Name // Change type from "struct" to the extracted type name
					}
					t.Children = nil // Clear children since we're using a reference
				}
			}
		}
	}
}

// collectStructSignatures recursively collects all struct signatures
func (g *generator) collectStructSignatures(t *Type, structMap map[string][]*Type) {
	if t == nil {
		return
	}

	// Process both inline structs and nullable structs
	if (t.Type == "struct" || t.Type == "*struct") && len(t.Children) > 0 {
		sig := g.getStructSignature(t)
		if sig != "" {
			structMap[sig] = append(structMap[sig], t)
		}
	}

	// For nullable structs, we want to force extraction even if they only appear once
	// This prevents the `*struct` without braces issue
	if t.Type == "*struct" && len(t.Children) > 0 {
		sig := g.getStructSignature(t) + ":nullable"
		if sig != "" {
			// Force this to be extracted by adding it to a separate signature
			structMap[sig] = append(structMap[sig], t)
		}
	}

	// Recurse into children
	for _, child := range t.Children {
		g.collectStructSignatures(child, structMap)
	}
}

// getStructSignature generates a signature for a struct based on its fields
func (g *generator) getStructSignature(t *Type) string {
	if (t.Type != "struct" && t.Type != "*struct") || len(t.Children) == 0 {
		return ""
	}

	// Build a signature from sorted field names and types
	var fields []string
	for _, child := range t.Children {
		fieldSig := fmt.Sprintf("%s:%s", child.Name, child.Type)
		if child.Repeated {
			fieldSig = "[]" + fieldSig
		}
		fields = append(fields, fieldSig)
	}

	sort.Strings(fields)
	signature := strings.Join(fields, ",")

	// For nullable structs, we want to extract even with fewer fields to avoid *struct rendering issues
	// For regular structs, only extract structs with at least 3 fields to avoid over-extraction
	if t.Type != "*struct" && len(fields) < 3 {
		return ""
	}

	return signature
}

// createExtractedType creates a new named type from a struct
func (g *generator) createExtractedType(t *Type, signature string) *Type {
	if t.Type != "struct" && t.Type != "*struct" {
		return nil
	}

	// Generate a name based on the struct's content
	name := g.generateStructName(t, signature)

	// Create a copy of the type with the new name
	// Always make the extracted type a regular struct, even if the original was *struct
	extracted := &Type{
		Name:     name,
		Type:     "struct",
		Children: make([]*Type, len(t.Children)),
		Config:   t.Config,
	}

	// Deep copy children
	for i, child := range t.Children {
		extracted.Children[i] = child.Copy()
	}

	return extracted
}

// generateStructName generates a meaningful name for an extracted struct
func (g *generator) generateStructName(t *Type, signature string) string {
	// Start with the root type name as prefix
	prefix := g.TypeName
	if prefix == "" {
		prefix = "Foo" // Default fallback
	}

	// Try to find a meaningful name from the fields
	// Look for common patterns like "stat", "token", etc.

	// Check if all fields start with a common prefix
	if len(t.Children) > 0 {
		// Look for fields like st_* which suggest "Stat"
		if hasCommonPrefix(t.Children, "St") {
			return prefix + "Stat"
		}

	}

	// Fallback: generate a name from a hash of the signature
	hash := md5.Sum([]byte(signature))
	return fmt.Sprintf("%sStruct%X", prefix, hash[:4])
}

// hasCommonPrefix checks if all fields share a common prefix
func hasCommonPrefix(fields []*Type, prefix string) bool {
	if len(fields) == 0 {
		return false
	}

	count := 0
	for _, field := range fields {
		if strings.HasPrefix(field.Name, prefix) {
			count++
		}
	}

	// Consider it a common prefix if at least 80% of fields have it
	return float64(count) >= float64(len(fields))*0.8
}

// Copy creates a deep copy of a Type
func (t *Type) Copy() *Type {
	if t == nil {
		return nil
	}

	copied := &Type{
		Name:              t.Name,
		Type:              t.Type,
		Repeated:          t.Repeated,
		Tags:              make(map[string]string),
		Config:            t.Config,
		Stat:              t.Stat,
		ExtractedTypeName: t.ExtractedTypeName,
	}

	// Copy tags
	for k, v := range t.Tags {
		copied.Tags[k] = v
	}

	// Deep copy children
	if len(t.Children) > 0 {
		copied.Children = make([]*Type, len(t.Children))
		for i, child := range t.Children {
			copied.Children[i] = child.Copy()
		}
	}

	return copied
}
