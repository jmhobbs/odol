package inspect

import (
	"fmt"
	"io"
	"strings"
)

// WriteText renders s as a human-readable report.
func WriteText(w io.Writer, s Summary) error {
	var b strings.Builder

	fmt.Fprintf(&b, "Model: %s\n", s.Name)
	fmt.Fprintf(&b, "Format: %s (source %s v%d)\n", s.DetectedFormat, s.SourceFamily, s.SourceVersion)
	fmt.Fprintf(&b, "Center of mass: (%g, %g, %g)\n", s.CenterOfMass.X, s.CenterOfMass.Y, s.CenterOfMass.Z)

	if len(s.Skeletons) > 0 {
		fmt.Fprintln(&b, "\nSkeletons:")
		for _, skel := range s.Skeletons {
			fmt.Fprintf(&b, "  %s (%d bones)\n", skel.Name, skel.BoneCount)
		}
	}

	if len(s.ModelClasses) > 0 {
		fmt.Fprintln(&b, "\nModel classes:")
		for _, mc := range s.ModelClasses {
			fmt.Fprintf(&b, "  %s", mc.Name)
			if mc.SkeletonName != "" {
				fmt.Fprintf(&b, " (skeleton: %s)", mc.SkeletonName)
			}
			fmt.Fprintf(&b, " - %d animation(s), %d section(s)\n", mc.AnimationCount, len(mc.Sections))
		}
	}

	fmt.Fprintf(&b, "\nLODs (%d):\n", len(s.LODs))
	for _, lod := range s.LODs {
		partial := ""
		if lod.Partial {
			partial = " (partially decoded)"
		}
		fmt.Fprintf(&b, "  [%d] %s (resolution %g)%s\n", lod.Index, lod.ResolutionName, lod.Resolution, partial)
		fmt.Fprintf(&b, "      vertices: %d, faces: %d, uv sets: %d\n", lod.VertexCount, lod.FaceCount, lod.UVSetCount)
		if lod.BoundingSize != nil {
			fmt.Fprintf(&b, "      bounding size (X,Y,Z): %.4fm x %.4fm x %.4fm\n",
				lod.BoundingSize.X, lod.BoundingSize.Y, lod.BoundingSize.Z)
		}
		if len(lod.Textures) > 0 {
			fmt.Fprintf(&b, "      textures: %s\n", strings.Join(lod.Textures, ", "))
		}
		if len(lod.Materials) > 0 {
			fmt.Fprintf(&b, "      materials: %s\n", strings.Join(lod.Materials, ", "))
		}
		if len(lod.Selections) > 0 {
			fmt.Fprintln(&b, "      named selections:")
			for _, sel := range lod.Selections {
				kind := "vertex"
				if sel.IsSectional {
					kind = "sectional"
				}
				fmt.Fprintf(&b, "        %s (%s, %d vertices, %d faces)\n", sel.Name, kind, sel.VertexCount, sel.FaceCount)
			}
		}
		if len(lod.Proxies) > 0 {
			fmt.Fprintln(&b, "      proxies:")
			for _, proxy := range lod.Proxies {
				fmt.Fprintf(&b, "        %s (sequence %d)\n", proxy.ModelPath, proxy.SequenceID)
			}
		}
	}

	fmt.Fprintf(&b, "\nTextures referenced (%d):\n", len(s.Textures))
	for _, tex := range s.Textures {
		fmt.Fprintf(&b, "  %s\n", tex)
	}

	fmt.Fprintf(&b, "\nMaterials referenced (%d):\n", len(s.Materials))
	for _, mat := range s.Materials {
		fmt.Fprintf(&b, "  %s\n", mat)
	}

	_, err := io.WriteString(w, b.String())
	return err
}
