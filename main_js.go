//go:build js
// +build js

package main

import (
	"strings"
	"syscall/js"
)

func jsonToStructFunction(this js.Value, p []js.Value) any {
	in := strings.NewReader(p[0].String())
	if output, err := generate(in, "Type", "main", &generator{}); err != nil {
		return js.ValueOf(err.Error())
	} else {
		return js.ValueOf(string(output))
	}
	return js.Null()
}

func main() {
	c := make(chan struct{}, 0)

	js.Global().Set("jsonToStruct", js.FuncOf(jsonToStructFunction))

	<-c
}
