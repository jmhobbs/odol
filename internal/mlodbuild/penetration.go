package mlodbuild

import (
	"fmt"
	"slices"
	"strings"
)

// penetrationMaterialDir is the vanilla DayZ directory holding the
// penetration .rvmat files that Fire Geometry faces reference. Lowercase to
// match vanilla MLODs (e.g. samples/mlod/55galdrum_mlod.p3d); RV engine
// paths are case-insensitive either way.
const penetrationMaterialDir = "dz\\data\\data\\penetration\\"

// penetrationMaterialNames is every vanilla .rvmat in penetrationMaterialDir,
// without directory or extension, sorted. Taken from the vanilla-rvmats
// listing at the repo root, minus its one non-material entry
// (impact_test_object.p3d).
var penetrationMaterialNames = []string{
	"armor_11mm_plate",
	"armor_16mm_plate",
	"armor_1mm_plate",
	"armor_23mm_plate",
	"armor_30mm_plate",
	"armor_3mm_plate",
	"armor_5mm_plate",
	"armor_7mm_plate",
	"armour",
	"bricks",
	"castiron",
	"castironplate",
	"cloth",
	"concrete",
	"coniferfoliage",
	"cushion",
	"dirt",
	"explosiontransparent",
	"fabric_thin",
	"fabricp",
	"foliage",
	"foliage_green",
	"glass",
	"glass_armored",
	"glass_house",
	"glass_impenetrable",
	"glassplate",
	"granite",
	"hay",
	"ice",
	"iron",
	"ironplate",
	"leather",
	"mattress",
	"meat",
	"meatbones",
	"metalplate",
	"metalplatefence",
	"plastic",
	"plastic_material",
	"plexiplate",
	"pottery",
	"rubber",
	"rubberplate",
	"snow",
	"thin_wire",
	"weaponlift",
	"wood",
	"wood_desk",
}

// PenetrationMaterialNames returns a copy of every short name
// PenetrationMaterialPath accepts, sorted.
func PenetrationMaterialNames() []string {
	return slices.Clone(penetrationMaterialNames)
}

// PenetrationMaterialPath resolves name to the full vanilla penetration
// .rvmat path for a Fire Geometry LOD. name is matched case-insensitively
// and may be the short name ("wood") or the full path
// ("DZ\data\data\penetration\wood.rvmat"). Anything outside the vanilla list
// is an error naming the valid choices.
func PenetrationMaterialPath(name string) (string, error) {
	shortName := strings.ToLower(name)
	shortName = strings.TrimPrefix(shortName, penetrationMaterialDir)
	shortName = strings.TrimSuffix(shortName, ".rvmat")
	if !slices.Contains(penetrationMaterialNames, shortName) {
		return "", fmt.Errorf("unknown penetration material %q, expected one of: %s", name, strings.Join(penetrationMaterialNames, ", "))
	}
	return penetrationMaterialDir + shortName + ".rvmat", nil
}
