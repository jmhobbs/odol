package fbx2p3d

import (
	"fmt"

	"github.com/jmhobbs/odol/internal/inspect"
	"github.com/jmhobbs/odol/p3d/detector"
	"github.com/jmhobbs/odol/p3d/lodname"
	"github.com/jmhobbs/odol/p3d/model"
)

// MeshBuffers is one LOD's geometry flattened for WebGL.
type MeshBuffers struct {
	// Positions holds x, y, z for every MLOD point, in meters, in P3D axes
	// (no mirroring - the same axes internal/fbxexport writes, and so the
	// same axes the P3D converter page's FBX preview shows).
	Positions []float32
	// Triangles holds 3 point indices per triangle, counter-clockwise front
	// faces (WebGL's default).
	Triangles []uint32
}

// TriangulateForWebGL fans every face from its first point, the same
// diagonal the engine uses for an MLOD quad, and reverses MLOD's clockwise
// front-face winding to WebGL's counter-clockwise one.
func TriangulateForWebGL(lod model.LOD) MeshBuffers {
	mesh := MeshBuffers{Positions: make([]float32, 0, len(lod.Vertices)*3)}
	for _, v := range lod.Vertices {
		mesh.Positions = append(mesh.Positions, v.X, v.Y, v.Z)
	}
	for _, face := range lod.Faces {
		for k := 1; k+1 < len(face.Indices); k++ {
			mesh.Triangles = append(mesh.Triangles, face.Indices[0], face.Indices[k+1], face.Indices[k])
		}
	}
	return mesh
}

// LODPreview is one built LOD as the browser page shows it.
type LODPreview struct {
	Name       string
	Resolution float32
	Graphical  bool
	// VertexCount is the MLOD point count, as Object Builder and p3dinspect
	// report it.
	VertexCount   int
	FaceCount     int
	TriangleCount int
	// Selections lists the LOD's named selection names. On the Memory LOD
	// each one names a single point.
	Selections []string
	Mesh       MeshBuffers
}

// ModelPreview is every built LOD plus a size check of the highest-detail
// graphical LOD.
type ModelPreview struct {
	LODs []LODPreview
	// HighestDetailIndex indexes LODs: the graphical LOD with the lowest
	// resolution, which the page draws as a ghost behind every other LOD.
	// -1 if there is no graphical LOD.
	HighestDetailIndex int
	// BoundingSize is the highest-detail LOD's (width, height, depth) in
	// meters, after scale and recentering.
	BoundingSize model.Vector3
	// HeightRatio is BoundingSize.Y over ReferenceHumanHeightMeters, the
	// same comparison the CLI prints before its y/N prompt.
	HeightRatio float64
}

// Preview summarizes every LOD of a built model for the browser page.
func Preview(m *model.Model) ModelPreview {
	summary := inspect.Summarize(m, detector.P3DFormat{Family: detector.P3DFamilyMLOD})
	preview := ModelPreview{HighestDetailIndex: -1}
	for i, lod := range m.LODs {
		lodSummary := summary.LODs[i]
		graphical := lodname.IsGraphical(lod.Resolution)

		name := lodSummary.ResolutionName
		if graphical {
			name = fmt.Sprintf("Resolution %g", lod.Resolution)
		}
		selections := make([]string, len(lodSummary.Selections))
		for j, sel := range lodSummary.Selections {
			selections[j] = sel.Name
		}

		preview.LODs = append(preview.LODs, LODPreview{
			Name:          name,
			Resolution:    lod.Resolution,
			Graphical:     graphical,
			VertexCount:   lodSummary.VertexCount,
			FaceCount:     lodSummary.FaceCount,
			TriangleCount: lodSummary.TriangleCount,
			Selections:    selections,
			Mesh:          TriangulateForWebGL(lod),
		})

		if graphical && (preview.HighestDetailIndex < 0 || lod.Resolution < m.LODs[preview.HighestDetailIndex].Resolution) {
			preview.HighestDetailIndex = i
		}
	}

	if preview.HighestDetailIndex >= 0 {
		preview.BoundingSize = BoundingBoxSize(m.LODs[preview.HighestDetailIndex].Vertices)
		preview.HeightRatio = float64(preview.BoundingSize.Y) / ReferenceHumanHeightMeters
	}
	return preview
}
