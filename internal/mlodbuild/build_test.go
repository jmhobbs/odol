package mlodbuild

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/mlod"
	"github.com/jmhobbs/odol/internal/model"
)

func sampleGraphicalLOD(resolution float32, offset float32) GraphicalLOD {
	return GraphicalLOD{
		Resolution: resolution,
		Vertices: []model.Vector3{
			{X: offset + 0, Y: 0, Z: 0},
			{X: offset + 1, Y: 0, Z: 0},
			{X: offset + 0, Y: 1, Z: 0},
		},
		Normals: []model.Vector3{
			{X: 0, Y: 0, Z: 1},
		},
		Faces: []model.Face{
			{
				Indices:       []uint32{0, 1, 2},
				NormalIndices: []uint32{0, 0, 0},
				UVs:           []model.UV{{U: 0, V: 0}, {U: 1, V: 0}, {U: 0, V: 1}},
			},
		},
	}
}

func defaultOptions() Options {
	return Options{
		Texture:  "ca\\camoground.paa",
		Material: "ca\\camoground.rvmat",
		Mass:     80,
	}
}

// findSelection returns the named selection from a LOD's Selections, failing
// the test if it isn't present.
func findSelection(t *testing.T, lod model.LOD, name string) model.Selection {
	t.Helper()
	for _, sel := range lod.Selections {
		if sel.Name == name {
			return sel
		}
	}
	t.Fatalf("no selection named %q", name)
	return model.Selection{}
}

func TestBuildProducesExpectedResolutions(t *testing.T) {
	m, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0), sampleGraphicalLOD(2.0, 0)}, defaultOptions())
	require.NoError(t, err)

	require.Len(t, m.LODs, 6)

	var resolutions []float32
	for _, lod := range m.LODs {
		resolutions = append(resolutions, lod.Resolution)
	}
	assert.Contains(t, resolutions, float32(1.0))
	assert.Contains(t, resolutions, float32(2.0))
	assert.Contains(t, resolutions, GeometryResolution)
	assert.Contains(t, resolutions, ViewGeometryResolution)
	assert.Contains(t, resolutions, FireGeometryResolution)
	assert.Contains(t, resolutions, MemoryResolution)

	// Exact-bit match against docs/P3DModelInfo.txt's documented hex values.
	assert.Equal(t, uint32(0x551184e7), math.Float32bits(GeometryResolution))
	assert.Equal(t, uint32(0x58635fa9), math.Float32bits(MemoryResolution))
	assert.Equal(t, uint32(0x59aa87bf), math.Float32bits(ViewGeometryResolution))
	assert.Equal(t, uint32(0x59c6f3b4), math.Float32bits(FireGeometryResolution))
}

func findLOD(t *testing.T, m *model.Model, resolution float32) model.LOD {
	t.Helper()
	for _, lod := range m.LODs {
		if lod.Resolution == resolution {
			return lod
		}
	}
	t.Fatalf("no LOD with resolution %g", resolution)
	return model.LOD{}
}

func TestBuildGeometryLODHasMassOthersDont(t *testing.T) {
	opts := defaultOptions()
	m, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0)}, opts)
	require.NoError(t, err)

	geom := findLOD(t, m, GeometryResolution)
	require.Len(t, geom.Vertices, 8, "geometry LOD must be an 8-point box")
	require.Len(t, geom.PointMasses, 8)
	var total float32
	for _, mass := range geom.PointMasses {
		total += mass
	}
	assert.InDelta(t, opts.Mass, total, 1e-4)

	for _, res := range []float32{ViewGeometryResolution, FireGeometryResolution, MemoryResolution} {
		lod := findLOD(t, m, res)
		assert.Empty(t, lod.PointMasses, "resolution %g must not carry #Mass#", res)
	}
}

