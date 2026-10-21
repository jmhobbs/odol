package fbximport

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/fbxbinary"
	"github.com/jmhobbs/odol/internal/fbxexport"
	"github.com/jmhobbs/odol/p3d/model"
)

func approxVec3(t *testing.T, want, got model.Vector3, tolerance float32, msg string) {
	t.Helper()
	assert.InDeltaf(t, want.X, got.X, float64(tolerance), "%s: X", msg)
	assert.InDeltaf(t, want.Y, got.Y, float64(tolerance), "%s: Y", msg)
	assert.InDeltaf(t, want.Z, got.Z, float64(tolerance), "%s: Z", msg)
}

// singleTriangleModel builds a minimal one-LOD model with one triangle,
// distinct per-vertex normals and UVs, so a round trip through the existing
// FBX exporter and the new importer can be checked field by field.
func singleTriangleModel() *model.Model {
	return &model.Model{
		Name: "triangle",
		LODs: []model.LOD{
			{
				Resolution: 1.0,
				Vertices: []model.Vector3{
					{X: 0, Y: 0, Z: 0},
					{X: 1, Y: 0, Z: 0},
					{X: 0, Y: 1, Z: 0},
				},
				Normals: []model.Vector3{
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
				},
				Faces: []model.Face{
					{
						Indices:       []uint32{0, 1, 2},
						NormalIndices: []uint32{0, 1, 2},
						UVs: []model.UV{
							{U: 0, V: 0},
							{U: 1, V: 0},
							{U: 0, V: 1},
						},
						Texture:  "ca\\camoground.paa",
						Material: "ca\\camoground.rvmat",
					},
				},
			},
		},
	}
}

// roundKey rounds v to the nearest 1/1000 so float round-trip noise doesn't
// break map-key/set comparisons.
func roundKey(v model.Vector3) [3]float32 {
	round := func(f float32) float32 { return float32(math.Round(float64(f)*1000) / 1000) }
	return [3]float32{round(v.X), round(v.Y), round(v.Z)}
}

// faceNormal returns the right-hand-rule normal of a triangle given in
// order a, b, c.
func faceNormal(a, b, c model.Vector3) model.Vector3 {
	e1 := model.Vector3{X: b.X - a.X, Y: b.Y - a.Y, Z: b.Z - a.Z}
	e2 := model.Vector3{X: c.X - a.X, Y: c.Y - a.Y, Z: c.Z - a.Z}
	return model.Vector3{
		X: e1.Y*e2.Z - e1.Z*e2.Y,
		Y: e1.Z*e2.X - e1.X*e2.Z,
		Z: e1.X*e2.Y - e1.Y*e2.X,
	}
}

// TestImportRoundTripsSyntheticTriangle checks geometric fidelity, not
// vertex-array-order identity: internal/fbxexport's writer renumbers
// vertices into a part-local order (see buildPart), so a reconstructed
// mesh's vertex array is never expected to match the original index for
// index - what must survive the round trip is the vertex position set, the
// per-vertex UV association, and the winding (checked via the outward
// normal direction), all independent of array order.
func TestImportRoundTripsSyntheticTriangle(t *testing.T) {
	want := singleTriangleModel()
	data, err := fbxexport.Marshal(want)
	require.NoError(t, err)

	mesh, err := Import(data, NormalConventionDayZ)
	require.NoError(t, err)

	wantLOD := want.LODs[0]
	require.Len(t, mesh.Vertices, len(wantLOD.Vertices))
	require.Len(t, mesh.Faces, 1)
	face := mesh.Faces[0]
	require.Len(t, face.Indices, 3)

	wantUVByPosition := map[[3]float32]model.UV{}
	for i, idx := range wantLOD.Faces[0].Indices {
		wantUVByPosition[roundKey(wantLOD.Vertices[idx])] = wantLOD.Faces[0].UVs[i]
	}

	gotPositions := make([]model.Vector3, 3)
	for i, idx := range face.Indices {
		require.Less(t, int(idx), len(mesh.Vertices))
		gotPositions[i] = mesh.Vertices[idx]

		key := roundKey(gotPositions[i])
		wantUV, ok := wantUVByPosition[key]
		require.Truef(t, ok, "reconstructed vertex %v not found in original position set", gotPositions[i])
		require.Len(t, face.UVs, 3)
		assert.InDelta(t, wantUV.U, face.UVs[i].U, 1e-4, "UV for vertex %v", gotPositions[i])
		assert.InDelta(t, wantUV.V, face.UVs[i].V, 1e-4, "UV for vertex %v", gotPositions[i])

		require.Less(t, int(face.NormalIndices[i]), len(mesh.Normals))
		approxVec3(t, wantLOD.Normals[0], mesh.Normals[face.NormalIndices[i]], 1e-4, "normal")
	}

	gotNormal := faceNormal(gotPositions[0], gotPositions[1], gotPositions[2])
	wantNormal := faceNormal(wantLOD.Vertices[0], wantLOD.Vertices[1], wantLOD.Vertices[2])
	approxVec3(t, wantNormal, gotNormal, 1e-3, "face winding (outward normal direction)")
}

