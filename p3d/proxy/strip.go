// Package proxy finds the placeholder triangles P3D models use to mark where
// proxied models go, strips them from a LOD, and recovers a proxy's transform
// from its triangle.
package proxy

import (
	"math"
	"strings"

	"github.com/jmhobbs/odol/p3d/model"
)

// SelectionPrefix starts the name of a named selection that marks a proxy.
const SelectionPrefix = "proxy:"

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

// Strip removes proxy placeholder selections (named "proxy:...", see
// docs/P3D File Format - MLOD.txt) and the faces/vertices they reference from
// lod, since that geometry only marks the proxy's local transform. In ODOL
// that transform is also in model.LOD.Proxies; in MLOD, TransformFromTriangle
// recovers it from the triangle before stripping. Orphaned proxy triangles
// (see orphanProxyFaces) are removed too. Per-vertex arrays (UV sets 1+,
// point masses, tangents and bitangents) are remapped with the vertices. It
// returns a new LOD; lod is not mutated. If lod has no proxy-prefixed
// selections and no orphaned proxy triangles, lod is returned as-is.
func Strip(lod model.LOD) model.LOD {
	var keptSelections []model.Selection
	removedFaces := make(map[uint32]struct{})
	for _, sel := range lod.Selections {
		if strings.HasPrefix(sel.Name, SelectionPrefix) {
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
	tangents := remapPerVertexVectors(lod.Tangents, vertexRemap, len(keptVertices))
	bitangents := remapPerVertexVectors(lod.Bitangents, vertexRemap, len(keptVertices))

	selections := make([]model.Selection, 0, len(keptSelections))
	for _, sel := range keptSelections {
		selections = append(selections, remapSelection(sel, vertexRemap, faceRemap))
	}

	out := lod
	out.Vertices = keptVertices
	out.Faces = keptFaces
	out.UVSets = uvSets
	out.PointMasses = pointMasses
	out.Tangents = tangents
	out.Bitangents = bitangents
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
	_, _, _, ok := proxyTriangleLegs(vertices, indices)
	return ok
}

// TransformFromTriangle recovers a proxy's transform from its placeholder
// triangle, in the layout model.Proxy.Transform uses: rows 0-2 the X, Y and Z
// axes, row 3 the position. The right-angle corner is the position, the long
// leg points along +Y and the short leg along +Z; X is Y x Z. The axes are
// unit length: the triangle's size is not a scale. This matches the
// transforms ODOL stores for the same proxies (checked on m4a1 against its
// MLOD source). ok is false when the triangle isn't proxy-shaped.
func TransformFromTriangle(vertices []model.Vector3, indices []uint32) (transform [4]model.Vector3, ok bool) {
	corner, long, short, ok := proxyTriangleLegs(vertices, indices)
	if !ok {
		return transform, false
	}
	y, z := normalize3(long), normalize3(short)
	x := cross3(y, z)
	for i, axis := range [][3]float64{x, y, z, corner} {
		transform[i] = model.Vector3{X: float32(axis[0]), Y: float32(axis[1]), Z: float32(axis[2])}
	}
	return transform, true
}

// proxyTriangleLegs finds the right-angle corner of a proxy triangle and its
// long and short legs, measured from that corner.
func proxyTriangleLegs(vertices []model.Vector3, indices []uint32) (corner, long, short [3]float64, ok bool) {
	if len(indices) != 3 {
		return corner, long, short, false
	}
	var corners [3][3]float64
	for i, vi := range indices {
		if int(vi) >= len(vertices) {
			return corner, long, short, false
		}
		v := vertices[vi]
		corners[i] = [3]float64{float64(v.X), float64(v.Y), float64(v.Z)}
	}

	for c := range corners {
		legA := subtract3(corners[(c+1)%3], corners[c])
		legB := subtract3(corners[(c+2)%3], corners[c])
		lengthA, lengthB := math.Sqrt(dot3(legA, legA)), math.Sqrt(dot3(legB, legB))
		if lengthA == 0 || lengthB == 0 {
			return corner, long, short, false
		}
		if math.Abs(dot3(legA, legB)/(lengthA*lengthB)) > proxyRightAngleCosTolerance {
			continue
		}
		ratio := math.Max(lengthA, lengthB) / math.Min(lengthA, lengthB)
		if math.Abs(ratio-proxyLegRatio) >= proxyLegRatioTolerance {
			return corner, long, short, false
		}
		if lengthA < lengthB {
			legA, legB = legB, legA
		}
		return corners[c], legA, legB, true
	}
	return corner, long, short, false
}

func subtract3(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func cross3(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func normalize3(a [3]float64) [3]float64 {
	length := math.Sqrt(dot3(a, a))
	return [3]float64{a[0] / length, a[1] / length, a[2] / length}
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

func remapPerVertexVectors(vectors []model.Vector3, vertexRemap map[uint32]uint32, keptCount int) []model.Vector3 {
	if vectors == nil {
		return nil
	}
	out := make([]model.Vector3, keptCount)
	for oldIdx, newIdx := range vertexRemap {
		if int(oldIdx) < len(vectors) {
			out[newIdx] = vectors[oldIdx]
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