// TestBuildMemoryLODHasFiveAutomaticPoints checks every Memory LOD point
// Build adds automatically (see memoryLOD): ce_center, invview,
// boundingbox_min, boundingbox_max, and ce_radius. Expected positions are
// hand-derived from sampleGraphicalLOD(1.0, 0)'s single triangle
// ((0,0,0),(1,0,0),(0,1,0)) after Build's unconditional recentering (X/Z
// centered, Y floored to 0): the triangle's box becomes
// min=(-0.5,0,0), max=(0.5,1,0).
func TestBuildMemoryLODHasFiveAutomaticPoints(t *testing.T) {
	m, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0)}, defaultOptions())
	require.NoError(t, err)

	mem := findLOD(t, m, MemoryResolution)
	require.Len(t, mem.Vertices, 5)
	assert.Empty(t, mem.Faces)
	require.Len(t, mem.Selections, 5)

	centerSel := findSelection(t, mem, MemoryCenterName)
	require.Len(t, centerSel.VertexIndices, 1)
	assert.Equal(t, model.Vector3{}, mem.Vertices[centerSel.VertexIndices[0]])

	minSel := findSelection(t, mem, BoundingBoxMinName)
	require.Len(t, minSel.VertexIndices, 1)
	assert.Equal(t, model.Vector3{X: -0.5, Y: 0, Z: 0}, mem.Vertices[minSel.VertexIndices[0]])

	maxSel := findSelection(t, mem, BoundingBoxMaxName)
	require.Len(t, maxSel.VertexIndices, 1)
	assert.Equal(t, model.Vector3{X: 0.5, Y: 1, Z: 0}, mem.Vertices[maxSel.VertexIndices[0]])

	// largest dimension is 1 (both X and Y extents are 1, Z is 0), so
	// invview sits at 2x that along +Z, vertically centered on the box
	// (max.Y/2 = 0.5) and horizontally centered (X=0, already true
	// post-recentering).
	invviewSel := findSelection(t, mem, InvViewName)
	require.Len(t, invviewSel.VertexIndices, 1)
	assert.Equal(t, model.Vector3{X: 0, Y: 0.5, Z: 2}, mem.Vertices[invviewSel.VertexIndices[0]])

	// largest horizontal (X/Z) extent is 1 (X), so ce_radius sits at half
	// that on both X and Z.
	radiusSel := findSelection(t, mem, CentralEconomyRadiusName)
	require.Len(t, radiusSel.VertexIndices, 1)
	assert.Equal(t, model.Vector3{X: 0.5, Y: 0, Z: 0.5}, mem.Vertices[radiusSel.VertexIndices[0]])
}

func TestBuildCamogroundSelectionCoversAllGeometry(t *testing.T) {
	m, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0)}, defaultOptions())
	require.NoError(t, err)

	lod := findLOD(t, m, 1.0)
	require.Len(t, lod.Selections, 1)
	sel := lod.Selections[0]
	assert.Equal(t, "camoground", sel.Name)
	assert.ElementsMatch(t, []uint32{0, 1, 2}, sel.VertexIndices)
	assert.ElementsMatch(t, []uint32{0}, sel.FaceIndices)
}

// TestBuildGeometryLODsHaveComponent01Selection checks that every collision
// LOD (Geometry, View Geometry, Fire Geometry) carries a "Component01" named
// selection covering its whole convex-box hull. Arma's convex-hull physics
// requires each convex piece of a collision LOD to be marked via a
// "ComponentNN" named selection - without one, Object Builder treats the LOD
// as having no components at all. Since these LODs are always a single
// convex box (see boxGeometry), one "Component01" selection covering
// everything is correct and sufficient.
func TestBuildGeometryLODsHaveComponent01Selection(t *testing.T) {
	m, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0)}, defaultOptions())
	require.NoError(t, err)

	for _, res := range []float32{GeometryResolution, ViewGeometryResolution, FireGeometryResolution} {
		lod := findLOD(t, m, res)
		var componentSel *model.Selection
		for i := range lod.Selections {
			if lod.Selections[i].Name == "Component01" {
				componentSel = &lod.Selections[i]
			}
		}
		require.NotNilf(t, componentSel, "resolution %g must have a Component01 selection", res)
		assert.Len(t, componentSel.VertexIndices, len(lod.Vertices))
		assert.Len(t, componentSel.FaceIndices, len(lod.Faces))
	}
}

