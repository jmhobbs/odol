// Package fbximport decodes a binary FBX file into a single merged mesh
// suitable for use as one MLOD graphical LOD. It is the inverse of
// internal/fbxexport: it reads the same node shapes that package writes
// (Vertices/PolygonVertexIndex/LayerElementNormal/LayerElementUV, all
// ByPolygonVertex) and also tolerates the IndexToDirect reference mode
// common in FBX files from other DCC tools.
package fbximport

import (
	"fmt"
	"math"

	"github.com/jmhobbs/odol/internal/fbxbinary"
	"github.com/jmhobbs/odol/internal/model"
)

// centimetersToMeters inverts internal/fbxexport/build.go's
// metersToCentimeters convention: MLOD/ODOL positions are meters, this
// repo's FBX writer scales by 100 to get FBX's raw units, so import divides
// back by 100. See plan_fbx-realworld-scale.md.
const centimetersToMeters = 1.0 / 100.0

// Mesh is a merged, single-LOD mesh decoded from one binary FBX file.
type Mesh struct {
	Vertices []model.Vector3
	Normals  []model.Vector3
	Faces    []model.Face

	// BestEffortPolygons counts source polygons that are self-intersecting
	// or have no area, so they could not be split into triangles that
	// exactly cover them. They are still imported, but some of their
	// triangles may be flipped or overlap. The fix is in the source model.
	BestEffortPolygons int
}

// Import decodes a binary FBX file and merges every Geometry node it
// contains into a single mesh, applying each geometry's parent Model
// transform and converting FBX's centimeter units back to MLOD's meters.
//
// Multiple Geometry nodes in one file are supported because canonical
// DayZ-style FBX exports split one LOD's mesh into per-connected-component
// parts (see internal/fbxexport/components.go) - merging here is the
// inverse of that split, and is a no-op for a plain single-mesh file.
func Import(data []byte) (*Mesh, error) {
	doc, err := fbxbinary.Decode(data)
	if err != nil {
		return nil, err
	}

	objects := doc.Top("Objects")
	if objects == nil {
		return nil, fmt.Errorf("fbximport: FBX file has no Objects node")
	}
	geomNodes := objects.ChildrenNamed("Geometry")
	if len(geomNodes) == 0 {
		return nil, fmt.Errorf("fbximport: FBX file has no Geometry node")
	}

	nodesByID := indexObjectsByID(objects)
	parentOf := indexParents(doc.Top("Connections"))

	mesh := &Mesh{}
	for _, g := range geomNodes {
		xf := identityTransform()
		if id, ok := nodeID(g); ok {
			if parentID, ok := parentOf[id]; ok {
				if parent, ok := nodesByID[parentID]; ok && parent.Name == "Model" {
					xf, err = readTransform(parent)
					if err != nil {
						return nil, fmt.Errorf("fbximport: geometry %q: %w", geometryLabel(g), err)
					}
				}
			}
		}

		geometry, err := decodeGeometry(g)
		if err != nil {
			return nil, fmt.Errorf("fbximport: geometry %q: %w", geometryLabel(g), err)
		}
		mesh.BestEffortPolygons += geometry.bestEffortPolygons

		vertexOffset := uint32(len(mesh.Vertices))
		for _, v := range geometry.vertices {
			mesh.Vertices = append(mesh.Vertices, scaleVector3(xf.applyPoint(v), centimetersToMeters))
		}

		normalOffset := uint32(len(mesh.Normals))
		for _, n := range geometry.normals {
			mesh.Normals = append(mesh.Normals, xf.applyNormal(n))
		}

		for _, f := range geometry.faces {
			for i := range f.Indices {
				f.Indices[i] += vertexOffset
			}
			for i := range f.NormalIndices {
				f.NormalIndices[i] += normalOffset
			}
			mesh.Faces = append(mesh.Faces, f)
		}
	}

	return mesh, nil
}

func nodeID(n *fbxbinary.Node) (int64, bool) {
	if len(n.Props) == 0 {
		return 0, false
	}
	id, ok := n.Props[0].(int64)
	return id, ok
}

func geometryLabel(n *fbxbinary.Node) string {
	if len(n.Props) > 1 {
		if s, ok := n.Props[1].(string); ok {
			return s
		}
	}
	return "unknown"
}

