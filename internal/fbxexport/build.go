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

	matKeys, matIndexOf := collectMaterialKeys(lod.Faces)
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

	groups := groupConnectedComponents(lod.Faces, len(lod.Vertices))
	parts := make([]meshPart, len(groups))
	for i, group := range groups {
		parts[i] = buildPart(lod, group, i+1, matIndexOf, ids)
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
		parts:            parts,
		materials:        materials,
		proxies:          proxies,
		selections:       selections,
		sourceFamily:     m.Source.Family,
		sourceVersion:    m.Source.Version,
		lodResolution:    lod.Resolution,
		odolProperties:   filteredProps,
		odolPropertyKeys: propKeys,
		iconColor:        lod.IconColor,
		selectedColor:    lod.SelectedColor,
	}, nil
}

// collectMaterialKeys returns the distinct (texture, material) pairs used
// across faces, in order of first appearance, along with a lookup from key
// to its index in that list. Shared across all parts of a scene, since a
// texture/material can be used by faces in more than one part (e.g. a camo
// pattern spread across many small physically-separate pieces).
func collectMaterialKeys(faces []model.Face) ([]materialKey, map[materialKey]int32) {
	var matKeys []materialKey
	matIndexOf := make(map[materialKey]int32)
	for _, face := range faces {
		key := materialKey{face.Texture, face.Material}
		if _, ok := matIndexOf[key]; !ok {
			matIndexOf[key] = int32(len(matKeys))
			matKeys = append(matKeys, key)
		}
	}
	return matKeys, matIndexOf
}

// buildPart builds one exported mesh object from a connected component of
// the source LOD's faces, remapping vertex/material references to the
// part's own local index space. partNumber is 1-based and becomes part of
// the part's name ("object_NNN{partNumber}").
func buildPart(lod *model.LOD, group componentGroup, partNumber int, matIndexOf map[materialKey]int32, ids *idCounter) meshPart {
	vertexRemap := make(map[uint32]int32, len(group.faceIndices))
	var vertices []float64
	var polyIndex []int32
	var normals []float64
	var primaryUVs []float64

	localVertexIndex := func(orig uint32) int32 {
		if li, ok := vertexRemap[orig]; ok {
			return li
		}
		li := int32(len(vertices) / 3)
		vertexRemap[orig] = li
		v := lod.Vertices[orig]
		vertices = append(vertices, float64(v.X), float64(v.Y), float64(v.Z))
		return li
	}

	// See the winding-order comment in the pre-split version of this
	// function (git history) for why vertices/normals/UVs are emitted in
	// reverse per-face order - unchanged by the per-part split, just scoped
	// to this component's own faces.
	for _, faceIdx := range group.faceIndices {
		face := lod.Faces[faceIdx]
		n := len(face.Indices)

		for slot := 0; slot < n; slot++ {
			localIdx := localVertexIndex(face.Indices[n-1-slot])
			if slot == n-1 {
				polyIndex = append(polyIndex, -localIdx-1)
			} else {
				polyIndex = append(polyIndex, localIdx)
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
		for _, faceIdx := range group.faceIndices {
			face := lod.Faces[faceIdx]
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

	var localMaterials []int
	localIndexOfGlobal := make(map[int32]int32)
	matIndex := make([]int32, len(group.faceIndices))
	for i, faceIdx := range group.faceIndices {
		face := lod.Faces[faceIdx]
		globalIdx := matIndexOf[materialKey{face.Texture, face.Material}]
		localIdx, ok := localIndexOfGlobal[globalIdx]
		if !ok {
			localIdx = int32(len(localMaterials))
			localIndexOfGlobal[globalIdx] = localIdx
			localMaterials = append(localMaterials, int(globalIdx))
		}
		matIndex[i] = localIdx
	}

	return meshPart{
		name:    fmt.Sprintf("object_NNN%d", partNumber),
		geomID:  ids.next(),
		modelID: ids.next(),
		geometry: geometryData{
			vertices:  vertices,
			polyIndex: polyIndex,
			normals:   normals,
			uvSets:    uvSets,
			matIndex:  matIndex,
		},
		localMaterials: localMaterials,
	}
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
