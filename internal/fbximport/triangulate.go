package fbximport

import (
	"math"

	"github.com/jmhobbs/odol/internal/model"
)

// point2D is a polygon corner projected onto the polygon's plane.
type point2D struct {
	x, y float64
}

// triangulatePolygon splits one polygon into len(corners)-2 triangles by ear
// clipping. Each triangle is three positions into corners, in the polygon's
// own winding order. It handles concave polygons, which a fan from corner 0
// gets wrong: the fan's triangles flip or span notches in the outline.
//
// The walk starts at corner 1 and clips the first valid ear it finds, then
// continues from the corner after it. On a strictly convex polygon every
// corner is an ear, so this clips corners 1, 2, 3, ... in order and yields
// exactly the fan from corner 0.
//
// clean is false when some step found no valid ear and had to clip a
// fallback one. That only happens for self-intersecting or zero-area
// polygons. The result still has len(corners)-2 triangles, but some of them
// may be flipped or overlap.
func triangulatePolygon(corners []model.Vector3) (triangles [][3]int, clean bool) {
	if len(corners) < 3 {
		return nil, false
	}
	points := projectOntoPolygonPlane(corners)
	epsilon := collinearEpsilon(points)

	ring := make([]int, len(corners))
	for i := range ring {
		ring[i] = i
	}

	clean = true
	cursor := 1
	for len(ring) > 3 {
		ear, found := findEar(points, ring, cursor, epsilon)
		if !found {
			clean = false
			ear = fallbackEar(points, ring, cursor, epsilon)
		}
		prev, next := ringNeighbors(ring, ear)
		triangles = append(triangles, [3]int{ring[prev], ring[ear], ring[next]})
		ring = append(ring[:ear], ring[ear+1:]...)
		cursor = ear % len(ring)
	}
	triangles = append(triangles, [3]int{ring[0], ring[1], ring[2]})
	return triangles, clean
}

// quadFoldsOnEngineDiagonal reports whether the two triangles from splitting
// a quad along corner 0 to corner 2, as the engine does for an MLOD quad
// face, have normals more than 90 degrees apart. That covers a concave quad
// whose reflex corner is 1 or 3, and a non-planar quad sharply creased along
// that diagonal.
//
// This is the same test Blender's mesh tessellation uses to draw such a quad
// split along corner 1 to corner 3 instead (is_quad_flip_v3_first_third_fast
// in source/blender/blenlib/intern/math_geom.cc), so the artist saw the 1-3
// split. It ignores the face normal on purpose, as Blender's does.
func quadFoldsOnEngineDiagonal(corners []model.Vector3) bool {
	first := triangleNormal(corners[0], corners[1], corners[2])
	second := triangleNormal(corners[0], corners[2], corners[3])
	return dot3(first, second) < 0
}

// findEar returns the ring position of the first valid ear at or after
// cursor, wrapping around the ring once.
func findEar(points []point2D, ring []int, cursor int, epsilon float64) (int, bool) {
	for step := range ring {
		i := (cursor + step) % len(ring)
		if isEar(points, ring, i, epsilon) {
			return i, true
		}
	}
	return 0, false
}

// isEar reports whether the corner at ring position i can be clipped: its
// triangle with both ring neighbors turns counterclockwise (the corner is
// convex, not collinear) and no other remaining corner lies inside or on
// that triangle. Corners at the same position as one of the triangle's
// corners are skipped, so duplicated points don't block every ear.
func isEar(points []point2D, ring []int, i int, epsilon float64) bool {
	prev, next := ringNeighbors(ring, i)
	a, b, c := points[ring[prev]], points[ring[i]], points[ring[next]]
	if turn(a, b, c) <= epsilon {
		return false
	}
	for j, corner := range ring {
		if j == prev || j == i || j == next {
			continue
		}
		p := points[corner]
		if p == a || p == b || p == c {
			continue
		}
		if insideOrOnTriangle(p, a, b, c, epsilon) {
			return false
		}
	}
	return true
}

// fallbackEar picks the ring position to clip when no valid ear exists. In
// order of preference: a collinear corner (its triangle has no area), a
// convex corner even though another corner lies inside its triangle, or the
// cursor corner.
func fallbackEar(points []point2D, ring []int, cursor int, epsilon float64) int {
	firstConvex := -1
	for step := range ring {
		i := (cursor + step) % len(ring)
		prev, next := ringNeighbors(ring, i)
		t := turn(points[ring[prev]], points[ring[i]], points[ring[next]])
		if math.Abs(t) <= epsilon {
			return i
		}
		if t > epsilon && firstConvex < 0 {
			firstConvex = i
		}
	}
	if firstConvex >= 0 {
		return firstConvex
	}
	return cursor
}