// twoDisconnectedTrianglesModel builds a LOD whose two triangles share no
// vertices, so internal/fbxexport's connected-component splitting emits two
// separate Geometry/Model parts - exercising Import's part-merging path.
func twoDisconnectedTrianglesModel() *model.Model {
	return &model.Model{
		Name: "two-triangles",
		LODs: []model.LOD{
			{
				Resolution: 1.0,
				Vertices: []model.Vector3{
					{X: 0, Y: 0, Z: 0},
					{X: 1, Y: 0, Z: 0},
					{X: 0, Y: 1, Z: 0},
					{X: 10, Y: 0, Z: 0},
					{X: 11, Y: 0, Z: 0},
					{X: 10, Y: 1, Z: 0},
				},
				Normals: []model.Vector3{
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
					{X: 0, Y: 0, Z: 1},
				},
				Faces: []model.Face{
					{
						Indices:       []uint32{0, 1, 2},
						NormalIndices: []uint32{0, 1, 2},
						UVs:           []model.UV{{U: 0, V: 0}, {U: 1, V: 0}, {U: 0, V: 1}},
					},
					{
						Indices:       []uint32{3, 4, 5},
						NormalIndices: []uint32{3, 4, 5},
						UVs:           []model.UV{{U: 0, V: 0}, {U: 1, V: 0}, {U: 0, V: 1}},
					},
				},
			},
		},
	}
}

func TestImportMergesDisconnectedComponents(t *testing.T) {
	m := twoDisconnectedTrianglesModel()
	data, err := fbxexport.Marshal(m)
	require.NoError(t, err)

	mesh, err := Import(data, NormalConventionDayZ)
	require.NoError(t, err)

	assert.Len(t, mesh.Vertices, 6, "both disconnected parts' vertices must be merged")
	assert.Len(t, mesh.Faces, 2, "both disconnected parts' faces must be merged")

	seen := map[[3]float32]bool{}
	for _, v := range mesh.Vertices {
		seen[[3]float32{
			float32(math.Round(float64(v.X)*1000) / 1000),
			float32(math.Round(float64(v.Y)*1000) / 1000),
			float32(math.Round(float64(v.Z)*1000) / 1000),
		}] = true
	}
	for _, v := range m.LODs[0].Vertices {
		key := [3]float32{v.X, v.Y, v.Z}
		assert.True(t, seen[key], "expected merged mesh to contain original vertex %v", v)
	}
}

// quadGeometryNode builds a single-quad Geometry node with distinct,
// identifiable per-occurrence normals and UVs (Direct reference mode) so a
// test can check exactly which output slot each one ends up in.
func quadGeometryNode() *fbxbinary.Node {
	return &fbxbinary.Node{
		Name:  "Geometry",
		Props: []any{int64(1), "Geometry::quad", "Mesh"},
		Children: []*fbxbinary.Node{
			{Name: "Vertices", Props: []any{[]float64{
				0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 0,
			}}},
			{Name: "PolygonVertexIndex", Props: []any{[]int32{0, 1, 2, ^int32(3)}}},
			{
				Name:  "LayerElementNormal",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "Normals", Props: []any{[]float64{
						0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1,
					}}},
				},
			},
			{
				Name:  "LayerElementUV",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "UV", Props: []any{[]float64{
						0, 0, 1, 0, 1, 1, 0, 1,
					}}},
				},
			},
		},
	}
}

