package modelcfg_test

import (
	"strings"
	"testing"

	"github.com/jmhobbs/odol/internal/model"
	"github.com/jmhobbs/odol/internal/modelcfg"
)

func TestRenderConfig(t *testing.T) {
	t.Parallel()

	discrete := true
	hideValue := float32(0.6)

	cfg := model.Config{
		Skeletons: []model.SkeletonClass{
			{
				Name:            "barrel_skeleton",
				SkeletonInherit: "",
				Bones: []model.SkeletonBone{
					{Name: "lid"},
					{Name: "lid2", Parent: "lid"},
				},
				IsDiscrete: &discrete,
			},
		},
		Models: []model.ModelClass{
			{
				Name:         "barrel",
				Parent:       "Default",
				SkeletonName: "barrel_skeleton",
				Sections:     []string{"lid", "camo"},
				Animations: []model.Animation{
					{
						ClassName:     "Lid",
						Type:          "hide",
						Source:        "lid",
						Selection:     "lid",
						SourceAddress: "clamp",
						MinValue:      0,
						MaxValue:      1,
						HideValue:     &hideValue,
					},
				},
			},
		},
	}

	got, err := modelcfg.Render(cfg)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	for _, want := range []string{
		"class CfgSkeletons",
		`class barrel_skeleton : Default`,
		`"lid", ""`,
		`"lid2", "lid"`,
		"class CfgModels",
		`class barrel : Default`,
		`skeletonName = "barrel_skeleton";`,
		`sections[] =`,
		`class Lid`,
		`type = "hide";`,
		`hideValue = 0.6;`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, got)
		}
	}
}
