package fbxexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

func triangleModel() *model.Model {
	return &model.Model{
		LODs: []model.LOD{
			{
				Resolution: 1.0,
				Vertices:   []model.Vector3{{X: 0}, {X: 1}, {X: 2}},
				Normals:    []model.Vector3{{X: 0, Y: 0, Z: 1}},
				Faces: []model.Face{
					{Indices: []uint32{0, 1, 2}, NormalIndices: []uint32{0, 0, 0}},
				},
			},
		},
	}
}

func TestBuildScenePolyIndex(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}, {}, {}, {}},
			Normals:    []model.Vector3{{X: 0, Y: 0, Z: 1}},
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}},    // triangle
				{Indices: []uint32{0, 1, 2, 3}}, // quad, shares vertices 0-2 with the triangle
				{Indices: []uint32{5, 0}},       // edge, shares vertex 0 - all three faces are one connected component
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1, "all three faces share vertices, so they form a single connected component")
	// Face vertices are reordered per docs/P3D Lod Faces.txt's documented
	// permutation (ODOL/MLOD winding is CW from outside; FBX expects CCW) -
	// see windingFixedSourceIndex in build.go: slot 0 keeps the source's
	// first descriptor, every other slot reads back-to-front. Vertex
	// indices are also remapped to each part's own local, 0-based space in
	// order of first appearance:
	//   triangle [0,1,2] -> read order orig 0,2,1 -> first-seen locals 0,1,2 -> emitted 0, 1, -(2+1)=-3
	//   quad [0,1,2,3] -> read order orig 0,3,2,1 -> orig 0 already seen (local 0); orig 3 is new (local 3); orig 2 already seen (local 1); orig 1 already seen (local 2) -> emitted 0, 3, 1, -(2+1)=-3
	//   edge [5,0] -> read order orig 5,0 -> orig 5 is new (local 4); orig 0 already seen (local 0) -> emitted 4, -(0+1)=-1
	assert.Equal(t, []int32{0, 1, -3, 0, 3, 1, -3, 4, -1}, s.parts[0].geometry.polyIndex)
}

func TestBuildSceneNormalsViaIndices(t *testing.T) {
	normals := []model.Vector3{
		{X: 1, Y: 0, Z: 0},
		{X: 0, Y: 1, Z: 0},
		{X: 0, Y: 0, Z: 1},
	}
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}},
			Normals:    normals,
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}, NormalIndices: []uint32{2, 0, 1}},
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	// Face slots are reordered per windingFixedSourceIndex (see
	// TestBuildScenePolyIndex), so NormalIndices [2,0,1] is read in the
	// order index 0, then 2, then 1: normals[2], normals[1], normals[0].
	// Z is negated (see TestBuildSceneNormalZIsNegated).
	want := []float64{0, 0, -1, 0, 1, 0, 1, 0, 0}
	assert.Equal(t, want, s.parts[0].geometry.normals)
}

// TestBuildSceneNormalZIsNegated documents a real, confirmed-by-comparison-
// against-canonical-DayZ-FBX-output convention mismatch: ODOL/MLOD normal
// data's Z component is inverted relative to what FBX expects, while X, Y,
// and all vertex positions are not. Confirmed by nearest-neighbor-matching
// our exported normals against samples/fbx/*.fbx (real DayZ FBX SDK output)
// for both the ODOLv6 drum and MLOD soda-can samples: X/Y consistently
// agreed, Z was consistently inverted, in every sampled point on both
// models. Without this, lit/shaded 3D viewers (e.g. ArmorPaint) render the
// mesh as flat black, since diffuse lighting depends on normal direction.
func TestBuildSceneNormalZIsNegated(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}},
			Normals:    []model.Vector3{{X: 0.6, Y: 0.2, Z: 0.8}},
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}, NormalIndices: []uint32{0, 0, 0}},
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	normals := s.parts[0].geometry.normals
	for i := 0; i < 3; i++ {
		assert.Equal(t, float64(float32(0.6)), normals[i*3], "X unchanged")
		assert.Equal(t, float64(float32(0.2)), normals[i*3+1], "Y unchanged")
		assert.Equal(t, -float64(float32(0.8)), normals[i*3+2], "Z negated")
	}
}

