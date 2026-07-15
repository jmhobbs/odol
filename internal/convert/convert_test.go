package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/model"
)

func TestConvertToFBXUnsupportedFormat(t *testing.T) {
	// "SP3D" is a recognized DEMO-family signature, which is not
	// convertible to FBX (unlike ODOL and MLOD).
	_, err := ConvertToFBX([]byte("SP3D"), "bogus", true, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported input format for FBX export")
}

func TestEnsureFullyDecodedReturnsPartialParseError(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{
			{Resolution: 1.0},
			{
				Resolution: 0.5,
				Properties: map[string]string{"odol_partial_parse": "1"},
			},
		},
	}

	err := ensureFullyDecoded(m)

	require.Error(t, err)
	var partialErr PartialParseError
	require.ErrorAs(t, err, &partialErr)
	assert.Equal(t, 1, partialErr.LODIndex)
	assert.Equal(t, float32(0.5), partialErr.Resolution)
	assert.Equal(t, "odol_partial_parse", partialErr.PropertyKey)
	assert.Contains(t, err.Error(), "partially decoded LOD 1")
}

func TestEnsureFullyDecodedNoMarkerReturnsNil(t *testing.T) {
	m := &model.Model{
		LODs: []model.LOD{{Resolution: 1.0}},
	}

	assert.NoError(t, ensureFullyDecoded(m))
}
