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
	Flags         uint32
	Texture       string
	Material      string
}

type Selection struct {
	Name          string
	IsSectional   bool
	VertexIndices []uint32
	VertexWeights []byte
	FaceIndices   []uint32
}

type Proxy struct {
	ModelPath           string
	SequenceID          int32
	NamedSelectionIndex int32
	NamedSelectionName  string
	SectionIndex        int32
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