func TestBuildAppliesTextureMaterialOnlyToGraphicalLODs(t *testing.T) {
	opts := defaultOptions()
	m, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0)}, opts)
	require.NoError(t, err)

	graphical := findLOD(t, m, 1.0)
	for _, f := range graphical.Faces {
		assert.Equal(t, opts.Texture, f.Texture)
		assert.Equal(t, opts.Material, f.Material)
	}

	for _, res := range []float32{GeometryResolution, ViewGeometryResolution, FireGeometryResolution} {
		lod := findLOD(t, m, res)
		for _, f := range lod.Faces {
			assert.Empty(t, f.Texture)
			assert.Empty(t, f.Material)
		}
	}
}

func TestCenteringOffset(t *testing.T) {
	vertices := []model.Vector3{{X: 1, Y: 2, Z: 3}, {X: 5, Y: 10, Z: 9}}
	got := centeringOffset(vertices)
	assert.Equal(t, model.Vector3{X: -3, Y: -2, Z: -6}, got)
}

func TestCenteringOffsetEmpty(t *testing.T) {
	assert.Equal(t, model.Vector3{}, centeringOffset(nil))
}

func TestTranslateVertices(t *testing.T) {
	in := []model.Vector3{{X: 1, Y: 2, Z: 3}, {X: -4, Y: 5, Z: -6}}
	got := translateVertices(in, model.Vector3{X: 1, Y: -1, Z: 2})
	assert.Equal(t, []model.Vector3{{X: 2, Y: 1, Z: 5}, {X: -3, Y: 4, Z: -4}}, got)
}

func TestTranslateVerticesDoesNotMutateInput(t *testing.T) {
	in := []model.Vector3{{X: 1, Y: 1, Z: 1}}
	_ = translateVertices(in, model.Vector3{X: 5, Y: 5, Z: 5})
	assert.Equal(t, model.Vector3{X: 1, Y: 1, Z: 1}, in[0], "translateVertices must not mutate its input slice")
}

// boxGraphicalLOD builds a GraphicalLOD whose vertex bounding box is
// exactly [min, max] - min and max are themselves vertices, plus a third
// point between them so faces reference 3 distinct indices.
func boxGraphicalLOD(resolution float32, min, max model.Vector3) GraphicalLOD {
	mid := model.Vector3{X: (min.X + max.X) / 2, Y: min.Y, Z: max.Z}
	return GraphicalLOD{
		Resolution: resolution,
		Vertices:   []model.Vector3{min, max, mid},
		Normals:    []model.Vector3{{X: 0, Y: 0, Z: 1}},
		Faces: []model.Face{{
			Indices:       []uint32{0, 1, 2},
			NormalIndices: []uint32{0, 0, 0},
			UVs:           []model.UV{{}, {}, {}},
		}},
	}
}

func vertexMinMax(t *testing.T, vertices []model.Vector3) (min, max model.Vector3) {
	t.Helper()
	require.NotEmpty(t, vertices)
	min, max = vertices[0], vertices[0]
	for _, v := range vertices[1:] {
		min.X, max.X = min32(min.X, v.X), max32(max.X, v.X)
		min.Y, max.Y = min32(min.Y, v.Y), max32(max.Y, v.Y)
		min.Z, max.Z = min32(min.Z, v.Z), max32(max.Z, v.Z)
	}
	return min, max
}

