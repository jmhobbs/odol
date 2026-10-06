package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunUnsupportedDemoFormat(t *testing.T) {
	tempDir := t.TempDir()
	demoPath := filepath.Join(tempDir, "demo.p3d")
	require.NoError(t, os.WriteFile(demoPath, []byte("SP3D"), 0o644))

	var buf bytes.Buffer
	err := run(demoPath, &buf, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestRunNonexistentFile(t *testing.T) {
	var buf bytes.Buffer
	err := run(filepath.Join(t.TempDir(), "missing.p3d"), &buf, false)
	require.Error(t, err)
}