// triangleGeometryNode is quadGeometryNode's 3-vertex counterpart.
func triangleGeometryNode() *fbxbinary.Node {
	return &fbxbinary.Node{
		Name:  "Geometry",
		Props: []any{int64(1), "Geometry::tri", "Mesh"},
		Children: []*fbxbinary.Node{
			{Name: "Vertices", Props: []any{[]float64{
				0, 0, 0, 1, 0, 0, 1, 1, 0,
			}}},
			{Name: "PolygonVertexIndex", Props: []any{[]int32{0, 1, ^int32(2)}}},
			{
				Name:  "LayerElementNormal",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "Normals", Props: []any{[]float64{
						0, 0, 1, 0, 0, 1, 0, 0, 1,
					}}},
				},
			},
			{
				Name:  "LayerElementUV",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "UV", Props: []any{[]float64{
						0, 0, 1, 0, 0, 1,
					}}},
				},
			},
		},
	}
}

// TestDecodeGeometryQuadWindingMatchesDocumentedPermutation checks the
// FBX-to-MLOD winding fix-up against docs/P3D Lod Faces.txt's documented
// table ("Polygon Vertex Order"): a 4-vertex polygon's descriptors must be
// reordered 1st, 4th, 3rd, 2nd - NOT a full reversal (4th, 3rd, 2nd, 1st).
// The two differ in which vertex ends up in slot 0, which is exactly the
// vertex the engine's fan triangulation uses for the quad's diagonal, so a
// full reversal silently picks the wrong diagonal on any quad where that
// matters (see conversation with the user - this is the "scrambled per
// face" bug, invisible to a per-corner UV value check since each corner's
// own UV stays individually correct).
func TestDecodeGeometryQuadWindingMatchesDocumentedPermutation(t *testing.T) {
	geometry, err := decodeGeometry(quadGeometryNode(), NormalConventionDayZ)
	require.NoError(t, err)
	faces := geometry.faces
	require.Len(t, faces, 1)
	face := faces[0]

	assert.Equal(t, []uint32{0, 3, 2, 1}, face.Indices, "1st,4th,3rd,2nd")
	// The FBX UVs (0,0),(1,0),(1,1),(0,1) become P3D (0,1),(1,1),(1,0),(0,0)
	// after the V flip, then take the same 1st,4th,3rd,2nd order.
	assert.Equal(t, []model.UV{{U: 0, V: 1}, {U: 0, V: 0}, {U: 1, V: 0}, {U: 1, V: 1}}, face.UVs)
}

// TestDecodeGeometryTriangleWindingMatchesDocumentedPermutation is the
// 3-vertex counterpart: docs/P3D Lod Faces.txt documents 1st, 3rd, 2nd for
// triangles, which (unlike the quad case) is cyclically equivalent to a
// full reversal - there's no diagonal ambiguity for a 3-vertex polygon, but
// this pins the exact index order down regardless.
func TestDecodeGeometryTriangleWindingMatchesDocumentedPermutation(t *testing.T) {
	geometry, err := decodeGeometry(triangleGeometryNode(), NormalConventionDayZ)
	require.NoError(t, err)
	faces := geometry.faces
	require.Len(t, faces, 1)
	face := faces[0]

	assert.Equal(t, []uint32{0, 2, 1}, face.Indices, "1st,3rd,2nd")
	// The FBX UVs (0,0),(1,0),(0,1) become P3D (0,1),(1,1),(0,0) after the
	// V flip, then take the same 1st,3rd,2nd order.
	assert.Equal(t, []model.UV{{U: 0, V: 1}, {U: 0, V: 0}, {U: 1, V: 1}}, face.UVs)
}

