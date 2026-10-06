package inspect

import (
	"sort"

	"github.com/jmhobbs/odol/internal/detector"
	"github.com/jmhobbs/odol/internal/model"
)

// partialParseProperty is the LOD property internal/odol sets when a LOD
// could not be fully decoded (see internal/convert.ensureFullyDecoded).
const partialParseProperty = "odol_partial_parse"

type SelectionSummary struct {
	Name        string `json:"name"`
	IsSectional bool   `json:"isSectional"`
	VertexCount int    `json:"vertexCount"`
	FaceCount   int    `json:"faceCount"`
}

type ProxySummary struct {
	ModelPath  string `json:"modelPath"`
	SequenceID int32  `json:"sequenceId"`
}

type LODSummary struct {
	Index          int     `json:"index"`
	Resolution     float32 `json:"resolution"`
	ResolutionName string  `json:"resolutionName"`
	Partial        bool    `json:"partial"`
	VertexCount    int     `json:"vertexCount"`
	FaceCount      int     `json:"faceCount"`
	UVSetCount     int     `json:"uvSetCount"`
	// BoundingSize is the (width, height, depth) extent, in meters, of this
	// LOD's vertices - the same bounding box cmd/fbx2mlod's conversion
	// confirmation prompt reports. Only measured for graphical LODs (see
	// IsGraphicalResolution); nil for functional/shadow/geometry/memory
	// LODs, where a bounding box isn't a meaningful measurement.
	BoundingSize *model.Vector3     `json:"boundingSize,omitempty"`
	Textures     []string           `json:"textures"`
	Materials    []string           `json:"materials"`
	Selections   []SelectionSummary `json:"selections"`
	Proxies      []ProxySummary     `json:"proxies"`
	Properties   map[string]string  `json:"properties,omitempty"`
}

type SkeletonSummary struct {
	Name      string `json:"name"`
	BoneCount int    `json:"boneCount"`
}

type ModelClassSummary struct {
	Name           string   `json:"name"`
	SkeletonName   string   `json:"skeletonName,omitempty"`
	Sections       []string `json:"sections,omitempty"`
	AnimationCount int      `json:"animationCount"`
}

// Summary is a curated, JSON-friendly report of a model.Model: everything
// p3dinspect prints, structured for scripting.
type Summary struct {
	Name           string              `json:"name"`
	DetectedFormat string              `json:"detectedFormat"`
	SourceFamily   string              `json:"sourceFamily"`
	SourceVersion  uint32              `json:"sourceVersion"`
	CenterOfMass   model.Vector3       `json:"centerOfMass"`
	Skeletons      []SkeletonSummary   `json:"skeletons,omitempty"`
	ModelClasses   []ModelClassSummary `json:"modelClasses,omitempty"`
	LODs           []LODSummary        `json:"lods"`
	Textures       []string            `json:"textures"`
	Materials      []string            `json:"materials"`
}

// Summarize builds a Summary of m for CLI/JSON reporting. format labels the
// on-disk format that produced m (from internal/detector).
func Summarize(m *model.Model, format detector.P3DFormat) Summary {
	s := Summary{
		Name:           m.Name,
		DetectedFormat: format.String(),
		SourceFamily:   m.Source.Family,
		SourceVersion:  m.Source.Version,
		CenterOfMass:   m.CenterOfMass,
		LODs:           make([]LODSummary, 0, len(m.LODs)),
	}

	allTextures := map[string]struct{}{}
	allMaterials := map[string]struct{}{}

	for i, lod := range m.LODs {
		lodSummary := summarizeLOD(i, lod)
		for _, tex := range lodSummary.Textures {
			allTextures[tex] = struct{}{}
		}
		for _, mat := range lodSummary.Materials {
			allMaterials[mat] = struct{}{}
		}
		s.LODs = append(s.LODs, lodSummary)
	}

	s.Textures = sortedKeys(allTextures)
	s.Materials = sortedKeys(allMaterials)

	for _, skel := range m.Config.Skeletons {
		s.Skeletons = append(s.Skeletons, SkeletonSummary{
			Name:      skel.Name,
			BoneCount: len(skel.Bones),
		})
	}

	for _, mc := range m.Config.Models {
		s.ModelClasses = append(s.ModelClasses, ModelClassSummary{
			Name:           mc.Name,
			SkeletonName:   mc.SkeletonName,
			Sections:       mc.Sections,
			AnimationCount: len(mc.Animations),
		})
	}

	return s
}

func summarizeLOD(index int, lod model.LOD) LODSummary {
	textures := map[string]struct{}{}
	materials := map[string]struct{}{}
	for _, face := range lod.Faces {
		if face.Texture != "" {
			textures[face.Texture] = struct{}{}
		}
		if face.Material != "" {
			materials[face.Material] = struct{}{}
		}
	}

	selections := make([]SelectionSummary, 0, len(lod.Selections))
	for _, sel := range lod.Selections {
		selections = append(selections, SelectionSummary{
			Name:        sel.Name,
			IsSectional: sel.IsSectional,
			VertexCount: len(sel.VertexIndices),
			FaceCount:   len(sel.FaceIndices),
		})
	}

	proxies := make([]ProxySummary, 0, len(lod.Proxies))
	for _, proxy := range lod.Proxies {
		proxies = append(proxies, ProxySummary{
			ModelPath:  proxy.ModelPath,
			SequenceID: proxy.SequenceID,
		})
	}

	_, partial := lod.Properties[partialParseProperty]

	var boundingSizeValue *model.Vector3
	if IsGraphicalResolution(lod.Resolution) {
		size := boundingSize(lod.Vertices)
		boundingSizeValue = &size
	}

	return LODSummary{
		Index:          index,
		Resolution:     lod.Resolution,
		ResolutionName: ResolutionName(lod.Resolution),
		Partial:        partial,
		VertexCount:    len(lod.Vertices),
		FaceCount:      len(lod.Faces),
		UVSetCount:     len(lod.UVSets),
		BoundingSize:   boundingSizeValue,
		Textures:       sortedKeys(textures),
		Materials:      sortedKeys(materials),
		Selections:     selections,
		Proxies:        proxies,
		Properties:     lod.Properties,
	}
}

// boundingSize returns the (width, height, depth) extent of vertices along
// X, Y, Z respectively (MLOD/ODOL use Y as the up axis) - the same min/max
// sweep cmd/fbx2mlod's preview.boundingBoxSize performs on imported FBX
// vertices. Returns the zero value for an empty slice.
func boundingSize(vertices []model.Vector3) model.Vector3 {
	if len(vertices) == 0 {
		return model.Vector3{}
	}
	min, max := vertices[0], vertices[0]
	for _, v := range vertices[1:] {
		if v.X < min.X {
			min.X = v.X
		}
		if v.Y < min.Y {
			min.Y = v.Y
		}
		if v.Z < min.Z {
			min.Z = v.Z
		}
		if v.X > max.X {
			max.X = v.X
		}
		if v.Y > max.Y {
			max.Y = v.Y
		}
		if v.Z > max.Z {
			max.Z = v.Z
		}
	}
	return model.Vector3{X: max.X - min.X, Y: max.Y - min.Y, Z: max.Z - min.Z}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
