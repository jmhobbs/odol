package fbxexport

// meshScene is the internal representation of an FBX scene. It holds all
// data needed to emit an FBX file and is independent of the serialization
// format (ASCII or binary).
//
// The exported LOD is split into one or more parts (connected components of
// its faces - see components.go), each becoming its own Geometry+Model pair
// so a DCC tool can select and texture it independently, matching how
// canonical DayZ FBX exports are structured (confirmed against
// samples/fbx/mich2001.fbx and samples/fbx/sv98.fbx). Materials, proxies,
// and named selections are scene-level: they're shared across parts rather
// than duplicated per part.
type meshScene struct {
	parts            []meshPart
	materials        []materialData
	proxies          []proxyData
	selections       []selectionData
	sourceFamily     string
	sourceVersion    uint32
	lodResolution    float32
	odolProperties   map[string]string
	odolPropertyKeys []string // ordered keys, filtered
	iconColor        uint32
	selectedColor    uint32
}

// meshPart is one exported mesh object: a connected component of the source
// LOD, named "object_NNN{i}" (1-based, in component order) to match the
// canonical naming convention. localMaterials holds the indices into
// meshScene.materials this part actually uses, in local material-slot order
// - geometry.matIndex values index into this slice, not directly into
// meshScene.materials, matching standard FBX LayerElementMaterial semantics
// (IndexToDirect against the materials connected to this specific Model).
type meshPart struct {
	name           string
	geomID         int64
	modelID        int64
	geometry       geometryData
	localMaterials []int
}

// geometryData holds the mesh geometry arrays in FBX-ready form.
type geometryData struct {
	vertices  []float64   // x,y,z triplets, upcasted from float32
	polyIndex []int32     // FBX negative-terminated polygon vertex indices
	normals   []float64   // x,y,z per polygon vertex, dereferenced via NormalIndices
	uvSets    [][]float64 // one entry per UV channel; uvSets[0] is primary (Face.UVs), uvSets[1+] from LOD.UVSets[1+]
	matIndex  []int32     // per-polygon index into the owning part's localMaterials
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