func indexObjectsByID(objects *fbxbinary.Node) map[int64]*fbxbinary.Node {
	out := make(map[int64]*fbxbinary.Node)
	for _, c := range objects.Children {
		if id, ok := nodeID(c); ok {
			out[id] = c
		}
	}
	return out
}

// indexParents maps each connection's child object ID to its parent object
// ID, from the Connections node's "C" ("OO"/"OP") entries.
func indexParents(connections *fbxbinary.Node) map[int64]int64 {
	out := make(map[int64]int64)
	if connections == nil {
		return out
	}
	for _, c := range connections.ChildrenNamed("C") {
		if len(c.Props) < 3 {
			continue
		}
		kind, ok := c.Props[0].(string)
		if !ok || (kind != "OO" && kind != "OP") {
			continue
		}
		child, ok1 := c.Props[1].(int64)
		parent, ok2 := c.Props[2].(int64)
		if ok1 && ok2 {
			out[child] = parent
		}
	}
	return out
}

// decodedGeometry is one Geometry node's mesh, before its parent Model
// transform and the unit conversion are applied.
type decodedGeometry struct {
	vertices           []model.Vector3
	normals            []model.Vector3
	faces              []model.Face
	bestEffortPolygons int
}

// decodeGeometry decodes one Geometry node's vertex positions (still in FBX
// raw/centimeter units - callers apply unit conversion), per-polygon-vertex
// normals, and triangle/quad faces with inline UVs. Only the
// "ByPolygonVertex" mapping mode is supported (matching internal/fbxexport's
// own writer and the overwhelming majority of real-world exports); "Direct"
// and "IndexToDirect" reference modes are both supported. MLOD's FaceType is
// only ever 3 or 4 (see docs/P3D Lod Faces.txt), so a polygon with more than
// 4 vertices is split into triangles by ear clipping (triangulatePolygon). A
// quad that would fold on the engine's corner 0 to corner 2 split diagonal
// is emitted as two triangles split along corner 1 to corner 3 instead, the
// way Blender draws it (quadFoldsOnEngineDiagonal).
func decodeGeometry(g *fbxbinary.Node) (decodedGeometry, error) {
	verticesNode := g.Child("Vertices")
	if verticesNode == nil || len(verticesNode.Props) == 0 {
		return decodedGeometry{}, fmt.Errorf("missing Vertices")
	}
	rawVerts, ok := verticesNode.Props[0].([]float64)
	if !ok || len(rawVerts)%3 != 0 {
		return decodedGeometry{}, fmt.Errorf("malformed Vertices array")
	}
	vertices := make([]model.Vector3, len(rawVerts)/3)
	for i := range vertices {
		vertices[i] = model.Vector3{
			X: float32(rawVerts[i*3]),
			Y: float32(rawVerts[i*3+1]),
			Z: float32(rawVerts[i*3+2]),
		}
	}

	polyIndexNode := g.Child("PolygonVertexIndex")
	if polyIndexNode == nil || len(polyIndexNode.Props) == 0 {
		return decodedGeometry{}, fmt.Errorf("missing PolygonVertexIndex")
	}
	rawPolyIndex, ok := polyIndexNode.Props[0].([]int32)
	if !ok {
		return decodedGeometry{}, fmt.Errorf("malformed PolygonVertexIndex array")
	}
	polygons := decodePolygons(rawPolyIndex)

	normalLayer := g.Child("LayerElementNormal")
	if normalLayer == nil {
		return decodedGeometry{}, fmt.Errorf("missing LayerElementNormal")
	}
	normalSrc, err := newVec3Layer(normalLayer, "Normals")
	if err != nil {
		return decodedGeometry{}, fmt.Errorf("LayerElementNormal: %w", err)
	}

	var uvSrc *vec2Layer
	if uvLayer := firstUVLayer(g); uvLayer != nil {
		uvSrc, err = newVec2Layer(uvLayer, "UV")
		if err != nil {
			return decodedGeometry{}, fmt.Errorf("LayerElementUV: %w", err)
		}
	}

	var normals []model.Vector3
	var faces []model.Face
	bestEffortPolygons := 0
	occurrence := 0
	for _, poly := range polygons {
		n := len(poly)
		if n < 3 {
			return decodedGeometry{}, fmt.Errorf("polygon with %d vertices is unsupported (need at least 3)", n)
		}

		indices := make([]uint32, n)
		normalIndices := make([]uint32, n)
		uvs := make([]model.UV, n)
		corners := make([]model.Vector3, n)
		for i, pointIdx := range poly {
			if pointIdx < 0 || int(pointIdx) >= len(vertices) {
				return decodedGeometry{}, fmt.Errorf("polygon vertex index %d out of range (%d points)", pointIdx, len(vertices))
			}
			indices[i] = uint32(pointIdx)
			corners[i] = vertices[pointIdx]

			normal, err := normalSrc.at(occurrence)
			if err != nil {
				return decodedGeometry{}, fmt.Errorf("normal lookup: %w", err)
			}
			normal.Z = -normal.Z // invert internal/fbxexport/build.go's export-side Z negation
			normals = append(normals, normal)
			normalIndices[i] = uint32(len(normals) - 1)

			if uvSrc != nil {
				uv, err := uvSrc.at(occurrence)
				if err != nil {
					return decodedGeometry{}, fmt.Errorf("UV lookup: %w", err)
				}
				uvs[i] = uv
			}

			occurrence++
		}

		// FBX's polygons are CCW-front; MLOD wants CW-front. Fix the winding
		// up to match, using the exact permutation docs/P3D Lod Faces.txt's
		// "Polygon Vertex Order" table documents (1st,4th,3rd,2nd for a
		// quad; 1st,3rd,2nd for a triangle) - NOT a full reversal. For a
		// quad those differ: MLOD stores it unsplit and the engine fan-
		// triangulates from vertex 0, so whichever vertex ends up in slot 0
		// picks the diagonal. A full reversal moves a different vertex into
		// slot 0 than the documented permutation does, silently choosing
		// the wrong diagonal on any quad where the two aren't equivalent -
		// invisible to a per-corner UV check since each corner's own UV
		// value stays correct either way.
		if n == 3 || (n == 4 && !quadFoldsOnEngineDiagonal(corners)) {
			face := model.Face{Indices: indices, NormalIndices: normalIndices, UVs: uvs}
			fixWinding(&face)
			faces = append(faces, face)
			continue
		}

		// Everything else is split into triangles. Each triangle shares this
		// polygon's already-pooled normals/UVs rather than duplicating them,
		// and gets the same winding fix-up.
		//
		// A quad that folds on the engine's 0-2 diagonal gets the 1-3 split
		// Blender draws it with, in the corner order Blender's tessellation
		// produces. MLOD has no face with more than 4 vertices (FaceType is
		// only ever 3 or 4), so larger polygons are split by ear clipping. A
		// fan from vertex 0 is only right for convex polygons, and real
		// exports have concave ones (cross outlines, cut-outs).
		var triangles [][3]int
		if n == 4 {
			triangles = [][3]int{{0, 1, 3}, {1, 2, 3}}
		} else {
			var clean bool
			triangles, clean = triangulatePolygon(corners)
			if !clean {
				bestEffortPolygons++
			}
		}
		for _, tri := range triangles {
			face := model.Face{
				Indices:       []uint32{indices[tri[0]], indices[tri[1]], indices[tri[2]]},
				NormalIndices: []uint32{normalIndices[tri[0]], normalIndices[tri[1]], normalIndices[tri[2]]},
				UVs:           []model.UV{uvs[tri[0]], uvs[tri[1]], uvs[tri[2]]},
			}
			fixWinding(&face)
			faces = append(faces, face)
		}
	}

	return decodedGeometry{
		vertices:           vertices,
		normals:            normals,
		faces:              faces,
		bestEffortPolygons: bestEffortPolygons,
	}, nil
}

