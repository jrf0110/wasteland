//go:build js

package main

import (
	"encoding/json"
	"syscall/js"
)

type jsResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func register() {
	js.Global().Set("wlBrowse", bridge(Browse))
	js.Global().Set("wlClaim", bridge(Claim))
	js.Global().Set("wlUnclaim", bridge(Unclaim))
	js.Global().Set("wlDone", bridge(Done))
	js.Global().Set("wlPost", bridge(Post))
	js.Global().Set("wlAccept", bridge(Accept))
	js.Global().Set("wlReject", bridge(Reject))
	js.Global().Set("wlClose", bridge(Close))
}

// bridge wraps a Go function in a JS callable that returns a Promise.
//
// The blocking Go work runs on its own goroutine (per syscall/js docs:
// "if one wrapped function blocks, JavaScript's event loop is blocked
// until that function returns. ... Therefore a blocking function should
// explicitly start a new goroutine."). The wrapper returns immediately
// with a JS Promise that the goroutine resolves once fn completes.
func bridge[T any, R any](fn func(T) (R, error)) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) != 1 || args[0].Type() != js.TypeString {
			return rejectedPromise("expected one JSON string argument")
		}
		input := args[0].String()
		return js.Global().Get("Promise").New(js.FuncOf(func(_ js.Value, executor []js.Value) interface{} {
			resolve := executor[0]
			reject := executor[1]
			go runOnGoroutine(fn, input, resolve, reject)
			return nil
		}))
	})
}

func runOnGoroutine[T any, R any](fn func(T) (R, error), raw string, resolve, reject js.Value) {
	var input T
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		reject.Invoke(encodeJS(jsResponse{OK: false, Error: err.Error()}))
		return
	}
	result, err := fn(input)
	if err != nil {
		reject.Invoke(encodeJS(jsResponse{OK: false, Error: err.Error()}))
		return
	}
	resolve.Invoke(encodeJS(jsResponse{OK: true, Data: result}))
}

func rejectedPromise(message string) js.Value {
	return js.Global().Get("Promise").Call("reject", encodeJS(jsResponse{OK: false, Error: message}))
}

func encodeJS(resp jsResponse) js.Value {
	b, err := json.Marshal(resp)
	if err != nil {
		b, _ = json.Marshal(jsResponse{OK: false, Error: err.Error()})
	}
	return js.ValueOf(string(b))
}
