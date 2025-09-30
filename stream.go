package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// ANSI escape codes
const (
	clearScreen = "\033[2J"
	moveCursor  = "\033[H"
)

// generateStream processes JSON input line by line with progressive display
func (g *generator) generateStream(output io.Writer, input io.Reader) error {
	stats := NewStructStats()
	g.stats = stats

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max line

	lineNum := 0
	var lastOutput string
	lastUpdateTime := time.Now()
	updateInterval := time.Duration(g.UpdateInterval) * time.Millisecond
	if updateInterval <= 0 {
		updateInterval = 500 * time.Millisecond // Default
	}
	const updateBatchSize = 10 // Or every 10 objects

	// Check if input looks like a JSON array
	var buffer bytes.Buffer
	teeReader := io.TeeReader(input, &buffer)
	firstByte := make([]byte, 1)
	_, err := teeReader.Read(firstByte)
	if err != nil && err != io.EOF {
		return err
	}

	// If it starts with '[', we need to handle it as an array
	if len(firstByte) > 0 && firstByte[0] == '[' {
		// Read entire array and process
		allBytes, err := io.ReadAll(teeReader)
		if err != nil {
			return err
		}
		fullInput := append(firstByte, allBytes...)
		return g.generateStreamFromArray(output, fullInput)
	}

	// Otherwise process line by line (JSONL format)
	combined := io.MultiReader(bytes.NewReader(firstByte), &buffer, input)
	scanner = bufio.NewScanner(combined)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		lineNum++

		// Try to parse as JSON object
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			// Skip non-JSON lines
			continue
		}

		// Process this object
		stats.ProcessJSON(obj, g)

		// Only update display periodically - use logarithmic scale for large datasets
		timeSinceUpdate := time.Since(lastUpdateTime)

		// Adaptive batch size: grows logarithmically with data size
		adaptiveBatchSize := updateBatchSize
		if lineNum > 1000 {
			adaptiveBatchSize = 100
		}
		if lineNum > 10000 {
			adaptiveBatchSize = 1000
		}
		if lineNum > 100000 {
			adaptiveBatchSize = 10000
		}

		shouldUpdate := timeSinceUpdate >= updateInterval ||
			lineNum%adaptiveBatchSize == 0 ||
			lineNum <= 5 || // Always show first few updates for responsiveness
			lineNum == 10 || lineNum == 100 || lineNum == 1000 || lineNum == 10000 || lineNum == 100000 || lineNum == 1000000 // Milestones

		if shouldUpdate {
			// Generate current struct
			typ := g.buildTypeFromStats(stats)
			src := g.renderFile(typ.String())

			// Format the code
			formatted, err := format.Source([]byte(src))
			if err != nil {
				// If formatting fails, use unformatted
				formatted = []byte(src)
			}

			// Clear screen and display
			currentOutput := string(formatted)
			if currentOutput != lastOutput {
				g.displayStreamOutput(output, currentOutput, lineNum, stats.TotalLines)
				lastOutput = currentOutput
				lastUpdateTime = time.Now()
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	if stats.TotalLines == 0 {
		return fmt.Errorf("no valid JSON objects found")
	}

	// Final output without clearing
	typ := g.buildTypeFromStats(stats)
	src := g.renderFile(typ.String())
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return fmt.Errorf("error formatting generated code: %w", err)
	}

	// Clear one more time and show final result
	g.displayStreamOutput(output, string(formatted), stats.TotalLines, stats.TotalLines)

	return nil
}

