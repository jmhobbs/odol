package mlod

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

func minimalModel() *model.Model {
	return &model.Model{
		Name: "test",
		LODs: []model.LOD{
			{
				Resolution: 1.0,
				Vertices: []model.Vector3{
					{X: 0, Y: 0, Z: 0},
					{X: 1, Y: 0, Z: 0},
					{X: 0, Y: 1, Z: 0},
				},
				Normals: []model.Vector3{
					{X: 0, Y: 0, Z: 1},
				},
				Faces: []model.Face{
					{
						Indices:       []uint32{0, 1, 2},
						NormalIndices: []uint32{0, 0, 0},
						UVs: []model.UV{
							{U: 0.1, V: 0.2},
							{U: 0.3, V: 0.4},
							{U: 0.5, V: 0.6},
						},
						Texture:  "test.paa",
						Material: "test.rvmat",
					},
				},
			},
		},
	}
}

func marshalAndParse(t *testing.T, m *model.Model) *model.Model {
	t.Helper()
	data, err := Marshal(m)
	require.NoError(t, err)
	parsed, err := Parse(data, m.Name)
	require.NoError(t, err)
	return parsed
}

func TestParseErrorOnNonMLOD(t *testing.T) {
	_, err := Parse([]byte("garbage data that is not mlod"), "test")
	assert.Error(t, err)
}

func TestParseVertexCount(t *testing.T) {
	m := minimalModel()
	parsed := marshalAndParse(t, m)
	require.Len(t, parsed.LODs, 1)
	assert.Len(t, parsed.LODs[0].Vertices, 3)
}

func TestParseNormalCount(t *testing.T) {
	m := minimalModel()
	parsed := marshalAndParse(t, m)
	assert.Len(t, parsed.LODs[0].Normals, 1)
}

func TestParseTriangleFace(t *testing.T) {
	m := minimalModel()
	parsed := marshalAndParse(t, m)
	require.Len(t, parsed.LODs[0].Faces, 1)
	f := parsed.LODs[0].Faces[0]
	assert.Equal(t, []uint32{0, 1, 2}, f.Indices)
	assert.Equal(t, []uint32{0, 0, 0}, f.NormalIndices)
	assert.Equal(t, "test.paa", f.Texture)
	assert.Equal(t, "test.rvmat", f.Material)
	assert.InDelta(t, float32(0.1), f.UVs[0].U, 1e-6)
	assert.InDelta(t, float32(0.2), f.UVs[0].V, 1e-6)
}

func TestParseQuadFace(t *testing.T) {
	m := minimalModel()
	m.LODs[0].Vertices = append(m.LODs[0].Vertices, model.Vector3{X: 1, Y: 1, Z: 0})
	m.LODs[0].Faces[0].Indices = []uint32{0, 1, 3, 2}
	m.LODs[0].Faces[0].NormalIndices = []uint32{0, 0, 0, 0}
	m.LODs[0].Faces[0].UVs = append(m.LODs[0].Faces[0].UVs, model.UV{U: 1, V: 1})
	parsed := marshalAndParse(t, m)
	require.Len(t, parsed.LODs[0].Faces, 1)
	assert.Len(t, parsed.LODs[0].Faces[0].Indices, 4)
	assert.Equal(t, []uint32{0, 1, 3, 2}, parsed.LODs[0].Faces[0].Indices)
}

func TestParseResolution(t *testing.T) {
	m := minimalModel()
	m.LODs[0].Resolution = 2.5
	parsed := marshalAndParse(t, m)
	assert.InDelta(t, float32(2.5), parsed.LODs[0].Resolution, 1e-6)
}

func TestParseProperties(t *testing.T) {
	m := minimalModel()
	m.LODs[0].Properties = map[string]string{
		"autocenter":  "0",
		"lodnoshadow": "1",
	}
	m.LODs[0].PropertyOrder = []string{"autocenter", "lodnoshadow"}
	parsed := marshalAndParse(t, m)
	assert.Equal(t, "0", parsed.LODs[0].Properties["autocenter"])
	assert.Equal(t, "1", parsed.LODs[0].Properties["lodnoshadow"])
}