func TestBuildSceneStripsProxySelectionsAndTheirGeometry(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
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
				{Name: "cargo", VertexIndices: []uint32{0, 1}},
				{Name: "proxy:\\ca\\weapons\\attachments\\foo.001", FaceIndices: []uint32{1}},
			},
		}},
	}

	s, err := buildScene(m)
	require.NoError(t, err)

	require.Len(t, s.selections, 1)
	assert.Equal(t, "cargo", s.selections[0].name)
	require.Len(t, s.parts, 1, "the proxy face's own vertices are stripped along with it")
	// The one surviving face is [0,1,2]; windingFixedSourceIndex visits
	// orig 0,2,1, assigning first-seen locals 0,1,2 respectively - so the
	// emitted vertex array is [vertex0, vertex2, vertex1]. Values are x100
	// (meters to centimeters - see metersToCentimeters in build.go).
	assert.Equal(t, []float64{0, 0, 0, 200, 0, 0, 100, 0, 0}, s.parts[0].geometry.vertices)
	assert.Len(t, s.parts[0].geometry.polyIndex, 3)
}

func TestBuildSceneNormalIndexOutOfRange(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}},
			Normals:    []model.Vector3{{X: 1, Y: 0, Z: 0}},
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}, NormalIndices: []uint32{0, 99, 0}},
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	// index 99 is out of range -> zero vector
	assert.Equal(t, []float64{1, 0, 0, 1, 0, 0, 0, 0, 0}, s.parts[0].geometry.normals)
}

func TestBuildSceneNormalsMissingIndices(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}},
			Normals:    []model.Vector3{{X: 1, Y: 0, Z: 0}},
			Faces: []model.Face{
				// NormalIndices absent: all default to index 0
				{Indices: []uint32{0, 1, 2}},
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	assert.Equal(t, []float64{1, 0, 0, 1, 0, 0, 1, 0, 0}, s.parts[0].geometry.normals)
}

func TestBuildSceneUVs(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}},
			Normals:    []model.Vector3{{X: 0, Y: 0, Z: 1}},
			Faces: []model.Face{
				{
					Indices: []uint32{0, 1, 2},
					UVs: []model.UV{
						{U: 0.1, V: 0.2},
						{U: 0.3, V: 0.4},
						{U: 0.5, V: 0.6},
					},
				},
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	// Face slots are reordered per windingFixedSourceIndex (see
	// TestBuildScenePolyIndex): index 0, then 2, then 1.
	want := []float64{
		float64(float32(0.1)), float64(float32(0.2)),
		float64(float32(0.5)), float64(float32(0.6)),
		float64(float32(0.3)), float64(float32(0.4)),
	}
	assert.Equal(t, want, s.parts[0].geometry.uvSets[0])
}

func TestBuildSceneUVsMissing(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}},
			Normals:    []model.Vector3{{X: 0, Y: 0, Z: 1}},
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}}, // no UVs
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	assert.Equal(t, []float64{0, 0, 0, 0, 0, 0}, s.parts[0].geometry.uvSets[0])
}

func TestBuildSceneSingleUVSetProducesLengthOne(t *testing.T) {
	s, err := buildScene(triangleModel())
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	assert.Len(t, s.parts[0].geometry.uvSets, 1)
}

