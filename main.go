//go:build !js
// +build !js

package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	flagName      = flag.String("name", "Foo", "the name of the struct")
	flagPkg       = flag.String("pkg", "main", "the name of the package for the generated code")
	flagOmitEmpty = flag.Bool("omitempty", true, "if true, emits struct field tags with 'omitempty'")
	flagTemplate  = flag.String("template", "", "path to txtar template file")
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
		OmitEmpty:   *flagOmitEmpty,
		Template:    *flagTemplate,
		TypeName:    *flagName,
		PackageName: *flagPkg,
	}
	if err := g.loadTemplates(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: failed to load templates, using defaults:", err)
	}
	return g.generate(os.Stdout, os.Stdin)
}

func isInteractive() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fileInfo.Mode()&(os.ModeCharDevice|os.ModeCharDevice) != 0
}
