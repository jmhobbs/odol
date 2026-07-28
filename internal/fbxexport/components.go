package fbxexport

import "github.com/jmhobbs/odol/internal/model"

// componentGroup holds the indices (into the original faces slice) of the
// faces belonging to one connected component.
type componentGroup struct {
	faceIndices []int
}

type unionFind struct {
	parent []int
}

func newUnionFind(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// groupConnectedComponents partitions faces into connected components by
// shared vertex index: two faces belong to the same component if they share
// a vertex, transitively. This matches canonical DayZ FBX exports, which
// split a mesh into separate named objects along exactly these boundaries
// (confirmed against samples/fbx/mich2001.fbx and samples/fbx/sv98.fbx -
// component vertex/face counts match every exported object exactly).
//
// Components are ordered by the index of their first (lowest-index) face,
// so output is deterministic and stable across runs for the same input.
func groupConnectedComponents(faces []model.Face, vertexCount int) []componentGroup {
	uf := newUnionFind(vertexCount)
	for _, f := range faces {
		for i := 1; i < len(f.Indices); i++ {
			uf.union(int(f.Indices[0]), int(f.Indices[i]))
		}
	}

	groupIndexOf := make(map[int]int)
	var groups []componentGroup
	for i, f := range faces {
		if len(f.Indices) == 0 {
			continue
		}
		root := uf.find(int(f.Indices[0]))
		idx, ok := groupIndexOf[root]
		if !ok {
			idx = len(groups)
			groupIndexOf[root] = idx
			groups = append(groups, componentGroup{})
		}
		groups[idx].faceIndices = append(groups[idx].faceIndices, i)
	}
	return groups
}
