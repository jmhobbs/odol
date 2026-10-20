package fbxexport

import (
	"math"
	"strings"

	"github.com/jmhobbs/odol/internal/model"
)

const proxySelectionPrefix = "proxy:"

// Proxy triangles have a right angle with legs in a 1:2 ratio (1m x 2m when
// unscaled, but sizes down to 0.02m x 0.04m appear in the samples). The
// tolerances leave room for float noise: across every proxy face in
// samples/ the worst right-angle cosine is 0.0009 and the worst leg ratio
// is off by 0.011.
const (
	proxyRightAngleCosTolerance = 0.01
	proxyLegRatio               = 2
	proxyLegRatioTolerance      = 0.05
)

// stripProxySelections removes proxy placeholder selections (named
// "proxy:...", see docs/P3D File Format - MLOD.txt) and the faces/vertices
// they reference from lod, since that geometry is a leftover marker for the
// proxy's local transform and is redundant with model.LOD.Proxies, which is
// exported separately as Null nodes. Orphaned proxy triangles (see
// orphanProxyFaces) are removed too. It returns a new LOD; lod is not
// mutated. If lod has no proxy-prefixed selections and no orphaned proxy
// triangles, lod is returned as-is.
func stripProxySelections(lod model.LOD) model.LOD {
	var keptSelections []model.Selection
	removedFaces := make(map[uint32]struct{})
	for _, sel := range lod.Selections {
		if strings.HasPrefix(sel.Name, proxySelectionPrefix) {
			for _, fi := range sel.FaceIndices {
				removedFaces[fi] = struct{}{}
			}
			continue
		}
		keptSelections = append(keptSelections, sel)
	}
	for _, fi := range orphanProxyFaces(lod) {
		removedFaces[fi] = struct{}{}
	}
	if len(keptSelections) == len(lod.Selections) && len(removedFaces) == 0 {
		return lod
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

// orphanProxyFaces returns the indices of proxy triangles that no named
// selection references. These are left behind when a proxy's selection is
// deleted but its triangle is not (scarh.p3d LOD 0 has four), so they can
// only be found by shape: an untextured, unmaterialed triangle whose
// corners form a proxy triangle. Faces in any named selection are skipped,
// since real geometry can share the shape (character_male.p3d has two such
// head triangles).
func orphanProxyFaces(lod model.LOD) []uint32 {
	inSelection := make(map[uint32]struct{})
	for _, sel := range lod.Selections {
		for _, fi := range sel.FaceIndices {
			inSelection[fi] = struct{}{}
		}
	}

	var orphans []uint32
	for i, face := range lod.Faces {
		if _, selected := inSelection[uint32(i)]; selected {
			continue
		}
		if face.Texture != "" || face.Material != "" {
			continue
		}
		if isProxyTriangle(lod.Vertices, face.Indices) {
			orphans = append(orphans, uint32(i))
		}
	}
	return orphans
}

// isProxyTriangle reports whether indices name a triangle in vertices with
// a right angle whose legs are in a proxyLegRatio:1 ratio.
func isProxyTriangle(vertices []model.Vector3, indices []uint32) bool {
	if len(indices) != 3 {
		return false
	}
	var corners [3][3]float64
	for i, vi := range indices {
		if int(vi) >= len(vertices) {
			return false
		}
		v := vertices[vi]
		corners[i] = [3]float64{float64(v.X), float64(v.Y), float64(v.Z)}
	}

	for c := range corners {
		legA := subtract3(corners[(c+1)%3], corners[c])
		legB := subtract3(corners[(c+2)%3], corners[c])
		lengthA, lengthB := math.Sqrt(dot3(legA, legA)), math.Sqrt(dot3(legB, legB))
		if lengthA == 0 || lengthB == 0 {
			return false
		}
		if math.Abs(dot3(legA, legB)/(lengthA*lengthB)) > proxyRightAngleCosTolerance {
			continue
		}
		ratio := math.Max(lengthA, lengthB) / math.Min(lengthA, lengthB)
		return math.Abs(ratio-proxyLegRatio) < proxyLegRatioTolerance
	}
	return false
}

func subtract3(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func dot3(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
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