func TestBuildRecentersGraphicalLOD(t *testing.T) {
	lod := boxGraphicalLOD(1.0, model.Vector3{X: 1, Y: 2, Z: 3}, model.Vector3{X: 5, Y: 10, Z: 9})
	m, err := Build([]GraphicalLOD{lod}, defaultOptions())
	require.NoError(t, err)

	out := findLOD(t, m, 1.0)
	min, max := vertexMinMax(t, out.Vertices)
	assert.InDelta(t, 0, (min.X+max.X)/2, 1e-4, "X center")
	assert.InDelta(t, 0, (min.Z+max.Z)/2, 1e-4, "Z center")
	assert.InDelta(t, 0, min.Y, 1e-4, "Y floor")
}

// TestBuildRecentersConsistentlyAcrossMultipleLODs proves the same offset
// (derived from the union of all LODs) is applied to every LOD, rather than
// each LOD being independently centered on itself: a known, fixed delta
// between two LODs' raw vertex positions must survive unchanged in the
// output.
func TestBuildRecentersConsistentlyAcrossMultipleLODs(t *testing.T) {
	lod0 := boxGraphicalLOD(1.0, model.Vector3{X: 0, Y: 0, Z: 0}, model.Vector3{X: 10, Y: 4, Z: 2})
	delta := model.Vector3{X: 3, Y: 1, Z: -1}
	lod1 := boxGraphicalLOD(2.0,
		model.Vector3{X: delta.X, Y: delta.Y, Z: delta.Z},
		model.Vector3{X: 10 + delta.X, Y: 4 + delta.Y, Z: 2 + delta.Z},
	)

	m, err := Build([]GraphicalLOD{lod0, lod1}, defaultOptions())
	require.NoError(t, err)

	out0 := findLOD(t, m, 1.0)
	out1 := findLOD(t, m, 2.0)
	require.Len(t, out0.Vertices, len(out1.Vertices))
	for i := range out0.Vertices {
		assert.InDelta(t, delta.X, out1.Vertices[i].X-out0.Vertices[i].X, 1e-4)
		assert.InDelta(t, delta.Y, out1.Vertices[i].Y-out0.Vertices[i].Y, 1e-4)
		assert.InDelta(t, delta.Z, out1.Vertices[i].Z-out0.Vertices[i].Z, 1e-4)
	}
}

func TestBuildAuxLODsAlsoRecentered(t *testing.T) {
	lod := boxGraphicalLOD(1.0, model.Vector3{X: 1, Y: 2, Z: 3}, model.Vector3{X: 5, Y: 10, Z: 9})
	m, err := Build([]GraphicalLOD{lod}, defaultOptions())
	require.NoError(t, err)

	geom := findLOD(t, m, GeometryResolution)
	min, max := vertexMinMax(t, geom.Vertices)
	assert.InDelta(t, 0, (min.X+max.X)/2, 1e-4, "Geometry LOD X center")
	assert.InDelta(t, 0, (min.Z+max.Z)/2, 1e-4, "Geometry LOD Z center")
	assert.InDelta(t, 0, min.Y, 1e-4, "Geometry LOD Y floor")
}