// fixWinding reorders a face's vertex/normal/UV descriptors in place to
// match docs/P3D Lod Faces.txt's documented permutation: the 1st descriptor
// stays fixed, and the remaining descriptors are reversed.
func fixWinding(f *model.Face) {
	for i, j := 1, len(f.Indices)-1; i < j; i, j = i+1, j-1 {
		f.Indices[i], f.Indices[j] = f.Indices[j], f.Indices[i]
	}
	for i, j := 1, len(f.NormalIndices)-1; i < j; i, j = i+1, j-1 {
		f.NormalIndices[i], f.NormalIndices[j] = f.NormalIndices[j], f.NormalIndices[i]
	}
	for i, j := 1, len(f.UVs)-1; i < j; i, j = i+1, j-1 {
		f.UVs[i], f.UVs[j] = f.UVs[j], f.UVs[i]
	}
}

// decodePolygons splits FBX's flat, negative-terminated PolygonVertexIndex
// array into per-polygon point-index lists (each index un-negated).
func decodePolygons(raw []int32) [][]int32 {
	var polygons [][]int32
	var current []int32
	for _, v := range raw {
		if v < 0 {
			current = append(current, ^v)
			polygons = append(polygons, current)
			current = nil
			continue
		}
		current = append(current, v)
	}
	if len(current) > 0 {
		polygons = append(polygons, current)
	}
	return polygons
}

