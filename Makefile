docs/json-to-struct.wasm: *.go
	@cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" docs/
	@GOOS=js GOARCH=wasm go build -o $@
