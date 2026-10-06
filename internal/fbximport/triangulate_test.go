package fbximport

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

// uShapeCorners is a concave, U-shaped 8-corner outline in the XY plane,
// wound CCW around +Z, with corner 0 at the base of the left prong. A fan
// from corner 0 covers the notch between the prongs and flips triangle
// (0,0)-(2,3)-(2,1), the same failure real concave n-gons (e.g. gravestone
// cross outlines) hit with the old fan triangulation.
func uShapeCorners() []model.Vector3 {
	return []model.Vector3{
		{X: 0, Y: 0}, {X: 3, Y: 0}, {X: 3, Y: 3}, {X: 2, Y: 3},
		{X: 2, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 3}, {X: 0, Y: 3},
	}
}

// dartQuadCorners is a concave quad wound CCW around +Z whose reflex corner
// is corner 1, so the engine's corner 0 to corner 2 split diagonal runs
// outside the quad.
func dartQuadCorners() []model.Vector3 {
	return []model.Vector3{{X: 0, Y: 0}, {X: 2, Y: 1}, {X: 4, Y: 0}, {X: 2, Y: 3}}
}

func toFloat64Vector(v model.Vector3) [3]float64 {
	return [3]float64{float64(v.X), float64(v.Y), float64(v.Z)}
}