func firstUVLayer(g *fbxbinary.Node) *fbxbinary.Node {
	for _, layer := range g.ChildrenNamed("LayerElementUV") {
		if len(layer.Props) > 0 {
			if idx, ok := layer.Props[0].(int32); ok && idx == 0 {
				return layer
			}
		}
	}
	layers := g.ChildrenNamed("LayerElementUV")
	if len(layers) > 0 {
		return layers[0]
	}
	return nil
}

// vec3Layer resolves a ByPolygonVertex Normals-shaped layer element (Direct
// or IndexToDirect) to a Vector3 per polygon-vertex occurrence.
type vec3Layer struct {
	direct []float64
	index  []int32 // nil unless ReferenceInformationType == "IndexToDirect"
}

func newVec3Layer(layer *fbxbinary.Node, directChildName string) (*vec3Layer, error) {
	mapping, err := childString(layer, "MappingInformationType")
	if err != nil {
		return nil, err
	}
	if mapping != "ByPolygonVertex" {
		return nil, fmt.Errorf("unsupported MappingInformationType %q (only ByPolygonVertex is supported)", mapping)
	}
	ref, err := childString(layer, "ReferenceInformationType")
	if err != nil {
		return nil, err
	}

	directNode := layer.Child(directChildName)
	if directNode == nil || len(directNode.Props) == 0 {
		return nil, fmt.Errorf("missing %s", directChildName)
	}
	direct, ok := directNode.Props[0].([]float64)
	if !ok {
		return nil, fmt.Errorf("malformed %s array", directChildName)
	}

	l := &vec3Layer{direct: direct}
	switch ref {
	case "Direct":
	case "IndexToDirect":
		idxNode := layer.Child(directChildName + "Index")
		if idxNode == nil || len(idxNode.Props) == 0 {
			return nil, fmt.Errorf("missing %sIndex for IndexToDirect reference", directChildName)
		}
		idx, ok := idxNode.Props[0].([]int32)
		if !ok {
			return nil, fmt.Errorf("malformed %sIndex array", directChildName)
		}
		l.index = idx
	default:
		return nil, fmt.Errorf("unsupported ReferenceInformationType %q", ref)
	}
	return l, nil
}

func (l *vec3Layer) at(occurrence int) (model.Vector3, error) {
	i := occurrence
	if l.index != nil {
		if occurrence >= len(l.index) {
			return model.Vector3{}, fmt.Errorf("occurrence %d out of range (%d indices)", occurrence, len(l.index))
		}
		i = int(l.index[occurrence])
	}
	if i*3+2 >= len(l.direct) || i < 0 {
		return model.Vector3{}, fmt.Errorf("direct index %d out of range (%d values)", i, len(l.direct))
	}
	return model.Vector3{
		X: float32(l.direct[i*3]),
		Y: float32(l.direct[i*3+1]),
		Z: float32(l.direct[i*3+2]),
	}, nil
}

// vec2Layer is vec3Layer's UV-shaped (2-component) counterpart.
type vec2Layer struct {
	direct []float64
	index  []int32
}

