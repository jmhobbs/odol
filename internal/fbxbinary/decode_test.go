package fbxbinary

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeRejectsNonFBXData(t *testing.T) {
	_, err := Decode([]byte("not an fbx file"))
	assert.Error(t, err)
}

func TestDecodeRejectsTruncatedHeader(t *testing.T) {
	_, err := Decode([]byte("Kaydara FBX Binary  \x00"))
	assert.Error(t, err)
}

func TestNodeChildAndChildrenNamed(t *testing.T) {
	n := &Node{
		Name: "Objects",
		Children: []*Node{
			{Name: "Geometry", Props: []any{int64(1), "Geometry::a", "Mesh"}},
			{Name: "Geometry", Props: []any{int64(2), "Geometry::b", "Mesh"}},
			{Name: "Model", Props: []any{int64(3), "Model::a", "Mesh"}},
		},
	}

	assert.Len(t, n.ChildrenNamed("Geometry"), 2)
	assert.NotNil(t, n.Child("Model"))
	assert.Nil(t, n.Child("Missing"))
}
