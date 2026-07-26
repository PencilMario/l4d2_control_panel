//go:build js && wasm

package main

import (
	"github.com/not0721here/l4d2-control-panel/internal/vpkcleaner"
	"syscall/js"
)

func main() { js.Global().Set("cleanVPKBytes", js.FuncOf(cleanVPKBytes)); select {} }
func cleanVPKBytes(_ js.Value, args []js.Value) any {
	resultObject := js.Global().Get("Object").New()
	if len(args) != 1 {
		resultObject.Set("error", "one Uint8Array is required")
		return resultObject
	}
	input := make([]byte, args[0].Get("byteLength").Int())
	js.CopyBytesToGo(input, args[0])
	result, err := vpkcleaner.CleanBytes(input)
	if err != nil {
		resultObject.Set("error", err.Error())
		return resultObject
	}
	output := js.Global().Get("Uint8Array").New(len(result.Data))
	js.CopyBytesToJS(output, result.Data)
	resultObject.Set("data", output)
	resultObject.Set("removed", result.Removed)
	return resultObject
}
