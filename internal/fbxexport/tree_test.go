package fbxexport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findChild(n *node, name string) *node {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func findChildren(n *node, name string) []*node {
	var out []*node
	for _, c := range n.children {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

func findTop(nodes []*node, name string) *node {
	for _, n := range nodes {
		if n.name == name {
			return n
		}
	}
	return nil
}

var testTime = time.Date(2026, 7, 13, 11, 30, 45, 250_000_000, time.UTC)

func TestBuildDocumentNodesHeaderExtension(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	hdr := findTop(nodes, "FBXHeaderExtension")
	require.NotNil(t, hdr)
	require.True(t, hdr.isList())

	assert.Equal(t, []any{int32(1004)}, findChild(hdr, "FBXHeaderVersion").props)
	assert.Equal(t, []any{int32(fbxBinaryVersion)}, findChild(hdr, "FBXVersion").props)
	assert.Equal(t, []any{"odol-convert"}, findChild(hdr, "Creator").props)

	ts := findChild(hdr, "CreationTimeStamp")
	require.NotNil(t, ts)
	assert.Equal(t, []any{int32(2026)}, findChild(ts, "Year").props)
	assert.Equal(t, []any{int32(7)}, findChild(ts, "Month").props)
	assert.Equal(t, []any{int32(13)}, findChild(ts, "Day").props)
	assert.Equal(t, []any{int32(11)}, findChild(ts, "Hour").props)
	assert.Equal(t, []any{int32(30)}, findChild(ts, "Minute").props)
	assert.Equal(t, []any{int32(45)}, findChild(ts, "Second").props)
	assert.Equal(t, []any{int32(250)}, findChild(ts, "Millisecond").props)

	assert.Equal(t, []any{int32(0)}, findChild(hdr, "EncryptionType").props)
	otherFlags := findChild(hdr, "OtherFlags")
	require.NotNil(t, otherFlags)
	assert.Equal(t, []any{int32(127)}, findChild(otherFlags, "TCDefinition").props)
}

func TestBuildDocumentNodesRootLevelFileIdCreationTimeCreator(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)

	fileID := findTop(nodes, "FileId")
	require.NotNil(t, fileID)
	require.Len(t, fileID.props, 1)
	raw, ok := fileID.props[0].([]byte)
	require.True(t, ok, "FileId prop should be []byte")
	assert.Len(t, raw, 16)

	assert.Equal(t, []any{"2026-07-13 11:30:45:250"}, findTop(nodes, "CreationTime").props)
	assert.Equal(t, []any{"odol-convert"}, findTop(nodes, "Creator").props)
}

func TestBuildDocumentNodesFileIDIsRandomPerCall(t *testing.T) {
	a := findTop(buildDocumentNodes(minimalScene(), testTime), "FileId").props[0].([]byte)
	b := findTop(buildDocumentNodes(minimalScene(), testTime), "FileId").props[0].([]byte)
	assert.NotEqual(t, a, b)
}

func TestBuildDocumentNodesTakesIsEmptyAndLast(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	require.Equal(t, "Takes", nodes[len(nodes)-1].name)
	takes := nodes[len(nodes)-1]
	assert.Equal(t, []any{""}, findChild(takes, "Current").props)
}

func TestBuildDocumentNodesGlobalSettings(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	gs := findTop(nodes, "GlobalSettings")
	require.NotNil(t, gs)
	props70 := findChild(gs, "Properties70")
	require.NotNil(t, props70)
	require.True(t, props70.isList())

	want := map[string][]any{
		"UpAxis":          {"UpAxis", "int", "Integer", "", int32(1)},
		"UpAxisSign":      {"UpAxisSign", "int", "Integer", "", int32(1)},
		"FrontAxis":       {"FrontAxis", "int", "Integer", "", int32(2)},
		"FrontAxisSign":   {"FrontAxisSign", "int", "Integer", "", int32(1)},
		"CoordAxis":       {"CoordAxis", "int", "Integer", "", int32(0)},
		"CoordAxisSign":   {"CoordAxisSign", "int", "Integer", "", int32(1)},
		"UnitScaleFactor": {"UnitScaleFactor", "double", "Number", "", float64(1)},
	}
	require.Len(t, props70.children, len(want))
	for _, p := range props70.children {
		name, ok := p.props[0].(string)
		require.True(t, ok)
		assert.Equal(t, want[name], p.props, "property %s", name)
	}
}

func TestBuildDocumentNodesReferencesIsEmptyList(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	refs := findTop(nodes, "References")
	require.NotNil(t, refs)
	assert.True(t, refs.isList())
	assert.Len(t, refs.children, 0)
}

func TestBuildDocumentNodesDefinitionsNoMaterials(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	defs := findTop(nodes, "Definitions")
	require.NotNil(t, defs)
	assert.Equal(t, []any{int32(3)}, findChild(defs, "Count").props)
	assert.Nil(t, findObjectType(defs, "Material"))

	model := findObjectType(defs, "Model")
	require.NotNil(t, model)
	assert.Equal(t, []any{int32(1)}, findChild(model, "Count").props)
}

func findObjectType(defs *node, typeName string) *node {
	for _, c := range defs.children {
		if c.name != "ObjectType" {
			continue
		}
		if len(c.props) == 1 && c.props[0] == typeName {
			return c
		}
	}
	return nil
}

func TestBuildDocumentNodesDefinitionsWithMaterials(t *testing.T) {
	s := &meshScene{
		parts:     []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		materials: []materialData{{id: 3, name: "Mat_0"}, {id: 4, name: "Mat_1"}},
	}
	nodes := buildDocumentNodes(s, testTime)
	defs := findTop(nodes, "Definitions")
	assert.Equal(t, []any{int32(6)}, findChild(defs, "Count").props)
	mat := findObjectType(defs, "Material")
	require.NotNil(t, mat)
	assert.Equal(t, []any{int32(2)}, findChild(mat, "Count").props)
}

func TestBuildDocumentNodesDefinitionsModelCountIncludesProxiesAndSelections(t *testing.T) {
	s := &meshScene{
		parts:      []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{{id: 5, name: "a"}, {id: 6, name: "b"}},
		proxies:    []proxyData{{id: 3}, {id: 4}},
	}
	nodes := buildDocumentNodes(s, testTime)
	defs := findTop(nodes, "Definitions")
	model := findObjectType(defs, "Model")
	// 1 part + 2 proxies + 2 selections = 5
	assert.Equal(t, []any{int32(5)}, findChild(model, "Count").props)
}

func TestBuildDocumentNodesDefinitionsGeometryCountMatchesPartCount(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{
			{name: "object_NNN1", geomID: 1, modelID: 2},
			{name: "object_NNN2", geomID: 3, modelID: 4},
			{name: "object_NNN3", geomID: 5, modelID: 6},
		},
	}
	nodes := buildDocumentNodes(s, testTime)
	defs := findTop(nodes, "Definitions")
	geom := findObjectType(defs, "Geometry")
	require.NotNil(t, geom)
	assert.Equal(t, []any{int32(3)}, findChild(geom, "Count").props)
}

func TestBuildDocumentNodesGeometryArrays(t *testing.T) {
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
	nodes := buildDocumentNodes(s, testTime)
	objs := findTop(nodes, "Objects")
	geom := findChild(objs, "Geometry")
	require.NotNil(t, geom)
	assert.Equal(t, []any{[]float64{0, 0, 0, 1, 0, 0, 0, 1, 0}}, findChild(geom, "Vertices").props)
	assert.Equal(t, []any{[]int32{0, 1, -3}}, findChild(geom, "PolygonVertexIndex").props)

	normElem := findChild(geom, "LayerElementNormal")
	require.NotNil(t, normElem)
	assert.Equal(t, []any{[]float64{0, 0, 1, 0, 0, 1, 0, 0, 1}}, findChild(normElem, "Normals").props)

	uvElems := findChildren(geom, "LayerElementUV")
	require.Len(t, uvElems, 1)
	assert.Equal(t, []any{[]float64{0, 0, 1, 0, 0.5, 1}}, findChild(uvElems[0], "UV").props)

	assert.Equal(t, []any{int32(124)}, findChild(geom, "GeometryVersion").props)
	assert.Equal(t, []any{int32(102)}, findChild(normElem, "Version").props)
}

func TestBuildDocumentNodesTwoUVSetsProduceTwoLayerElementsAndLayerEntries(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:     "object_NNN1",
			geomID:   1,
			modelID:  2,
			geometry: geometryData{uvSets: [][]float64{{0, 0}, {1, 1}}},
		}},
	}
	nodes := buildDocumentNodes(s, testTime)
	geom := findChild(findTop(nodes, "Objects"), "Geometry")
	uvElems := findChildren(geom, "LayerElementUV")
	require.Len(t, uvElems, 2)
	assert.Equal(t, []any{int32(0)}, uvElems[0].props)
	assert.Equal(t, []any{int32(1)}, uvElems[1].props)
	assert.Equal(t, []any{"UVChannel_1"}, findChild(uvElems[0], "Name").props)
	assert.Equal(t, []any{"UVChannel_2"}, findChild(uvElems[1], "Name").props)

	layer := findChild(geom, "Layer")
	require.NotNil(t, layer)
	layerElems := findChildren(layer, "LayerElement")
	// normal + 2 UV sets = 3
	require.Len(t, layerElems, 3)
}

