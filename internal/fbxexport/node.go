package fbxexport

// node is a generic FBX document node: a name, a tuple of typed properties,
// and an optional nested list of child nodes. It mirrors the binary FBX node
// record described in docs/fbx_binary_specification.md and is the shared
// representation walked by both the ASCII and binary renderers.
//
// Supported concrete prop types: string, int32, int64, float64, bool,
// shadingFlag, []int32, []float64.
type node struct {
	name     string
	props    []any
	children []*node
}

// isList reports whether n carries a nested list, even an empty one.
// children == nil means "leaf, no list at all"; children != nil (len 0 or
// more) means "has a list" - in ASCII this renders braces, in binary it
// gets a terminating NULL-record.
func (n *node) isList() bool {
	return n.children != nil
}

// shadingFlag models the FBX "Shading: T"/"Shading: F" bareword convention,
// the one property in this exporter that renders unquoted in ASCII instead
// of through the usual string/int/double formatting.
type shadingFlag bool
