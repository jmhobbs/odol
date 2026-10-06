package inspect

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func fromBits(bits uint32) float32 {
	return math.Float32frombits(bits)
}

func TestResolutionName(t *testing.T) {
	tests := []struct {
		name       string
		resolution float32
		want       string
	}{
		{"graphical LOD, resolution 1", 1.0, "Graphical LOD"},
		{"graphical LOD, near boundary", 999.0, "Graphical LOD"},
		{"view gunner", fromBits(0x447a0000), "View Gunner"},
		{"view pilot", fromBits(0x44898000), "View Pilot"},
		{"view cargo", fromBits(0x44960000), "View Cargo"},
		{"shadow volume", fromBits(0x461c4000), "Shadow Volume"},
		{"shadow volume 2", fromBits(0x461c6800), "Shadow Volume 2"},
		{"stencil shadow", fromBits(0x462be000), "Stencil Shadow"},
		{"stencil shadow 2", fromBits(0x462c0800), "Stencil Shadow 2"},
		{"geometry", fromBits(0x551184e7), "Geometry"},
		{"memory", fromBits(0x58635fa9), "Memory"},
		{"land contact", fromBits(0x58e35fa9), "Land Contact"},
		{"roadway", fromBits(0x592a87bf), "Roadway"},
		{"paths", fromBits(0x59635fa9), "Paths"},
		{"hitpoints", fromBits(0x598e1bca), "HitPoints"},
		{"view geometry", fromBits(0x59aa87bf), "View Geometry"},
		{"fire geometry", fromBits(0x59c6f3b4), "Fire Geometry"},
		{"view cargo geometry", fromBits(0x59e35fa9), "View Cargo Geometry"},
		{"view cargo fire geometry", fromBits(0x59ffcb9e), "View Cargo Fire Geometry"},
		{"view commander", fromBits(0x5a0e1bca), "View Commander"},
		{"view commander geometry", fromBits(0x5a1c51c4), "View Commander Geometry"},
		{"view commander fire geometry", fromBits(0x5a2a87bf), "View Commander Fire Geometry"},
		{"view pilot geometry", fromBits(0x5a38bdb9), "View Pilot Geometry"},
		{"view pilot fire geometry", fromBits(0x5a46f3b4), "View Pilot Fire Geometry"},
		{"view gunner geometry", fromBits(0x5a5529af), "View Gunner Geometry"},
		{"view gunner fire geometry", fromBits(0x5a635fa9), "View Gunner Fire Geometry"},
		{"sub parts", fromBits(0x5a7195a4), "Sub Parts"},
		{"shadow volume view cargo", fromBits(0x5a7fcb9e), "Shadow Volume View Cargo"},
		{"shadow volume view pilot", fromBits(0x5a8700cc), "Shadow Volume View Pilot"},
		{"shadow volume view gunner", fromBits(0x5a8e1bca), "Shadow Volume View Gunner"},
		{"wreck", fromBits(0x5a9536c7), "Wreck"},
		{"unknown functional LOD", 1500.0, "Unknown functional LOD"},
		{"unknown shadow LOD", 123456.0, "Unknown shadow LOD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolutionName(tt.resolution))
		})
	}
}
