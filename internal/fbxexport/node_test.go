package fbxexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeLeafHasNilChildrenAndIsNotAList(t *testing.T) {
	n := &node{name: "Version", props: []any{int32(100)}}
	assert.Nil(t, n.children)
	assert.False(t, n.isList())
}

func TestNodeExplicitEmptyChildrenIsAList(t *testing.T) {
	n := &node{name: "References", children: []*node{}}
	assert.NotNil(t, n.children)
	assert.Len(t, n.children, 0)
	assert.True(t, n.isList())
}

func TestNodeWithChildrenIsAList(t *testing.T) {
	child := &node{name: "Count", props: []any{int32(1)}}
	n := &node{name: "ObjectType", props: []any{"GlobalSettings"}, children: []*node{child}}
	assert.True(t, n.isList())
	assert.Len(t, n.children, 1)
	assert.Equal(t, "Count", n.children[0].name)
}

func TestNodePropsPreserveOrderAndType(t *testing.T) {
	n := &node{
		name: "P",
		props: []any{
			"UpAxis", "int", "Integer", "", int32(1),
		},
	}
	assert.Equal(t, "UpAxis", n.props[0])
	assert.Equal(t, int32(1), n.props[4])
}

func TestShadingFlagIsDistinctBoolType(t *testing.T) {
	var f shadingFlag = true
	assert.True(t, bool(f))
	f = false
	assert.False(t, bool(f))
}