// TestDecodeGeometryFlipsV checks that FBX UVs (V = 0 at the bottom of the
// texture) are converted to P3D UVs (V = 0 at the top) as 1 - V.
func TestDecodeGeometryFlipsV(t *testing.T) {
	corners := []model.Vector3{{X: 0, Y: 0, Z: 0}, {X: 1, Y: 0, Z: 0}, {X: 0, Y: 1, Z: 0}}
	fbxUVs := []model.UV{{U: 0.25, V: 0.1}, {U: 0.25, V: 0.1}, {U: 0.25, V: 0.1}}
	geometry, err := decodeGeometry(singlePolygonGeometryNode(corners, fbxUVs), NormalConventionDayZ)
	require.NoError(t, err)
	require.Len(t, geometry.faces, 1)

	for _, uv := range geometry.faces[0].UVs {
		assert.Equal(t, float32(0.25), uv.U)
		assert.Equal(t, float32(1-float64(float32(0.1))), uv.V)
	}
}

// TestImportRoundTripKeepsUVsExact checks that the export-side and
// import-side V flips cancel bit for bit, so P3D -> FBX -> P3D does not
// drift UVs.
func TestImportRoundTripKeepsUVsExact(t *testing.T) {
	want := singleTriangleModel()
	want.LODs[0].Faces[0].UVs = []model.UV{
		{U: 0.123456, V: 0.987654},
		{U: 0.5, V: 0.0057190657},
		{U: 0.99340814, V: 0.333333},
	}
	data, err := fbxexport.Marshal(want)
	require.NoError(t, err)

	mesh, err := Import(data, NormalConventionDayZ)
	require.NoError(t, err)
	require.Len(t, mesh.Faces, 1)

	wantLOD := want.LODs[0]
	wantUVByPosition := map[[3]float32]model.UV{}
	for i, idx := range wantLOD.Faces[0].Indices {
		wantUVByPosition[roundKey(wantLOD.Vertices[idx])] = wantLOD.Faces[0].UVs[i]
	}
	face := mesh.Faces[0]
	require.Len(t, face.UVs, 3)
	for i, idx := range face.Indices {
		wantUV, ok := wantUVByPosition[roundKey(mesh.Vertices[idx])]
		require.True(t, ok)
		assert.Equal(t, wantUV, face.UVs[i])
	}
}

// TestDecodeGeometryFanTriangulatesLargerPolygons checks that a polygon with
// more than 4 vertices - which real-world DCC exports do produce (e.g. an
// unwelded rounded cap left untriangulated) - is fan-triangulated from
// vertex 0 rather than rejected. MLOD's FaceType is only ever 3 or 4 (see
// docs/P3D Lod Faces.txt), so there's no format-level way to represent a
// larger face; triangulating on import is the only option.
func TestDecodeGeometryFanTriangulatesLargerPolygons(t *testing.T) {
	g := &fbxbinary.Node{
		Name:  "Geometry",
		Props: []any{int64(1), "Geometry::pentagon", "Mesh"},
		Children: []*fbxbinary.Node{
			{Name: "Vertices", Props: []any{[]float64{
				0, 0, 0, 100, 0, 0, 100, 100, 0, 0, 100, 0, -100, 50, 0,
			}}},
			{Name: "PolygonVertexIndex", Props: []any{[]int32{0, 1, 2, 3, ^int32(4)}}},
			{
				Name:  "LayerElementNormal",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "Normals", Props: []any{make([]float64, 5*3)}},
				},
			},
		},
	}

	geometry, err := decodeGeometry(g, NormalConventionDayZ)
	require.NoError(t, err)
	normals, faces := geometry.normals, geometry.faces

	// A fan from vertex 0 over a 5-vertex polygon yields 3 triangles:
	// (0,1,2), (0,2,3), (0,3,4) in FBX's native order, each then reordered
	// by fixWinding (1st,3rd,2nd for a triangle).
	require.Len(t, faces, 3)
	assert.Equal(t, []uint32{0, 2, 1}, faces[0].Indices)
	assert.Equal(t, []uint32{0, 3, 2}, faces[1].Indices)
	assert.Equal(t, []uint32{0, 4, 3}, faces[2].Indices)

	for _, f := range faces {
		require.Len(t, f.NormalIndices, 3)
		require.Len(t, f.UVs, 3)
		for _, ni := range f.NormalIndices {
			assert.Less(t, int(ni), len(normals))
		}
	}

	// Each of the polygon's 5 occurrences must produce exactly one pooled
	// normal, shared across whichever triangles reuse that vertex - not
	// duplicated per triangle.
	assert.Len(t, normals, 5)
}

