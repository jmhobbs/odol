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
			Vertices:   []model.Vector3{{}, {}, {}, {}},
			Normals:    []model.Vector3{{X: 0, Y: 0, Z: 1}},
			Faces: []model.Face{
				{Indices: []uint32{0, 1, 2}},    // triangle
				{Indices: []uint32{0, 1, 2, 3}}, // quad
				{Indices: []uint32{5, 0}},       // edge: vertex 0 as last -> -1
			},
		}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	// Face vertices are emitted in reverse per-face order (ODOL/MLOD winding
	// is CCW from outside; FBX expects CW) - see the comment in build.go.
	// triangle [0,1,2] reversed -> 2, 1, -(0+1) = -1
	// quad     [0,1,2,3] reversed -> 3, 2, 1, -(0+1) = -1
	// edge     [5,0] reversed -> 0, -(5+1) = -6
	assert.Equal(t, []int32{2, 1, -1, 3, 2, 1, -1, 0, -6}, s.geometry.polyIndex)
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
	// Face slots are reversed (see TestBuildScenePolyIndex), so
	// NormalIndices [2,0,1] is read back-to-front: normals[1], normals[0], normals[2].
	// Z is negated (see TestBuildSceneNormalZIsNegated).
	want := []float64{0, 1, 0, 1, 0, 0, 0, 0, -1}
	assert.Equal(t, want, s.geometry.normals)
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
	for i := 0; i < 3; i++ {
		assert.Equal(t, float64(float32(0.6)), s.geometry.normals[i*3], "X unchanged")
		assert.Equal(t, float64(float32(0.2)), s.geometry.normals[i*3+1], "Y unchanged")
		assert.Equal(t, -float64(float32(0.8)), s.geometry.normals[i*3+2], "Z negated")
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

	require.Len(t, s.mesh.selections, 1)
	assert.Equal(t, "cargo", s.mesh.selections[0].name)
	// Only the real triangle's 3 vertices remain (9 floats), and the
	// proxy face is gone from polyIndex (3 entries instead of 6).
	assert.Equal(t, []float64{0, 0, 0, 1, 0, 0, 2, 0, 0}, s.geometry.vertices)
	assert.Len(t, s.geometry.polyIndex, 3)
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
	// index 99 is out of range -> zero vector
	assert.Equal(t, []float64{1, 0, 0, 0, 0, 0, 1, 0, 0}, s.geometry.normals)
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
	assert.Equal(t, []float64{1, 0, 0, 1, 0, 0, 1, 0, 0}, s.geometry.normals)
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
	// Face slots are reversed (see TestBuildScenePolyIndex), so UVs are
	// read back-to-front.
	want := []float64{
		float64(float32(0.5)), float64(float32(0.6)),
		float64(float32(0.3)), float64(float32(0.4)),
		float64(float32(0.1)), float64(float32(0.2)),
	}
	assert.Equal(t, want, s.geometry.uvSets[0])
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
	assert.Equal(t, []float64{0, 0, 0, 0, 0, 0}, s.geometry.uvSets[0])
}

func TestBuildSceneSingleUVSetProducesLengthOne(t *testing.T) {
	s, err := buildScene(triangleModel())
	require.NoError(t, err)
	assert.Len(t, s.geometry.uvSets, 1)
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
	assert.Len(t, s.geometry.uvSets, 2)
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
	// Face slots are reversed (see TestBuildScenePolyIndex): reading
	// face.Indices = [2,0,1] back-to-front gives 1, 0, 2 →
	// uvSets[1][1], uvSets[1][0], uvSets[1][2]
	want := []float64{
		float64(float32(0.3)), float64(float32(0.4)),
		float64(float32(0.1)), float64(float32(0.2)),
		float64(float32(0.5)), float64(float32(0.6)),
	}
	assert.Equal(t, want, s.geometry.uvSets[1])
}

func TestBuildSceneAdditionalUVIndexOutOfRangeFallsToZero(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Indices = []uint32{0, 1, 99} // index 99 exceeds uvSet length
	m.LODs[0].UVSets = [][]model.UV{
		nil,
		{{U: 0.1, V: 0.2}, {U: 0.3, V: 0.4}}, // only 2 entries
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	// Face slots are reversed, so vertex 99 (originally last) is read
	// first: slot 0 → out of range → {0, 0}
	assert.Equal(t, float64(0), s.geometry.uvSets[1][0])
	assert.Equal(t, float64(0), s.geometry.uvSets[1][1])
}

func TestBuildSceneUVSetsOnlyPrimaryEmitsOneSet(t *testing.T) {
	m := triangleModel()
	// LOD.UVSets has only index 0; no additional sets should be emitted
	m.LODs[0].UVSets = [][]model.UV{
		{{U: 0.5, V: 0.5}, {U: 0.5, V: 0.5}, {U: 0.5, V: 0.5}},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	assert.Len(t, s.geometry.uvSets, 1)
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
	assert.Len(t, s.materials, 2)
	assert.Equal(t, []int32{0, 1, 0}, s.geometry.matIndex)
}

func TestBuildSceneVertexUpcast(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Vertices = []model.Vector3{{X: 1.5, Y: -2.5, Z: 0.125}}
	s, err := buildScene(m)
	require.NoError(t, err)
	require.Len(t, s.geometry.vertices, 3)
	assert.Equal(t, float64(float32(1.5)), s.geometry.vertices[0])
	assert.Equal(t, float64(float32(-2.5)), s.geometry.vertices[1])
	assert.Equal(t, float64(float32(0.125)), s.geometry.vertices[2])
}

func TestBuildSceneIDOrdering(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Texture = "x.paa"
	s, err := buildScene(m)
	require.NoError(t, err)
	assert.Equal(t, int64(1), s.geometry.id)
	assert.Equal(t, int64(2), s.mesh.id)
	assert.Equal(t, int64(3), s.materials[0].id)
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
	assert.Empty(t, s.mesh.selections)
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
	require.Len(t, s.mesh.selections, 1)
	sel := s.mesh.selections[0]
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
	require.Len(t, s.mesh.selections, 1)
	assert.Nil(t, s.mesh.selections[0].vertexWeights)
}

func TestBuildSceneSelectionIDsAfterProxies(t *testing.T) {
	m := triangleModel()
	m.LODs[0].Faces[0].Texture = "x.paa"
	m.LODs[0].Proxies = []model.Proxy{{ModelPath: "proxy.p3d"}}
	m.LODs[0].Selections = []model.Selection{
		{Name: "sel_a"},
		{Name: "sel_b"},
	}
	s, err := buildScene(m)
	require.NoError(t, err)
	// geom=1, mesh=2, mat=3, mat.videoID=4, mat.textureID=5, proxy=6, sel_a=7, sel_b=8
	assert.Equal(t, int64(7), s.mesh.selections[0].id)
	assert.Equal(t, int64(8), s.mesh.selections[1].id)
}
