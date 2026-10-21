package model

type Vector3 struct {
	X float32
	Y float32
	Z float32
}

type UV struct {
	U float32
	V float32
}

type Face struct {
	Indices       []uint32
	NormalIndices []uint32
	UVs           []UV
	// UVSets[k] holds UV set k (k >= 1) for each corner, in Indices order;
	// UVSets[0] is unused because UVs holds the primary set. Only MLOD fills
	// it: MLOD stores extra UV sets per corner, so seams live here, while the
	// per-vertex LOD.UVSets copy keeps only the first value at each vertex.
	// ODOL splits vertices at seams, so its LOD.UVSets are already exact.
	UVSets   [][]UV
	Flags    uint32
	Texture  string
	Material string
}

type Selection struct {
	Name          string
	IsSectional   bool
	VertexIndices []uint32
	VertexWeights []byte
	FaceIndices   []uint32
}

type Proxy struct {
	ModelPath string
	// Transform places the proxied model in this model's space (the same
	// space as LOD.Vertices): rows 0-2 are its X, Y and Z axes, row 3 its
	// position. Only ODOL stores it; see the proxy package for MLOD.
	Transform           [4]Vector3
	SequenceID          int32
	NamedSelectionIndex int32
	NamedSelectionName  string
	SectionIndex        int32
}

// Material is the copy of an .rvmat that binarization embeds in an ODOL LOD.
// The engine renders with this copy, not with the .rvmat file. Faces refer to
// it by name through Face.Material.
type Material struct {
	Name          string
	Type          uint32 // embedded format version; DayZ writes 20
	Emissive      [4]float32
	Ambient       [4]float32
	Diffuse       [4]float32
	ForcedDiffuse [4]float32
	Specular      [4]float32
	Specular2     [4]float32
	SpecularPower float32
	PixelShader   uint32 // engine enum, see PixelShaderName
	VertexShader  uint32 // engine enum, see VertexShaderName
	MainLight     uint32
	FogMode       uint32
	Surface       string
	RenderFlags   uint32 // RenderFlag bits
	// Stages[n] is the rvmat's StageN, including Stage0. Binarization fills
	// stages the rvmat leaves out with defaults.
	Stages  []MaterialStage
	TexGens []TexGen
	StageTI *MaterialStage
	// Partial is set when only the name could be read; every other field is
	// then zero.
	Partial bool
}

type MaterialStage struct {
	Texture string
	Filter  uint32
	// TexGen indexes Material.TexGens. Default stages can point past the end
	// of it, which means the identity transform on the first UV set.
	TexGen uint32
	Extra  uint8
}

type TexGen struct {
	// UVSource follows the engine's enum, not the UV set index; see the
	// UVSource constants.
	UVSource  uint32
	Transform [4]Vector3 // aside, up, dir, pos
}

type LOD struct {
	Resolution    float32
	Vertices      []Vector3
	Normals       []Vector3
	Faces         []Face
	UVSets        [][]UV
	Selections    []Selection
	Properties    map[string]string
	PropertyOrder []string
	PointMasses   []float32
	Proxies       []Proxy
	IconColor     uint32
	SelectedColor uint32
	Materials     []Material
	// Tangents and Bitangents are ODOL's per-vertex ST pairs: the S and T
	// axes of the tangent space normal maps are sampled in. A zero tangent
	// means the vertex has no frame (e.g. degenerate UVs). Empty for MLOD.
	Tangents   []Vector3
	Bitangents []Vector3
}

// MaterialByName returns the embedded material a face names, or nil when the
// LOD has none by that name (always the case for MLOD).
func (l *LOD) MaterialByName(name string) *Material {
	for i := range l.Materials {
		if l.Materials[i].Name == name {
			return &l.Materials[i]
		}
	}
	return nil
}

type SourceFormat struct {
	Family  string
	Version uint32
}

type SkeletonBone struct {
	Name   string
	Parent string
}

type Animation struct {
	ClassName     string
	Type          string
	Source        string
	Selection     string
	Axis          string
	Memory        *bool
	SourceAddress string
	MinValue      float32
	MaxValue      float32
	Angle0        *float32
	Angle1        *float32
	Offset0       *float32
	Offset1       *float32
	HideValue     *float32
	AxisPosition  *Vector3
	AxisDirection *Vector3
}

type ModelClass struct {
	Name            string
	Parent          string
	Sections        []string
	SectionsInherit string
	SkeletonName    string
	Animations      []Animation
}

type SkeletonClass struct {
	Name            string
	Parent          string
	SkeletonInherit string
	Bones           []SkeletonBone
	IsDiscrete      *bool
	PivotsModel     string
}

type Config struct {
	Skeletons []SkeletonClass
	Models    []ModelClass
}

type Model struct {
	Name         string
	Source       SourceFormat
	CenterOfMass Vector3
	LODs         []LOD
	Config       Config
}
