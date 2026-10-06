package fbxbinary

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRejectsNonFBXData(t *testing.T) {
	_, err := Decode([]byte("not an fbx file"))
	assert.Error(t, err)
}

func TestDecodeRejectsTruncatedHeader(t *testing.T) {
	_, err := Decode([]byte("Kaydara FBX Binary  \x00"))
	assert.Error(t, err)
}

func TestDecodeSampleFiles(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "samples", "fbx", "*.fbx"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "expected sample fbx fixtures")

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			doc, err := Decode(data)
			require.NoError(t, err)

			assert.NotZero(t, doc.Version)
			assert.NotEmpty(t, doc.Nodes)

			objects := doc.Top("Objects")
			require.NotNil(t, objects, "expected top-level Objects node")
			assert.NotEmpty(t, objects.ChildrenNamed("Geometry"), "expected at least one Geometry node")

			connections := doc.Top("Connections")
			require.NotNil(t, connections, "expected top-level Connections node")
			assert.NotEmpty(t, connections.Children)
		})
	}
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
