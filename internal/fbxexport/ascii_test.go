package fbxexport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimalScene() *meshScene {
	return &meshScene{
		parts: []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
	}
}

func TestWriteASCIIHeader(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, minimalScene()))

	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "; FBX 7.4.0 project file"), "output should start with FBX header comment")
	assert.Contains(t, out, "FBXHeaderExtension:")
	assert.Contains(t, out, "FBXVersion: 7700")
	assert.Contains(t, out, `Creator: "odol-convert"`)
}

func TestWriteASCIIGlobalSettings(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, minimalScene()))

	out := buf.String()
	assert.Contains(t, out, "GlobalSettings:")
	// Property spacing follows the same "name, type, label, flags, value"
	// convention (space after each comma) as every other P: line in the
	// file - see plan_fbx-binary-export.md.
	assert.Contains(t, out, `P: "UpAxis", "int", "Integer", "", 1`)
	assert.Contains(t, out, `P: "UpAxisSign", "int", "Integer", "", 1`)
	assert.Contains(t, out, `P: "FrontAxis", "int", "Integer", "", 2`)
	assert.Contains(t, out, `P: "FrontAxisSign", "int", "Integer", "", 1`)
	assert.Contains(t, out, `P: "CoordAxis", "int", "Integer", "", 0`)
	assert.Contains(t, out, `P: "CoordAxisSign", "int", "Integer", "", 1`)
	assert.Contains(t, out, `P: "UnitScaleFactor", "double", "Number", "", 1.000000`)
}

func TestWriteASCIIDefinitionsNoMaterials(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, minimalScene()))

	out := buf.String()
	assert.Contains(t, out, "Definitions:")
	// 3 ObjectType entries when no materials: GlobalSettings, Geometry, Model
	assert.Contains(t, out, "\tCount: 3\n")
	assert.Contains(t, out, `ObjectType: "GlobalSettings"`)
	assert.Contains(t, out, `ObjectType: "Geometry"`)
	assert.Contains(t, out, `ObjectType: "Model"`)
	assert.NotContains(t, out, `ObjectType: "Material"`)
}