func newVec2Layer(layer *fbxbinary.Node, directChildName string) (*vec2Layer, error) {
	mapping, err := childString(layer, "MappingInformationType")
	if err != nil {
		return nil, err
	}
	if mapping != "ByPolygonVertex" {
		return nil, fmt.Errorf("unsupported MappingInformationType %q (only ByPolygonVertex is supported)", mapping)
	}
	ref, err := childString(layer, "ReferenceInformationType")
	if err != nil {
		return nil, err
	}

	directNode := layer.Child(directChildName)
	if directNode == nil || len(directNode.Props) == 0 {
		return nil, fmt.Errorf("missing %s", directChildName)
	}
	direct, ok := directNode.Props[0].([]float64)
	if !ok {
		return nil, fmt.Errorf("malformed %s array", directChildName)
	}

	l := &vec2Layer{direct: direct}
	switch ref {
	case "Direct":
	case "IndexToDirect":
		idxNode := layer.Child(directChildName + "Index")
		if idxNode == nil || len(idxNode.Props) == 0 {
			return nil, fmt.Errorf("missing %sIndex for IndexToDirect reference", directChildName)
		}
		idx, ok := idxNode.Props[0].([]int32)
		if !ok {
			return nil, fmt.Errorf("malformed %sIndex array", directChildName)
		}
		l.index = idx
	default:
		return nil, fmt.Errorf("unsupported ReferenceInformationType %q", ref)
	}
	return l, nil
}

func (l *vec2Layer) at(occurrence int) (model.UV, error) {
	i := occurrence
	if l.index != nil {
		if occurrence >= len(l.index) {
			return model.UV{}, fmt.Errorf("occurrence %d out of range (%d indices)", occurrence, len(l.index))
		}
		i = int(l.index[occurrence])
	}
	if i*2+1 >= len(l.direct) || i < 0 {
		return model.UV{}, fmt.Errorf("direct index %d out of range (%d values)", i, len(l.direct))
	}
	return model.UV{U: float32(l.direct[i*2]), V: float32(l.direct[i*2+1])}, nil
}

func childString(n *fbxbinary.Node, name string) (string, error) {
	c := n.Child(name)
	if c == nil || len(c.Props) == 0 {
		return "", fmt.Errorf("missing %s", name)
	}
	s, ok := c.Props[0].(string)
	if !ok {
		return "", fmt.Errorf("%s is not a string", name)
	}
	return s, nil
}

// transform is a local Translate*Rotate*Scale transform, decoded from an FBX
// Model node's "Lcl Translation"/"Lcl Rotation"/"Lcl Scaling" properties.
// Rotation/scaling pivots are assumed zero (see plan_fbx2p3d.md) - this
// matches how internal/fbxexport's own writer emits models (no pivots at
// all) and most non-rigged static-prop DCC exports.
type transform struct {
	translate model.Vector3
	rotate    [3][3]float32
	scale     model.Vector3
}

func identityTransform() transform {
	return transform{scale: model.Vector3{X: 1, Y: 1, Z: 1}, rotate: identityMatrix()}
}

func identityMatrix() [3][3]float32 {
	return [3][3]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
}

// rotationMatrixXYZDegrees builds the rotation matrix for FBX's default
// eEulerXYZ order: rotate about X, then Y, then Z (composed as Rz*Ry*Rx,
// applied to a column vector).
func rotationMatrixXYZDegrees(xDeg, yDeg, zDeg float32) [3][3]float32 {
	rad := func(d float32) float64 { return float64(d) * math.Pi / 180 }
	sx, cx := math.Sincos(rad(xDeg))
	sy, cy := math.Sincos(rad(yDeg))
	sz, cz := math.Sincos(rad(zDeg))

	rx := [3][3]float64{{1, 0, 0}, {0, cx, -sx}, {0, sx, cx}}
	ry := [3][3]float64{{cy, 0, sy}, {0, 1, 0}, {-sy, 0, cy}}
	rz := [3][3]float64{{cz, -sz, 0}, {sz, cz, 0}, {0, 0, 1}}

	zy := matMul(rz, ry)
	zyx := matMul(zy, rx)

	var out [3][3]float32
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = float32(zyx[i][j])
		}
	}
	return out
}