// singlePolygonGeometryNode builds a Geometry node holding one polygon over
// corners, in order, with a +Z normal and the given UV per corner.
func singlePolygonGeometryNode(corners []model.Vector3, uvs []model.UV) *fbxbinary.Node {
	var rawVertices, rawNormals, rawUVs []float64
	polygonVertexIndex := make([]int32, len(corners))
	for i, c := range corners {
		rawVertices = append(rawVertices, float64(c.X), float64(c.Y), float64(c.Z))
		rawNormals = append(rawNormals, 0, 0, 1)
		rawUVs = append(rawUVs, float64(uvs[i].U), float64(uvs[i].V))
		polygonVertexIndex[i] = int32(i)
	}
	polygonVertexIndex[len(corners)-1] = ^int32(len(corners) - 1)

	return &fbxbinary.Node{
		Name:  "Geometry",
		Props: []any{int64(1), "Geometry::polygon", "Mesh"},
		Children: []*fbxbinary.Node{
			{Name: "Vertices", Props: []any{rawVertices}},
			{Name: "PolygonVertexIndex", Props: []any{polygonVertexIndex}},
			{
				Name:  "LayerElementNormal",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "Normals", Props: []any{rawNormals}},
				},
			},
			{
				Name:  "LayerElementUV",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "UV", Props: []any{rawUVs}},
				},
			},
		},
	}
}

// uvForCorner gives every corner a distinct UV so a test can check that UVs
// stay attached to their corner through triangulation and fixWinding.
func uvForCorner(corner uint32) model.UV {
	return model.UV{U: float32(corner), V: float32(corner) + 0.5}
}

// importedUVForCorner is uvForCorner after decodeGeometry converts it from
// FBX V (0 at the bottom) to P3D V (0 at the top).
func importedUVForCorner(corner uint32) model.UV {
	fbxUV := uvForCorner(corner)
	return model.UV{U: fbxUV.U, V: float32(1 - float64(fbxUV.V))}
}

func distinctCornerUVs(count int) []model.UV {
	uvs := make([]model.UV, count)
	for i := range uvs {
		uvs[i] = uvForCorner(uint32(i))
	}
	return uvs
}

// TestDecodeGeometryTriangulatesConcavePolygon runs the concave U shape
// through decodeGeometry. The polygon's corners map 1:1 onto vertex and
// normal indices, so each face corner's normal index and UV must match its
// vertex index.
func TestDecodeGeometryTriangulatesConcavePolygon(t *testing.T) {
	corners := uShapeCorners()
	geometry, err := decodeGeometry(singlePolygonGeometryNode(corners, distinctCornerUVs(len(corners))), NormalConventionDayZ)
	require.NoError(t, err)
	assert.Zero(t, geometry.bestEffortPolygons)
	assert.Len(t, geometry.normals, len(corners), "one pooled normal per polygon occurrence")

	var sourceWindingTriangles [][3]int
	for _, f := range geometry.faces {
		require.Len(t, f.Indices, 3)
		require.Len(t, f.NormalIndices, 3)
		require.Len(t, f.UVs, 3)
		for i, vertex := range f.Indices {
			assert.Equal(t, vertex, f.NormalIndices[i], "normal must follow its corner")
			assert.Equal(t, importedUVForCorner(vertex), f.UVs[i], "UV must follow its corner")
		}

		// fixWinding turns FBX's CCW-front triangles into MLOD's CW-front
		// ones, so every face must now face away from the polygon's +Z.
		n := faceNormal(geometry.vertices[f.Indices[0]], geometry.vertices[f.Indices[1]], geometry.vertices[f.Indices[2]])
		assert.Less(t, n.Z, float32(0), "face %v must have MLOD CW winding", f.Indices)

		// Undo fixWinding (1st,3rd,2nd) to get back the source winding.
		sourceWindingTriangles = append(sourceWindingTriangles, [3]int{int(f.Indices[0]), int(f.Indices[2]), int(f.Indices[1])})
	}
	assertCoversPolygon(t, corners, sourceWindingTriangles)
}