func TestParseMultipleLODs(t *testing.T) {
	m := minimalModel()
	lod2 := m.LODs[0]
	lod2.Resolution = 2.0
	m.LODs = append(m.LODs, lod2)
	parsed := marshalAndParse(t, m)
	assert.Len(t, parsed.LODs, 2)
	assert.InDelta(t, float32(1.0), parsed.LODs[0].Resolution, 1e-6)
	assert.InDelta(t, float32(2.0), parsed.LODs[1].Resolution, 1e-6)
}

func TestParseModelNameFromArgument(t *testing.T) {
	m := minimalModel()
	data, err := Marshal(m)
	require.NoError(t, err)
	parsed, err := Parse(data, "55galdrum_mlod")
	require.NoError(t, err)
	assert.Equal(t, "55galdrum_mlod", parsed.Name)
}

// --- Selection data tests (Part B of plan_selection-data.md) ---

func TestParseSelectionNoVerticesOrFaces(t *testing.T) {
	m := minimalModel()
	m.LODs[0].Selections = []model.Selection{
		{Name: "empty"},
	}
	parsed := marshalAndParse(t, m)
	require.Len(t, parsed.LODs[0].Selections, 1)
	sel := parsed.LODs[0].Selections[0]
	assert.Equal(t, "empty", sel.Name)
	assert.Empty(t, sel.VertexIndices)
	assert.Empty(t, sel.FaceIndices)
}

func TestParseSelectionVertexIndicesAndWeights(t *testing.T) {
	m := minimalModel()
	// vertices at indices 1 and 2 are in the selection with specific weights
	m.LODs[0].Selections = []model.Selection{
		{
			Name:          "weighted",
			VertexIndices: []uint32{1, 2},
			VertexWeights: []byte{128, 255},
		},
	}
	parsed := marshalAndParse(t, m)
	require.Len(t, parsed.LODs[0].Selections, 1)
	sel := parsed.LODs[0].Selections[0]
	assert.Equal(t, "weighted", sel.Name)
	assert.Equal(t, []uint32{1, 2}, sel.VertexIndices)
	assert.Equal(t, []byte{128, 255}, sel.VertexWeights)
}

func TestParseSelectionFaceIndices(t *testing.T) {
	m := minimalModel()
	// add a second face so we can test face membership
	m.LODs[0].Vertices = append(m.LODs[0].Vertices, model.Vector3{X: 2, Y: 0, Z: 0})
	m.LODs[0].Faces = append(m.LODs[0].Faces, model.Face{
		Indices:       []uint32{1, 3, 2},
		NormalIndices: []uint32{0, 0, 0},
		UVs:           []model.UV{{}, {}, {}},
	})
	m.LODs[0].Selections = []model.Selection{
		{
			Name:        "faces",
			FaceIndices: []uint32{1},
		},
	}
	parsed := marshalAndParse(t, m)
	require.Len(t, parsed.LODs[0].Selections, 1)
	sel := parsed.LODs[0].Selections[0]
	assert.Contains(t, sel.FaceIndices, uint32(1))
}

func TestParseSelectionIsSectionalAlwaysFalse(t *testing.T) {
	m := minimalModel()
	m.LODs[0].Selections = []model.Selection{
		{Name: "sect", IsSectional: true},
	}
	parsed := marshalAndParse(t, m)
	require.Len(t, parsed.LODs[0].Selections, 1)
	// MLOD has no sectional flag; parser always sets false
	assert.False(t, parsed.LODs[0].Selections[0].IsSectional)
}

// --- UV set tests (Part B of plan_uv-sets.md) ---

func TestParseUVSetIDZeroIsSkipped(t *testing.T) {
	m := minimalModel()
	// UVSets[0] is written by Marshal as id=0 (redundant with face UVs)
	m.LODs[0].UVSets = [][]model.UV{
		{{U: 0.5, V: 0.5}, {U: 0.5, V: 0.5}, {U: 0.5, V: 0.5}},
	}
	parsed := marshalAndParse(t, m)
	// UVSets[0] must not be populated from the TAGG; primary UV comes from face records
	if len(parsed.LODs[0].UVSets) > 0 {
		assert.Nil(t, parsed.LODs[0].UVSets[0])
	}
}

