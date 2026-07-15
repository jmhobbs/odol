package fbxexport

// meshScene is the internal representation of a single-mesh FBX scene.
// It holds all data needed to emit an FBX file and is independent of
// the serialization format (ASCII or binary).
type meshScene struct {
	modelName string
	geometry  geometryData
	mesh      meshData
	materials []materialData
	proxies   []proxyData
}

// geometryData holds the mesh geometry arrays in FBX-ready form.
type geometryData struct {
	id        int64
	vertices  []float64   // x,y,z triplets, upcasted from float32
	polyIndex []int32     // FBX negative-terminated polygon vertex indices
	normals   []float64   // x,y,z per polygon vertex, dereferenced via NormalIndices
	uvSets    [][]float64 // one entry per UV channel; uvSets[0] is primary (Face.UVs), uvSets[1+] from LOD.UVSets[1+]
	matIndex  []int32     // per-polygon material index
}

// selectionData holds data for one named selection, emitted as a Null FBX Model.
type selectionData struct {
	id            int64
	name          string
	isSectional   bool
	vertexIndices []uint32
	vertexWeights []byte
	faceIndices   []uint32
}

// meshData holds the FBX Model node data for the exported mesh.
type meshData struct {
	id               int64
	sourceFamily     string
	sourceVersion    uint32
	lodResolution    float32
	odolProperties   map[string]string
	odolPropertyKeys []string // ordered keys, filtered
	selections       []selectionData
	iconColor        uint32
	selectedColor    uint32
}

// materialData holds data for one FBX Material node, plus the Video/Texture
// object pair (1:1:1 cardinality, matching canonical DayZ FBX exports) that
// links the material's texture so consumers like Blender/Substance can
// display it, instead of only carrying it as a custom string property.
type materialData struct {
	id        int64
	name      string
	texture   string
	mat       string
	videoID   int64
	textureID int64
}

// proxyData holds data for one ODOL proxy, emitted as a Null FBX Model.
type proxyData struct {
	id            int64
	name          string
	path          string
	sequenceID    int32
	selectionName string
	sectionIndex  int32
}