// generateStreamFromArray processes a JSON array with progressive display
func (g *generator) generateStreamFromArray(output io.Writer, input []byte) error {
	stats := NewStructStats()
	g.stats = stats

	// Parse as array
	var array []any
	if err := json.Unmarshal(input, &array); err != nil {
		return fmt.Errorf("error parsing JSON array: %w", err)
	}

	var lastOutput string
	lastUpdateTime := time.Now()
	updateInterval := time.Duration(g.UpdateInterval) * time.Millisecond
	if updateInterval <= 0 {
		updateInterval = 500 * time.Millisecond // Default
	}
	const updateBatchSize = 10

	for i, item := range array {
		if obj, ok := item.(map[string]any); ok {
			stats.ProcessJSON(obj, g)

			// Only update display periodically - use logarithmic scale for large datasets
			timeSinceUpdate := time.Since(lastUpdateTime)

			// Adaptive batch size for large arrays
			adaptiveBatchSize := updateBatchSize
			if i > 1000 {
				adaptiveBatchSize = 100
			}
			if i > 10000 {
				adaptiveBatchSize = 1000
			}
			if i > 100000 {
				adaptiveBatchSize = 10000
			}

			shouldUpdate := timeSinceUpdate >= updateInterval ||
				(i+1)%adaptiveBatchSize == 0 ||
				i < 5 || // Show first few
				i == 9 || i == 99 || i == 999 || i == 9999 || i == 99999 || i == 999999 || // Milestones
				i == len(array)-1 // Always show final

			if shouldUpdate {
				// Generate current struct
				typ := g.buildTypeFromStats(stats)
				src := g.renderFile(typ.String())

				// Format the code
				formatted, err := format.Source([]byte(src))
				if err != nil {
					formatted = []byte(src)
				}

				// Display progressive output
				currentOutput := string(formatted)
				if currentOutput != lastOutput {
					g.displayStreamOutput(output, currentOutput, i+1, len(array))
					lastOutput = currentOutput
					lastUpdateTime = time.Now()
				}
			}
		}
	}

	if stats.TotalLines == 0 {
		return fmt.Errorf("no valid JSON objects found in array")
	}

	// Final output
	typ := g.buildTypeFromStats(stats)
	src := g.renderFile(typ.String())
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return fmt.Errorf("error formatting generated code: %w", err)
	}

	g.displayStreamOutput(output, string(formatted), len(array), len(array))

	return nil
}

// displayStreamOutput clears the terminal and displays the current output
func (g *generator) displayStreamOutput(w io.Writer, content string, current, total int) {
	// Check if output is a terminal
	if file, ok := w.(*os.File); ok && isTerminal(file) {
		// For final output, show everything
		if current == total {
			// Clear screen and show full final result
			fmt.Fprint(w, clearScreen+moveCursor)
			fmt.Fprint(w, content)
			fmt.Fprintf(w, "\n\n✅ Complete! Processed %d objects\n", total)
			return
		}

		// Get terminal height for progressive display
		rows := getTerminalRows()

		// Clear screen and move cursor to top
		fmt.Fprint(w, clearScreen+moveCursor)

		// Show progress header (2 lines)
		fmt.Fprintf(w, "=== Processing JSON objects: %d/%d ===\n\n", current, total)

		// Calculate available lines (rows - header(2) - footer(3) - safety margin(2))
		availableLines := rows - 7
		if availableLines < 10 {
			availableLines = 10 // Minimum to show something useful
		}

		// Split content into lines and truncate if needed
		lines := strings.Split(content, "\n")
		if len(lines) > availableLines {
			// Write truncated content
			for i := 0; i < availableLines-1; i++ {
				fmt.Fprintln(w, lines[i])
			}
			fmt.Fprintf(w, "... (%d more lines)", len(lines)-availableLines+1)
		} else {
			// Write full content
			fmt.Fprint(w, content)
		}

		// Add footer
		fmt.Fprintf(w, "\n\n⏳ Processing... (%d/%d)", current, total)
	} else {
		// Non-terminal output: just write content
		fmt.Fprint(w, content)
	}
}

// getTerminalRows returns the terminal height using term.GetSize
func getTerminalRows() int {
	_, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || rows <= 0 {
		// Default to 24 rows if not a terminal or error
		return 24
	}
	return rows
}

// isTerminal checks if a file descriptor is a terminal
func isTerminal(f *os.File) bool {
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return fileInfo.Mode()&os.ModeCharDevice != 0
}
