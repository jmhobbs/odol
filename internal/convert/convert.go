// Package convert holds the byte-oriented P3D -> MLOD/FBX conversion rules
// shared by the CLI (cmd/convert) and the browser/WASM entry point
// (cmd/wasm), so both have exactly one place that knows the conversion
// rules (which input formats are accepted, when a partial decode must be
// rejected, and which internal packages to call).
package convert

import (
	"bytes"
	"fmt"

	"github.com/jmhobbs/odol/internal/detector"
	"github.com/jmhobbs/odol/internal/fbxexport"
	"github.com/jmhobbs/odol/internal/mlod"
	"github.com/jmhobbs/odol/internal/model"
	"github.com/jmhobbs/odol/internal/modelcfg"
	"github.com/jmhobbs/odol/internal/odol"
)

// PartialParseError indicates the input ODOL data could not be fully
// decoded, so emitting MLOD/FBX from it would silently lose data.
type PartialParseError struct {
	LODIndex    int
	Resolution  float32
	PropertyKey string
}

func (e PartialParseError) Error() string {
	return fmt.Sprintf(
		"input contains partially decoded LOD %d (resolution %g, marker %q); refusing to emit lossy output",
		e.LODIndex,
		e.Resolution,
		e.PropertyKey,
	)
}

// StepLogger narrates conversion progress. A nil StepLogger performs no
// logging; it exists so callers that want step-by-step narration (the CLI)
// and callers that don't (the WASM bridge) can share the same conversion
// functions.
type StepLogger func(format string, args ...any)

func logStep(log StepLogger, format string, args ...any) {
	if log == nil {
		return
	}
	log(format, args...)
}

// ConvertToMLOD detects, parses, and validates data as an ODOL P3D, and
// returns the MLOD P3D bytes and rendered model.cfg text. name is the
// model name (typically the input filename without extension).
func ConvertToMLOD(data []byte, name string, log StepLogger) (p3d []byte, modelCfg string, err error) {
	format, err := detector.Detect(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	logStep(log, "detected input format: %s", format.String())
	if format.Family != detector.P3DFamilyODOL {
		return nil, "", fmt.Errorf("unsupported input format: %s", format.String())
	}
	logStep(log, "converting %s to MLOD + model.cfg", format.String())

	logStep(log, "parsing ODOL data")
	parsed, err := odol.ParseStrict(data, name)
	if err != nil {
		return nil, "", err
	}
	logStep(log, "validating full decode")
	if err := ensureFullyDecoded(parsed); err != nil {
		return nil, "", err
	}

	logStep(log, "writing output files")
	p3d, err = mlod.Marshal(parsed)
	if err != nil {
		return nil, "", err
	}

	modelCfg, err = modelcfg.Render(parsed.Config)
	if err != nil {
		return nil, "", err
	}

	return p3d, modelCfg, nil
}

// ConvertToFBX detects and parses data as either ODOL or MLOD P3D, and
// returns FBX bytes (binary, or ASCII if ascii is true). name is the model
// name (typically the input filename without extension).
func ConvertToFBX(data []byte, name string, ascii bool, log StepLogger) ([]byte, error) {
	format, err := detector.Detect(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	logStep(log, "detected input format: %s", format.String())

	var parsed *model.Model
	switch format.Family {
	case detector.P3DFamilyODOL:
		logStep(log, "converting %s to FBX", format.String())
		logStep(log, "parsing ODOL data")
		parsed, err = odol.ParseStrict(data, name)
		if err != nil {
			return nil, err
		}
		logStep(log, "validating full decode")
		if err := ensureFullyDecoded(parsed); err != nil {
			return nil, err
		}
	case detector.P3DFamilyMLOD:
		logStep(log, "converting MLOD/P3DM to FBX")
		logStep(log, "parsing MLOD data")
		parsed, err = mlod.Parse(data, name)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported input format for FBX export: %s", format.String())
	}

	logStep(log, "writing FBX")
	if ascii {
		return fbxexport.MarshalASCII(parsed)
	}
	return fbxexport.Marshal(parsed)
}

func ensureFullyDecoded(parsed *model.Model) error {
	for i, lod := range parsed.LODs {
		if _, ok := lod.Properties["odol_partial_parse"]; ok {
			return PartialParseError{
				LODIndex:    i,
				Resolution:  lod.Resolution,
				PropertyKey: "odol_partial_parse",
			}
		}
	}

	return nil
}