func TestBuildDocumentNodesLayerElementMaterialOmittedWhenNoMaterials(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	geom := findChild(findTop(nodes, "Objects"), "Geometry")
	assert.Nil(t, findChild(geom, "LayerElementMaterial"))
}

func TestBuildDocumentNodesLayerElementMaterialPresentWithMaterials(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{
			name:           "object_NNN1",
			geomID:         1,
			modelID:        2,
			geometry:       geometryData{matIndex: []int32{0, 1}},
			localMaterials: []int{0, 1},
		}},
		materials: []materialData{{id: 3, name: "Mat_0"}, {id: 4, name: "Mat_1"}},
	}
	nodes := buildDocumentNodes(s, testTime)
	geom := findChild(findTop(nodes, "Objects"), "Geometry")
	matElem := findChild(geom, "LayerElementMaterial")
	require.NotNil(t, matElem)
	assert.Equal(t, []any{[]int32{0, 1}}, findChild(matElem, "Materials").props)
}

func TestBuildDocumentNodesMeshModelProperties(t *testing.T) {
	s := &meshScene{
		parts:         []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		sourceFamily:  "ODOL",
		sourceVersion: 7,
		lodResolution: 1.5,
	}
	nodes := buildDocumentNodes(s, testTime)
	model := findChild(findTop(nodes, "Objects"), "Model")
	require.NotNil(t, model)
	assert.Equal(t, []any{int64(2), "Model::object_NNN1", "Mesh"}, model.props)

	props70 := findChild(model, "Properties70")
	require.NotNil(t, props70)
	assert.Equal(t, []any{"ScalingMax", "Vector3D", "Vector", "", int32(0), int32(0), int32(0)}, findPropByName(props70, "ScalingMax"))
	assert.Equal(t, []any{"ODOL_SourceFamily", "KString", "", "", "ODOL"}, findPropByName(props70, "ODOL_SourceFamily"))
	assert.Equal(t, []any{"ODOL_SourceVersion", "int", "Integer", "", int64(7)}, findPropByName(props70, "ODOL_SourceVersion"))
	assert.Equal(t, []any{"ODOL_LODResolution", "double", "Number", "", float64(float32(1.5))}, findPropByName(props70, "ODOL_LODResolution"))

	assert.Equal(t, []any{shadingFlag(true)}, findChild(model, "Shading").props)
	assert.Equal(t, []any{"CullingOff"}, findChild(model, "Culling").props)
}