func TestWriteASCIIDefinitionsWithMaterials(t *testing.T) {
	s := &meshScene{
		parts:     []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		materials: []materialData{{id: 3, name: "Mat_0"}, {id: 4, name: "Mat_1"}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// 6 ObjectType entries: GlobalSettings, Geometry, Model, Material, Texture, Video
	assert.Contains(t, out, "\tCount: 6\n")
	assert.Contains(t, out, `ObjectType: "Material"`)
	assert.Contains(t, out, `ObjectType: "Texture"`)
	assert.Contains(t, out, `ObjectType: "Video"`)
	assert.Contains(t, out, "\t\tCount: 2\n")
}

func TestWriteASCIIDefinitionsModelCountIncludesProxies(t *testing.T) {
	s := &meshScene{
		parts:   []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		proxies: []proxyData{{id: 3}, {id: 4}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// Model count = 1 part + 2 proxies = 3
	assert.Contains(t, out, "\t\tCount: 3\n")
}

func TestWriteASCIIDefinitionsModelCountIncludesAllParts(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{
			{name: "object_NNN1", geomID: 1, modelID: 2},
			{name: "object_NNN2", geomID: 3, modelID: 4},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// One ObjectType entry each for Geometry and Model, both with count 2
	// (2 parts, no proxies/selections) - not one entry per part.
	assert.Equal(t, 2, strings.Count(out, "\t\tCount: 2\n"))
}

func TestWriteASCIIGeometryVertexArrays(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:    "object_NNN1",
			geomID:  1,
			modelID: 2,
			geometry: geometryData{
				vertices:  []float64{0, 0, 0, 1, 0, 0, 0, 1, 0},
				polyIndex: []int32{0, 1, -3},
				normals:   []float64{0, 0, 1, 0, 0, 1, 0, 0, 1},
				uvSets:    [][]float64{{0, 0, 1, 0, 0.5, 1}},
			},
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// 3 vertices * 3 components = 9 floats
	assert.Contains(t, out, "Vertices: *9 {")
	// 3 indices in one triangle
	assert.Contains(t, out, "PolygonVertexIndex: *3 {")
	// 3 per-polygon-vertex normals * 3 components = 9
	assert.Contains(t, out, "Normals: *9 {")
	// 3 per-polygon-vertex UVs * 2 components = 6
	assert.Contains(t, out, "UV: *6 {")
}

func TestWriteASCIISingleUVSetEmitsLayerElementUV0Only(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:     "object_NNN1",
			geomID:   1,
			modelID:  2,
			geometry: geometryData{uvSets: [][]float64{{0, 0, 1, 0, 0.5, 1}}},
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, "LayerElementUV: 0")
	assert.NotContains(t, out, "LayerElementUV: 1")
}

func TestWriteASCIITwoUVSetsEmitsBothLayerElements(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:     "object_NNN1",
			geomID:   1,
			modelID:  2,
			geometry: geometryData{uvSets: [][]float64{{0, 0, 1, 0}, {0.5, 0.5, 0.6, 0.6}}},
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, "LayerElementUV: 0")
	assert.Contains(t, out, "LayerElementUV: 1")
	assert.Contains(t, out, `Name: "UVChannel_1"`)
	assert.Contains(t, out, `Name: "UVChannel_2"`)
}

func TestWriteASCIITwoUVSetsLayerBlockContainsBothEntries(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:     "object_NNN1",
			geomID:   1,
			modelID:  2,
			geometry: geometryData{uvSets: [][]float64{{0, 0}, {1, 1}}},
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// Both TypedIndex entries must appear within the Layer: 0 block
	assert.Contains(t, out, "TypedIndex: 0")
	assert.Contains(t, out, "TypedIndex: 1")
}

func TestWriteASCIIUVArrayCountCorrectPerChannel(t *testing.T) {
	// 3 face-vertices per channel, 2 UV sets
	s := &meshScene{
		parts: []meshPart{{
			name:    "object_NNN1",
			geomID:  1,
			modelID: 2,
			geometry: geometryData{
				uvSets: [][]float64{{0, 0, 1, 0, 0.5, 1}, {0.1, 0.1, 0.2, 0.2, 0.3, 0.3}},
			},
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// 3 UV pairs * 2 components = 6, should appear twice (once per channel)
	assert.Equal(t, 2, strings.Count(out, "UV: *6 {"))
}

func TestWriteASCIIConnections(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:           "object_NNN1",
			geomID:         1,
			modelID:        2,
			localMaterials: []int{0},
		}},
		materials: []materialData{{id: 3, name: "Mat_0"}},
		proxies:   []proxyData{{id: 4, name: "Proxy_0"}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, "Connections:")
	// Geometry -> Model
	assert.Contains(t, out, `C: "OO",1,2`)
	// Model -> root (0)
	assert.Contains(t, out, `C: "OO",2,0`)
	// Material -> Model (this part uses it, per localMaterials)
	assert.Contains(t, out, `C: "OO",3,2`)
	// Proxy -> root (0) - proxies are scene-level, not attached to any one part
	assert.Contains(t, out, `C: "OO",4,0`)
}

func TestWriteASCIIMeshModelProperties(t *testing.T) {
	s := &meshScene{
		parts:         []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		sourceFamily:  "ODOL",
		sourceVersion: 7,
		lodResolution: 1.5,
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, `"Model::object_NNN1"`)
	assert.Contains(t, out, `"ODOL_SourceFamily", "KString", "", "", "ODOL"`)
	assert.Contains(t, out, `"ODOL_SourceVersion", "int", "Integer", "", 7`)
	assert.NotContains(t, out, `"ODOL_Selections"`)
}

func TestWriteASCIINoSelectionsProducesNoSelectionNodes(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, minimalScene()))

	out := buf.String()
	assert.NotContains(t, out, "ODOL_SelectionVertexIndices")
}

func TestWriteASCIISelectionNullNodeName(t *testing.T) {
	s := &meshScene{
		parts:      []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{{id: 3, name: "engine"}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, `"Model::engine", "Null"`)
}

func TestWriteASCIISelectionProperties(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{
			{
				id:            3,
				name:          "cargo",
				vertexIndices: []uint32{0, 2},
				vertexWeights: []byte{128, 255},
				faceIndices:   []uint32{1, 3},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, `"ODOL_SelectionVertexIndices", "KString", "", "", "0,2"`)
	assert.Contains(t, out, `"ODOL_SelectionVertexWeights", "KString", "", "", "128,255"`)
	assert.Contains(t, out, `"ODOL_SelectionFaceIndices", "KString", "", "", "1,3"`)
}

func TestWriteASCIISelectionIsSectional(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{
			{id: 3, name: "sect", isSectional: true},
			{id: 4, name: "nosect", isSectional: false},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, `"ODOL_SelectionIsSectional", "bool", "", "", 1`)
	assert.Contains(t, out, `"ODOL_SelectionIsSectional", "bool", "", "", 0`)
}

func TestWriteASCIISelectionEmptyIndicesEmitsEmptyString(t *testing.T) {
	s := &meshScene{
		parts:      []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{{id: 3, name: "empty"}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, `"ODOL_SelectionVertexIndices", "KString", "", "", ""`)
	assert.Contains(t, out, `"ODOL_SelectionFaceIndices", "KString", "", "", ""`)
}

func TestWriteASCIIDefinitionsModelCountIncludesSelections(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{
			{id: 5, name: "a"},
			{id: 6, name: "b"},
		},
		proxies: []proxyData{{id: 3}, {id: 4}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// Model count = 1 part + 2 proxies + 2 selections = 5
	assert.Contains(t, out, "\t\tCount: 5\n")
}

func TestWriteASCIIConnectionsIncludeSelections(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{
			{id: 3, name: "sel_a"},
			{id: 4, name: "sel_b"},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	// selections connect to root (0), not to any one part
	assert.Contains(t, out, `C: "OO",3,0`)
	assert.Contains(t, out, `C: "OO",4,0`)
}

func TestWriteASCIINoTrailingWhitespace(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, minimalScene()))

	for i, line := range strings.Split(buf.String(), "\n") {
		trimmed := strings.TrimRight(line, " \t")
		assert.Equal(t, trimmed, line, "line %d has trailing whitespace: %q", i+1, line)
	}
}

func TestWriteASCIILayerElementMaterialOmittedWhenNoMaterials(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, minimalScene()))

	out := buf.String()
	assert.NotContains(t, out, "LayerElementMaterial")
}

func TestWriteASCIIMaterialProperties(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:           "object_NNN1",
			geomID:         1,
			modelID:        2,
			geometry:       geometryData{matIndex: []int32{0}},
			localMaterials: []int{0},
		}},
		materials: []materialData{{
			id:      3,
			name:    "Mat_0",
			texture: `textures\barrel_co.paa`,
			mat:     "OFP2_ManBody",
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeASCII(&buf, s))

	out := buf.String()
	assert.Contains(t, out, `Material: 3,`)
	assert.Contains(t, out, `"Material::Mat_0"`)
	assert.Contains(t, out, `"ODOL_Texture", "KString", "", "", "textures\barrel_co.paa"`)
	assert.Contains(t, out, `"ODOL_Material", "KString", "", "", "OFP2_ManBody"`)
}
