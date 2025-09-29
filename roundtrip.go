//go:build !js
// +build !js

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runRoundtripTest generates a round-trip validation program, compiles it, and runs it with the input data
func runRoundtripTest(g *generator) error {
	// Read input data first - we'll need it twice
	inputData, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	return runRoundtripTestWithData(g, inputData)
}

// runRoundtripTestWithData runs a round-trip validation test with the provided input data
func runRoundtripTestWithData(g *generator, inputData []byte) error {

	// Force main package for roundtrip test
	origPkg := g.PackageName
	g.PackageName = "main"

	// Generate the struct definition
	var structBuf bytes.Buffer
	if err := g.generate(&structBuf, bytes.NewReader(inputData)); err != nil {
		return fmt.Errorf("failed to generate struct: %w", err)
	}

	g.PackageName = origPkg // Restore original

	// Create temp directory for the test program
	tmpDir, err := os.MkdirTemp("", "json-to-struct-roundtrip-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate the round-trip test program
	testProgram := generateRoundtripProgram(g.TypeName, structBuf.String())

	// Write the program to a file
	programPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(programPath, []byte(testProgram), 0644); err != nil {
		return fmt.Errorf("failed to write test program: %w", err)
	}


	// Run the program directly with go run and capture output
	runCmd := exec.Command("go", "run", "main.go")
	runCmd.Dir = tmpDir // Set working directory to the temp directory
	runCmd.Stdin = bytes.NewReader(inputData)

	output, err := runCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("round-trip test failed: %w\nOutput: %s", err, output)
	}

	// Parse the output to extract summary statistics
	lines := strings.Split(string(output), "\n")
	var totalRecords, successfulParse, parseErrors int
	var fieldCoverage []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Total Records:") {
			fmt.Sscanf(line, "Total Records: %d", &totalRecords)
		} else if strings.HasPrefix(line, "Successful Parse:") {
			fmt.Sscanf(line, "Successful Parse: %d", &successfulParse)
		} else if strings.HasPrefix(line, "Parse Errors:") {
			fmt.Sscanf(line, "Parse Errors: %d", &parseErrors)
		} else if strings.Contains(line, "records (") && strings.Contains(line, "%)") {
			// Field coverage line
			fieldCoverage = append(fieldCoverage, line)
		} else if strings.HasPrefix(line, "Successfully round-tripped") {
			fieldCoverage = append(fieldCoverage, line)
		}
	}

	// Report summary to stderr
	if parseErrors > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  Round-trip: %d/%d records parsed (%d errors)\n",
			successfulParse, totalRecords, parseErrors)
	} else {
		fmt.Fprintf(os.Stderr, "✓ Round-trip: %d/%d records validated successfully\n",
			successfulParse, totalRecords)
	}

	// If there are issues, show more details
	if parseErrors > 0 || len(fieldCoverage) > 0 {
		for _, fc := range fieldCoverage {
			if strings.Contains(fc, "round-tripped") {
				fmt.Fprintf(os.Stderr, "  %s\n", fc)
			}
		}
	}

	return nil
}

