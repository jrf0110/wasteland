//go:build !js

// Package main is the libwl wasm entry point. The functional code is in
// the `js` build (see main.go); this stub exists so `go build ./...` and
// `go test ./...` on host machines do not fail with "function main is
// undeclared in the main package".
package main

func main() {
	// No-op on host. Real entry point is main.go (//go:build js).
}
