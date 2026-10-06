// Package inspect summarizes a parsed P3D model.Model for reporting: LOD
// list, point/face counts, textures, materials, and named selections.
package inspect

import "math"

type resolutionEntry struct {
	bits uint32
	name string
}

// namedResolutions lists Bohemia's documented resolution values
// (docs/P3DModelInfo.txt) that map to a specific LOD type. Comparisons use
// the exact float32 bit pattern, since these constants are only meaningful
// bit-for-bit (the same technique internal/odol uses for its Geometry LOD
// check).
var namedResolutions = []resolutionEntry{
	{0x447a0000, "View Gunner"},
	{0x44898000, "View Pilot"},
	{0x44960000, "View Cargo"},
	{0x461c4000, "Shadow Volume"},
	{0x461c6800, "Shadow Volume 2"},
	{0x462be000, "Stencil Shadow"},
	{0x462c0800, "Stencil Shadow 2"},
	{0x551184e7, "Geometry"},
	{0x58635fa9, "Memory"},
	{0x58e35fa9, "Land Contact"},
	{0x592a87bf, "Roadway"},
	{0x59635fa9, "Paths"},
	{0x598e1bca, "HitPoints"},
	{0x59aa87bf, "View Geometry"},
	{0x59c6f3b4, "Fire Geometry"},
	{0x59e35fa9, "View Cargo Geometry"},
	{0x59ffcb9e, "View Cargo Fire Geometry"},
	{0x5a0e1bca, "View Commander"},
	{0x5a1c51c4, "View Commander Geometry"},
	{0x5a2a87bf, "View Commander Fire Geometry"},
	{0x5a38bdb9, "View Pilot Geometry"},
	{0x5a46f3b4, "View Pilot Fire Geometry"},
	{0x5a5529af, "View Gunner Geometry"},
	{0x5a635fa9, "View Gunner Fire Geometry"},
	{0x5a7195a4, "Sub Parts"},
	{0x5a7fcb9e, "Shadow Volume View Cargo"},
	{0x5a8700cc, "Shadow Volume View Pilot"},
	{0x5a8e1bca, "Shadow Volume View Gunner"},
	{0x5a9536c7, "Wreck"},
}

// ResolutionName maps a LOD's raw resolution value to a human-readable name
// using Bohemia's documented resolution table. Values below 1000 are
// graphical LODs (their specific rank isn't encoded in the P3D itself, only
// in Oxygen/model.cfg), and unrecognized values fall back to a generic
// functional/shadow label based on the documented range boundaries.
func ResolutionName(resolution float32) string {
	bits := math.Float32bits(resolution)
	for _, entry := range namedResolutions {
		if entry.bits == bits {
			return entry.name
		}
	}

	switch {
	case IsGraphicalResolution(resolution):
		return "Graphical LOD"
	case resolution < 10000:
		return "Unknown functional LOD"
	default:
		return "Unknown shadow LOD"
	}
}

// IsGraphicalResolution reports whether resolution identifies a graphical
// LOD - one meant to be rendered, as opposed to Bohemia's functional/
// shadow/geometry/memory LODs - per the <1000 threshold documented in
// docs/P3DModelInfo.txt.
func IsGraphicalResolution(resolution float32) bool {
	return resolution < 1000
}
