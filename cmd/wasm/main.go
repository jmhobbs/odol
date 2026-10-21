//go:build js && wasm

// Command wasm exposes odol's P3D -> MLOD/FBX conversion and the fbx2p3d
// FBX -> MLOD build to JavaScript via syscall/js, for use from a browser
// (see web/). It is a thin bridge: all conversion rules live in
// internal/convert and internal/fbx2p3d; this file only marshals js.Value
// <-> Go types and packages results for JS to consume.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"path"
	"strings"
	"syscall/js"

	"github.com/jmhobbs/odol/internal/convert"
	"github.com/jmhobbs/odol/internal/fbx2p3d"
	"github.com/jmhobbs/odol/internal/mlodbuild"
	"github.com/jmhobbs/odol/p3d/detector"
)

func main() {
	js.Global().Set("odolConvertToMLOD", js.FuncOf(convertToMLOD))
	js.Global().Set("odolConvertToFBX", js.FuncOf(convertToFBX))
	js.Global().Set("odolDetectFormat", js.FuncOf(detectFormat))
	js.Global().Set("odolPenetrationMaterials", js.FuncOf(penetrationMaterials))
	js.Global().Set("odolFBXToP3D", js.FuncOf(fbxToP3D))
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

// detectFormat(data Uint8Array) -> {ok, family, error}
// family is one of detector.P3DFamily's values ("ODOL", "MLOD", "DEMO").
// Callers only need the file's first ~16 bytes to get an answer - the
// caller decides how much of the file to send.
func detectFormat(_ js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = errorResult(fmt.Sprintf("panic: %v", r))
		}
	}()

	if len(args) < 1 {
		return errorResult("odolDetectFormat: expected 1 argument (data)")
	}
	format, err := detector.Detect(bytes.NewReader(bytesToGo(args[0])))
	if err != nil {
		return errorResult(err.Error())
	}

	return js.ValueOf(map[string]any{
		"ok":     true,
		"family": string(format.Family),
	})
}

// penetrationMaterials() -> string[]
// Every vanilla Fire Geometry material name odolFBXToP3D accepts as
// fireMaterial, sorted.
func penetrationMaterials(_ js.Value, _ []js.Value) any {
	return stringsToJS(mlodbuild.PenetrationMaterialNames())
}

// fbxToP3D(request) -> {ok, data: Uint8Array, preview, warnings, error}
//
// request is {lods: [{resolution, name, data: Uint8Array}], texture,
// material, fireMaterial, mass, scale}. preview mirrors fbx2p3d.ModelPreview, with each
// LOD's mesh as positions: Float32Array and triangles: Uint32Array.
func fbxToP3D(_ js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = errorResult(fmt.Sprintf("panic: %v", r))
		}
	}()

	if len(args) < 1 {
		return errorResult("odolFBXToP3D: expected 1 argument (request)")
	}
	request := args[0]

	jsLODs := request.Get("lods")
	sources := make([]fbx2p3d.LODSource, jsLODs.Length())
	for i := range sources {
		lod := jsLODs.Index(i)
		data := bytesToGo(lod.Get("data"))
		sources[i] = fbx2p3d.LODSource{
			Resolution: float32(lod.Get("resolution").Float()),
			Name:       optionalString(lod.Get("name")),
			Load:       func() ([]byte, error) { return data, nil },
		}
	}

	converted, err := fbx2p3d.Convert(fbx2p3d.ConvertRequest{
		LODs:         sources,
		Texture:      optionalString(request.Get("texture")),
		Material:     optionalString(request.Get("material")),
		FireMaterial: optionalString(request.Get("fireMaterial")),
		Mass:         float32(request.Get("mass").Float()),
		Scale:        float32(request.Get("scale").Float()),
	})
	if err != nil {
		return errorResult(err.Error())
	}

	return js.ValueOf(map[string]any{
		"ok":       true,
		"data":     bytesToJS(converted.P3D),
		"preview":  previewToJS(converted.Preview),
		"warnings": stringsToJS(converted.Warnings),
	})
}

func previewToJS(preview fbx2p3d.ModelPreview) map[string]any {
	lods := make([]any, len(preview.LODs))
	for i, lod := range preview.LODs {
		lods[i] = map[string]any{
			"name":          lod.Name,
			"resolution":    float64(lod.Resolution),
			"graphical":     lod.Graphical,
			"vertexCount":   lod.VertexCount,
			"faceCount":     lod.FaceCount,
			"triangleCount": lod.TriangleCount,
			"selections":    stringsToJS(lod.Selections),
			"positions":     float32sToJS(lod.Mesh.Positions),
			"triangles":     uint32sToJS(lod.Mesh.Triangles),
		}
	}
	return map[string]any{
		"lods":               lods,
		"highestDetailIndex": preview.HighestDetailIndex,
		"boundingSize": map[string]any{
			"x": float64(preview.BoundingSize.X),
			"y": float64(preview.BoundingSize.Y),
			"z": float64(preview.BoundingSize.Z),
		},
		"heightRatio":                preview.HeightRatio,
		"referenceHumanHeightMeters": fbx2p3d.ReferenceHumanHeightMeters,
	}
}

// optionalString returns v as a Go string, or "" when v is not a JS string.
// js.Value.String turns undefined into "<undefined>", which would otherwise
// land in the P3D as a texture path.
func optionalString(v js.Value) string {
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

func stringsToJS(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func bytesToGo(v js.Value) []byte {
	data := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(data, v)
	return data
}

func bytesToJS(data []byte) js.Value {
	jsData := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsData, data)
	return jsData
}

// float32sToJS and uint32sToJS copy through a Uint8Array, the only typed
// array syscall/js copies into, and view its buffer as the wider type. Both
// sides are little-endian (wasm always is, and so is every browser target).
func float32sToJS(values []float32) js.Value {
	data := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	return js.Global().Get("Float32Array").New(bytesToJS(data).Get("buffer"))
}

func uint32sToJS(values []uint32) js.Value {
	data := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], v)
	}
	return js.Global().Get("Uint32Array").New(bytesToJS(data).Get("buffer"))
}

// readNameAndBytes parses the (name string, data Uint8Array) argument pair
// shared by both exported functions, deriving a path/extension-free base
// name for use in output filenames.
func readNameAndBytes(args []js.Value) (name string, data []byte, err error) {
	if len(args) < 2 {
		return "", nil, fmt.Errorf("expected at least 2 arguments (name, data)")
	}
	name = baseName(args[0].String())
	return name, bytesToGo(args[1]), nil
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
	return js.ValueOf(map[string]any{
		"ok":       true,
		"filename": filename,
		"data":     bytesToJS(data),
	})
}

func errorResult(message string) js.Value {
	return js.ValueOf(map[string]any{
		"ok":    false,
		"error": message,
	})
}
