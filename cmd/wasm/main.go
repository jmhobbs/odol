//go:build js && wasm

// Command wasm exposes odol's P3D -> MLOD/FBX conversion to JavaScript via
// syscall/js, for use from a browser (see web/). It is a thin bridge: all
// conversion rules live in internal/convert; this file only marshals
// js.Value <-> Go types and packages results for JS to consume.
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path"
	"strings"
	"syscall/js"

	"github.com/jmhobbs/odol/internal/convert"
)

func main() {
	js.Global().Set("odolConvertToMLOD", js.FuncOf(convertToMLOD))
	js.Global().Set("odolConvertToFBX", js.FuncOf(convertToFBX))
	select {}
}

// convertToMLOD(name string, data Uint8Array) -> {ok, filename, data: Uint8Array, error}
// The MLOD P3D and rendered model.cfg are bundled into a single zip, since
// triggering two separate browser downloads from one click is unreliable.
func convertToMLOD(_ js.Value, args []js.Value) any {
	return withRecover(func() (string, []byte, error) {
		name, data, err := readNameAndBytes(args)
		if err != nil {
			return "", nil, err
		}

		p3d, modelCfg, err := convert.ConvertToMLOD(data, name, nil)
		if err != nil {
			return "", nil, err
		}

		zipped, err := zipMLODOutput(name, p3d, modelCfg)
		if err != nil {
			return "", nil, err
		}
		return name + "_mlod.zip", zipped, nil
	})
}

// convertToFBX(name string, data Uint8Array, ascii bool) -> {ok, filename, data: Uint8Array, error}
func convertToFBX(_ js.Value, args []js.Value) any {
	return withRecover(func() (string, []byte, error) {
		name, data, err := readNameAndBytes(args)
		if err != nil {
			return "", nil, err
		}
		if len(args) < 3 {
			return "", nil, fmt.Errorf("odolConvertToFBX: expected 3 arguments (name, data, ascii)")
		}
		ascii := args[2].Bool()

		fbx, err := convert.ConvertToFBX(data, name, ascii, nil)
		if err != nil {
			return "", nil, err
		}
		return name + ".fbx", fbx, nil
	})
}

// readNameAndBytes parses the (name string, data Uint8Array) argument pair
// shared by both exported functions, deriving a path/extension-free base
// name for use in output filenames.
func readNameAndBytes(args []js.Value) (name string, data []byte, err error) {
	if len(args) < 2 {
		return "", nil, fmt.Errorf("expected at least 2 arguments (name, data)")
	}
	name = baseName(args[0].String())
	data = make([]byte, args[1].Get("length").Int())
	js.CopyBytesToGo(data, args[1])
	return name, data, nil
}

// baseName strips any path and extension from a browser-supplied filename.
func baseName(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}

func zipMLODOutput(name string, p3d []byte, modelCfg string) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	p3dWriter, err := w.Create(name + "_mlod.p3d")
	if err != nil {
		return nil, err
	}
	if _, err := p3dWriter.Write(p3d); err != nil {
		return nil, err
	}

	cfgWriter, err := w.Create(name + ".model.cfg")
	if err != nil {
		return nil, err
	}
	if _, err := cfgWriter.Write([]byte(modelCfg)); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// withRecover runs fn and converts both returned errors and panics (e.g.
// from a malformed/truncated input file deep in a parser) into a JS result
// object, so a bad file never kills the WASM instance.
func withRecover(fn func() (filename string, data []byte, err error)) (result js.Value) {
	defer func() {
		if r := recover(); r != nil {
			result = errorResult(fmt.Sprintf("panic: %v", r))
		}
	}()

	filename, data, err := fn()
	if err != nil {
		return errorResult(err.Error())
	}
	return successResult(filename, data)
}

func successResult(filename string, data []byte) js.Value {
	jsData := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsData, data)
	return js.ValueOf(map[string]any{
		"ok":       true,
		"filename": filename,
		"data":     jsData,
	})
}

func errorResult(message string) js.Value {
	return js.ValueOf(map[string]any{
		"ok":    false,
		"error": message,
	})
}