func TestDecodeGeometrySplitsQuadThatFoldsOnEngineDiagonal(t *testing.T) {
	corners := dartQuadCorners()
	geometry, err := decodeGeometry(singlePolygonGeometryNode(corners, distinctCornerUVs(len(corners))), NormalConventionDayZ)
	require.NoError(t, err)

	// Blender's 1-3 split, triangles (0,1,3) and (1,2,3), each reordered by
	// fixWinding (1st,3rd,2nd). Corner data must follow each corner.
	require.Len(t, geometry.faces, 2)
	assert.Equal(t, []uint32{0, 3, 1}, geometry.faces[0].Indices)
	assert.Equal(t, []uint32{1, 3, 2}, geometry.faces[1].Indices)
	for _, f := range geometry.faces {
		for i, vertex := range f.Indices {
			assert.Equal(t, vertex, f.NormalIndices[i])
			assert.Equal(t, importedUVForCorner(vertex), f.UVs[i])
		}
	}
	assert.Zero(t, geometry.bestEffortPolygons)
}

func TestDecodeGeometryCountsBestEffortPolygons(t *testing.T) {
	collinear := []model.Vector3{{X: 0}, {X: 1}, {X: 2}, {X: 3}, {X: 4}}
	geometry, err := decodeGeometry(singlePolygonGeometryNode(collinear, distinctCornerUVs(len(collinear))), NormalConventionDayZ)
	require.NoError(t, err)

	assert.Len(t, geometry.faces, 3)
	assert.Equal(t, 1, geometry.bestEffortPolygons)
}

func TestDecodeGeometryRejectsDegeneratePolygons(t *testing.T) {
	g := &fbxbinary.Node{
		Name:  "Geometry",
		Props: []any{int64(1), "Geometry::bad", "Mesh"},
		Children: []*fbxbinary.Node{
			{Name: "Vertices", Props: []any{[]float64{0, 0, 0, 100, 0, 0}}},
			{Name: "PolygonVertexIndex", Props: []any{[]int32{^int32(1)}}},
			{
				Name:  "LayerElementNormal",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygonVertex"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "Normals", Props: []any{make([]float64, 2*3)}},
				},
			},
		},
	}

	_, err := decodeGeometry(g, NormalConventionDayZ)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestDecodeEulerTransformAppliesRotationTranslationScale(t *testing.T) {
	xf := transform{
		translate: model.Vector3{X: 5, Y: 0, Z: 0},
		rotate:    rotationMatrixXYZDegrees(0, 0, 90),
		scale:     model.Vector3{X: 2, Y: 1, Z: 1},
	}
	got := xf.applyPoint(model.Vector3{X: 1, Y: 0, Z: 0})
	// scale X by 2 -> (2,0,0); rotate 90 deg about Z -> (0,2,0); translate +5 X -> (5,2,0)
	approxVec3(t, model.Vector3{X: 5, Y: 2, Z: 0}, got, 1e-4, "rotated+scaled+translated point")
}

// twoTriangleGeometry builds a square split into two triangles that share
// points 0 and 2 (4 points, 6 polygon-vertex occurrences), with the given
// LayerElementNormal children.
func twoTriangleGeometry(normalLayerChildren []*fbxbinary.Node) *fbxbinary.Node {
	return &fbxbinary.Node{
		Name:  "Geometry",
		Props: []any{int64(1), "Geometry::square", "Mesh"},
		Children: []*fbxbinary.Node{
			{Name: "Vertices", Props: []any{[]float64{0, 0, 0, 100, 0, 0, 100, 100, 0, 0, 100, 0}}},
			{Name: "PolygonVertexIndex", Props: []any{[]int32{0, 1, ^int32(2), 0, 2, ^int32(3)}}},
			{Name: "LayerElementNormal", Props: []any{int32(0)}, Children: normalLayerChildren},
		},
	}
}

// pointNormal is a distinct normal per control point. Its Z is non-zero so
// the import-side Z negation is checked too.
func pointNormal(point int) model.Vector3 {
	return model.Vector3{X: float32(point + 1), Y: 0, Z: float32(point+1) * 10}
}