func TestBuildDocumentNodesMultiplePartsGetSequentialNames(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{
			{name: "object_NNN1", geomID: 1, modelID: 2},
			{name: "object_NNN2", geomID: 3, modelID: 4},
		},
	}
	nodes := buildDocumentNodes(s, testTime)
	models := findChildren(findTop(nodes, "Objects"), "Model")
	require.Len(t, models, 2)
	assert.Equal(t, []any{int64(2), "Model::object_NNN1", "Mesh"}, models[0].props)
	assert.Equal(t, []any{int64(4), "Model::object_NNN2", "Mesh"}, models[1].props)

	geoms := findChildren(findTop(nodes, "Objects"), "Geometry")
	require.Len(t, geoms, 2)
	assert.Equal(t, []any{int64(1), "Geometry::object_NNN1", "Mesh"}, geoms[0].props)
	assert.Equal(t, []any{int64(3), "Geometry::object_NNN2", "Mesh"}, geoms[1].props)
}

func findPropByName(props70 *node, name string) []any {
	for _, p := range props70.children {
		if s, ok := p.props[0].(string); ok && s == name {
			return p.props
		}
	}
	return nil
}

func testMaterialScene() *meshScene {
	return &meshScene{
		parts: []meshPart{{
			name:           "object_NNN1",
			geomID:         1,
			modelID:        2,
			geometry:       geometryData{matIndex: []int32{0}},
			localMaterials: []int{0},
		}},
		materials: []materialData{{
			id:        3,
			name:      "Mat_0",
			texture:   `textures\barrel_co.paa`,
			mat:       "OFP2_ManBody",
			videoID:   10,
			textureID: 11,
		}},
	}
}

