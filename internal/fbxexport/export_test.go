package fbxexport

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jmhobbs/odol/internal/model"
)

// The LOD is split into one part per disconnected piece of geometry
// (the drum body, hoops, bung plugs, etc.) - see components.go.

// 55galdrum.p3d has 9 LODs; find the expected lowest resolution

// Only the lowest-resolution LOD is exported, split into its 14
// disconnected pieces (see TestMarshalASCIIODOLv6Sample) - a different
// LOD selection would produce a different vertex/face count and, in
// turn, a different component count.

// m4a1.p3d's view LOD carries "proxy:"-named selections marking
// placeholder geometry for its 11 attachment proxies; those selections
// must not leak into the export as their own Null nodes, even though
// the Proxy_N nodes themselves still legitimately reference the name
// via ODOL_ProxySelectionName.

func TestMarshalASCIIEmptyModelReturnsError(t *testing.T) {
	_, err := MarshalASCII(&model.Model{})
	assert.Error(t, err)
}

func TestMarshalBinaryEmptyModelReturnsError(t *testing.T) {
	_, err := Marshal(&model.Model{})
	assert.Error(t, err)
}

// Only the lowest-resolution LOD is exported, split into its 14
// disconnected pieces (see TestMarshalASCIIODOLv6Sample).

// decodeTopLevel decodes a full binary FBX document's top-level node list
// and returns it along with the trailing footer bytes, using the test-only
// decoder defined in binary_test.go.
