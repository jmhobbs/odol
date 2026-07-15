package main

import (
	"testing"
)

func TestOutputPaths(t *testing.T) {
	t.Parallel()

	gotP3D, gotCfg := outputPaths("/tmp/example.p3d")
	if gotP3D != "/tmp/example_mlod.p3d" {
		t.Fatalf("output p3d = %q, want %q", gotP3D, "/tmp/example_mlod.p3d")
	}
	if gotCfg != "/tmp/example.model.cfg" {
		t.Fatalf("output cfg = %q, want %q", gotCfg, "/tmp/example.model.cfg")
	}
}

func TestFBXOutputPath(t *testing.T) {
	t.Parallel()

	got := fbxOutputPath("/tmp/example.p3d")
	if got != "/tmp/example.fbx" {
		t.Fatalf("fbxOutputPath = %q, want %q", got, "/tmp/example.fbx")
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