func TestBuildDocumentNodesMaterialProperties(t *testing.T) {
	nodes := buildDocumentNodes(testMaterialScene(), testTime)
	mat := findChild(findTop(nodes, "Objects"), "Material")
	require.NotNil(t, mat)
	assert.Equal(t, []any{int64(3), "Material::Mat_0", ""}, mat.props)
	assert.Equal(t, []any{"phong"}, findChild(mat, "ShadingModel").props)

	props70 := findChild(mat, "Properties70")
	assert.Equal(t, []any{"Emissive", "Vector3D", "Vector", "", float64(0), float64(0), float64(0)}, findPropByName(props70, "Emissive"))
	assert.Equal(t, []any{"Ambient", "Vector3D", "Vector", "", 0.2, 0.2, 0.2}, findPropByName(props70, "Ambient"))
	assert.Equal(t, []any{"Diffuse", "Vector3D", "Vector", "", 0.8, 0.8, 0.8}, findPropByName(props70, "Diffuse"))
	assert.Equal(t, []any{"Specular", "Vector3D", "Vector", "", 0.2, 0.2, 0.2}, findPropByName(props70, "Specular"))
	assert.Equal(t, []any{"Shininess", "double", "Number", "", float64(20)}, findPropByName(props70, "Shininess"))
	assert.Equal(t, []any{"Opacity", "double", "Number", "", float64(1)}, findPropByName(props70, "Opacity"))
	assert.Equal(t, []any{"Reflectivity", "double", "Number", "", float64(0)}, findPropByName(props70, "Reflectivity"))
	assert.Equal(t, []any{"ODOL_Texture", "KString", "", "", `textures\barrel_co.paa`}, findPropByName(props70, "ODOL_Texture"))
	assert.Equal(t, []any{"ODOL_Material", "KString", "", "", "OFP2_ManBody"}, findPropByName(props70, "ODOL_Material"))
}

func TestBuildDocumentNodesVideoNode(t *testing.T) {
	nodes := buildDocumentNodes(testMaterialScene(), testTime)
	video := findChild(findTop(nodes, "Objects"), "Video")
	require.NotNil(t, video)
	assert.Equal(t, int64(10), video.props[0])
	assert.Equal(t, "Clip", video.props[2])
	assert.Equal(t, []any{"Clip"}, findChild(video, "Type").props)
	assert.Equal(t, []any{""}, findChild(video, "Filename").props)
	assert.Equal(t, []any{`textures\barrel_co.paa`}, findChild(video, "RelativeFilename").props)

	props70 := findChild(video, "Properties70")
	require.NotNil(t, props70)
	assert.Equal(t, []any{"RelPath", "KString", "XRefUrl", "", `textures\barrel_co.paa`}, findPropByName(props70, "RelPath"))
}