func TestDecodeGeometryResolvesByVerticeNormals(t *testing.T) {
	var directNormals, reversedNormals []float64
	for point := 0; point < 4; point++ {
		n := pointNormal(point)
		directNormals = append(directNormals, float64(n.X), float64(n.Y), float64(n.Z))
		r := pointNormal(3 - point)
		reversedNormals = append(reversedNormals, float64(r.X), float64(r.Y), float64(r.Z))
	}

	tests := []struct {
		name     string
		children []*fbxbinary.Node
	}{
		{
			name: "Direct",
			children: []*fbxbinary.Node{
				{Name: "MappingInformationType", Props: []any{"ByVertice"}},
				{Name: "ReferenceInformationType", Props: []any{"Direct"}},
				{Name: "Normals", Props: []any{directNormals}},
			},
		},
		{
			name: "IndexToDirect",
			children: []*fbxbinary.Node{
				{Name: "MappingInformationType", Props: []any{"ByVertice"}},
				{Name: "ReferenceInformationType", Props: []any{"IndexToDirect"}},
				{Name: "Normals", Props: []any{reversedNormals}},
				{Name: "NormalsIndex", Props: []any{[]int32{3, 2, 1, 0}}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			geometry, err := decodeGeometry(twoTriangleGeometry(tc.children), NormalConventionDayZ)
			require.NoError(t, err)
			require.Len(t, geometry.faces, 2)

			for _, face := range geometry.faces {
				for corner, point := range face.Indices {
					want := pointNormal(int(point))
					want.Z = -want.Z
					assert.Equalf(t, want, geometry.normals[face.NormalIndices[corner]], "normal for point %d", point)
				}
			}
		})
	}
}

func TestDecodeGeometryResolvesPerPolygonAndAllSameNormals(t *testing.T) {
	t.Run("ByPolygon", func(t *testing.T) {
		geometry, err := decodeGeometry(twoTriangleGeometry([]*fbxbinary.Node{
			{Name: "MappingInformationType", Props: []any{"ByPolygon"}},
			{Name: "ReferenceInformationType", Props: []any{"Direct"}},
			{Name: "Normals", Props: []any{[]float64{1, 0, 0, 0, 1, 0}}},
		}), NormalConventionDayZ)
		require.NoError(t, err)
		require.Len(t, geometry.faces, 2)

		wantByFace := []model.Vector3{{X: 1, Y: 0, Z: 0}, {X: 0, Y: 1, Z: 0}}
		for faceIndex, face := range geometry.faces {
			for _, normalIndex := range face.NormalIndices {
				assert.Equalf(t, wantByFace[faceIndex], geometry.normals[normalIndex], "face %d", faceIndex)
			}
		}
	})

	t.Run("AllSame", func(t *testing.T) {
		geometry, err := decodeGeometry(twoTriangleGeometry([]*fbxbinary.Node{
			{Name: "MappingInformationType", Props: []any{"AllSame"}},
			{Name: "ReferenceInformationType", Props: []any{"Direct"}},
			{Name: "Normals", Props: []any{[]float64{0, 1, 0}}},
		}), NormalConventionDayZ)
		require.NoError(t, err)
		require.Len(t, geometry.normals, 6)

		for _, normal := range geometry.normals {
			assert.Equal(t, model.Vector3{X: 0, Y: 1, Z: 0}, normal)
		}
	})
}

func TestDecodeGeometryRejectsUnknownMapping(t *testing.T) {
	_, err := decodeGeometry(twoTriangleGeometry([]*fbxbinary.Node{
		{Name: "MappingInformationType", Props: []any{"ByEdge"}},
		{Name: "ReferenceInformationType", Props: []any{"Direct"}},
		{Name: "Normals", Props: []any{make([]float64, 5*3)}},
	}), NormalConventionDayZ)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported MappingInformationType "ByEdge"`)
}

func TestParseNormalConvention(t *testing.T) {
	for _, want := range []NormalConvention{NormalConventionAuto, NormalConventionDayZ, NormalConventionStandard} {
		got, err := ParseNormalConvention(want.String())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	_, err := ParseNormalConvention("blender")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto, dayz, standard")
}

// cubeOutwardNormals lists the outward normal of each cubeGeometry face, in
// polygon order.
var cubeOutwardNormals = []model.Vector3{
	{X: 0, Y: 0, Z: -1},
	{X: 0, Y: 0, Z: 1},
	{X: 0, Y: -1, Z: 0},
	{X: 0, Y: 1, Z: 0},
	{X: -1, Y: 0, Z: 0},
	{X: 1, Y: 0, Z: 0},
}

// cubeGeometry builds a 100-unit cube of six quads, counter-clockwise from
// outside as FBX expects, with one ByPolygon normal per face computed by
// rawNormal from that face's outward normal.
func cubeGeometry(rawNormal func(outward model.Vector3) model.Vector3) *fbxbinary.Node {
	var vertices []float64
	for point := 0; point < 8; point++ {
		vertices = append(vertices, float64(point&1)*100, float64(point>>1&1)*100, float64(point>>2&1)*100)
	}
	quads := [][4]int32{{0, 2, 3, 1}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 6, 7, 3}, {0, 4, 6, 2}, {1, 3, 7, 5}}
	var polygonVertexIndex []int32
	var normals []float64
	for face, quad := range quads {
		polygonVertexIndex = append(polygonVertexIndex, quad[0], quad[1], quad[2], ^quad[3])
		n := rawNormal(cubeOutwardNormals[face])
		normals = append(normals, float64(n.X), float64(n.Y), float64(n.Z))
	}
	return &fbxbinary.Node{
		Name:  "Geometry",
		Props: []any{int64(1), "Geometry::cube", "Mesh"},
		Children: []*fbxbinary.Node{
			{Name: "Vertices", Props: []any{vertices}},
			{Name: "PolygonVertexIndex", Props: []any{polygonVertexIndex}},
			{
				Name:  "LayerElementNormal",
				Props: []any{int32(0)},
				Children: []*fbxbinary.Node{
					{Name: "MappingInformationType", Props: []any{"ByPolygon"}},
					{Name: "ReferenceInformationType", Props: []any{"Direct"}},
					{Name: "Normals", Props: []any{normals}},
				},
			},
		},
	}
}

// standardFBXNormal is how Blender writes a face's normal: outward, agreeing
// with the counter-clockwise winding.
func standardFBXNormal(outward model.Vector3) model.Vector3 { return outward }

// dayzFBXNormal is how DayZ tools write a face's normal: the inward MLOD
// normal with Z mirrored.
func dayzFBXNormal(outward model.Vector3) model.Vector3 {
	return model.Vector3{X: -outward.X, Y: -outward.Y, Z: outward.Z}
}

func TestDecodeGeometryAutoDetectsNormalConvention(t *testing.T) {
	tests := []struct {
		name      string
		rawNormal func(outward model.Vector3) model.Vector3
		want      NormalConvention
	}{
		{name: "standard", rawNormal: standardFBXNormal, want: NormalConventionStandard},
		{name: "dayz", rawNormal: dayzFBXNormal, want: NormalConventionDayZ},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			geometry, err := decodeGeometry(cubeGeometry(tc.rawNormal), NormalConventionAuto)
			require.NoError(t, err)
			assert.Equal(t, tc.want, geometry.normalConvention)

			// MLOD normals point along the right-hand normal of the
			// clockwise (MLOD-order) face, the way every sample P3D does.
			for faceIndex, face := range geometry.faces {
				rightHand := faceNormal(geometry.vertices[face.Indices[0]], geometry.vertices[face.Indices[1]], geometry.vertices[face.Indices[2]])
				for _, normalIndex := range face.NormalIndices {
					n := geometry.normals[normalIndex]
					assert.Positivef(t, rightHand.X*n.X+rightHand.Y*n.Y+rightHand.Z*n.Z, "face %d normal %v points against MLOD convention", faceIndex, n)
				}
			}
		})
	}
}

func TestDecodeGeometryExplicitNormalConventionOverridesDetection(t *testing.T) {
	geometry, err := decodeGeometry(cubeGeometry(standardFBXNormal), NormalConventionDayZ)
	require.NoError(t, err)
	assert.Equal(t, NormalConventionDayZ, geometry.normalConvention)

	require.Len(t, geometry.faces, len(cubeOutwardNormals))
	for faceIndex, face := range geometry.faces {
		outward := cubeOutwardNormals[faceIndex]
		want := model.Vector3{X: outward.X, Y: outward.Y, Z: -outward.Z}
		for _, normalIndex := range face.NormalIndices {
			assert.Equalf(t, want, geometry.normals[normalIndex], "face %d", faceIndex)
		}
	}
}
