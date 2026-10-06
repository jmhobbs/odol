package inspect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/detector"
	"github.com/jmhobbs/odol/internal/model"
)

func fixtureModel() *model.Model {
	return &model.Model{
		Name:         "test_model",
		Source:       model.SourceFormat{Family: "ODOL", Version: 7},
		CenterOfMass: model.Vector3{X: 1, Y: 2, Z: 3},
		Config: model.Config{
			Skeletons: []model.SkeletonClass{
				{Name: "test_skeleton", Bones: []model.SkeletonBone{{Name: "a"}, {Name: "b"}}},
			},
			Models: []model.ModelClass{
				{
					Name:         "test_model",
					SkeletonName: "test_skeleton",
					Sections:     []string{"camo"},
					Animations:   []model.Animation{{ClassName: "anim1"}},
				},
			},
		},
		LODs: []model.LOD{
			{
				Resolution: 1.0,
				Vertices: []model.Vector3{
					{X: -1, Y: 0, Z: -2},
					{X: 1, Y: 0, Z: 2},
					{X: 0, Y: 3, Z: 0},
					{X: 0, Y: -1, Z: 0},
				},
				Faces: []model.Face{
					{Texture: "a.paa", Material: "a.rvmat"},
					{Texture: "a.paa", Material: "b.rvmat"},
					{Texture: "b.paa", Material: ""},
				},
				UVSets: [][]model.UV{{}, {}},
				Selections: []model.Selection{
					{Name: "wheel_1", VertexIndices: []uint32{0, 1}, FaceIndices: []uint32{0}},
					{Name: "geometry", IsSectional: true, VertexIndices: []uint32{0, 1, 2, 3}, FaceIndices: []uint32{0, 1, 2}},
				},
				Proxies: []model.Proxy{
					{ModelPath: "proxy_door.p3d", SequenceID: 1},
				},
			},
			{
				Resolution: float32(1e13), // Geometry
				Vertices:   make([]model.Vector3, 2),
				Faces:      []model.Face{{Texture: "b.paa", Material: "a.rvmat"}},
				Properties: map[string]string{"odol_partial_parse": "1"},
			},
		},
	}
}

func TestSummarizeLODCounts(t *testing.T) {
	s := Summarize(fixtureModel(), detector.P3DFormat{Family: detector.P3DFamilyODOL, Version: 7})
	require.Len(t, s.LODs, 2)

	assert.Equal(t, 4, s.LODs[0].VertexCount)
	assert.Equal(t, 3, s.LODs[0].FaceCount)
	assert.Equal(t, 2, s.LODs[0].UVSetCount)
	assert.Equal(t, "Graphical LOD", s.LODs[0].ResolutionName)
	assert.False(t, s.LODs[0].Partial)

	assert.Equal(t, 2, s.LODs[1].VertexCount)
	assert.Equal(t, 1, s.LODs[1].FaceCount)
	assert.Equal(t, "Geometry", s.LODs[1].ResolutionName)
	assert.True(t, s.LODs[1].Partial)
}

func TestSummarizeTextureMaterialDedup(t *testing.T) {
	s := Summarize(fixtureModel(), detector.P3DFormat{})

	// LOD 0 faces reference a.paa twice, b.paa once -> unique a.paa, b.paa.
	assert.ElementsMatch(t, []string{"a.paa", "b.paa"}, s.LODs[0].Textures)
	assert.ElementsMatch(t, []string{"a.rvmat", "b.rvmat"}, s.LODs[0].Materials)

	// Aggregate across both LODs stays deduped.
	assert.ElementsMatch(t, []string{"a.paa", "b.paa"}, s.Textures)
	assert.ElementsMatch(t, []string{"a.rvmat", "b.rvmat"}, s.Materials)
}

func TestSummarizeSelections(t *testing.T) {
	s := Summarize(fixtureModel(), detector.P3DFormat{})
	require.Len(t, s.LODs[0].Selections, 2)

	wheel := s.LODs[0].Selections[0]
	assert.Equal(t, "wheel_1", wheel.Name)
	assert.False(t, wheel.IsSectional)
	assert.Equal(t, 2, wheel.VertexCount)
	assert.Equal(t, 1, wheel.FaceCount)

	geom := s.LODs[0].Selections[1]
	assert.True(t, geom.IsSectional)
	assert.Equal(t, 4, geom.VertexCount)
	assert.Equal(t, 3, geom.FaceCount)
}

func TestSummarizeProxies(t *testing.T) {
	s := Summarize(fixtureModel(), detector.P3DFormat{})
	require.Len(t, s.LODs[0].Proxies, 1)
	assert.Equal(t, "proxy_door.p3d", s.LODs[0].Proxies[0].ModelPath)
	assert.Equal(t, int32(1), s.LODs[0].Proxies[0].SequenceID)
	assert.Empty(t, s.LODs[1].Proxies)
}

func TestSummarizeConfig(t *testing.T) {
	s := Summarize(fixtureModel(), detector.P3DFormat{})
	require.Len(t, s.Skeletons, 1)
	assert.Equal(t, "test_skeleton", s.Skeletons[0].Name)
	assert.Equal(t, 2, s.Skeletons[0].BoneCount)

	require.Len(t, s.ModelClasses, 1)
	assert.Equal(t, "test_model", s.ModelClasses[0].Name)
	assert.Equal(t, 1, s.ModelClasses[0].AnimationCount)
	assert.Equal(t, []string{"camo"}, s.ModelClasses[0].Sections)
}

func TestSummarizeBoundingSizeGraphicalLOD(t *testing.T) {
	// LOD 0 is a graphical LOD (resolution 1.0 < 1000), so its bounding
	// size must be measured from its vertices.
	s := Summarize(fixtureModel(), detector.P3DFormat{})
	require.NotNil(t, s.LODs[0].BoundingSize)
	assert.Equal(t, model.Vector3{X: 2, Y: 4, Z: 4}, *s.LODs[0].BoundingSize)
}

func TestSummarizeBoundingSizeSkipsNonGraphicalLODs(t *testing.T) {
	// LOD 1 is a Geometry LOD (resolution 1e13): it must not be measured,
	// even though it has vertices.
	s := Summarize(fixtureModel(), detector.P3DFormat{})
	assert.Nil(t, s.LODs[1].BoundingSize)
}

func TestSummarizeMetadata(t *testing.T) {
	s := Summarize(fixtureModel(), detector.P3DFormat{Family: detector.P3DFamilyODOL, Version: 7})
	assert.Equal(t, "test_model", s.Name)
	assert.Equal(t, "ODOLv7", s.DetectedFormat)
	assert.Equal(t, "ODOL", s.SourceFamily)
	assert.Equal(t, uint32(7), s.SourceVersion)
	assert.Equal(t, model.Vector3{X: 1, Y: 2, Z: 3}, s.CenterOfMass)
}
