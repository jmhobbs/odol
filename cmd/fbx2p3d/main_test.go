package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/internal/mlodbuild"
)

// runMainEnv makes the test binary call main() with its arguments instead
// of running tests. main calls os.Exit, so tests that check its flag
// handling run it in a child process through runMain.
const runMainEnv = "FBX2P3D_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runMain runs main() in a child process with args and returns its
// combined output. The error is an *exec.ExitError on a non-zero exit.

func TestParseLODFlagRejectsEmptyPath(t *testing.T) {
	_, err := parseLODFlag("1.0=")
	assert.Error(t, err)
}

func TestOutputPathExplicitOverride(t *testing.T) {
	lods := []lodEntry{{Resolution: 1.0, Path: "a.fbx"}}
	assert.Equal(t, "out.p3d", outputPath(lods, "out.p3d"))
}

func TestOutputPathDerivedFromLowestResolution(t *testing.T) {
	lods := []lodEntry{
		{Resolution: 2.0, Path: filepath.Join("dir", "low.fbx")},
		{Resolution: 1.0, Path: filepath.Join("dir", "high.fbx")},
	}
	assert.Equal(t, filepath.Join("dir", "high.p3d"), outputPath(lods, ""))
}

func TestLODFlagListSetAccumulates(t *testing.T) {
	var l lodFlagList
	require.NoError(t, l.Set("1.0=a.fbx"))
	require.NoError(t, l.Set("2.0=b.fbx"))
	require.Len(t, l, 2)
	assert.Equal(t, float32(1.0), l[0].Resolution)
	assert.Equal(t, float32(2.0), l[1].Resolution)
}

// TestWrapListFitsWidthAndKeepsEveryItem checks the -help fire material
// list: every name appears once, in order, and no line runs past the width.
func TestWrapListFitsWidthAndKeepsEveryItem(t *testing.T) {
	names := mlodbuild.PenetrationMaterialNames()
	got := wrapList(names, "  ", 80)

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	assert.Greater(t, len(lines), 1, "expected the list to wrap")

	var listed []string
	for _, line := range lines {
		assert.LessOrEqual(t, len(line), 80)
		assert.True(t, strings.HasPrefix(line, "  "), "line %q must be indented", line)
		for _, field := range strings.Fields(line) {
			listed = append(listed, strings.TrimSuffix(field, ","))
		}
	}
	assert.Equal(t, names, listed)
}

func TestWrapListGivesOverlongItemItsOwnLine(t *testing.T) {
	assert.Equal(t, "  ab,\n  abcdefghij\n", wrapList([]string{"ab", "abcdefghij"}, "  ", 6))
}