func TestBuildSceneTwoUVSetsProducesLengthTwo(t *testing.T) {
	m := triangleModel()
	// LOD.UVSets[0] is the primary (unused by fbxexport), LOD.UVSets[1] is additional
	m.LODs[0].UVSets = [][]model.UV{
		nil, // index 0 is ignored; primary comes from Face.UVs
		{{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}, {U: 0.5, V: 0.6}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	assert.Len(t, s.parts[0].geometry.uvSets, 2)
}

func TestBuildSceneAdditionalUVSetIndexedByFaceIndices(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Indices = []uint32{2, 0, 1}
	m.LODs[0].UVSets = [][]model.UV{
		nil,
		{{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}, {U: 0.5, V: 0.6}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	// Face slots are reordered per windingFixedSourceIndex (see
	// TestBuildScenePolyIndex): reading face.Indices = [2,0,1] in order
	// index 0, then 2, then 1 gives 2, 1, 0 →
	// uvSets[1][2], uvSets[1][1], uvSets[1][0]
	// (this additional UV set is looked up by original vertex index, not
	// the part's remapped local index - see the setIdx loop in buildPart)
	want := []float64{
		float64(float32(0.5)), float64(float32(0.6)),
		float64(float32(0.3)), float64(float32(0.4)),
		float64(float32(0.1)), float64(float32(0.2)),
	}
	assert.Equal(t, want, s.parts[0].geometry.uvSets[1])
}

func TestBuildSceneAdditionalUVIndexOutOfRangeFallsToZero(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Indices = []uint32{0, 1, 2} // valid vertex indices; UV lookup below is what's out of range
	m.LODs[0].UVSets = [][]model.UV{
		nil,
		{{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}}, // only 2 entries; vertex index 2 exceeds it
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	// Face slots are reordered per windingFixedSourceIndex, so slot 1 reads
	// vertex 2 (uvSet index 2, out of range -> {0,0}); slots 0 and 2 read
	// vertices 0 and 1, both in range.
	want := []float64{
		float64(float32(0.1)), float64(float32(0.2)),
		0, 0,
		float64(float32(0.3)), float64(float32(0.4)),
	}
	assert.Equal(t, want, s.parts[0].geometry.uvSets[1])
}

func TestBuildSceneUVSetsOnlyPrimaryEmitsOneSet(t *testing.T) {
	m := triangleModel()
	// LOD.UVSets has only index 0; no additional sets should be emitted
	m.LODs[0].UVSets = [][]model.UV{
		{{U: 0.5, V: 0.5}, {U: 0.5, V: 0.5}, {U: 0.5, V: 0.5}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	assert.Len(t, s.parts[0].geometry.uvSets, 1)
}

func TestBuildSceneMaterialBucketing(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{}, {}, {}},
			Normals:    []model.Vector3{{X: 0, Y: 0, Z: 1}},
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}, Texture: "a.paa", Material: ""},
				{Indices: []uint32{0, 1, 2}, Texture: "b.paa", Material: ""},
				{Indices: []uint32{0, 1, 2}, Texture: "a.paa", Material: ""},
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1, "all three faces share the same vertices, so they're one connected component")
	assert.Len(t, s.materials, 2)
	assert.Equal(t, []int32{0, 1, 0}, s.parts[0].geometry.matIndex)
}

func TestBuildSceneVertexUpcastAndScale(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Vertices = []model.Vector3{{X: 1.5, Y: -2.5, Z: 0.125}, {}, {}}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	verts := s.parts[0].geometry.vertices
	require.Len(t, verts, 9)
	// The single face [0,1,2] reads slot 0 from source index 0 (see
	// windingFixedSourceIndex), so vertex 0 (the interesting one) is
	// emitted first, as the first local vertex. ODOL/MLOD vertex positions
	// are meters; exported FBX vertices are centimeters (see
	// metersToCentimeters in build.go), so each component is upcast to
	// float64 and then scaled by 100.
	assert.Equal(t, float64(float32(1.5))*100, verts[0])
	assert.Equal(t, float64(float32(-2.5))*100, verts[1])
	assert.Equal(t, float64(float32(0.125))*100, verts[2])
}

func TestBuildSceneIDOrdering(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Texture = "x.paa"
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 1)
	// materials are allocated (and their IDs assigned) before parts.
	assert.Equal(t, int64(1), s.materials[0].id)
	assert.Equal(t, int64(4), s.parts[0].geomID)
	assert.Equal(t, int64(5), s.parts[0].modelID)
}

func TestBuildSceneMaterialAllocatesVideoAndTextureIDs(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Texture = "x.paa"
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.materials, 1)
	mat := s.materials[0]
	assert.NotZero(t, mat.videoID)
	assert.NotZero(t, mat.textureID)
	assert.NotEqual(t, mat.id, mat.videoID)
	assert.NotEqual(t, mat.id, mat.textureID)
	assert.NotEqual(t, mat.videoID, mat.textureID)
}

func TestBuildSceneEmptyModel(t *testing.T) {
	_, err := buildScene(&model.Model{})
	assert.ErrorIs(t, err, errNoLODs)
}

func TestBuildSceneNoSelectionsProducesEmptySlice(t *testing.T) {
	m := triangleModel()
	s, err := buildScene(m)
	require.NoError(t, err)
	assert.Empty(t, s.selections)
}

func TestBuildSceneSelectionFieldsCopied(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Selections = []model.Selection{
		{
			Name:          "cargo",
			IsSectional:   true,
			VertexIndices: []uint32{0, 2},
			VertexWeights: []byte{128, 255},
			FaceIndices:   []uint32{1, 3},
		},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.selections, 1)
	sel := s.selections[0]
	assert.Equal(t, "cargo", sel.name)
	assert.True(t, sel.isSectional)
	assert.Equal(t, []uint32{0, 2}, sel.vertexIndices)
	assert.Equal(t, []byte{128, 255}, sel.vertexWeights)
	assert.Equal(t, []uint32{1, 3}, sel.faceIndices)
}

func TestBuildSceneSelectionNilWeightsNoParanic(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Selections = []model.Selection{
		{Name: "body", VertexIndices: []uint32{1, 2}, VertexWeights: nil},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.selections, 1)
	assert.Nil(t, s.selections[0].vertexWeights)
}

func TestBuildSceneSelectionIDsAfterProxiesAndParts(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Texture = "x.paa"
	m.LODs[0].Proxies = []model.Proxy{{ModelPath: "proxy.p3d"}}
	m.LODs[0].Selections = []model.Selection{
		{Name: "sel_a"},
		{Name: "sel_b"},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	// mat=1, mat.videoID=2, mat.textureID=3, part.geomID=4, part.modelID=5, proxy=6, sel_a=7, sel_b=8
	require.Len(t, s.selections, 2)
	assert.Equal(t, int64(7), s.selections[0].id)
	assert.Equal(t, int64(8), s.selections[1].id)
}

func twoDisjointTrianglesModel() *model.Model {
	return &model.Model{
		LODs: []model.LOD{{
			Resolution: 1.0,
			Vertices:   []model.Vector3{{X: 0}, {X: 1}, {X: 2}, {X: 10}, {X: 11}, {X: 12}},
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}, Texture: "a.paa"},
				{Indices: []uint32{3, 4, 5}, Texture: "b.paa"},
			},
		}},
	}
}

