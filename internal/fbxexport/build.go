package fbxexport

import (
	"fmt"
	"sort"

	"github.com/jmhobbs/odol/internal/model"
)

type materialKey struct {
	texture  string
	material string
}

type idCounter struct {
	current int64
}

func (c *idCounter) next() int64 {
	c.current++
	return c.current
}

func buildScene(m *model.Model) (*meshScene, error) {
	lod, err := selectLOD(m)
	if err != nil {
		return nil, err
	}
	stripped := stripProxySelections(*lod)
	lod = &stripped

	ids := &idCounter{}
	geomID := ids.next()
	meshID := ids.next()

	vertices := make([]float64, 0, len(lod.Vertices)*3)
	for _, v := range lod.Vertices {
		vertices = append(vertices, float64(v.X), float64(v.Y), float64(v.Z))
	}

	var polyIndex []int32
	var normals []float64
	var primaryUVs []float64

	// ODOL/MLOD face vertices are wound counter-clockwise when viewed from
	// outside the surface; FBX (and DirectX/GPU-culling conventions
	// generally) expects clockwise front-facing winding. Emitting vertices
	// in reverse per-face order corrects this - readers that derive
	// culling from winding order (e.g. ArmorPaint, real-time viewports)
	// would otherwise cull every face, since our winding disagreed with
	// our own (correctly-oriented) explicit normals. Confirmed by
	// comparing the geometric normal (cross product from winding) against
	// the exported vertex normal for both a canonical DayZ FBX export and
	// our own output. NormalIndices and UVs are per-polygon-vertex
	// ("Direct"-mapped) too, so they must be reversed in the same order to
	// stay aligned slot-for-slot with the reversed indices.
	for _, face := range lod.Faces {
		n := len(face.Indices)

		for slot := 0; slot < n; slot++ {
			idx := face.Indices[n-1-slot]
			if slot == n-1 {
				polyIndex = append(polyIndex, -int32(idx)-1)
			} else {
				polyIndex = append(polyIndex, int32(idx))
			}
		}

		for slot := 0; slot < n; slot++ {
			srcI := n - 1 - slot
			normalIdx := uint32(0)
			if srcI < len(face.NormalIndices) {
				normalIdx = face.NormalIndices[srcI]
			}
			if int(normalIdx) < len(lod.Normals) {
				nrm := lod.Normals[normalIdx]
				// Z is inverted relative to FBX's convention - confirmed
				// against canonical DayZ FBX SDK output; X, Y, and vertex
				// positions are not affected. See TestBuildSceneNormalZIsNegated.
				normals = append(normals, float64(nrm.X), float64(nrm.Y), -float64(nrm.Z))
			} else {
				normals = append(normals, 0, 0, 0)
			}
		}

		for slot := 0; slot < n; slot++ {
			srcI := n - 1 - slot
			var uv model.UV
			if srcI < len(face.UVs) {
				uv = face.UVs[srcI]
			}
			primaryUVs = append(primaryUVs, float64(uv.U), float64(uv.V))
		}
	}

	uvSets := [][]float64{primaryUVs}
	for setIdx := 1; setIdx < len(lod.UVSets); setIdx++ {
		uvSet := lod.UVSets[setIdx]
		var channel []float64
		for _, face := range lod.Faces {
			n := len(face.Indices)
			for slot := 0; slot < n; slot++ {
				vi := face.Indices[n-1-slot]
				var uv model.UV
				if int(vi) < len(uvSet) {
					uv = uvSet[vi]
				}
				channel = append(channel, float64(uv.U), float64(uv.V))
			}
		}
		uvSets = append(uvSets, channel)
	}

	var matKeys []materialKey
	matIndexOf := make(map[materialKey]int32)
	for _, face := range lod.Faces {
		key := materialKey{face.Texture, face.Material}
		if _, ok := matIndexOf[key]; !ok {
			matIndexOf[key] = int32(len(matKeys))
			matKeys = append(matKeys, key)
		}
	}

	matIndex := make([]int32, len(lod.Faces))
	for i, face := range lod.Faces {
		matIndex[i] = matIndexOf[materialKey{face.Texture, face.Material}]
	}

	materials := make([]materialData, len(matKeys))
	for i, key := range matKeys {
		materials[i] = materialData{
			id:        ids.next(),
			name:      fmt.Sprintf("Mat_%d", i),
			texture:   key.texture,
			mat:       key.material,
			videoID:   ids.next(),
			textureID: ids.next(),
		}
	}

	proxies := make([]proxyData, len(lod.Proxies))
	for i, p := range lod.Proxies {
		proxies[i] = proxyData{
			id:            ids.next(),
			name:          fmt.Sprintf("Proxy_%d", i),
			path:          p.ModelPath,
			sequenceID:    p.SequenceID,
			selectionName: p.NamedSelectionName,
			sectionIndex:  p.SectionIndex,
		}
	}

	selections := make([]selectionData, len(lod.Selections))
	for i, sel := range lod.Selections {
		selections[i] = selectionData{
			id:            ids.next(),
			name:          sel.Name,
			isSectional:   sel.IsSectional,
			vertexIndices: sel.VertexIndices,
			vertexWeights: sel.VertexWeights,
			faceIndices:   sel.FaceIndices,
		}
	}

	propKeys, filteredProps := filteredProperties(lod)

	return &meshScene{
		modelName: m.Name,
		geometry: geometryData{
			id:        geomID,
			vertices:  vertices,
			polyIndex: polyIndex,
			normals:   normals,
			uvSets:    uvSets,
			matIndex:  matIndex,
		},
		mesh: meshData{
			id:               meshID,
			sourceFamily:     m.Source.Family,
			sourceVersion:    m.Source.Version,
			lodResolution:    lod.Resolution,
			odolProperties:   filteredProps,
			odolPropertyKeys: propKeys,
			selections:       selections,
			iconColor:        lod.IconColor,
			selectedColor:    lod.SelectedColor,
		},
		materials: materials,
		proxies:   proxies,
	}, nil
}

// filteredProperties returns the LOD property map and an ordered key slice,
// with the odol_partial_parse sentinel removed.
func filteredProperties(lod *model.LOD) ([]string, map[string]string) {
	filtered := make(map[string]string, len(lod.Properties))
	for k, v := range lod.Properties {
		if k != "odol_partial_parse" {
			filtered[k] = v
		}
	}

	seen := make(map[string]struct{}, len(filtered))
	var keys []string
	for _, k := range lod.PropertyOrder {
		if _, ok := filtered[k]; !ok {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		keys = append(keys, k)
		seen[k] = struct{}{}
	}

	var extra []string
	for k := range filtered {
		if _, ok := seen[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)

	return keys, filtered
}