func TestBuildDocumentNodesTextureNode(t *testing.T) {
	nodes := buildDocumentNodes(testMaterialScene(), testTime)
	tex := findChild(findTop(nodes, "Objects"), "Texture")
	require.NotNil(t, tex)
	assert.Equal(t, int64(11), tex.props[0])
	assert.Equal(t, "Texture::Mat_0", tex.props[1])

	assert.Equal(t, []any{"TextureVideoClip"}, findChild(tex, "Type").props)
	assert.Equal(t, []any{int32(202)}, findChild(tex, "Version").props)
	assert.Equal(t, []any{`textures\barrel_co.paa`}, findChild(tex, "RelativeFilename").props)
	assert.Equal(t, []any{float64(0), float64(0)}, findChild(tex, "ModelUVTranslation").props)
	assert.Equal(t, []any{float64(1), float64(1)}, findChild(tex, "ModelUVScaling").props)
	assert.Equal(t, []any{"None"}, findChild(tex, "Texture_Alpha_Source").props)
	assert.Equal(t, []any{int32(0), int32(0), int32(0), int32(0)}, findChild(tex, "Cropping").props)

	// Media references the Video node by "Class::Name"-style identifier -
	// gets the same binary encoding treatment as the node identity props.
	assert.Equal(t, []any{"Video::Mat_0_Video"}, findChild(tex, "Media").props)

	props70 := findChild(tex, "Properties70")
	require.NotNil(t, props70)
	assert.Equal(t, []any{"UVSet", "KString", "", "", "UVChannel_1"}, findPropByName(props70, "UVSet"))
}

func TestBuildDocumentNodesNoMaterialsEmitsNoVideoOrTexture(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	objects := findTop(nodes, "Objects")
	assert.Nil(t, findChild(objects, "Video"))
	assert.Nil(t, findChild(objects, "Texture"))
}

func TestBuildDocumentNodesTextureVideoConnections(t *testing.T) {
	nodes := buildDocumentNodes(testMaterialScene(), testTime)
	conns := findTop(nodes, "Connections")
	require.NotNil(t, conns)

	var got [][]any
	for _, c := range conns.children {
		got = append(got, c.props)
	}
	assert.Contains(t, got, []any{"OO", int64(10), int64(11)}, "Video -> Texture")
	assert.Contains(t, got, []any{"OP", int64(11), int64(3), "DiffuseColor"}, "Texture -> Material.DiffuseColor")
}

func TestBuildDocumentNodesDefinitionsIncludesTextureAndVideoObjectTypes(t *testing.T) {
	nodes := buildDocumentNodes(testMaterialScene(), testTime)
	defs := findTop(nodes, "Definitions")
	// 3 base (GlobalSettings, Geometry, Model) + 3 for materials (Material, Texture, Video)
	assert.Equal(t, []any{int32(6)}, findChild(defs, "Count").props)

	tex := findObjectType(defs, "Texture")
	require.NotNil(t, tex)
	assert.Equal(t, []any{int32(1)}, findChild(tex, "Count").props)

	video := findObjectType(defs, "Video")
	require.NotNil(t, video)
	assert.Equal(t, []any{int32(1)}, findChild(video, "Count").props)
}

func TestBuildDocumentNodesProxyModelProperties(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		proxies: []proxyData{{
			id:            3,
			name:          "Proxy_0",
			path:          `\ca\weapons\proxy`,
			sequenceID:    5,
			selectionName: "sel",
			sectionIndex:  9,
		}},
	}
	nodes := buildDocumentNodes(s, testTime)
	// mesh model and proxy model are both "Model" nodes; proxy is second.
	models := findChildren(findTop(nodes, "Objects"), "Model")
	require.Len(t, models, 2)
	proxy := models[1]
	assert.Equal(t, []any{int64(3), "Model::Proxy_0", "Null"}, proxy.props)
	props70 := findChild(proxy, "Properties70")
	assert.Equal(t, []any{"ODOL_ProxyPath", "KString", "", "", `\ca\weapons\proxy`}, findPropByName(props70, "ODOL_ProxyPath"))
	assert.Equal(t, []any{"ODOL_ProxySequenceID", "int", "Integer", "", int64(5)}, findPropByName(props70, "ODOL_ProxySequenceID"))
	assert.Equal(t, []any{"ODOL_ProxySelectionName", "KString", "", "", "sel"}, findPropByName(props70, "ODOL_ProxySelectionName"))
	assert.Equal(t, []any{"ODOL_ProxySectionIndex", "int", "Integer", "", int64(9)}, findPropByName(props70, "ODOL_ProxySectionIndex"))
}

