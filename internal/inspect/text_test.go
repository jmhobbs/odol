package inspect

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/detector"
)

func TestWriteTextContainsKeyInfo(t *testing.T) {
	s := Summarize(fixtureModel(), detector.P3DFormat{Family: detector.P3DFamilyODOL, Version: 7})

	var buf strings.Builder
	require.NoError(t, WriteText(&buf, s))
	out := buf.String()

	for _, want := range []string{
		"Model: test_model",
		"ODOLv7",
		"LODs (2):",
		"Graphical LOD",
		"Geometry",
		"(partially decoded)",
		"wheel_1",
		"geometry",
		"proxy_door.p3d",
		"a.paa",
		"b.paa",
		"a.rvmat",
		"b.rvmat",
		"test_skeleton",
		"Skeletons:",
	} {
		assert.Contains(t, out, want)
	}

	// Only LOD 0 (graphical) has a measured bounding size; LOD 1 (Geometry)
	// must not print one.
	assert.Equal(t, 1, strings.Count(out, "bounding size"))
}
