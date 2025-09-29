//go:build !js
// +build !js

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
)

var (
	flagName         = flag.String("name", "Foo", "the name of the struct")
	flagPkg          = flag.String("pkg", "main", "the name of the package for the generated code")
	flagOmitEmpty    = flag.Bool("omitempty", true, "if true, emits struct field tags with 'omitempty'")
	flagTemplate     = flag.String("template", "", "path to txtar template file")
	flagRoundtrip    = flag.Bool("roundtrip", false, "if true, generates and runs a round-trip validation test")
	flagStatComments = flag.Bool("stat-comments", false, "if true, adds field statistics as comments")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "json-to-struct error:", err)
		os.Exit(1)
	}
}

func run() error {
	if isInteractive() {
		flag.Usage()
		return fmt.Errorf("no input on stdin")
	}

	g := &generator{
		OmitEmpty:    *flagOmitEmpty,
		Template:     *flagTemplate,
		TypeName:     *flagName,
		PackageName:  *flagPkg,
		StatComments: *flagStatComments,
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
