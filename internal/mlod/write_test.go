package mlod

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jmhobbs/odol/internal/detector"
	"github.com/jmhobbs/odol/internal/model"
)

func TestMarshalProducesMLOD(t *testing.T) {
	t.Parallel()

	m := &model.Model{
		Name: "barrel",
		LODs: []model.LOD{
			{
				Resolution: 1,
				Vertices: []model.Vector3{
					{X: 0, Y: 0, Z: 0},
					{X: 1, Y: 0, Z: 0},
					{X: 0, Y: 1, Z: 0},
				},
				Normals: []model.Vector3{
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
				},
				Faces: []model.Face{
					{
						Indices:       []uint32{0, 1, 2},
						NormalIndices: []uint32{0, 1, 2},
						UVs: []model.UV{
							{U: 0, V: 0},
							{U: 1, V: 0},
							{U: 0, V: 1},
						},
						Texture:  "tex.paa",
						Material: "mat.rvmat",
					},
				},
				Properties: map[string]string{"autocenter": "0"},
				PointMasses: []float32{
					1, 1, 1,
				},
				Selections: []model.Selection{
					{Name: "camo", VertexIndices: []uint32{0, 1, 2}, FaceIndices: []uint32{0}},
				},
			},
		},
	}

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	format, err := detector.Detect(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if format.Family != detector.P3DFamilyMLOD || format.LODSignature != "P3DM" {
		t.Fatalf("unexpected format: %#v", format)
	}

	text := string(data)
	for _, want := range []string{"TAGG", "#Mass#", "#UVSet#", "#Property#", "#EndOfFile#", "camo"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Marshal output missing %q", want)
		}
	}
}
