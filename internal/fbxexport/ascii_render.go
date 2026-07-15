package fbxexport

import (
	"fmt"
	"strconv"
	"strings"
)

// renderNode writes n and its descendants as ASCII FBX text.
//
// A node whose sole property is a float64/int32 slice is a real FBX "array"
// property and gets the special *N { a: ... } layout regardless of whether
// it has children (it never does). Every other node renders as
// "Name: prop, prop, ... {" with a nested block when it isList(), or as a
// single "Name: prop, prop, ..." line when it's a leaf.
func renderNode(fw *fbxWriter, n *node) {
	if len(n.props) == 1 {
		switch v := n.props[0].(type) {
		case []float64:
			fw.floatArray(n.name, v)
			return
		case []int32:
			fw.int32Array(n.name, v)
			return
		}
	}

	line := n.name + ":"
	if len(n.props) > 0 {
		line += " " + joinProps(n.name, n.props)
	}

	if !n.isList() {
		fw.rawLine(line)
		return
	}

	fw.openBlock(line)
	for _, c := range n.children {
		renderNode(fw, c)
	}
	fw.closeBlock()
}

// joinProps formats a node's property tuple for ASCII output. Every node
// joins properties with ", " except "C" (Connections entries), which uses a
// bare "," - matching the pre-existing, already-tested Connections format.
func joinProps(nodeName string, props []any) string {
	sep := ", "
	if nodeName == "C" {
		sep = ","
	}
	parts := make([]string, len(props))
	for i, v := range props {
		parts[i] = formatProp(v)
	}
	return strings.Join(parts, sep)
}

func formatProp(v any) string {
	switch t := v.(type) {
	case string:
		return `"` + escapeFBXString(t) + `"`
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', 6, 64)
	case shadingFlag:
		if t {
			return "T"
		}
		return "F"
	case []byte:
		return joinBytesRaw(t)
	default:
		panic(fmt.Sprintf("fbxexport: unsupported ASCII property type %T", v))
	}
}

// joinBytesRaw renders a raw ('R'-typed) byte property as ASCII FBX does:
// unquoted, comma-separated decimal byte values.
func joinBytesRaw(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = strconv.Itoa(int(v))
	}
	return strings.Join(parts, ",")
}