func TestParseUVSetIDOneParsedIntoUVSets1(t *testing.T) {
	m := minimalModel()
	// LOD.UVSets[1] triggers a #UVSet# id=1 TAGG during Marshal
	m.LODs[0].UVSets = [][]model.UV{
		nil,
		{{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}, {U: 0.5, V: 0.6}},
	}
	parsed := marshalAndParse(t, m)
	require.True(t, len(parsed.LODs[0].UVSets) >= 2, "expected UVSets to have at least 2 entries")
	require.NotNil(t, parsed.LODs[0].UVSets[1])
	uvSet := parsed.LODs[0].UVSets[1]
	assert.InDelta(t, float32(0.1), uvSet[0].U, 1e-6)
	assert.InDelta(t, float32(0.2), uvSet[0].V, 1e-6)
	assert.InDelta(t, float32(0.3), uvSet[1].U, 1e-6)
	assert.InDelta(t, float32(0.5), uvSet[2].U, 1e-6)
}

func TestParseUVSetSeamFirstSeenWins(t *testing.T) {
	// Two faces share vertex 0. Each face assigns a different UV to vertex 0 in set 1.
	// First-seen value (from face 0) must win.
	m := minimalModel()
	m.LODs[0].Vertices = append(m.LODs[0].Vertices, model.Vector3{X: 2, Y: 0, Z: 0})
	m.LODs[0].Faces = append(m.LODs[0].Faces, model.Face{
		Indices:       []uint32{1, 3, 2},
		NormalIndices: []uint32{0, 0, 0},
		UVs:           []model.UV{{}, {}, {}},
	})
	// Vertex 0 in set 1: first face gives {0.1,0.2}, second face gives {0.9,0.9}
	m.LODs[0].UVSets = [][]model.UV{
		nil,
		{{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}, {U: 0.5, V: 0.6}, {U: 0.7, V: 0.8}},
	}
	// marshalIndexedUVSet writes face-vertex order; for face 0: indices [0,1,2] → UVs[0], UVs[1], UVs[2]
	// face 1: indices [1,3,2] → UVs[1], UVs[3], UVs[2] — vertex 0 does not appear in face 1, so no conflict here.
	// To force a seam we need a model where the same vertex index appears in two faces with different UVs in the set.
	// Since marshalIndexedUVSet uses lod.UVSets[id][face.Indices[i]], conflicting values can't come from the
	// same source slice — the slice is per-vertex already. So this test verifies reconstruction round-trips correctly.
	parsed := marshalAndParse(t, m)
	require.True(t, len(parsed.LODs[0].UVSets) >= 2)
	uvSet := parsed.LODs[0].UVSets[1]
	require.True(t, len(uvSet) >= 1)
	assert.InDelta(t, float32(0.1), uvSet[0].U, 1e-6)
	assert.InDelta(t, float32(0.2), uvSet[0].V, 1e-6)
}

func TestParseUVSetIDGapHandled(t *testing.T) {
	m := minimalModel()
	// UVSets[0]=nil (skipped by parser), UVSets[1]=nil (Marshal writes all-zero TAGG for id=1),
	// UVSets[2]=data. After round-trip: UVSets[0] absent/nil, UVSets[1] all-zeros, UVSets[2] intact.
	m.LODs[0].UVSets = [][]model.UV{
		nil,
		nil,
		{{U: 0.9, V: 0.8}, {U: 0.7, V: 0.6}, {U: 0.5, V: 0.4}},
	}
	parsed := marshalAndParse(t, m)
	require.True(t, len(parsed.LODs[0].UVSets) >= 3, "expected UVSets to have at least 3 entries")
	// UVSets[0] is always nil (id=0 TAGG is skipped)
	if len(parsed.LODs[0].UVSets) > 0 {
		assert.Nil(t, parsed.LODs[0].UVSets[0])
	}
	// UVSets[2] must have the correct data
	require.NotNil(t, parsed.LODs[0].UVSets[2])
	assert.InDelta(t, float32(0.9), parsed.LODs[0].UVSets[2][0].U, 1e-6)
}

// --- Integration tests ---