// generateRoundtripProgram generates a complete Go program for round-trip testing
func generateRoundtripProgram(typeName, structDef string) string {
	// Extract just the struct definition, removing package declaration
	lines := strings.Split(structDef, "\n")
	var structOnly []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "package ") && line != "" {
			structOnly = append(structOnly, line)
		}
	}
	structDefClean := strings.Join(structOnly, "\n")

	return fmt.Sprintf(`package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
)

%s

type ValidationStats struct {
	TotalRecords    int                        ` + "`json:\"total_records\"`" + `
	SuccessfulParse int                        ` + "`json:\"successful_parse\"`" + `
	ParseErrors     int                        ` + "`json:\"parse_errors\"`" + `
	FieldStats      map[string]FieldValidation ` + "`json:\"field_stats\"`" + `
	TypeMismatches  []TypeMismatch             ` + "`json:\"type_mismatches,omitempty\"`" + `
}

type FieldValidation struct {
	ActualCount   int      ` + "`json:\"actual_count\"`" + `
	NilCount      int      ` + "`json:\"nil_count\"`" + `
	TypeErrors    []string ` + "`json:\"type_errors,omitempty\"`" + `
}

type TypeMismatch struct {
	Record      int    ` + "`json:\"record\"`" + `
	Field       string ` + "`json:\"field\"`" + `
	Expected    string ` + "`json:\"expected\"`" + `
	Actual      string ` + "`json:\"actual\"`" + `
	OriginalVal any    ` + "`json:\"original_value\"`" + `
}

func main() {
	stats := &ValidationStats{
		FieldStats: make(map[string]FieldValidation),
	}

	scanner := bufio.NewScanner(os.Stdin)
	recordNum := 0
	var allInputs []string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		allInputs = append(allInputs, line)
		recordNum++
		stats.TotalRecords++

		// Try to parse as single object or array
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			// Handle array
			var rawArray []map[string]any
			if err := json.Unmarshal([]byte(line), &rawArray); err != nil {
				var objArray []any
				if err := json.Unmarshal([]byte(line), &objArray); err != nil {
					log.Printf("Record %%d: Failed to parse array: %%v", recordNum, err)
					stats.ParseErrors++
					continue
				}
				// Process each object in array
				for i, obj := range objArray {
					if objMap, ok := obj.(map[string]any); ok {
						validateRecord(objMap, recordNum*1000+i, stats)
					}
				}
				continue
			}
			// Process each object in array
			for i, obj := range rawArray {
				validateRecord(obj, recordNum*1000+i, stats)
			}
		} else {
			// Parse as single object
			var rawData map[string]any
			if err := json.Unmarshal([]byte(line), &rawData); err != nil {
				log.Printf("Record %%d: Failed to parse as JSON object: %%v", recordNum, err)
				stats.ParseErrors++
				continue
			}
			validateRecord(rawData, recordNum, stats)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %%v", err)
	}

	// Output validation statistics
	fmt.Printf("\n=== ROUND-TRIP VALIDATION RESULTS ===\n")
	fmt.Printf("Total Records: %%d\n", stats.TotalRecords)
	fmt.Printf("Successful Parse: %%d (%%0.1f%%%%)\n",
		stats.SuccessfulParse,
		float64(stats.SuccessfulParse)/float64(stats.TotalRecords)*100)
	fmt.Printf("Parse Errors: %%d\n", stats.ParseErrors)

	if len(stats.TypeMismatches) > 0 {
		fmt.Printf("\n=== TYPE MISMATCHES ===\n")
		for _, mismatch := range stats.TypeMismatches {
			fmt.Printf("Record %%d, Field '%%s': Expected %%s, got %%s (value: %%v)\n",
				mismatch.Record, mismatch.Field, mismatch.Expected, mismatch.Actual, mismatch.OriginalVal)
		}
	}

	// Field coverage analysis
	fmt.Printf("\n=== FIELD COVERAGE ===\n")
	for fieldName, validation := range stats.FieldStats {
		coverage := float64(validation.ActualCount) / float64(stats.SuccessfulParse) * 100
		fmt.Printf("%%s: %%d/%%d records (%%0.1f%%%%), %%d nil values\n",
			fieldName, validation.ActualCount, stats.SuccessfulParse, coverage, validation.NilCount)

		if len(validation.TypeErrors) > 0 {
			fmt.Printf("  Type errors: %%v\n", validation.TypeErrors)
		}
	}

	// Re-marshal test
	fmt.Printf("\n=== RE-MARSHAL TEST ===\n")
	successCount := 0
	for i, input := range allInputs {
		var original map[string]any
		if strings.HasPrefix(strings.TrimSpace(input), "[") {
			// Skip arrays for re-marshal test
			continue
		}

		if err := json.Unmarshal([]byte(input), &original); err != nil {
			continue
		}

		var generated %s
		if err := json.Unmarshal([]byte(input), &generated); err != nil {
			fmt.Printf("Record %%d: Failed to unmarshal: %%v\n", i+1, err)
			continue
		}

		remarshaled, err := json.Marshal(generated)
		if err != nil {
			fmt.Printf("Record %%d: Failed to re-marshal: %%v\n", i+1, err)
			continue
		}

		var remarshaledMap map[string]any
		if err := json.Unmarshal(remarshaled, &remarshaledMap); err != nil {
			fmt.Printf("Record %%d: Failed to parse re-marshaled data: %%v\n", i+1, err)
			continue
		}

		// Deep comparison of values
		mismatch := false
		for key, origVal := range original {
			remarshaledVal, exists := remarshaledMap[key]
			if !exists {
				fmt.Printf("Record %%d: Missing field '%%s' in remarshaled data\n", i+1, key)
				mismatch = true
				continue
			}
			// Compare values (note: JSON numbers are always float64)
			if !compareJSONValues(origVal, remarshaledVal) {
				fmt.Printf("Record %%d: Value mismatch for field '%%s': original=%%v, remarshaled=%%v\n",
					i+1, key, origVal, remarshaledVal)
				mismatch = true
			}
		}

		// Check for extra fields in remarshaled
		for key := range remarshaledMap {
			if _, exists := original[key]; !exists {
				fmt.Printf("Record %%d: Extra field '%%s' in remarshaled data\n", i+1, key)
				mismatch = true
			}
		}

		if !mismatch {
			successCount++
		}
	}

	if successCount > 0 {
		fmt.Printf("Successfully round-tripped %%d/%%d single objects\n", successCount, len(allInputs))
	}
}

func validateRecord(rawData map[string]any, recordNum int, stats *ValidationStats) {
	// Parse into generated struct
	var generated %s
	rawBytes, _ := json.Marshal(rawData)

	if err := json.Unmarshal(rawBytes, &generated); err != nil {
		log.Printf("Record %%d: Failed to unmarshal into generated struct: %%v", recordNum, err)
		stats.ParseErrors++
		return
	}

	stats.SuccessfulParse++

	// Analyze each field
	generatedValue := reflect.ValueOf(generated)
	generatedType := reflect.TypeOf(generated)

	for i := 0; i < generatedValue.NumField(); i++ {
		field := generatedType.Field(i)
		fieldValue := generatedValue.Field(i)

		// Get JSON tag name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			jsonTag = field.Name
		}
		// Remove ,omitempty suffix
		if comma := strings.Index(jsonTag, ","); comma != -1 {
			jsonTag = jsonTag[:comma]
		}

		// Initialize field stats if not exists
		if _, exists := stats.FieldStats[field.Name]; !exists {
			stats.FieldStats[field.Name] = FieldValidation{}
		}
		fieldStat := stats.FieldStats[field.Name]

		// Check if field exists in original data
		originalValue, exists := rawData[jsonTag]
		if exists {
			fieldStat.ActualCount++

			// Check for type compatibility
			if err := validateFieldType(field.Name, fieldValue, originalValue, recordNum, stats); err != nil {
				fieldStat.TypeErrors = append(fieldStat.TypeErrors, err.Error())
			}
		}

		// Check for nil values in pointer fields
		if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
			fieldStat.NilCount++
		}

		stats.FieldStats[field.Name] = fieldStat
	}
}

// compareJSONValues compares two JSON values for equality
func compareJSONValues(a, b any) bool {
	// Handle nil
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare types
	switch aVal := a.(type) {
	case float64:
		if bVal, ok := b.(float64); ok {
			return aVal == bVal
		}
	case string:
		if bVal, ok := b.(string); ok {
			return aVal == bVal
		}
	case bool:
		if bVal, ok := b.(bool); ok {
			return aVal == bVal
		}
	case map[string]any:
		if bVal, ok := b.(map[string]any); ok {
			if len(aVal) != len(bVal) {
				return false
			}
			for key, aSubVal := range aVal {
				if bSubVal, exists := bVal[key]; !exists || !compareJSONValues(aSubVal, bSubVal) {
					return false
				}
			}
			return true
		}
	case []any:
		if bVal, ok := b.([]any); ok {
			if len(aVal) != len(bVal) {
				return false
			}
			for i := range aVal {
				if !compareJSONValues(aVal[i], bVal[i]) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func validateFieldType(fieldName string, structField reflect.Value, originalValue any, recordNum int, stats *ValidationStats) error {
	if originalValue == nil {
		// Nil values should work with pointer types
		if structField.Kind() != reflect.Ptr {
			mismatch := TypeMismatch{
				Record:      recordNum,
				Field:       fieldName,
				Expected:    "pointer type (for nil)",
				Actual:      structField.Type().String(),
				OriginalVal: originalValue,
			}
			stats.TypeMismatches = append(stats.TypeMismatches, mismatch)
			return fmt.Errorf("nil value but field is not pointer")
		}
		return nil
	}

	// Check basic type compatibility
	originalType := reflect.TypeOf(originalValue)
	expectedType := structField.Type()

	// Handle pointer types
	if expectedType.Kind() == reflect.Ptr {
		expectedType = expectedType.Elem()
	}

	// Basic compatibility check (simplified)
	compatible := false
	switch originalType.Kind() {
	case reflect.Float64:
		compatible = expectedType.Kind() == reflect.Float64
	case reflect.String:
		compatible = expectedType.Kind() == reflect.String
	case reflect.Bool:
		compatible = expectedType.Kind() == reflect.Bool
	case reflect.Map, reflect.Slice:
		compatible = true // Complex types need deeper validation
	}

	if !compatible && expectedType != reflect.TypeOf((*any)(nil)).Elem() {
		mismatch := TypeMismatch{
			Record:      recordNum,
			Field:       fieldName,
			Expected:    expectedType.String(),
			Actual:      originalType.String(),
			OriginalVal: originalValue,
		}
		stats.TypeMismatches = append(stats.TypeMismatches, mismatch)
		return fmt.Errorf("type mismatch: expected %%s, got %%s", expectedType, originalType)
	}

	return nil
}
`, structDefClean, typeName, typeName)
}