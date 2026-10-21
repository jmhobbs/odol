package main

import (
	"path/filepath"
	"testing"
)

func TestOutputPaths(t *testing.T) {
	t.Parallel()

	// outputPaths builds paths with filepath, so the separators are the OS's.
	gotP3D, gotCfg := outputPaths("/tmp/example.p3d")
	if want := filepath.FromSlash("/tmp/example_mlod.p3d"); gotP3D != want {
		t.Fatalf("output p3d = %q, want %q", gotP3D, want)
	}
	if want := filepath.FromSlash("/tmp/example.model.cfg"); gotCfg != want {
		t.Fatalf("output cfg = %q, want %q", gotCfg, want)
	}
}

func TestFBXOutputPath(t *testing.T) {
	t.Parallel()

	got := fbxOutputPath("/tmp/example.p3d")
	if want := filepath.FromSlash("/tmp/example.fbx"); got != want {
		t.Fatalf("fbxOutputPath = %q, want %q", got, want)
	}
}

func TestResolveFBXMode(t *testing.T) {
	tests := []struct {
		name             string
		fbx, fbxASCII    bool
		wantFBX, wantASC bool
	}{
		{"neither flag means MLOD", false, false, false, false},
		{"--fbx alone means binary FBX", true, false, true, false},
		{"--fbx-ascii alone implies --fbx and selects ASCII", false, true, true, true},
		{"both flags means ASCII FBX", true, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFBX, gotASCII := resolveFBXMode(tt.fbx, tt.fbxASCII)
			if gotFBX != tt.wantFBX || gotASCII != tt.wantASC {
				t.Fatalf("resolveFBXMode(%v, %v) = (%v, %v), want (%v, %v)",
					tt.fbx, tt.fbxASCII, gotFBX, gotASCII, tt.wantFBX, tt.wantASC)
			}
		})
	}
}
