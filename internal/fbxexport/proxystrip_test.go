package fbxexport

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jmhobbs/odol/internal/model"
)

func TestStripProxySelectionsNoProxySelectionsIsNoOp(t *testing.T) {
	lod := model.LOD{
		Resolution: 1.0,
		Vertices:   []model.Vector3{{X: 0}, {X: 1}, {X: 2}},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
		},
		Selections: []model.Selection{
			{Name: "cargo", VertexIndices: []uint32{0, 1}},
		},
	}

	got := stripProxySelections(lod)

	assert.Equal(t, lod, got)
}

func TestStripProxySelectionsFaceOnlyRemovesFaceAndItsVertices(t *testing.T) {
	// Mirrors the real-world m4a1.p3d shape: a proxy selection with an
	// empty VertexIndices/VertexWeights but a populated FaceIndices.
	lod := model.LOD{
		Resolution: 0,
		Vertices: []model.Vector3{
			{X: 0}, {X: 1}, {X: 2}, // real triangle
			{X: 10}, {X: 11}, {X: 12}, // proxy placeholder triangle
		},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
			{Indices: []uint32{3, 4, 5}},
		},
		Selections: []model.Selection{
			{Name: "proxy:\\ca\\weapons\\attachments\\foo.001", FaceIndices: []uint32{1}},
		},
	}

	got := stripProxySelections(lod)

	assert.Empty(t, got.Selections)
	assert.Equal(t, []model.Vector3{{X: 0}, {X: 1}, {X: 2}}, got.Vertices)
	assert.Equal(t, []model.Face{{Indices: []uint32{0, 1, 2}}}, got.Faces)
}

func TestStripProxySelectionsSharedVertexIsKept(t *testing.T) {
	// The proxy placeholder face shares vertex 2 with a kept face.
	lod := model.LOD{
		Vertices: []model.Vector3{{X: 0}, {X: 1}, {X: 2}, {X: 3}},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
			{Indices: []uint32{2, 3, 0}}, // proxy face, reuses vertex 2 and 0
		},
		Selections: []model.Selection{
			{Name: "proxy:\\ca\\weapons\\attachments\\bar.001", FaceIndices: []uint32{1}},
		},
	}

	got := stripProxySelections(lod)

	assert.Empty(t, got.Selections)
	assert.Len(t, got.Faces, 1)
	assert.Equal(t, []uint32{0, 1, 2}, got.Faces[0].Indices)
	// Vertex 3 was used only by the removed proxy face, so it is dropped;
	// vertices 0, 1, 2 remain (0 and 2 kept because face 0 still uses them).
	assert.Equal(t, []model.Vector3{{X: 0}, {X: 1}, {X: 2}}, got.Vertices)
}

func TestStripProxySelectionsMultipleProxiesAreAllRemoved(t *testing.T) {
	lod := model.LOD{
		Vertices: []model.Vector3{
			{X: 0}, {X: 1}, {X: 2}, // real triangle
			{X: 10}, {X: 11}, {X: 12}, // proxy A
			{X: 20}, {X: 21}, {X: 22}, // proxy B
		},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
			{Indices: []uint32{3, 4, 5}},
			{Indices: []uint32{6, 7, 8}},
		},
		Selections: []model.Selection{
			{Name: "proxy:a.001", FaceIndices: []uint32{1}},
			{Name: "proxy:b.001", FaceIndices: []uint32{2}},
		},
	}

	got := stripProxySelections(lod)

	assert.Empty(t, got.Selections)
	assert.Equal(t, []model.Vector3{{X: 0}, {X: 1}, {X: 2}}, got.Vertices)
	assert.Equal(t, []model.Face{{Indices: []uint32{0, 1, 2}}}, got.Faces)
}

func TestStripProxySelectionsRemapsKeptSelections(t *testing.T) {
	lod := model.LOD{
		Vertices: []model.Vector3{
			{X: 0}, {X: 1}, {X: 2}, // real triangle (vertices 0,1,2)
			{X: 10}, {X: 11}, {X: 12}, // proxy placeholder
		},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
			{Indices: []uint32{3, 4, 5}},
		},
		Selections: []model.Selection{
			{
				Name:          "cargo",
				VertexIndices: []uint32{1, 2},
				VertexWeights: []byte{255, 128},
				FaceIndices:   []uint32{0},
			},
			{Name: "proxy:x.001", FaceIndices: []uint32{1}},
		},
	}

	got := stripProxySelections(lod)

	assert.Len(t, got.Selections, 1)
	assert.Equal(t, "cargo", got.Selections[0].Name)
	// Vertex indices 1,2 are unaffected since no lower-numbered vertex was removed.
	assert.Equal(t, []uint32{1, 2}, got.Selections[0].VertexIndices)
	assert.Equal(t, []byte{255, 128}, got.Selections[0].VertexWeights)
	assert.Equal(t, []uint32{0}, got.Selections[0].FaceIndices)
}

func TestStripProxySelectionsRemapsUVSetsAndPointMasses(t *testing.T) {
	lod := model.LOD{
		Vertices: []model.Vector3{
			{X: 0}, {X: 1}, {X: 2}, // real triangle
			{X: 10}, {X: 11}, {X: 12}, // proxy placeholder
		},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
			{Indices: []uint32{3, 4, 5}},
		},
		UVSets: [][]model.UV{
			{{U: 0, V: 0}}, // primary set (unused by this transform, but must not be touched incorrectly)
			{
				{U: 0.1, V: 0.1}, {U: 0.2, V: 0.2}, {U: 0.3, V: 0.3},
				{U: 0.4, V: 0.4}, {U: 0.5, V: 0.5}, {U: 0.6, V: 0.6},
			},
		},
		PointMasses: []float32{1, 2, 3, 4, 5, 6},
		Selections: []model.Selection{
			{Name: "proxy:x.001", FaceIndices: []uint32{1}},
		},
	}

	got := stripProxySelections(lod)

	assert.Equal(t, [][]model.UV{
		{{U: 0, V: 0}},
		{{U: 0.1, V: 0.1}, {U: 0.2, V: 0.2}, {U: 0.3, V: 0.3}},
	}, got.UVSets)
	assert.Equal(t, []float32{1, 2, 3}, got.PointMasses)
}

func TestStripProxySelectionsAllGeometryIsProxyProducesEmptyLOD(t *testing.T) {
	lod := model.LOD{
		Vertices: []model.Vector3{{X: 0}, {X: 1}, {X: 2}},
		Faces: []model.Face{
			{Indices: []uint32{0, 1, 2}},
		},
		Selections: []model.Selection{
			{Name: "proxy:x.001", FaceIndices: []uint32{0}},
		},
	}

	got := stripProxySelections(lod)

	assert.Empty(t, got.Vertices)
	assert.Empty(t, got.Faces)
	assert.Empty(t, got.Selections)
}
