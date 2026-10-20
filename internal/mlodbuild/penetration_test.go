package mlodbuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPenetrationMaterialPathResolvesShortName(t *testing.T) {
	for _, name := range []string{"wood", "WOOD"} {
		got, err := PenetrationMaterialPath(name)
		require.NoError(t, err)
		assert.Equal(t, "dz\\data\\data\\penetration\\wood.rvmat", got)
	}
}

func TestPenetrationMaterialPathResolvesFullPath(t *testing.T) {
	got, err := PenetrationMaterialPath("DZ\\data\\data\\penetration\\metalplate.rvmat")
	require.NoError(t, err)
	assert.Equal(t, "dz\\data\\data\\penetration\\metalplate.rvmat", got)
}

// TestPenetrationMaterialNamesReturnsCopy guards the allow-list: a caller
// editing the returned slice must not change what PenetrationMaterialPath
// accepts.
func TestPenetrationMaterialNamesReturnsCopy(t *testing.T) {
	names := PenetrationMaterialNames()
	require.NotEmpty(t, names)
	names[0] = "lava"

	_, err := PenetrationMaterialPath("lava")
	assert.Error(t, err)
}

// TestPenetrationMaterialPathRejectsUnknown covers a name that was never in
// the vanilla list, the one non-rvmat entry in it (impact_test_object.p3d),
// and empty input. The error must name the valid choices so the user can
// correct the flag without looking anything up.
func TestPenetrationMaterialPathRejectsUnknown(t *testing.T) {
	for _, name := range []string{"lava", "impact_test_object", ""} {
		_, err := PenetrationMaterialPath(name)
		require.Errorf(t, err, "%q must be rejected", name)
		assert.Contains(t, err.Error(), "metalplate")
		assert.Contains(t, err.Error(), "wood_desk")
	}
}