func TestBuildDocumentNodesSelectionModelProperties(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{
			{
				id:            3,
				name:          "cargo",
				isSectional:   true,
				vertexIndices: []uint32{0, 2},
				vertexWeights: []byte{128, 255},
				faceIndices:   []uint32{1, 3},
			},
		},
	}
	nodes := buildDocumentNodes(s, testTime)
	sel := findChildren(findTop(nodes, "Objects"), "Model")[1]
	assert.Equal(t, []any{int64(3), "Model::cargo", "Null"}, sel.props)
	props70 := findChild(sel, "Properties70")
	assert.Equal(t, []any{"ODOL_SelectionIsSectional", "bool", "", "", int64(1)}, findPropByName(props70, "ODOL_SelectionIsSectional"))
	assert.Equal(t, []any{"ODOL_SelectionVertexIndices", "KString", "", "", "0,2"}, findPropByName(props70, "ODOL_SelectionVertexIndices"))
	assert.Equal(t, []any{"ODOL_SelectionVertexWeights", "KString", "", "", "128,255"}, findPropByName(props70, "ODOL_SelectionVertexWeights"))
	assert.Equal(t, []any{"ODOL_SelectionFaceIndices", "KString", "", "", "1,3"}, findPropByName(props70, "ODOL_SelectionFaceIndices"))
}

func TestBuildDocumentNodesSelectionEmptyIndicesEmitEmptyString(t *testing.T) {
	s := &meshScene{
		parts:      []meshPart{{name: "object_NNN1", geomID: 1, modelID: 2}},
		selections: []selectionData{{id: 3, name: "empty"}},
	}
	nodes := buildDocumentNodes(s, testTime)
	sel := findChildren(findTop(nodes, "Objects"), "Model")[1]
	props70 := findChild(sel, "Properties70")
	assert.Equal(t, []any{"ODOL_SelectionVertexIndices", "KString", "", "", ""}, findPropByName(props70, "ODOL_SelectionVertexIndices"))
	assert.Equal(t, []any{"ODOL_SelectionFaceIndices", "KString", "", "", ""}, findPropByName(props70, "ODOL_SelectionFaceIndices"))
}

func TestBuildDocumentNodesConnections(t *testing.T) {
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
	nodes := buildDocumentNodes(s, testTime)
	conns := findTop(nodes, "Connections")
	require.NotNil(t, conns)
	require.True(t, conns.isList())

	var got [][]any
	for _, c := range conns.children {
		got = append(got, c.props)
	}
	assert.Contains(t, got, []any{"OO", int64(1), int64(2)}, "geometry -> model")
	assert.Contains(t, got, []any{"OO", int64(2), int64(0)}, "model -> root")
	assert.Contains(t, got, []any{"OO", int64(3), int64(2)}, "material -> model")
	assert.Contains(t, got, []any{"OO", int64(4), int64(0)}, "proxy -> root")
}

func TestBuildDocumentNodesSharedMaterialConnectsToEveryPartThatUsesIt(t *testing.T) {
	s := &meshScene{
		parts: []meshPart{
			{name: "object_NNN1", geomID: 1, modelID: 2, localMaterials: []int{0}},
			{name: "object_NNN2", geomID: 3, modelID: 4, localMaterials: []int{0}},
		},
		materials: []materialData{{id: 5, name: "Mat_0"}},
	}
	nodes := buildDocumentNodes(s, testTime)
	conns := findTop(nodes, "Connections")
	require.NotNil(t, conns)

	var got [][]any
	for _, c := range conns.children {
		got = append(got, c.props)
	}
	// One shared material connects to both parts' Model nodes - matching
	// the mich2001.fbx reference case (one Material, 163 Model nodes).
	assert.Contains(t, got, []any{"OO", int64(5), int64(2)}, "material -> part 1's model")
	assert.Contains(t, got, []any{"OO", int64(5), int64(4)}, "material -> part 2's model")
}

func TestBuildDocumentNodesDocuments(t *testing.T) {
	nodes := buildDocumentNodes(minimalScene(), testTime)
	docs := findTop(nodes, "Documents")
	require.NotNil(t, docs)
	assert.Equal(t, []any{int32(1)}, findChild(docs, "Count").props)
	doc := findChild(docs, "Document")
	require.NotNil(t, doc)
	assert.Equal(t, []any{int64(1000000000), "", "Scene"}, doc.props)
	assert.Equal(t, []any{int64(0)}, findChild(doc, "RootNode").props)
}