func ringNeighbors(ring []int, i int) (prev, next int) {
	return (i + len(ring) - 1) % len(ring), (i + 1) % len(ring)
}

// turn returns twice the signed area of triangle a, b, c. It is positive
// when a, b, c wind counterclockwise.
func turn(a, b, c point2D) float64 {
	return (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
}

// insideOrOnTriangle reports whether p lies inside or on the edges of the
// counterclockwise triangle a, b, c.
func insideOrOnTriangle(p, a, b, c point2D, epsilon float64) bool {
	return turn(a, b, p) >= -epsilon && turn(b, c, p) >= -epsilon && turn(c, a, p) >= -epsilon
}

// collinearEpsilon is the turn magnitude at or below which three projected
// corners count as collinear. It scales with the outline's extent, so the
// same relative tolerance applies whatever the source units are.
func collinearEpsilon(points []point2D) float64 {
	minX, maxX := points[0].x, points[0].x
	minY, maxY := points[0].y, points[0].y
	for _, p := range points[1:] {
		minX, maxX = math.Min(minX, p.x), math.Max(maxX, p.x)
		minY, maxY = math.Min(minY, p.y), math.Max(maxY, p.y)
	}
	extent := math.Max(maxX-minX, maxY-minY)
	return extent * extent * 1e-12
}

// projectOntoPolygonPlane drops the axis the polygon's Newell normal points
// along most. The other two axes keep their cyclic order (Y,Z or Z,X or X,Y)
// and are swapped when that normal component is negative, so the projected
// outline always winds counterclockwise. A zero-area polygon has a zero
// normal and is projected onto X,Y.
func projectOntoPolygonPlane(corners []model.Vector3) []point2D {
	normal := newellNormal(corners)
	absX, absY, absZ := math.Abs(normal[0]), math.Abs(normal[1]), math.Abs(normal[2])

	uAxis, vAxis, dominant := 0, 1, normal[2]
	switch {
	case absX > absY && absX > absZ:
		uAxis, vAxis, dominant = 1, 2, normal[0]
	case absY > absZ:
		uAxis, vAxis, dominant = 2, 0, normal[1]
	}
	if dominant < 0 {
		uAxis, vAxis = vAxis, uAxis
	}

	points := make([]point2D, len(corners))
	for i, c := range corners {
		p := vector3ToFloat64(c)
		points[i] = point2D{x: p[uAxis], y: p[vAxis]}
	}
	return points
}

// newellNormal returns the polygon's Newell normal: twice its vector area,
// pointing along the side its corners wind counterclockwise around. It is
// well defined for concave and slightly non-planar polygons.
func newellNormal(corners []model.Vector3) [3]float64 {
	var normal [3]float64
	for i := range corners {
		a := vector3ToFloat64(corners[i])
		b := vector3ToFloat64(corners[(i+1)%len(corners)])
		normal[0] += (a[1] - b[1]) * (a[2] + b[2])
		normal[1] += (a[2] - b[2]) * (a[0] + b[0])
		normal[2] += (a[0] - b[0]) * (a[1] + b[1])
	}
	return normal
}

// triangleNormal returns the right-hand-rule normal of triangle a, b, c,
// scaled by twice its area.
func triangleNormal(a, b, c model.Vector3) [3]float64 {
	pa, pb, pc := vector3ToFloat64(a), vector3ToFloat64(b), vector3ToFloat64(c)
	e1 := [3]float64{pb[0] - pa[0], pb[1] - pa[1], pb[2] - pa[2]}
	e2 := [3]float64{pc[0] - pa[0], pc[1] - pa[1], pc[2] - pa[2]}
	return [3]float64{
		e1[1]*e2[2] - e1[2]*e2[1],
		e1[2]*e2[0] - e1[0]*e2[2],
		e1[0]*e2[1] - e1[1]*e2[0],
	}
}

func dot3(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func vector3ToFloat64(v model.Vector3) [3]float64 {
	return [3]float64{float64(v.X), float64(v.Y), float64(v.Z)}
}
