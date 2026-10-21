package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmhobbs/odol/p3d/model"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestFindP3DsMatchesExtensionCaseInsensitivelyAndSorts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "b.P3D"), []byte("x"))
	writeFile(t, filepath.Join(dir, "sub", "a.p3d"), []byte("x"))
	writeFile(t, filepath.Join(dir, "notes.txt"), []byte("x"))

	paths, err := findP3Ds(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{
		filepath.Join(dir, "b.P3D"),
		filepath.Join(dir, "sub", "a.p3d"),
	}, paths)
}

func TestParseOneUnknownSignatureIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.p3d")
	writeFile(t, path, []byte("JUNKJUNKJUNKJUNK"))

	res := parseOne(path)
	assert.NotEmpty(t, res.Error)
	assert.False(t, res.Panic)
	assert.Empty(t, res.Format)
}

func TestParseOneTruncatedMLODIsAnErrorNotAPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short.p3d")
	writeFile(t, path, []byte("MLOD\x01\x01\x00\x00\x05\x00\x00\x00P3DM"))

	res := parseOne(path)
	assert.NotEmpty(t, res.Error)
	assert.False(t, res.Panic)
	assert.Equal(t, "MLOD/P3DM", res.Format)
}

// The earlier run saw different geometry for the soda can and parsed the
// junk file, so both should be reported.

func TestFingerprintTracksLODShape(t *testing.T) {
	a := &model.Model{LODs: []model.LOD{{Resolution: 1, Vertices: make([]model.Vector3, 3)}}}
	b := &model.Model{LODs: []model.LOD{{Resolution: 1, Vertices: make([]model.Vector3, 4)}}}
	assert.Equal(t, fingerprint(a), fingerprint(a))
	assert.NotEqual(t, fingerprint(a), fingerprint(b))
}

func TestPercentileUsesNearestRank(t *testing.T) {
	sorted := []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for i := range sorted {
		sorted[i] *= time.Millisecond
	}
	assert.Equal(t, 5*time.Millisecond, percentile(sorted, 50))
	assert.Equal(t, 10*time.Millisecond, percentile(sorted, 95))
	assert.Equal(t, time.Millisecond, percentile(sorted[:1], 50))
}