func subtractFloat64Vectors(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func crossFloat64Vectors(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func dotFloat64Vectors(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func normalizeFloat64Vector(v [3]float64) [3]float64 {
	length := math.Sqrt(dotFloat64Vectors(v, v))
	return [3]float64{v[0] / length, v[1] / length, v[2] / length}
}

// polygonAreaVector returns twice the polygon's vector area, as the sum of
// cross products over a fan from corner 0. Signed areas cancel, so this is
// correct for concave polygons too, and it is independent of the Newell
// formula the implementation uses.
func polygonAreaVector(corners []model.Vector3) [3]float64 {
	var sum [3]float64
	origin := toFloat64Vector(corners[0])
	for i := 1; i < len(corners)-1; i++ {
		c := crossFloat64Vectors(
			subtractFloat64Vectors(toFloat64Vector(corners[i]), origin),
			subtractFloat64Vectors(toFloat64Vector(corners[i+1]), origin),
		)
		sum = [3]float64{sum[0] + c[0], sum[1] + c[1], sum[2] + c[2]}
	}
	return sum
}

// inPlaneCoordinates maps corners onto 2D coordinates in the plane with the
// given unit normal.
func inPlaneCoordinates(corners []model.Vector3, unitNormal [3]float64) [][2]float64 {
	helper := [3]float64{1, 0, 0}
	if math.Abs(unitNormal[0]) > 0.9 {
		helper = [3]float64{0, 1, 0}
	}
	u := normalizeFloat64Vector(crossFloat64Vectors(helper, unitNormal))
	v := crossFloat64Vectors(unitNormal, u)

	out := make([][2]float64, len(corners))
	for i, c := range corners {
		p := toFloat64Vector(c)
		out[i] = [2]float64{dotFloat64Vectors(p, u), dotFloat64Vectors(p, v)}
	}
	return out
}

// pointInsideOutline is an even-odd crossing test.
func pointInsideOutline(p [2]float64, outline [][2]float64) bool {
	inside := false
	for i, j := 0, len(outline)-1; i < len(outline); j, i = i, i+1 {
		a, b := outline[i], outline[j]
		if (a[1] > p[1]) != (b[1] > p[1]) {
			crossingX := a[0] + (p[1]-a[1])*(b[0]-a[0])/(b[1]-a[1])
			if p[0] < crossingX {
				inside = !inside
			}
		}
	}
	return inside
}

// assertCoversPolygon checks that triangles exactly cover the polygon: n-2
// triangles, each facing the same way as the polygon (no flipped or
// zero-area triangles), triangle areas summing to the polygon's area (no
// overlap), and every triangle's centroid inside the outline (no triangle
// spanning a notch).
func assertCoversPolygon(t *testing.T, corners []model.Vector3, triangles [][3]int) {
	t.Helper()
	require.Len(t, triangles, len(corners)-2)

	areaVector := polygonAreaVector(corners)
	polygonArea := math.Sqrt(dotFloat64Vectors(areaVector, areaVector)) / 2
	require.Greater(t, polygonArea, 0.0)
	unitNormal := normalizeFloat64Vector(areaVector)
	outline := inPlaneCoordinates(corners, unitNormal)

	var triangleAreaSum float64
	for _, tri := range triangles {
		for _, corner := range tri {
			require.GreaterOrEqual(t, corner, 0)
			require.Less(t, corner, len(corners))
		}
		a := toFloat64Vector(corners[tri[0]])
		b := toFloat64Vector(corners[tri[1]])
		c := toFloat64Vector(corners[tri[2]])
		signedArea := dotFloat64Vectors(crossFloat64Vectors(subtractFloat64Vectors(b, a), subtractFloat64Vectors(c, a)), unitNormal) / 2
		assert.Greaterf(t, signedArea, 1e-9*polygonArea, "triangle %v is flipped or zero-area", tri)
		triangleAreaSum += math.Abs(signedArea)

		centroid := [2]float64{
			(outline[tri[0]][0] + outline[tri[1]][0] + outline[tri[2]][0]) / 3,
			(outline[tri[0]][1] + outline[tri[1]][1] + outline[tri[2]][1]) / 3,
		}
		assert.Truef(t, pointInsideOutline(centroid, outline), "triangle %v lies outside the polygon", tri)
	}
	assert.InDelta(t, polygonArea, triangleAreaSum, 1e-6*polygonArea, "triangle areas must sum to the polygon area")
}

// TestTriangulatePolygonConvexMatchesFanFromCornerZero pins that a strictly
// convex polygon still produces exactly the fan from corner 0 that the
// importer emitted before ear clipping, so convex n-gons import unchanged.
func TestTriangulatePolygonConvexMatchesFanFromCornerZero(t *testing.T) {
	corners := []model.Vector3{
		{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 100}, {X: 0, Y: 100}, {X: -100, Y: 50},
	}

	triangles, clean := triangulatePolygon(corners)

	assert.True(t, clean)
	assert.Equal(t, [][3]int{{0, 1, 2}, {0, 2, 3}, {0, 3, 4}}, triangles)
	assertCoversPolygon(t, corners, triangles)
}

func TestTriangulatePolygonConcaveUShape(t *testing.T) {
	corners := uShapeCorners()

	triangles, clean := triangulatePolygon(corners)

	assert.True(t, clean)
	assertCoversPolygon(t, corners, triangles)
}

// TestTriangulatePolygonConcaveUShapeInPlaneFacingNegativeY lays the U shape
// in the XZ plane so its normal is -Y. This exercises projecting along a
// different dominant axis, and the axis swap for a negative normal component.
func TestTriangulatePolygonConcaveUShapeInPlaneFacingNegativeY(t *testing.T) {
	var corners []model.Vector3
	for _, c := range uShapeCorners() {
		corners = append(corners, model.Vector3{X: c.X, Y: 0, Z: c.Y})
	}
	areaVector := polygonAreaVector(corners)
	require.Less(t, areaVector[1], 0.0, "fixture must face -Y")

	triangles, clean := triangulatePolygon(corners)

	assert.True(t, clean)
	assertCoversPolygon(t, corners, triangles)
}

func TestTriangulatePolygonDartQuadUsesOtherDiagonal(t *testing.T) {
	corners := dartQuadCorners()

	triangles, clean := triangulatePolygon(corners)

	assert.True(t, clean)
	assert.Equal(t, [][3]int{{1, 2, 3}, {0, 1, 3}}, triangles)
	assertCoversPolygon(t, corners, triangles)
}

// TestTriangulatePolygonCollinearCornerAvoidsZeroAreaTriangles uses a square
// with an extra corner in the middle of one edge (a T-junction left by an
// edge loop). assertCoversPolygon rejects zero-area triangles.
func TestTriangulatePolygonCollinearCornerAvoidsZeroAreaTriangles(t *testing.T) {
	corners := []model.Vector3{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 2}, {X: 0, Y: 2}}

	triangles, clean := triangulatePolygon(corners)

	assert.True(t, clean)
	assertCoversPolygon(t, corners, triangles)
}

// TestTriangulatePolygonAllCollinearFallsBack checks that a polygon with no
// area at all still terminates with n-2 triangles and reports that it needed
// a fallback.
func TestTriangulatePolygonAllCollinearFallsBack(t *testing.T) {
	corners := []model.Vector3{{X: 0}, {X: 1}, {X: 2}, {X: 3}, {X: 4}}

	triangles, clean := triangulatePolygon(corners)

	assert.False(t, clean)
	require.Len(t, triangles, 3)
	for _, tri := range triangles {
		assert.NotEqual(t, tri[0], tri[1])
		assert.NotEqual(t, tri[1], tri[2])
		assert.NotEqual(t, tri[0], tri[2])
		for _, corner := range tri {
			assert.GreaterOrEqual(t, corner, 0)
			assert.Less(t, corner, len(corners))
		}
	}
}

func TestQuadFoldsOnEngineDiagonal(t *testing.T) {
	square := []model.Vector3{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}
	assert.False(t, quadFoldsOnEngineDiagonal(square), "convex quad")

	assert.True(t, quadFoldsOnEngineDiagonal(dartQuadCorners()), "reflex corner 1 puts the 0-2 diagonal outside the quad")

	dart := dartQuadCorners()
	reflexAtCornerZero := []model.Vector3{dart[1], dart[2], dart[3], dart[0]}
	assert.False(t, quadFoldsOnEngineDiagonal(reflexAtCornerZero), "the 0-2 diagonal runs through a reflex corner 0, so it stays inside the quad")

	// A non-planar quad creased along its 0-2 diagonal. Both halves still
	// face along the quad's overall +Z normal, but their normals, (0,4,2)
	// and (0,-4,2), are more than 90 degrees apart. Blender splits this
	// along 1-3, as seen in real Body Bag and altar exports.
	creased := []model.Vector3{{X: 0, Y: 0, Z: 0}, {X: 1, Y: -1, Z: 2}, {X: 2, Y: 0, Z: 0}, {X: 1, Y: 1, Z: 2}}
	assert.True(t, quadFoldsOnEngineDiagonal(creased), "halves more than 90 degrees apart")
}
