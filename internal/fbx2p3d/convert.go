package fbx2p3d

import (
	"fmt"

	"github.com/jmhobbs/odol/internal/mlodbuild"
	"github.com/jmhobbs/odol/p3d/mlod"
)

// ConvertRequest is everything the browser page collects for one build.
type ConvertRequest struct {
	LODs []LODSource
	// Texture and Material are written to every graphical LOD, like the
	// CLI's --texture/--material. Either may be empty: a P3D face can have
	// no texture or material.
	Texture  string
	Material string
	// FireMaterial is a vanilla penetration material name or full path (see
	// mlodbuild.PenetrationMaterialPath), or empty for none.
	FireMaterial string
	Mass         float32
	Scale        float32
}

// ConvertResult is one build's output: the P3D bytes and the preview data.
type ConvertResult struct {
	P3D     []byte
	Preview ModelPreview
	// Warnings holds Hooks.Warnf messages, already formatted.
	Warnings []string
}

// Convert validates req, builds the model, and marshals it to MLOD bytes.
// It is the single entry point cmd/wasm calls.
func Convert(req ConvertRequest) (*ConvertResult, error) {
	if err := ValidateScale(float64(req.Scale)); err != nil {
		return nil, err
	}
	opts := mlodbuild.Options{Texture: req.Texture, Material: req.Material, Mass: req.Mass}
	if req.FireMaterial != "" {
		path, err := mlodbuild.PenetrationMaterialPath(req.FireMaterial)
		if err != nil {
			return nil, err
		}
		opts.FireGeometryMaterial = path
	}

	result := &ConvertResult{}
	hooks := Hooks{Warnf: func(format string, args ...any) {
		result.Warnings = append(result.Warnings, fmt.Sprintf(format, args...))
	}}

	m, err := Build(req.LODs, opts, req.Scale, hooks)
	if err != nil {
		return nil, err
	}
	p3d, err := mlod.Marshal(m)
	if err != nil {
		return nil, err
	}

	result.P3D = p3d
	result.Preview = Preview(m)
	return result, nil
}
