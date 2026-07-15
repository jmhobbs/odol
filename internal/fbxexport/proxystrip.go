package fbxexport

import (
	"strings"

	"github.com/jmhobbs/odol/internal/model"
)

const proxySelectionPrefix = "proxy:"

// stripProxySelections removes proxy placeholder selections (named
// "proxy:...", see docs/P3D File Format - MLOD.txt) and the faces/vertices
// they reference from lod, since that geometry is a leftover marker for the
// proxy's local transform and is redundant with model.LOD.Proxies, which is
// exported separately as Null nodes. It returns a new LOD; lod is not
// mutated. If lod has no proxy-prefixed selections, lod is returned as-is.
func stripProxySelections(lod model.LOD) model.LOD {
	var proxySelections, keptSelections []model.Selection
	for _, sel := range lod.Selections {
		if strings.HasPrefix(sel.Name, proxySelectionPrefix) {
			proxySelections = append(proxySelections, sel)
			continue
		}
		keptSelections = append(keptSelections, sel)
	}
	if len(proxySelections) == 0 {
		return lod
	}

	removedFaces := make(map[uint32]struct{})
	for _, sel := range proxySelections {
		for _, fi := range sel.FaceIndices {
			removedFaces[fi] = struct{}{}
		}
	}

	faceRemap := make(map[uint32]uint32, len(lod.Faces))
	keptFaces := make([]model.Face, 0, len(lod.Faces))
	for i, face := range lod.Faces {
		if _, removed := removedFaces[uint32(i)]; removed {
			continue
		}
		faceRemap[uint32(i)] = uint32(len(keptFaces))
		keptFaces = append(keptFaces, face)
	}

	usedVertex := make(map[uint32]struct{})
	for _, face := range keptFaces {
		for _, vi := range face.Indices {
			usedVertex[vi] = struct{}{}
		}
	}

	vertexRemap := make(map[uint32]uint32, len(lod.Vertices))
	keptVertices := make([]model.Vector3, 0, len(lod.Vertices))
	for i, v := range lod.Vertices {
		if _, used := usedVertex[uint32(i)]; !used {
			continue
		}
		vertexRemap[uint32(i)] = uint32(len(keptVertices))
		keptVertices = append(keptVertices, v)
	}

	for i := range keptFaces {
		remapped := make([]uint32, len(keptFaces[i].Indices))
		for j, vi := range keptFaces[i].Indices {
			remapped[j] = vertexRemap[vi]
		}
		keptFaces[i].Indices = remapped
	}

	uvSets := make([][]model.UV, len(lod.UVSets))
	for i, set := range lod.UVSets {
		if i == 0 {
			uvSets[i] = set
			continue
		}
		uvSets[i] = remapPerVertexUVs(set, vertexRemap, len(keptVertices))
	}

	pointMasses := remapPointMasses(lod.PointMasses, vertexRemap, len(keptVertices))

	selections := make([]model.Selection, 0, len(keptSelections))
	for _, sel := range keptSelections {
		selections = append(selections, remapSelection(sel, vertexRemap, faceRemap))
	}

	out := lod
	out.Vertices = keptVertices
	out.Faces = keptFaces
	out.UVSets = uvSets
	out.PointMasses = pointMasses
	out.Selections = selections
	return out
}

func remapPerVertexUVs(uvs []model.UV, vertexRemap map[uint32]uint32, keptCount int) []model.UV {
	if uvs == nil {
		return nil
	}
	out := make([]model.UV, keptCount)
	for oldIdx, newIdx := range vertexRemap {
		if int(oldIdx) < len(uvs) {
			out[newIdx] = uvs[oldIdx]
		}
	}
	return out
}

func remapPointMasses(masses []float32, vertexRemap map[uint32]uint32, keptCount int) []float32 {
	if masses == nil {
		return nil
	}
	out := make([]float32, keptCount)
	for oldIdx, newIdx := range vertexRemap {
		if int(oldIdx) < len(masses) {
			out[newIdx] = masses[oldIdx]
		}
	}
	return out
}

func remapSelection(sel model.Selection, vertexRemap map[uint32]uint32, faceRemap map[uint32]uint32) model.Selection {
	var vertexIndices []uint32
	var vertexWeights []byte
	for i, vi := range sel.VertexIndices {
		newIdx, ok := vertexRemap[vi]
		if !ok {
			continue
		}
		vertexIndices = append(vertexIndices, newIdx)
		if i < len(sel.VertexWeights) {
			vertexWeights = append(vertexWeights, sel.VertexWeights[i])
		}
	}

	var faceIndices []uint32
	for _, fi := range sel.FaceIndices {
		newIdx, ok := faceRemap[fi]
		if !ok {
			continue
		}
		faceIndices = append(faceIndices, newIdx)
	}

	sel.VertexIndices = vertexIndices
	sel.VertexWeights = vertexWeights
	sel.FaceIndices = faceIndices
	return sel
}