func TestBuildSceneSplitsDisconnectedGeometryIntoParts(t *testing.T) {
	s, err := buildScene(twoDisjointTrianglesModel())
	require.NoError(t, err)
	require.Len(t, s.parts, 2)
	assert.Equal(t, "object_NNN1", s.parts[0].name)
	assert.Equal(t, "object_NNN2", s.parts[1].name)
	// Each part only has its own 3 vertices, not the whole LOD's 6.
	assert.Len(t, s.parts[0].geometry.vertices, 9)
	assert.Len(t, s.parts[1].geometry.vertices, 9)
}

func TestBuildScenePartMaterialsScopedToUsage(t *testing.T) {
	s, err := buildScene(twoDisjointTrianglesModel())
	require.NoError(t, err)
	require.Len(t, s.parts, 2)
	require.Len(t, s.materials, 2)

	// Each part uses a different texture, so each should reference exactly
	// the one global material it actually needs, not both.
	require.Len(t, s.parts[0].localMaterials, 1)
	require.Len(t, s.parts[1].localMaterials, 1)
	assert.NotEqual(t, s.parts[0].localMaterials[0], s.parts[1].localMaterials[0])
	assert.Equal(t, "a.paa", s.materials[s.parts[0].localMaterials[0]].texture)
	assert.Equal(t, "b.paa", s.materials[s.parts[1].localMaterials[0]].texture)
}

func TestBuildScenePartsShareMaterialAcrossParts(t *testing.T) {
	m := twoDisjointTrianglesModel()
	m.LODs[0].Faces[1].Texture = "a.paa" // both disconnected parts now use the same texture

	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.parts, 2)
	// Only one material should be created, matching the mich2001.fbx
	// reference case (163 disconnected parts, all sharing one Material) -
	// see TestParityConnectedComponentsMich2001.
	require.Len(t, s.materials, 1)
	require.Len(t, s.parts[0].localMaterials, 1)
	require.Len(t, s.parts[1].localMaterials, 1)
	assert.Equal(t, s.parts[0].localMaterials[0], s.parts[1].localMaterials[0])
}