func matMul(a, b [3][3]float64) [3][3]float64 {
	var out [3][3]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = a[i][0]*b[0][j] + a[i][1]*b[1][j] + a[i][2]*b[2][j]
		}
	}
	return out
}

func (t transform) applyPoint(v model.Vector3) model.Vector3 {
	sx, sy, sz := v.X*t.scale.X, v.Y*t.scale.Y, v.Z*t.scale.Z
	rx := t.rotate[0][0]*sx + t.rotate[0][1]*sy + t.rotate[0][2]*sz
	ry := t.rotate[1][0]*sx + t.rotate[1][1]*sy + t.rotate[1][2]*sz
	rz := t.rotate[2][0]*sx + t.rotate[2][1]*sy + t.rotate[2][2]*sz
	return model.Vector3{
		X: rx + t.translate.X,
		Y: ry + t.translate.Y,
		Z: rz + t.translate.Z,
	}
}

func scaleVector3(v model.Vector3, s float32) model.Vector3 {
	return model.Vector3{X: v.X * s, Y: v.Y * s, Z: v.Z * s}
}

// applyNormal rotates and scales (no translation) a normal. Non-uniform
// scale technically requires the inverse-transpose rather than the forward
// matrix; this is a documented limitation (see plan_fbx2p3d.md) acceptable
// for the near-uniform scales real static-prop exports use.
func (t transform) applyNormal(v model.Vector3) model.Vector3 {
	sx, sy, sz := v.X*t.scale.X, v.Y*t.scale.Y, v.Z*t.scale.Z
	rx := t.rotate[0][0]*sx + t.rotate[0][1]*sy + t.rotate[0][2]*sz
	ry := t.rotate[1][0]*sx + t.rotate[1][1]*sy + t.rotate[1][2]*sz
	rz := t.rotate[2][0]*sx + t.rotate[2][1]*sy + t.rotate[2][2]*sz
	length := float32(math.Sqrt(float64(rx*rx + ry*ry + rz*rz)))
	if length == 0 {
		return model.Vector3{}
	}
	return model.Vector3{X: rx / length, Y: ry / length, Z: rz / length}
}

// readTransform decodes a Model node's local Translation/Rotation/Scaling
// properties from its Properties70 block, defaulting to identity for any
// that are absent.
func readTransform(modelNode *fbxbinary.Node) (transform, error) {
	xf := identityTransform()
	props := modelNode.Child("Properties70")
	if props == nil {
		return xf, nil
	}

	if v, ok := findVector3Property(props, "Lcl Translation"); ok {
		xf.translate = v
	}
	rotateDeg := model.Vector3{}
	if v, ok := findVector3Property(props, "Lcl Rotation"); ok {
		rotateDeg = v
	}
	xf.rotate = rotationMatrixXYZDegrees(rotateDeg.X, rotateDeg.Y, rotateDeg.Z)
	if v, ok := findVector3Property(props, "Lcl Scaling"); ok {
		xf.scale = v
	} else {
		xf.scale = model.Vector3{X: 1, Y: 1, Z: 1}
	}

	return xf, nil
}

// findVector3Property scans a Properties70 node's "P" children for one
// named propName, in the standard FBX layout: props =
// [name, dataType, subType, flags, x, y, z, ...].
func findVector3Property(properties70 *fbxbinary.Node, propName string) (model.Vector3, bool) {
	for _, p := range properties70.ChildrenNamed("P") {
		if len(p.Props) < 7 {
			continue
		}
		name, ok := p.Props[0].(string)
		if !ok || name != propName {
			continue
		}
		x, ok1 := toFloat32(p.Props[4])
		y, ok2 := toFloat32(p.Props[5])
		z, ok3 := toFloat32(p.Props[6])
		if ok1 && ok2 && ok3 {
			return model.Vector3{X: x, Y: y, Z: z}, true
		}
	}
	return model.Vector3{}, false
}

func toFloat32(v any) (float32, bool) {
	switch t := v.(type) {
	case float64:
		return float32(t), true
	case float32:
		return t, true
	case int32:
		return float32(t), true
	case int64:
		return float32(t), true
	default:
		return 0, false
	}
}
