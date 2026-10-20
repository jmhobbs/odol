package fbximport

import (
	"fmt"

	"github.com/jmhobbs/odol/internal/model"
)

// NormalConvention says how an FBX file's normals relate to its geometry,
// and so how they convert to MLOD normals. MLOD stores normals inverted:
// along the right-hand normal of its clockwise face order (see
// docs/P3D File Format - MLOD.txt). Every sample P3D measures this way.
type NormalConvention int

const (
	// NormalConventionAuto picks DayZ or Standard per Geometry node, using
	// whichever puts more normals along the MLOD convention for that
	// geometry's own faces. A tie picks DayZ.
	NormalConventionAuto NormalConvention = iota
	// NormalConventionDayZ is what DayZ tools and internal/fbxexport write:
	// the MLOD normal with Z mirrored (see the exporter's
	// TestBuildSceneNormalZIsNegated). Converting mirrors Z back.
	NormalConventionDayZ
	// NormalConventionStandard is plain FBX, as Blender writes it: normals
	// agree with the counter-clockwise winding. Converting negates all three
	// components.
	NormalConventionStandard
)

var normalConventionNames = []string{
	NormalConventionAuto:     "auto",
	NormalConventionDayZ:     "dayz",
	NormalConventionStandard: "standard",
}

func (c NormalConvention) String() string {
	if c < 0 || int(c) >= len(normalConventionNames) {
		return fmt.Sprintf("NormalConvention(%d)", int(c))
	}
	return normalConventionNames[c]
}

// Set implements flag.Value.
func (c *NormalConvention) Set(name string) error {
	parsed, err := ParseNormalConvention(name)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

// ParseNormalConvention returns the convention named name: "auto", "dayz"
// or "standard".
func ParseNormalConvention(name string) (NormalConvention, error) {
	for convention, conventionName := range normalConventionNames {
		if conventionName == name {
			return NormalConvention(convention), nil
		}
	}
	return 0, fmt.Errorf("unknown normal convention %q (want one of: auto, dayz, standard)", name)
}

// toMLOD converts one raw FBX normal under convention c, which must not be
// NormalConventionAuto.
func (c NormalConvention) toMLOD(raw model.Vector3) model.Vector3 {
	if c == NormalConventionStandard {
		return model.Vector3{X: -raw.X, Y: -raw.Y, Z: -raw.Z}
	}
	return model.Vector3{X: raw.X, Y: raw.Y, Z: -raw.Z}
}

// detectNormalConvention counts, for each convention, the face corners whose
// converted normal points along the Newell normal of the face in MLOD
// (clockwise) order, and returns the convention with more. faces must
// already be in MLOD order and index rawNormals through NormalIndices.
func detectNormalConvention(vertices []model.Vector3, faces []model.Face, rawNormals []model.Vector3) NormalConvention {
	dayzAgree, standardAgree := 0, 0
	for _, face := range faces {
		corners := make([]model.Vector3, len(face.Indices))
		for i, pointIdx := range face.Indices {
			corners[i] = vertices[pointIdx]
		}
		faceNormal := newellNormal(corners)
		for _, normalIdx := range face.NormalIndices {
			raw := rawNormals[normalIdx]
			if dot3(faceNormal, vector3ToFloat64(NormalConventionDayZ.toMLOD(raw))) > 0 {
				dayzAgree++
			}
			if dot3(faceNormal, vector3ToFloat64(NormalConventionStandard.toMLOD(raw))) > 0 {
				standardAgree++
			}
		}
	}
	if standardAgree > dayzAgree {
		return NormalConventionStandard
	}
	return NormalConventionDayZ
}
