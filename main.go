//go:build !js
// +build !js

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"strings"
)

var (
	flagName           = flag.String("name", "Foo", "the name of the struct")
	flagPkg            = flag.String("pkg", "main", "the name of the package for the generated code")
	flagOmitEmpty      = flag.Bool("omitempty", true, "if true, emits struct field tags with 'omitempty'")
	flagTemplate       = flag.String("template", "", "path to txtar template file")
	flagRoundtrip      = flag.Bool("roundtrip", false, "if true, generates and runs a round-trip validation test")
	flagStatComments   = flag.Bool("stat-comments", false, "if true, adds field statistics as comments")
	flagStream         = flag.Bool("stream", false, "if true, shows progressive output with terminal clearing")
	flagExtractStructs = flag.Bool("extract-structs", false, "if true, extracts repeated nested structs to reduce duplication")
	flagUpdateInterval = flag.Int("update-interval", 500, "milliseconds between stream mode updates")
	flagPprofAddr      = flag.String("pprof", "", "pprof server address (e.g., :6060)")
	flagCpuProfile     = flag.String("cpuprofile", "", "write CPU profile to file")
	flagFieldOrder     = flag.String("field-order", "alphabetical", "field ordering: alphabetical, encounter, common-first, or rare-first")
)

func main() {
	flag.Parse()

	// Start pprof server if requested
	if *flagPprofAddr != "" {
		go func() {
			log.Printf("Starting pprof server on %s", *flagPprofAddr)
			log.Printf("CPU profile: http://%s/debug/pprof/profile?seconds=30", *flagPprofAddr)
			log.Printf("Heap profile: http://%s/debug/pprof/heap", *flagPprofAddr)
			if err := http.ListenAndServe(*flagPprofAddr, nil); err != nil {
				log.Printf("pprof server failed: %v", err)
			}
		}()
	}

	// Start CPU profiling if requested
	if *flagCpuProfile != "" {
		f, err := os.Create(*flagCpuProfile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	if err := run(); err != nil {
		// Check if it's a FormatError
		if fmtErr, ok := err.(*FormatError); ok {
			displayFormatError(fmtErr)
		} else {
			fmt.Fprintln(os.Stderr, "json-to-struct error:", err)
		}
		os.Exit(1)
	}
}

func displayFormatError(e *FormatError) {
	lines := strings.Split(e.Source, "\n")

	fmt.Fprintf(os.Stderr, "\n🔴 Syntax error in generated Go code:\n")
	fmt.Fprintf(os.Stderr, "   %s\n\n", e.OriginalError)

	if e.LineNum > 0 && e.LineNum <= len(lines) {
		// Show context around the error
		start := e.LineNum - 5
		if start < 1 {
			start = 1
		}
		end := e.LineNum + 5
		if end > len(lines) {
			end = len(lines)
		}

		fmt.Fprintf(os.Stderr, "Code around line %d:\n", e.LineNum)
		fmt.Fprintf(os.Stderr, "─────────────────────────────────────────────\n")

		for i := start; i <= end; i++ {
			marker := "  "
			if i == e.LineNum {
				marker = "→ "
				fmt.Fprintf(os.Stderr, "\033[31m%s%3d: %s\033[0m\n", marker, i, lines[i-1])
			} else {
				fmt.Fprintf(os.Stderr, "%s%3d: %s\n", marker, i, lines[i-1])
			}
		}
		fmt.Fprintf(os.Stderr, "─────────────────────────────────────────────\n\n")
	} else {
		// Show first 50 lines if we can't pinpoint the error
		fmt.Fprintf(os.Stderr, "First 50 lines of problematic code:\n")
		fmt.Fprintf(os.Stderr, "─────────────────────────────────────────────\n")
		for i, line := range lines {
			if i >= 50 {
				fmt.Fprintf(os.Stderr, "... (%d more lines)\n", len(lines)-50)
				break
			}
			fmt.Fprintf(os.Stderr, "%3d: %s\n", i+1, line)
		}
		fmt.Fprintf(os.Stderr, "─────────────────────────────────────────────\n")
	}
}

func run() error {
	if isInteractive() {
		flag.Usage()
		return fmt.Errorf("no input on stdin")
	}

	g := &generator{
		OmitEmpty:      *flagOmitEmpty,
		Template:       *flagTemplate,
		TypeName:       *flagName,
		PackageName:    *flagPkg,
		StatComments:   *flagStatComments,
		Stream:         *flagStream,
		ExtractStructs: *flagExtractStructs,
		UpdateInterval: *flagUpdateInterval,
		FieldOrder:     *flagFieldOrder,
	}
	if err := g.loadTemplates(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: failed to load templates, using defaults:", err)
	}

	// If we need roundtrip, capture input with TeeReader
	var input io.Reader = os.Stdin
	var capturedInput bytes.Buffer

	if *flagRoundtrip {
		// Use TeeReader to capture input for both generation and validation
		input = io.TeeReader(os.Stdin, &capturedInput)
	}

	// Generate the struct (output to stdout)
	if err := g.generate(os.Stdout, input); err != nil {
		return err
	}

	// Run roundtrip validation if requested
	if *flagRoundtrip {
		return runRoundtripTestWithData(g, capturedInput.Bytes())
	}

	return nil
}

func isInteractive() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fileInfo.Mode()&(os.ModeCharDevice|os.ModeCharDevice) != 0
}