func TestBoundingBoxCorners(t *testing.T) {
	vertices := []model.Vector3{
		{X: -1, Y: -2, Z: -3},
		{X: 4, Y: 5, Z: 6},
		{X: 0, Y: 0, Z: 0},
	}
	corners := boundingBox(vertices)
	require.Len(t, corners, 8)

	var minV, maxV model.Vector3
	minV = corners[0]
	maxV = corners[0]
	for _, c := range corners {
		minV.X, maxV.X = min32(minV.X, c.X), max32(maxV.X, c.X)
		minV.Y, maxV.Y = min32(minV.Y, c.Y), max32(maxV.Y, c.Y)
		minV.Z, maxV.Z = min32(minV.Z, c.Z), max32(maxV.Z, c.Z)
	}
	assert.Equal(t, model.Vector3{X: -1, Y: -2, Z: -3}, minV)
	assert.Equal(t, model.Vector3{X: 4, Y: 5, Z: 6}, maxV)

	seen := map[model.Vector3]bool{}
	for _, c := range corners {
		seen[c] = true
	}
	assert.Len(t, seen, 8, "expected 8 distinct corners")
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func TestBoxGeometryNormalsPointOutward(t *testing.T) {
	corners := boundingBox([]model.Vector3{{X: -1, Y: -1, Z: -1}, {X: 1, Y: 1, Z: 1}})
	normals, faces := boxGeometry(corners)

	var center model.Vector3
	for _, c := range corners {
		center.X += c.X / 8
		center.Y += c.Y / 8
		center.Z += c.Z / 8
	}

	require.Len(t, faces, 6, "8-point box hull must be 6 quad faces")
	for _, f := range faces {
		require.Len(t, f.Indices, 4)
		require.Len(t, f.NormalIndices, 4)

		var centroid model.Vector3
		for _, idx := range f.Indices {
			v := corners[idx]
			centroid.X += v.X / 4
			centroid.Y += v.Y / 4
			centroid.Z += v.Z / 4
		}
		outward := model.Vector3{X: centroid.X - center.X, Y: centroid.Y - center.Y, Z: centroid.Z - center.Z}

		n := normals[f.NormalIndices[0]]
		dot := n.X*outward.X + n.Y*outward.Y + n.Z*outward.Z
		assert.Greaterf(t, dot, float32(0), "face normal %v should point away from box center (outward %v)", n, outward)
	}
}

// TestBoxGeometryHasTwelveEdgesNoDiagonals pins down the wireframe shape the
// user expects when inspecting a Geometry LOD box: exactly the cube's own 12
// edges, with no triangulation diagonals cutting across a side.
func TestBoxGeometryHasTwelveEdgesNoDiagonals(t *testing.T) {
	corners := boundingBox([]model.Vector3{{X: -1, Y: -1, Z: -1}, {X: 1, Y: 1, Z: 1}})
	_, faces := boxGeometry(corners)

	type edge struct{ a, b uint32 }
	edges := map[edge]bool{}
	for _, f := range faces {
		n := len(f.Indices)
		for i := 0; i < n; i++ {
			a, b := f.Indices[i], f.Indices[(i+1)%n]
			if a > b {
				a, b = b, a
			}
			edges[edge{a, b}] = true
		}
	}
	assert.Len(t, edges, 12, "box wireframe must be exactly the cube's 12 edges, with no diagonals")
}

func TestBuildRejectsNonGraphicalResolution(t *testing.T) {
	_, err := Build([]GraphicalLOD{sampleGraphicalLOD(GeometryResolution, 0)}, defaultOptions())
	assert.Error(t, err)
}

func TestBuildRejectsDuplicateResolutions(t *testing.T) {
	_, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0), sampleGraphicalLOD(1.0, 5)}, defaultOptions())
	assert.Error(t, err)
}

func TestBuildRejectsNoGraphicalLODs(t *testing.T) {
	_, err := Build(nil, defaultOptions())
	assert.Error(t, err)
}

// TestBuildOutputRoundTripsThroughMLODMarshalAndParse validates the whole
// assembled model (including the zero-face Memory LOD, an edge case
// internal/mlod's existing tests never exercise) against the real MLOD
// writer and parser, end to end.
func TestBuildOutputRoundTripsThroughMLODMarshalAndParse(t *testing.T) {
	opts := defaultOptions()
	m, err := Build([]GraphicalLOD{sampleGraphicalLOD(1.0, 0), sampleGraphicalLOD(2.0, 0)}, opts)
	require.NoError(t, err)

	data, err := mlod.Marshal(m)
	require.NoError(t, err)

	parsed, err := mlod.Parse(data, "test")
	require.NoError(t, err)

	require.Len(t, parsed.LODs, 6)
	mem := findLOD(t, parsed, MemoryResolution)
	assert.Len(t, mem.Vertices, 5)
	assert.Empty(t, mem.Faces)

	geom := findLOD(t, parsed, GeometryResolution)
	assert.Len(t, geom.PointMasses, 8)
}
