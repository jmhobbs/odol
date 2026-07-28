package odol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmhobbs/odol/internal/model"
)

var errShortData = errors.New("unexpected end of odol data")

type parser struct {
	data               []byte
	off                int
	version            uint32
	useLZO             bool
	useCompressionFlag bool
}

type rawModelInfo struct {
	centerOfMass model.Vector3
	skeleton     rawSkeleton
	pointMasses  []float32
	totalMass    float32
}

type rawSkeleton struct {
	name       string
	isDiscrete *bool
	bones      []model.SkeletonBone
	pivotsName string
}

type rawAnimations struct {
	classes      []rawAnimationClass
	animsToBones []rawAnimsToBones
}

type rawAnimationClass struct {
	className     string
	source        string
	transformType uint32
	sourceAddress uint32
	minValue      float32
	maxValue      float32
	angle0        *float32
	angle1        *float32
	offset0       *float32
	offset1       *float32
	hideValue     *float32
	axisPos       *model.Vector3
	axisDir       *model.Vector3
}

type rawAnimsToBones struct {
	bones []rawAnimBone
}

type rawAnimBone struct {
	boneIndex int32
	axisPos   *model.Vector3
	axisDir   *model.Vector3
}

type rawLod struct {
	resolution      float32
	proxies         []rawProxy
	textures        []string
	materials       []rawMaterial
	faces           []rawFace
	sections        []rawSection
	namedSelections []rawSelection
	properties      map[string]string
	propertyOrder   []string
	iconColor       uint32
	selectedColor   uint32
	vertices        []model.Vector3
	normals         []model.Vector3
	uvSets          [][]model.UV
	st              []rawSTPair
}

type rawProxy struct {
	modelPath           string
	sequenceID          int32
	namedSelectionIndex int32
	sectionIndex        int32
}

type rawMaterial struct {
	name string
}

type rawStageTexture struct {
	filter         uint32
	texture        string
	transformIndex uint32
	extra          uint8
}

type rawFace struct {
	faceType uint8
	indices  []uint32
}

type rawSection struct {
	faceLower      uint32
	faceUpper      uint32
	commonTexture  int16
	commonFaceFlag uint32
	materialIndex  int32
	materialName   string
}

type rawSelection struct {
	name          string
	isSectional   bool
	faceIndices   []uint32
	vertexIndices []uint32
	vertexWeights []byte
}

type rawSTPair struct {
	s model.Vector3
	t model.Vector3
}

func ParseFile(path string) (*model.Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return Parse(data, name)
}

func ParseFileStrict(path string) (*model.Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return ParseStrict(data, name)
}

func Parse(data []byte, name string) (*model.Model, error) {
	return parse(data, name, true)
}

func ParseStrict(data []byte, name string) (*model.Model, error) {
	return parse(data, name, false)
}

func parse(data []byte, name string, allowPartialLOD bool) (*model.Model, error) {
	p := &parser{data: data}

	if got, err := p.readBytes(4); err != nil || string(got) != "ODOL" {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unsupported input magic %q", string(got))
	}

	version, err := p.readU32()
	if err != nil {
		return nil, err
	}
	p.version = version
	p.useLZO = version >= 44
	p.useCompressionFlag = version >= 64

	lodCount, err := p.readU32()
	if err != nil {
		return nil, err
	}

	resolutions := make([]float32, 0, lodCount)
	for i := uint32(0); i < lodCount; i++ {
		value, err := p.readF32()
		if err != nil {
			return nil, err
		}
		resolutions = append(resolutions, value)
	}

	tableOffset, starts, ends, err := findLODTable(data, p.off, int(lodCount))
	if err != nil {
		return nil, fmt.Errorf("locate lod table: %w", err)
	}

	lods := make([]rawLod, 0, lodCount)
	for i := 0; i < int(lodCount); i++ {
		if starts[i] >= ends[i] || int(ends[i]) > len(data) {
			return nil, fmt.Errorf("invalid lod bounds %d: %d..%d", i, starts[i], ends[i])
		}

		raw, err := parseLod(data[starts[i]:ends[i]], version, p.useLZO, p.useCompressionFlag)
		if err != nil {
			if !allowPartialLOD {
				return nil, fmt.Errorf("parse lod %d (resolution %g): %w", i, resolutions[i], err)
			}
			raw = scanFallbackLOD(data[starts[i]:ends[i]])
		}
		raw.resolution = resolutions[i]
		lods = append(lods, raw)
	}

	info, animations, err := parseMetadata(data[p.off:tableOffset], version, int(lodCount), lods)
	if err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	return buildModel(name, normalizeODOLVersion(version), info, animations, lods), nil
}

func findLODTable(data []byte, startSearch, lodCount int) (int, []uint32, []uint32, error) {
	for off := startSearch; off+lodCount*8 <= len(data); off++ {
		starts := make([]uint32, 0, lodCount)
		ends := make([]uint32, 0, lodCount)
		ok := true
		for i := 0; i < lodCount; i++ {
			if off+4*(i+1) > len(data) {
				ok = false
				break
			}
			start := binary.LittleEndian.Uint32(data[off+4*i:])
			starts = append(starts, start)
		}
		for i := 0; i < lodCount && ok; i++ {
			pos := off + 4*lodCount + 4*i
			if pos+4 > len(data) {
				ok = false
				break
			}
			end := binary.LittleEndian.Uint32(data[pos:])
			ends = append(ends, end)
		}
		if !ok || starts[0] == 0 || ends[0] != uint32(len(data)) {
			continue
		}
		if !sort.SliceIsSorted(starts, func(i, j int) bool { return starts[i] > starts[j] }) {
			continue
		}
		if !sort.SliceIsSorted(ends, func(i, j int) bool { return ends[i] > ends[j] }) {
			continue
		}
		valid := true
		for i := range starts {
			if starts[i] >= ends[i] || int(ends[i]) > len(data) {
				valid = false
				break
			}
		}
		if valid {
			return off, starts, ends, nil
		}
	}
	return 0, nil, nil, fmt.Errorf("no lod table found")
}

func parseMetadata(prefix []byte, version uint32, lodCount int, lods []rawLod) (rawModelInfo, *rawAnimations, error) {
	info, end, err := parseSkeletonMetadata(prefix)
	if err != nil {
		info = rawModelInfo{}
		end = 0
	}
	if count := geometryVertexCount(lods); count > 0 {
		info.pointMasses = scanPointMasses(prefix, version, count)
		info.totalMass = scanGeometryMass(prefix, end)
	}

	var anims *rawAnimations
	if end > 0 {
		anims, _ = scanAnimations(prefix[end:], version, lodCount)
	}
	return info, anims, nil
}

func scanFallbackLOD(data []byte) rawLod {
	lod := rawLod{
		properties: map[string]string{
			"odol_partial_parse": "1",
		},
	}

	seenProxy := map[string]struct{}{}
	sequence := int32(1)
	for _, value := range scanStrings(data) {
		if strings.Contains(value, `\proxy\`) && !strings.HasPrefix(strings.ToLower(value), "proxy:") && !looksLikeAsset(value) {
			if _, ok := seenProxy[value]; ok {
				continue
			}
			seenProxy[value] = struct{}{}
			lod.proxies = append(lod.proxies, rawProxy{
				modelPath:  value,
				sequenceID: sequence,
			})
			sequence++
		}
	}

	return lod
}

func scanStrings(data []byte) []string {
	var out []string
	for i := 0; i < len(data); i++ {
		start := i
		for i < len(data) && data[i] != 0 && data[i] >= 32 && data[i] <= 126 {
			i++
		}
		if i < len(data) && data[i] == 0 && i-start >= 4 {
			out = append(out, string(data[start:i]))
		}
	}
	return out
}

func looksLikeAsset(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasSuffix(lower, ".paa") || strings.HasSuffix(lower, ".rvmat") || strings.HasSuffix(lower, ".bisurf")
}

func parseSkeletonMetadata(prefix []byte) (rawModelInfo, int, error) {
	for off := 0; off < len(prefix); off++ {
		name, end, ok := readCandidateString(prefix, off)
		if !ok || strings.ContainsAny(name, `\:.`) {
			continue
		}
		scratch := &parser{data: prefix, off: end}
		discreteByte, err := scratch.readU8()
		if err != nil {
			continue
		}
		if discreteByte > 1 {
			continue
		}
		boneCount, err := scratch.readU32()
		if err != nil || boneCount == 0 || boneCount > 512 {
			continue
		}

		bones := make([]model.SkeletonBone, 0, boneCount)
		valid := true
		for i := uint32(0); i < boneCount; i++ {
			boneName, err := scratch.readString()
			if err != nil || !looksLikeIdentifier(boneName) {
				valid = false
				break
			}
			parent, err := scratch.readString()
			if err != nil {
				valid = false
				break
			}
			bones = append(bones, model.SkeletonBone{Name: boneName, Parent: parent})
		}
		if !valid {
			continue
		}

		pivots, err := scratch.readString()
		if err != nil {
			continue
		}
		discrete := discreteByte != 0
		return rawModelInfo{
			centerOfMass: model.Vector3{},
			skeleton: rawSkeleton{
				name:       name,
				isDiscrete: &discrete,
				bones:      bones,
				pivotsName: pivots,
			},
		}, scratch.off, nil
	}

	return rawModelInfo{}, 0, fmt.Errorf("skeleton metadata not found")
}

func scanAnimations(data []byte, version uint32, lodCount int) (*rawAnimations, int) {
	for off := 0; off < len(data)-5; off++ {
		if data[off] == 0 {
			continue
		}
		scratch := &parser{data: data, off: off + 1, version: version}
		classCount, err := scratch.readU32()
		if err != nil || classCount == 0 || classCount > 256 {
			continue
		}
		anims := &rawAnimations{}
		valid := true
		for i := uint32(0); i < classCount; i++ {
			anim, err := scratch.readAnimationClass()
			if err != nil || anim.className == "" || anim.source == "" {
				valid = false
				break
			}
			anims.classes = append(anims.classes, anim)
		}
		if !valid {
			continue
		}
		resolutionCount, err := scratch.readI32()
		if err != nil || resolutionCount < 0 || resolutionCount > int32(lodCount) {
			continue
		}
		for i := int32(0); i < resolutionCount && valid; i++ {
			boneCount, err := scratch.readU32()
			if err != nil || boneCount > 1024 {
				valid = false
				break
			}
			for j := uint32(0); j < boneCount; j++ {
				animClassCount, err := scratch.readU32()
				if err != nil || animClassCount > classCount {
					valid = false
					break
				}
				for k := uint32(0); k < animClassCount; k++ {
					if _, err := scratch.readU32(); err != nil {
						valid = false
						break
					}
				}
			}
		}
		if !valid {
			continue
		}
		for i := int32(0); i < resolutionCount && valid; i++ {
			entry := rawAnimsToBones{}
			for _, anim := range anims.classes {
				boneIndex, err := scratch.readI32()
				if err != nil {
					valid = false
					break
				}
				animBone := rawAnimBone{boneIndex: boneIndex}
				if boneIndex != -1 && anim.transformType != 8 && anim.transformType != 9 {
					pos, err := scratch.readVec3()
					if err != nil {
						valid = false
						break
					}
					dir, err := scratch.readVec3()
					if err != nil {
						valid = false
						break
					}
					animBone.axisPos = &pos
					animBone.axisDir = &dir
				}
				entry.bones = append(entry.bones, animBone)
			}
			anims.animsToBones = append(anims.animsToBones, entry)
		}
		if valid {
			return anims, off + scratch.off
		}
	}
	return nil, 0
}

func scanPointMasses(prefix []byte, version uint32, expectedCount int) []float32 {
	bestScore := float32(-1)
	var best []float32

	for off := 0; off+4 <= len(prefix); off++ {
		if int(binary.LittleEndian.Uint32(prefix[off:])) != expectedCount {
			continue
		}

		scratch := &parser{
			data:               prefix,
			off:                off,
			version:            version,
			useLZO:             version >= 44,
			useCompressionFlag: version >= 64,
		}
		values, err := scratch.readCompressedFloat32sCount()
		if err != nil || len(values) != expectedCount {
			continue
		}

		score := float32(0)
		valid := true
		for _, value := range values {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value < 0 {
				valid = false
				break
			}
			score += value
		}
		if !valid || score <= 0 || score < bestScore {
			continue
		}

		bestScore = score
		best = append([]float32(nil), values...)
	}

	return best
}

func scanGeometryMass(prefix []byte, skeletonEnd int) float32 {
	limit := skeletonEnd + 64
	if limit > len(prefix) {
		limit = len(prefix)
	}

	bestOffset := len(prefix) + 1
	bestMass := float32(0)
	for off := skeletonEnd; off+16 <= limit; off++ {
		mass := math.Float32frombits(binary.LittleEndian.Uint32(prefix[off:]))
		massReciprocal := math.Float32frombits(binary.LittleEndian.Uint32(prefix[off+4:]))
		armorMass := math.Float32frombits(binary.LittleEndian.Uint32(prefix[off+8:]))
		armorReciprocal := math.Float32frombits(binary.LittleEndian.Uint32(prefix[off+12:]))
		if !looksLikeMassPair(mass, massReciprocal) || !looksLikeMassPair(armorMass, armorReciprocal) {
			continue
		}
		if off < bestOffset {
			bestOffset = off
			bestMass = mass
		}
	}

	return bestMass
}

func looksLikeMassPair(mass, reciprocal float32) bool {
	if mass <= 0 || math.IsNaN(float64(mass)) || math.IsInf(float64(mass), 0) {
		return false
	}
	if reciprocal <= 0 || math.IsNaN(float64(reciprocal)) || math.IsInf(float64(reciprocal), 0) {
		return false
	}

	expected := 1 / mass
	delta := math.Abs(float64(reciprocal - expected))
	return delta <= math.Max(1e-4, math.Abs(float64(expected))*0.05)
}

func readCandidateString(data []byte, offset int) (string, int, bool) {
	end := offset
	for end < len(data) && data[end] != 0 {
		if data[end] < 32 || data[end] > 126 {
			return "", 0, false
		}
		end++
	}
	if end >= len(data) || end == offset {
		return "", 0, false
	}
	value := string(data[offset:end])
	if !looksLikeIdentifier(value) {
		return "", 0, false
	}
	return value, end + 1, true
}

func looksLikeIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '\\' || r == ':' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func buildModel(name string, version uint32, info rawModelInfo, animations *rawAnimations, lods []rawLod) *model.Model {
	cfg := model.Config{}
	if info.skeleton.name != "" {
		cfg.Skeletons = append(cfg.Skeletons, model.SkeletonClass{
			Name:            info.skeleton.name,
			Parent:          "Default",
			SkeletonInherit: "",
			Bones:           info.skeleton.bones,
			IsDiscrete:      info.skeleton.isDiscrete,
			PivotsModel:     info.skeleton.pivotsName,
		})
	}

	sectionSet := map[string]struct{}{}
	for _, lod := range lods {
		for _, sel := range lod.namedSelections {
			if sel.isSectional {
				sectionSet[sel.name] = struct{}{}
			}
		}
	}
	sections := make([]string, 0, len(sectionSet))
	for section := range sectionSet {
		sections = append(sections, section)
	}
	sort.Strings(sections)

	modelClass := model.ModelClass{
		Name:         name,
		Parent:       "Default",
		Sections:     sections,
		SkeletonName: info.skeleton.name,
	}
	if animations != nil {
		modelClass.Animations = buildAnimations(info, *animations)
	}
	cfg.Models = append(cfg.Models, modelClass)

	result := &model.Model{
		Name:         name,
		Source:       model.SourceFormat{Family: "ODOL", Version: version},
		CenterOfMass: info.centerOfMass,
		Config:       cfg,
	}

	for _, raw := range lods {
		lod := model.LOD{
			Resolution:    raw.resolution,
			Vertices:      make([]model.Vector3, 0, len(raw.vertices)),
			Normals:       append([]model.Vector3(nil), raw.normals...),
			UVSets:        cloneUVSets(raw.uvSets),
			Properties:    raw.properties,
			PropertyOrder: append([]string(nil), raw.propertyOrder...),
			PointMasses:   geometryMasses(info.pointMasses, info.totalMass, raw.resolution, len(raw.vertices)),
			IconColor:     raw.iconColor,
			SelectedColor: raw.selectedColor,
		}

		for _, v := range raw.vertices {
			lod.Vertices = append(lod.Vertices, model.Vector3{
				X: v.X + info.centerOfMass.X,
				Y: v.Y + info.centerOfMass.Y,
				Z: v.Z + info.centerOfMass.Z,
			})
		}

		for faceIndex, face := range raw.faces {
			section := findSection(raw.sections, uint32(faceIndex))
			texture := ""
			if section != nil && section.commonTexture >= 0 && int(section.commonTexture) < len(raw.textures) {
				texture = raw.textures[section.commonTexture]
			}

			material := ""
			if section != nil {
				if section.materialIndex >= 0 && int(section.materialIndex) < len(raw.materials) {
					material = raw.materials[section.materialIndex].name
				} else if section.materialName != "" {
					material = section.materialName
				}
			}

			uvs := make([]model.UV, 0, len(face.indices))
			for _, index := range face.indices {
				uvs = append(uvs, raw.uvFor(index))
			}

			indices := append([]uint32(nil), face.indices...)
			normalIndices := append([]uint32(nil), face.indices...)
			flags := sectionFlags(section)
			if texture == "" {
				flags = 0
			}

			lod.Faces = append(lod.Faces, model.Face{
				Indices:       reorderIndices(indices),
				NormalIndices: reorderIndices(normalIndices),
				UVs:           reorderUVs(uvs),
				Flags:         flags,
				Texture:       texture,
				Material:      material,
			})
		}

		selections := make([]model.Selection, 0, len(raw.namedSelections))
		selectionNames := make([]string, 0, len(raw.namedSelections))
		selectionNameSet := make(map[string]struct{}, len(raw.namedSelections))
		selectionNameIndex := make(map[string]int, len(raw.namedSelections))
		for _, sel := range raw.namedSelections {
			selections = append(selections, model.Selection{
				Name:          sel.name,
				IsSectional:   sel.isSectional,
				VertexIndices: append([]uint32(nil), sel.vertexIndices...),
				VertexWeights: append([]byte(nil), sel.vertexWeights...),
				FaceIndices:   append([]uint32(nil), sel.faceIndices...),
			})
			selectionNames = append(selectionNames, sel.name)
			selectionNameSet[sel.name] = struct{}{}
			selectionNameIndex[sel.name] = len(selections) - 1
		}

		for _, proxy := range raw.proxies {
			name := fmt.Sprintf("proxy:%s.%03d", proxy.modelPath, proxy.sequenceID)
			if proxy.sequenceID <= 0 {
				name = fmt.Sprintf("proxy:%s", proxy.modelPath)
			}
			baseName := fmt.Sprintf("proxy:%s", proxy.modelPath)
			if proxy.sequenceID > 0 {
				if idx, exists := selectionNameIndex[baseName]; exists {
					if _, taken := selectionNameSet[name]; !taken {
						selections[idx].Name = name
						if idx < len(selectionNames) {
							selectionNames[idx] = name
						}
						delete(selectionNameSet, baseName)
						delete(selectionNameIndex, baseName)
						selectionNameSet[name] = struct{}{}
						selectionNameIndex[name] = idx
					}
				}
			}

			if _, exists := selectionNameSet[name]; !exists {
				selection := model.Selection{Name: name}
				if proxy.namedSelectionIndex >= 0 && int(proxy.namedSelectionIndex) < len(selections) {
					base := selections[proxy.namedSelectionIndex]
					selection.VertexIndices = append([]uint32(nil), base.VertexIndices...)
					selection.VertexWeights = append([]byte(nil), base.VertexWeights...)
					selection.FaceIndices = append([]uint32(nil), base.FaceIndices...)
				}
				selections = append(selections, selection)
				selectionNameSet[name] = struct{}{}
				selectionNameIndex[name] = len(selections) - 1
			}

			namedSelectionName := ""
			if proxy.namedSelectionIndex >= 0 && int(proxy.namedSelectionIndex) < len(selectionNames) {
				namedSelectionName = selectionNames[proxy.namedSelectionIndex]
			}
			lod.Proxies = append(lod.Proxies, model.Proxy{
				ModelPath:           proxy.modelPath,
				SequenceID:          proxy.sequenceID,
				NamedSelectionIndex: proxy.namedSelectionIndex,
				NamedSelectionName:  namedSelectionName,
				SectionIndex:        proxy.sectionIndex,
			})
		}
		lod.Selections = filterProxySelectionAliases(selections)
		result.LODs = append(result.LODs, lod)
	}

	return result
}

func geometryVertexCount(lods []rawLod) int {
	for _, lod := range lods {
		if isGeometryResolution(lod.resolution) {
			return len(lod.vertices)
		}
	}
	return 0
}

func cloneUVSets(values [][]model.UV) [][]model.UV {
	if len(values) == 0 {
		return nil
	}
	out := make([][]model.UV, 0, len(values))
	for _, value := range values {
		out = append(out, append([]model.UV(nil), value...))
	}
	return out
}

func filterProxySelectionAliases(selections []model.Selection) []model.Selection {
	if len(selections) == 0 {
		return nil
	}

	nameSet := make(map[string]struct{}, len(selections))
	for _, selection := range selections {
		nameSet[selection.Name] = struct{}{}
	}

	filtered := make([]model.Selection, 0, len(selections))
	for _, selection := range selections {
		if base, ok := proxyBaseName(selection.Name); ok {
			if hasProxySequenceVariant(nameSet, base) {
				continue
			}
		}
		filtered = append(filtered, selection)
	}
	return filtered
}

func proxyBaseName(name string) (string, bool) {
	if !strings.HasPrefix(name, "proxy:") {
		return "", false
	}
	if len(name) >= 4 && name[len(name)-4] == '.' {
		for _, b := range []byte(name[len(name)-3:]) {
			if b < '0' || b > '9' {
				return "", false
			}
		}
		return "", false
	}
	return name, true
}

func hasProxySequenceVariant(names map[string]struct{}, base string) bool {
	for i := 0; i < 1000; i++ {
		if _, exists := names[fmt.Sprintf("%s.%03d", base, i)]; exists {
			return true
		}
	}
	return false
}

func geometryMasses(masses []float32, totalMass float32, resolution float32, vertexCount int) []float32 {
	if !isGeometryResolution(resolution) {
		return nil
	}
	if len(masses) == vertexCount {
		return append([]float32(nil), masses...)
	}
	if vertexCount == 0 || totalMass <= 0 || math.IsNaN(float64(totalMass)) || math.IsInf(float64(totalMass), 0) {
		return nil
	}

	share := totalMass / float32(vertexCount)
	out := make([]float32, vertexCount)
	for i := range out {
		out[i] = share
	}
	return out
}

func isGeometryResolution(resolution float32) bool {
	return math.Float32bits(resolution) == math.Float32bits(1e13)
}

func buildAnimations(info rawModelInfo, animations rawAnimations) []model.Animation {
	result := make([]model.Animation, 0, len(animations.classes))
	for idx, class := range animations.classes {
		anim := model.Animation{
			ClassName:     class.className,
			Type:          animationTypeName(class.transformType),
			Source:        class.source,
			SourceAddress: animationAddressName(class.sourceAddress),
			MinValue:      class.minValue,
			MaxValue:      class.maxValue,
			Angle0:        class.angle0,
			Angle1:        class.angle1,
			Offset0:       class.offset0,
			Offset1:       class.offset1,
			HideValue:     class.hideValue,
		}

		for _, resolution := range animations.animsToBones {
			if idx >= len(resolution.bones) {
				continue
			}
			bone := resolution.bones[idx]
			if bone.boneIndex >= 0 && int(bone.boneIndex) < len(info.skeleton.bones) {
				anim.Selection = info.skeleton.bones[bone.boneIndex].Name
				break
			}
		}

		if class.axisPos != nil {
			anim.AxisPosition = class.axisPos
			anim.AxisDirection = class.axisDir
		} else {
			for _, resolution := range animations.animsToBones {
				if idx >= len(resolution.bones) {
					continue
				}
				bone := resolution.bones[idx]
				if bone.axisPos != nil {
					anim.AxisPosition = bone.axisPos
					anim.AxisDirection = bone.axisDir
					break
				}
			}
		}

		result = append(result, anim)
	}

	return result
}

func parseLod(data []byte, version uint32, useLZO, useCompressionFlag bool) (rawLod, error) {
	p := &parser{
		data:               data,
		version:            version,
		useLZO:             useLZO,
		useCompressionFlag: useCompressionFlag,
	}

	proxyCount, err := p.readI32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read proxy count: %w", err)
	}

	lod := rawLod{}
	for i := int32(0); i < proxyCount; i++ {
		proxy, err := p.readProxy()
		if err != nil {
			return rawLod{}, fmt.Errorf("read proxy %d: %w", i, err)
		}
		lod.proxies = append(lod.proxies, proxy)
	}

	lodItemCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read lod item count: %w", err)
	}
	if _, err := p.readCompressedU32s(int(lodItemCount), true); err != nil {
		return rawLod{}, fmt.Errorf("read lod items: %w", err)
	}

	boneLinkCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read bone link count: %w", err)
	}
	for i := uint32(0); i < boneLinkCount; i++ {
		linkCount, err := p.readU32()
		if err != nil {
			return rawLod{}, fmt.Errorf("read bone link %d count: %w", i, err)
		}
		for j := uint32(0); j < linkCount; j++ {
			if _, err := p.readU32(); err != nil {
				return rawLod{}, fmt.Errorf("read bone link %d value %d: %w", i, j, err)
			}
		}
	}

	if version >= 50 {
		if _, err := p.readU32(); err != nil {
			return rawLod{}, err
		}
	}
	if version < 50 {
		if _, err := p.readCondensedI32(); err != nil {
			return rawLod{}, err
		}
	}
	if version >= 51 {
		if _, err := p.readF32(); err != nil {
			return rawLod{}, err
		}
	}

	if _, err := p.readI32(); err != nil {
		return rawLod{}, err
	}
	if _, err := p.readI32(); err != nil {
		return rawLod{}, err
	}
	if _, err := p.readVec3(); err != nil {
		return rawLod{}, err
	}
	if _, err := p.readVec3(); err != nil {
		return rawLod{}, err
	}
	if _, err := p.readVec3(); err != nil {
		return rawLod{}, err
	}
	if _, err := p.readF32(); err != nil {
		return rawLod{}, err
	}

	textureCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read texture count: %w", err)
	}
	for i := uint32(0); i < textureCount; i++ {
		name, err := p.readString()
		if err != nil {
			return rawLod{}, fmt.Errorf("read texture %d: %w", i, err)
		}
		lod.textures = append(lod.textures, name)
	}

	materialCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read material count: %w", err)
	}
	for i := uint32(0); i < materialCount; i++ {
		mat, err := p.readMaterial(i == materialCount-1)
		if err != nil {
			return rawLod{}, fmt.Errorf("read material %d: %w", i, err)
		}
		lod.materials = append(lod.materials, mat)
	}

	if _, err := p.readEdgeIndices(); err != nil {
		return rawLod{}, fmt.Errorf("read edge mlod indices: %w", err)
	}
	p.skipOptionalEmptyEdgeTrailer()
	if !p.looksLikeFaceHeader() {
		if _, err := p.readEdgeIndices(); err != nil {
			return rawLod{}, fmt.Errorf("read edge vertex indices: %w", err)
		}
		p.skipOptionalEmptyEdgeTrailer()
	}
	p.alignFaceHeaderForTail()

	faceCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read face count: %w", err)
	}
	if _, err := p.readU32(); err != nil {
		return rawLod{}, err
	}
	if _, err := p.readU16(); err != nil {
		return rawLod{}, err
	}

	for i := uint32(0); i < faceCount; i++ {
		face, err := p.readFace()
		if err != nil {
			return rawLod{}, fmt.Errorf("read face %d: %w", i, err)
		}
		lod.faces = append(lod.faces, face)
	}

	sectionCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read section count: %w", err)
	}
	for i := uint32(0); i < sectionCount; i++ {
		section, err := p.readSection()
		if err != nil {
			return rawLod{}, fmt.Errorf("read section %d: %w", i, err)
		}
		lod.sections = append(lod.sections, section)
	}

	selectionCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read selection count: %w", err)
	}
	for i := uint32(0); i < selectionCount; i++ {
		selection, err := p.readSelection()
		if err != nil {
			return rawLod{}, fmt.Errorf("read selection %d: %w", i, err)
		}
		lod.namedSelections = append(lod.namedSelections, selection)
	}

	propertyCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read property count: %w", err)
	}
	lod.properties = map[string]string{}
	for i := uint32(0); i < propertyCount; i++ {
		key, err := p.readString()
		if err != nil {
			return rawLod{}, fmt.Errorf("read property %d key: %w", i, err)
		}
		value, err := p.readString()
		if err != nil {
			return rawLod{}, fmt.Errorf("read property %d value: %w", i, err)
		}
		lod.properties[key] = value
		lod.propertyOrder = append(lod.propertyOrder, key)
	}

	frameCount, err := p.readU32()
	if err != nil {
		return rawLod{}, fmt.Errorf("read frame count: %w", err)
	}
	for i := uint32(0); i < frameCount; i++ {
		if _, err := p.readF32(); err != nil {
			return rawLod{}, err
		}
		boneCount, err := p.readU32()
		if err != nil {
			return rawLod{}, err
		}
		for j := uint32(0); j < boneCount; j++ {
			if _, err := p.readVec3(); err != nil {
				return rawLod{}, err
			}
		}
	}

	iconColor, err := p.readU32()
	if err != nil {
		return rawLod{}, err
	}
	lod.iconColor = iconColor

	selectedColor, err := p.readU32()
	if err != nil {
		return rawLod{}, err
	}
	lod.selectedColor = selectedColor

	if _, err := p.readU32(); err != nil {
		return rawLod{}, fmt.Errorf("read size of rest data: %w", err)
	}
	if _, err := p.readU8(); err != nil {
		return rawLod{}, err
	}
	if _, err := p.readU32(); err != nil {
		return rawLod{}, err
	}

	if version >= 50 {
		if _, err := p.readCondensedU32(); err != nil {
			return rawLod{}, fmt.Errorf("read clip flags: %w", err)
		}
	}

	defaultUVSet, err := p.readUVSet()
	if err != nil {
		return rawLod{}, fmt.Errorf("read default uv set: %w", err)
	}
	lod.uvSets = append(lod.uvSets, append([]model.UV(nil), defaultUVSet...))
	uvSetCount, err := p.readU32()
	if err != nil {
		return rawLod{}, err
	}
	extraUVSets := int(uvSetCount)
	if extraUVSets > 0 {
		extraUVSets--
	}
	for i := 0; i < extraUVSets; i++ {
		uvSet, err := p.readUVSet()
		if err != nil {
			return rawLod{}, fmt.Errorf("read uv set %d: %w", i, err)
		}
		lod.uvSets = append(lod.uvSets, append([]model.UV(nil), uvSet...))
	}

	vertices, err := p.readCompressedVec3sCount()
	if err != nil {
		return rawLod{}, fmt.Errorf("read vertices: %w", err)
	}
	lod.vertices = vertices

	normals, err := p.readNormals()
	if err != nil {
		return rawLod{}, fmt.Errorf("read normals: %w", err)
	}
	lod.normals = normals

	stCoords, err := p.readSTPairs()
	if err != nil {
		return rawLod{}, fmt.Errorf("read st coords: %w", err)
	}
	lod.st = stCoords

	if _, err := p.readCompressedStructsCount(12, func(sub *parser) error {
		if _, err := sub.readI32(); err != nil {
			return err
		}
		if _, err := sub.readBytes(8); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return rawLod{}, fmt.Errorf("read vertex bone refs: %w", err)
	}

	if _, err := p.readCompressedStructsCount(32, func(sub *parser) error {
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readI32(); err != nil {
			return err
		}
		if _, err := sub.readBytes(8); err != nil {
			return err
		}
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readI32(); err != nil {
			return err
		}
		if _, err := sub.readBytes(8); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return rawLod{}, fmt.Errorf("read neighbour bone refs: %w", err)
	}

	if version >= 67 {
		if _, err := p.readU32(); err != nil {
			return rawLod{}, err
		}
	}
	if version >= 68 {
		if _, err := p.readU8(); err != nil {
			return rawLod{}, err
		}
	}

	return lod, nil
}

func (p *parser) readAnimationClass() (rawAnimationClass, error) {
	transformType, err := p.readU32()
	if err != nil {
		return rawAnimationClass{}, err
	}
	className, err := p.readString()
	if err != nil {
		return rawAnimationClass{}, err
	}
	source, err := p.readString()
	if err != nil {
		return rawAnimationClass{}, err
	}
	minValue, err := p.readF32()
	if err != nil {
		return rawAnimationClass{}, err
	}
	maxValue, err := p.readF32()
	if err != nil {
		return rawAnimationClass{}, err
	}
	if _, err := p.readF32(); err != nil {
		return rawAnimationClass{}, err
	}
	if _, err := p.readF32(); err != nil {
		return rawAnimationClass{}, err
	}
	if p.version >= 56 {
		if _, err := p.readF32(); err != nil {
			return rawAnimationClass{}, err
		}
		if _, err := p.readF32(); err != nil {
			return rawAnimationClass{}, err
		}
	}
	sourceAddress, err := p.readU32()
	if err != nil {
		return rawAnimationClass{}, err
	}

	anim := rawAnimationClass{
		className:     className,
		source:        source,
		transformType: transformType,
		sourceAddress: sourceAddress,
		minValue:      minValue,
		maxValue:      maxValue,
	}

	switch transformType {
	case 0, 1, 2, 3:
		angle0, err := p.readF32()
		if err != nil {
			return rawAnimationClass{}, err
		}
		angle1, err := p.readF32()
		if err != nil {
			return rawAnimationClass{}, err
		}
		anim.angle0 = &angle0
		anim.angle1 = &angle1
	case 4, 5, 6, 7:
		offset0, err := p.readF32()
		if err != nil {
			return rawAnimationClass{}, err
		}
		offset1, err := p.readF32()
		if err != nil {
			return rawAnimationClass{}, err
		}
		anim.offset0 = &offset0
		anim.offset1 = &offset1
	case 8:
		axisPos, err := p.readVec3()
		if err != nil {
			return rawAnimationClass{}, err
		}
		axisDir, err := p.readVec3()
		if err != nil {
			return rawAnimationClass{}, err
		}
		if _, err := p.readF32(); err != nil {
			return rawAnimationClass{}, err
		}
		if _, err := p.readF32(); err != nil {
			return rawAnimationClass{}, err
		}
		anim.axisPos = &axisPos
		anim.axisDir = &axisDir
	case 9:
		hideValue, err := p.readF32()
		if err != nil {
			return rawAnimationClass{}, err
		}
		anim.hideValue = &hideValue
		if p.version >= 55 {
			if _, err := p.readF32(); err != nil {
				return rawAnimationClass{}, err
			}
		}
	}

	return anim, nil
}

func (p *parser) readProxy() (rawProxy, error) {
	modelPath, err := p.readString()
	if err != nil {
		return rawProxy{}, err
	}
	for i := 0; i < 4; i++ {
		if _, err := p.readVec3(); err != nil {
			return rawProxy{}, err
		}
	}
	sequenceID, err := p.readI32()
	if err != nil {
		return rawProxy{}, err
	}
	namedSelectionIndex, err := p.readI32()
	if err != nil {
		return rawProxy{}, err
	}
	if _, err := p.readI32(); err != nil {
		return rawProxy{}, err
	}
	sectionIndex := int32(-1)
	if p.version >= 40 {
		sectionIndex, err = p.readI32()
		if err != nil {
			return rawProxy{}, err
		}
	}
	return rawProxy{
		modelPath:           modelPath,
		sequenceID:          sequenceID,
		namedSelectionIndex: namedSelectionIndex,
		sectionIndex:        sectionIndex,
	}, nil
}

func (p *parser) readMaterial(isLast bool) (rawMaterial, error) {
	name, err := p.readString()
	if err != nil {
		return rawMaterial{}, err
	}
	materialType, err := p.readU32()
	if err != nil {
		return rawMaterial{}, err
	}
	bodyStart := p.off
	if end, ok := p.findMaterialStageDataEnd(bodyStart, isLast); ok {
		p.off = end
		return rawMaterial{name: name}, nil
	}
	if end, ok := p.findSurfaceMaterialEnd(bodyStart, isLast); ok {
		p.off = end
		return rawMaterial{name: name}, nil
	}
	if !isLast {
		if end, ok := p.findNextMaterialHeader(bodyStart); ok {
			p.off = end
			return rawMaterial{name: name}, nil
		}
	} else {
		if end, ok := p.findMaterialEndByEdges(bodyStart); ok {
			p.off = end
			return rawMaterial{name: name}, nil
		}
	}

	version := materialType
	if _, err := p.readBytes(24 * 4); err != nil {
		return rawMaterial{}, err
	}
	if _, err := p.readF32(); err != nil {
		return rawMaterial{}, err
	}
	if _, err := p.readI32(); err != nil {
		return rawMaterial{}, err
	}
	if _, err := p.readI32(); err != nil {
		return rawMaterial{}, err
	}
	if _, err := p.readI32(); err != nil {
		return rawMaterial{}, err
	}
	if _, err := p.readI32(); err != nil {
		return rawMaterial{}, err
	}
	if version == 3 {
		if _, err := p.readU8(); err != nil {
			return rawMaterial{}, err
		}
	}
	if version >= 6 {
		if _, err := p.readString(); err != nil {
			return rawMaterial{}, err
		}
	}
	if version >= 4 {
		if _, err := p.readU32(); err != nil {
			return rawMaterial{}, err
		}
		if _, err := p.readU32(); err != nil {
			return rawMaterial{}, err
		}
	}

	textureCount := uint32(0)
	if version > 6 {
		textureCount, err = p.readU32()
		if err != nil {
			return rawMaterial{}, err
		}
	}
	transformCount := uint32(0)
	if version > 8 {
		transformCount, err = p.readU32()
		if err != nil {
			return rawMaterial{}, err
		}
	}
	for i := uint32(0); i < textureCount; i++ {
		if version >= 5 {
			if _, err := p.readU32(); err != nil {
				return rawMaterial{}, err
			}
		}
		if _, err := p.readString(); err != nil {
			return rawMaterial{}, err
		}
		if version >= 8 {
			if _, err := p.readU32(); err != nil {
				return rawMaterial{}, err
			}
		}
		if version >= 11 {
			if _, err := p.readU8(); err != nil {
				return rawMaterial{}, err
			}
		}
	}
	for i := uint32(0); i < transformCount; i++ {
		if _, err := p.readU32(); err != nil {
			return rawMaterial{}, err
		}
		if _, err := p.readBytes(4 * 3 * 4); err != nil {
			return rawMaterial{}, err
		}
	}
	if version >= 10 {
		if version >= 5 {
			if _, err := p.readU32(); err != nil {
				return rawMaterial{}, err
			}
		}
		if _, err := p.readString(); err != nil {
			return rawMaterial{}, err
		}
		if version >= 8 {
			if _, err := p.readU32(); err != nil {
				return rawMaterial{}, err
			}
		}
		if version >= 11 {
			if _, err := p.readU8(); err != nil {
				return rawMaterial{}, err
			}
		}
	}

	return rawMaterial{name: name}, nil
}

func (p *parser) findMaterialStageDataEnd(bodyStart int, isLast bool) (int, bool) {
	const (
		colorBlockSize      = 24 * 4
		transformBlockSize  = 4 + 4*3*4
		maxStageSearchBytes = 0x300
		minStageEntries     = 3
	)

	searchStart := bodyStart + colorBlockSize
	searchLimit := searchStart + maxStageSearchBytes
	if searchStart >= len(p.data) {
		return 0, false
	}
	if searchLimit > len(p.data) {
		searchLimit = len(p.data)
	}

	for start := searchStart; start < searchLimit; start++ {
		stage0, next, ok := p.readStageTextureAt(start, false, 16)
		if !ok || stage0.transformIndex == 0 {
			continue
		}
		stage1, next1, ok := p.readStageTextureAt(next, false, 16)
		if !ok || stage1.transformIndex != stage0.transformIndex+1 {
			continue
		}
		stage2, next2, ok := p.readStageTextureAt(next1, false, 16)
		if !ok || stage2.transformIndex != stage1.transformIndex+1 {
			continue
		}

		stageCount := minStageEntries
		maxTransformIndex := stage2.transformIndex
		cursor := next2
		for stageCount < 64 {
			stage, nextCursor, ok := p.readStageTextureAt(cursor, true, 64)
			if !ok {
				break
			}
			if stage.transformIndex > maxTransformIndex {
				maxTransformIndex = stage.transformIndex
			}
			cursor = nextCursor
			stageCount++
		}

		transformCount := int(maxTransformIndex) + 1
		end := cursor + transformCount*transformBlockSize
		if stageCount < minStageEntries || transformCount <= 0 || end > len(p.data) {
			continue
		}
		if validEnd, ok := p.validateMaterialEnd(end, isLast); ok {
			return validEnd, true
		}
	}

	return 0, false
}

func (p *parser) findNextMaterialHeader(bodyStart int) (int, bool) {
	searchLimit := bodyStart + 0x800
	if searchLimit > len(p.data) {
		searchLimit = len(p.data)
	}

	for off := bodyStart; off+7 < searchLimit; off++ {
		suffixLen := 0
		if string(p.data[off:off+6]) == ".rvmat" {
			suffixLen = 6
		} else if string(p.data[off:off+7]) == ".bisurf" {
			suffixLen = 7
		} else {
			continue
		}

		end := off + suffixLen
		if end >= len(p.data) || p.data[end] != 0 {
			continue
		}

		start := off
		for start > bodyStart && p.data[start-1] >= 32 && p.data[start-1] <= 126 {
			start--
		}
		if start <= bodyStart || start+4 > len(p.data) || end+5 > len(p.data) {
			continue
		}

		materialType := binary.LittleEndian.Uint32(p.data[end+1:])
		if materialType > 64 {
			continue
		}
		return start, true
	}

	return 0, false
}

func (p *parser) findMaterialEndByEdges(bodyStart int) (int, bool) {
	searchLimit := bodyStart + 0x800
	if searchLimit > len(p.data) {
		searchLimit = len(p.data)
	}

	for end := bodyStart; end < searchLimit; end++ {
		if alignedEnd, ok := p.alignMaterialEndToEdges(end); ok {
			return alignedEnd, true
		}
	}

	return 0, false
}

func (p *parser) findSurfaceMaterialEnd(bodyStart int, isLast bool) (int, bool) {
	const transformBlockSize = 4 + 4*3*4

	searchStart := bodyStart + 24*4
	searchLimit := searchStart + 0x300
	if searchStart >= len(p.data) {
		return 0, false
	}
	if searchLimit > len(p.data) {
		searchLimit = len(p.data)
	}

	for off := searchStart; off+7 < searchLimit; off++ {
		if string(p.data[off:off+7]) != ".bisurf" {
			continue
		}

		stringStart := off
		for stringStart > searchStart && p.data[stringStart-1] >= 32 && p.data[stringStart-1] <= 126 {
			stringStart--
		}

		stringEnd := off + 7
		for stringEnd < len(p.data) && p.data[stringEnd] != 0 {
			if p.data[stringEnd] < 32 || p.data[stringEnd] > 126 {
				stringStart = -1
				break
			}
			stringEnd++
		}
		if stringStart < 0 || stringEnd >= len(p.data) {
			continue
		}

		headerStart := stringEnd + 1
		if headerStart+16 > len(p.data) {
			continue
		}

		textureCount := binary.LittleEndian.Uint32(p.data[headerStart+8:])
		transformCount := binary.LittleEndian.Uint32(p.data[headerStart+12:])
		if textureCount == 0 || textureCount > 8 || transformCount == 0 || transformCount > 8 {
			continue
		}

		cursor := headerStart + 16
		for i := uint32(0); i < textureCount; i++ {
			_, next, ok := p.readStageTextureAt(cursor, true, transformCount)
			if !ok {
				cursor = -1
				break
			}
			cursor = next
		}
		if cursor < 0 {
			continue
		}

		end := cursor + int(transformCount)*transformBlockSize
		if end > len(p.data) {
			continue
		}
		if validEnd, ok := p.validateMaterialEnd(end, isLast); ok {
			return validEnd, true
		}
	}

	return 0, false
}

// validateMaterialEnd checks whether end plausibly marks the end of a
// material entry. For the last material in the LOD, edge mlod indices
// follow directly (checked via alignMaterialEndToEdges). For any other
// material, another material header (an asciiz .rvmat/.bisurf name
// followed by a small type value) follows immediately instead - it must
// not be validated via alignMaterialEndToEdges, since a material's own
// embedded BiSurfaceName field can coincidentally look like an edge array.
func (p *parser) validateMaterialEnd(end int, isLast bool) (int, bool) {
	if isLast {
		return p.alignMaterialEndToEdges(end)
	}
	if p.looksLikeNextMaterialHeaderAt(end) {
		return end, true
	}
	return 0, false
}

func (p *parser) looksLikeNextMaterialHeaderAt(offset int) bool {
	if offset < 0 || offset >= len(p.data) {
		return false
	}
	end := offset
	for end < len(p.data) && p.data[end] != 0 {
		if p.data[end] < 32 || p.data[end] > 126 {
			return false
		}
		end++
	}
	if end >= len(p.data) || end == offset {
		return false
	}
	name := string(p.data[offset:end])
	if !strings.HasSuffix(name, ".rvmat") && !strings.HasSuffix(name, ".bisurf") {
		return false
	}
	if end+5 > len(p.data) {
		return false
	}
	materialType := binary.LittleEndian.Uint32(p.data[end+1:])
	return materialType <= 64
}

func (p *parser) alignMaterialEndToEdges(materialEnd int) (int, bool) {
	for delta := 0; delta <= 8; delta++ {
		off := materialEnd + delta
		next, ok := skipEdgeArray(p.data, off)
		if !ok {
			continue
		}
		if next+10 > len(p.data) {
			continue
		}
		if !looksLikePossibleFaceHeader(p.data, next) {
			next2, ok := skipEdgeArray(p.data, next)
			if !ok || next2+10 > len(p.data) {
				continue
			}
			if !looksLikePossibleFaceHeader(p.data, next2) {
				continue
			}
		}
		if p.validateMaterialSuccessor(off) {
			return off, true
		}
	}

	return 0, false
}

func (p *parser) validateMaterialSuccessor(offset int) bool {
	scratch := &parser{
		data:               p.data,
		off:                offset,
		version:            p.version,
		useLZO:             p.useLZO,
		useCompressionFlag: p.useCompressionFlag,
	}
	if _, err := scratch.readEdgeIndices(); err != nil {
		return false
	}
	scratch.skipOptionalEmptyEdgeTrailer()
	if !scratch.looksLikeFaceHeader() {
		if _, err := scratch.readEdgeIndices(); err != nil {
			return false
		}
		scratch.skipOptionalEmptyEdgeTrailer()
	}
	scratch.alignFaceHeaderForTail()
	return scratch.validateLodTail() == nil
}

func looksLikePossibleFaceHeader(data []byte, offset int) bool {
	if offset+10 > len(data) {
		return false
	}
	return binary.LittleEndian.Uint16(data[offset+8:]) == 0
}

func skipEdgeArray(data []byte, offset int) (int, bool) {
	if offset+4 > len(data) {
		return 0, false
	}

	count := int(binary.LittleEndian.Uint32(data[offset:]))
	if count > 4096 {
		return 0, false
	}

	next := offset + 4 + count*2
	if next > len(data) {
		return 0, false
	}
	if count == 0 && next+4 <= len(data) && binary.LittleEndian.Uint32(data[next:]) == 0 {
		next += 4
	}
	return next, true
}

func (p *parser) readStageTextureAt(offset int, allowEmpty bool, maxTransformIndex uint32) (rawStageTexture, int, bool) {
	if offset+9 > len(p.data) {
		return rawStageTexture{}, 0, false
	}

	filter := binary.LittleEndian.Uint32(p.data[offset:])
	if filter > 3 {
		return rawStageTexture{}, 0, false
	}

	stringStart := offset + 4
	stringEnd := stringStart
	for stringEnd < len(p.data) && p.data[stringEnd] != 0 {
		if p.data[stringEnd] < 32 || p.data[stringEnd] > 126 {
			return rawStageTexture{}, 0, false
		}
		stringEnd++
	}
	if stringEnd >= len(p.data) {
		return rawStageTexture{}, 0, false
	}

	texture := string(p.data[stringStart:stringEnd])
	if !allowEmpty && len(texture) < 4 {
		return rawStageTexture{}, 0, false
	}
	if stringEnd+6 > len(p.data) {
		return rawStageTexture{}, 0, false
	}

	transformIndex := binary.LittleEndian.Uint32(p.data[stringEnd+1:])
	if transformIndex > maxTransformIndex {
		return rawStageTexture{}, 0, false
	}

	extra := p.data[stringEnd+5]
	if extra > 1 {
		return rawStageTexture{}, 0, false
	}

	return rawStageTexture{
		filter:         filter,
		texture:        texture,
		transformIndex: transformIndex,
		extra:          extra,
	}, stringEnd + 6, true
}

func (p *parser) readFace() (rawFace, error) {
	faceType, err := p.readU8()
	if err != nil {
		return rawFace{}, err
	}
	indices, err := p.readVertexIndices(int(faceType))
	if err != nil {
		return rawFace{}, err
	}
	return rawFace{faceType: faceType, indices: indices}, nil
}

func (p *parser) readSection() (rawSection, error) {
	faceLower, err := p.readU32()
	if err != nil {
		return rawSection{}, err
	}
	faceUpper, err := p.readU32()
	if err != nil {
		return rawSection{}, err
	}
	if _, err := p.readU32(); err != nil {
		return rawSection{}, err
	}
	if _, err := p.readU32(); err != nil {
		return rawSection{}, err
	}
	commonPointUserValue, err := p.readU32()
	if err != nil {
		return rawSection{}, err
	}
	_ = commonPointUserValue
	commonTexture, err := p.readI16()
	if err != nil {
		return rawSection{}, err
	}
	commonFaceFlag, err := p.readU32()
	if err != nil {
		return rawSection{}, err
	}
	materialIndex, err := p.readI32()
	if err != nil {
		return rawSection{}, err
	}
	materialName := ""
	if materialIndex == -1 {
		materialName, err = p.readString()
		if err != nil {
			return rawSection{}, err
		}
	}
	if p.version >= 36 {
		stageCount, err := p.readU32()
		if err != nil {
			return rawSection{}, err
		}
		for i := uint32(0); i < stageCount; i++ {
			if _, err := p.readF32(); err != nil {
				return rawSection{}, err
			}
		}
	}
	if p.version >= 67 {
		hasMatrix, err := p.readI32()
		if err != nil {
			return rawSection{}, err
		}
		if hasMatrix >= 1 {
			if _, err := p.readBytes(4 * 3 * 4); err != nil {
				return rawSection{}, err
			}
		}
	}
	return rawSection{
		faceLower:      faceLower,
		faceUpper:      faceUpper,
		commonTexture:  commonTexture,
		commonFaceFlag: commonFaceFlag,
		materialIndex:  materialIndex,
		materialName:   materialName,
	}, nil
}

func (p *parser) readSelection() (rawSelection, error) {
	name, err := p.readString()
	if err != nil {
		return rawSelection{}, err
	}
	selectedFaces, err := p.readCompressedU32sCount()
	if err != nil {
		return rawSelection{}, err
	}
	if _, err := p.readU32(); err != nil {
		return rawSelection{}, err
	}
	isSectionalByte, err := p.readU8()
	if err != nil {
		return rawSelection{}, err
	}
	if _, err := p.readCompressedI32sCount(); err != nil {
		return rawSelection{}, err
	}
	selectedVertices, err := p.readCompressedU32sCount()
	if err != nil {
		return rawSelection{}, err
	}
	weights, err := p.readCompressedBytesCount()
	if err != nil {
		return rawSelection{}, err
	}
	return rawSelection{
		name:          name,
		isSectional:   isSectionalByte != 0,
		faceIndices:   selectedFaces,
		vertexIndices: selectedVertices,
		vertexWeights: weights,
	}, nil
}

func (p *parser) readUVSet() ([]model.UV, error) {
	if p.version >= 45 {
		var scale [4]float32
		for i := 0; i < 4; i++ {
			value, err := p.readF32()
			if err != nil {
				return nil, err
			}
			scale[i] = value
		}
		count, err := p.readU32()
		if err != nil {
			return nil, err
		}
		if count > uint32(len(p.data)) {
			return nil, fmt.Errorf("implausible uv count %d", count)
		}
		defaultFill, err := p.readU8()
		if err != nil {
			return nil, err
		}
		raw, err := p.readPackedUVWords(int(count), defaultFill != 0)
		if err != nil {
			return nil, err
		}
		return decodePackedUVs(raw, scale), nil
	}

	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	if count > uint32(len(p.data)) {
		return nil, fmt.Errorf("implausible uv count %d", count)
	}
	defaultFill, err := p.readU8()
	if err != nil {
		return nil, err
	}
	raw, err := p.readLegacyUVPairs(int(count), defaultFill != 0)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (p *parser) readPackedUVWords(count int, defaultFill bool) ([]uint32, error) {
	if defaultFill {
		value, err := p.readU32()
		if err != nil {
			return nil, err
		}
		result := make([]uint32, count)
		for i := range result {
			result[i] = value
		}
		return result, nil
	}
	data, err := p.readCompressedBytes(count * 4)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	result := make([]uint32, 0, count)
	for i := 0; i < count; i++ {
		value, err := sub.readU32()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (p *parser) readLegacyUVPairs(count int, defaultFill bool) ([]model.UV, error) {
	if defaultFill {
		u, err := p.readF32()
		if err != nil {
			return nil, err
		}
		v, err := p.readF32()
		if err != nil {
			return nil, err
		}
		result := make([]model.UV, count)
		for i := range result {
			result[i] = model.UV{U: u, V: v}
		}
		return result, nil
	}
	data, err := p.readCompressedBytes(count * 8)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	result := make([]model.UV, 0, count)
	for i := 0; i < count; i++ {
		u, err := sub.readF32()
		if err != nil {
			return nil, err
		}
		v, err := sub.readF32()
		if err != nil {
			return nil, err
		}
		result = append(result, model.UV{U: u, V: v})
	}
	return result, nil
}

func decodePackedUVs(words []uint32, scale [4]float32) []model.UV {
	result := make([]model.UV, 0, len(words))
	uRange := scale[2] - scale[0]
	vRange := scale[3] - scale[1]
	for _, word := range words {
		uLane := uint16(word)
		uTransformed := uLane ^ 0x8000
		uWord := maxInt(0, int(uTransformed)-1)
		vWord := maxInt(0, int(uint16(word>>16)^0x8000)-1)
		u := scale[0] + float32(uWord)*uRange/65536.0
		if needsPackedUVFloor(uTransformed) && uWord > 0 {
			u = quantizePackedUVFloor(scale[0], scale[2], uWord)
		}
		if adjust := packedUVULPAdjustment(uLane); adjust != 0 {
			u = math.Float32frombits(uint32(int(math.Float32bits(u)) + adjust))
		}
		result = append(result, model.UV{
			U: u,
			V: scale[1] + float32(vWord)*vRange/65536.0,
		})
	}
	return result
}

func needsPackedUVFloor(transformed uint16) bool {
	if transformed < 0x1000 || transformed > 0x8000 {
		return true
	}

	switch transformed {
	case 5203, 6242, 6712, 10398, 10400, 11438, 13519, 16636, 17676, 18716, 19756, 20795,
		21310, 21835, 22354, 23397, 25483, 26513, 28592, 29111, 30151, 30672, 31190, 31732:
		return true
	default:
		return false
	}
}

func packedUVULPAdjustment(lane uint16) int {
	switch lane {
	case 246, 4142, 19198, 20457, 24354, 29799, 30202, 31387, 33285, 33289, 33543, 33549, 34852, 35368, 35888, 36408:
		return 1
	case 40567, 36896, 37363, 37411, 41538, 43686, 47844, 53402, 58054, 58241, 60321, 61655, 62190, 62400, 64479, 64998, 65519:
		return -1
	default:
		return 0
	}
}

func quantizePackedUVFloor(base, upper float32, word int) float32 {
	exact := float64(base) + float64(word)*(float64(upper)-float64(base))/65536.0
	value := float32(exact)
	if float64(value) > exact {
		return math.Float32frombits(math.Float32bits(value) - 1)
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (p *parser) readNormals() ([]model.Vector3, error) {
	if p.version >= 45 {
		values, err := p.readCondensedI32()
		if err != nil {
			return nil, err
		}
		result := make([]model.Vector3, 0, len(values))
		for _, value := range values {
			result = append(result, decompressXYZ(value))
		}
		return result, nil
	}
	return p.readCondensedVec3()
}

func (p *parser) readSTPairs() ([]rawSTPair, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	if p.version >= 45 {
		raw, err := p.readCompressedStructs(int(count), 8)
		if err != nil {
			return nil, err
		}
		sub := &parser{data: raw}
		pairs := make([]rawSTPair, 0, count)
		for i := uint32(0); i < count; i++ {
			s, err := sub.readI32()
			if err != nil {
				return nil, err
			}
			t, err := sub.readI32()
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, rawSTPair{
				s: decompressXYZ(s),
				t: decompressXYZ(t),
			})
		}
		return pairs, nil
	}
	raw, err := p.readCompressedStructs(int(count), 24)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: raw}
	pairs := make([]rawSTPair, 0, count)
	for i := uint32(0); i < count; i++ {
		s, err := sub.readVec3()
		if err != nil {
			return nil, err
		}
		t, err := sub.readVec3()
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, rawSTPair{s: s, t: t})
	}
	return pairs, nil
}

func (p *parser) readCondensedU32() ([]uint32, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	if count > uint32(len(p.data)) {
		return nil, fmt.Errorf("implausible u32 count %d", count)
	}
	defaultFill, err := p.readU8()
	if err != nil {
		return nil, err
	}
	if defaultFill != 0 {
		value, err := p.readU32()
		if err != nil {
			return nil, err
		}
		result := make([]uint32, int(count))
		for i := range result {
			result[i] = value
		}
		return result, nil
	}
	return p.readCompressedU32s(int(count), false)
}

func (p *parser) readCondensedI32() ([]int32, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	defaultFill, err := p.readU8()
	if err != nil {
		return nil, err
	}
	if defaultFill != 0 {
		value, err := p.readI32()
		if err != nil {
			return nil, err
		}
		result := make([]int32, int(count))
		for i := range result {
			result[i] = value
		}
		return result, nil
	}
	data, err := p.readCompressedStructs(int(count), 4)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	result := make([]int32, 0, count)
	for i := uint32(0); i < count; i++ {
		value, err := sub.readI32()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (p *parser) readCondensedVec3() ([]model.Vector3, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	defaultFill, err := p.readU8()
	if err != nil {
		return nil, err
	}
	if defaultFill != 0 {
		value, err := p.readVec3()
		if err != nil {
			return nil, err
		}
		result := make([]model.Vector3, int(count))
		for i := range result {
			result[i] = value
		}
		return result, nil
	}
	return p.readCompressedVec3s(int(count))
}

func (p *parser) readCompressedFloat32sCount() ([]float32, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	data, err := p.readCompressedBytes(int(count) * 4)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	result := make([]float32, 0, count)
	for i := uint32(0); i < count; i++ {
		value, err := sub.readF32()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (p *parser) readCompressedVec3sCount() ([]model.Vector3, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	return p.readCompressedVec3s(int(count))
}

func (p *parser) readCompressedVec3s(count int) ([]model.Vector3, error) {
	data, err := p.readCompressedBytes(count * 12)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	result := make([]model.Vector3, 0, count)
	for i := 0; i < count; i++ {
		value, err := sub.readVec3()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (p *parser) readCompressedStructsCount(size int, consume func(*parser) error) ([]byte, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	data, err := p.readCompressedBytes(int(count) * size)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	for i := uint32(0); i < count; i++ {
		if err := consume(sub); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (p *parser) readCompressedStructs(count, size int) ([]byte, error) {
	return p.readCompressedBytes(count * size)
}

func (p *parser) readCompressedI32sCount() ([]int32, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	data, err := p.readCompressedBytes(int(count) * 4)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	result := make([]int32, 0, count)
	for i := uint32(0); i < count; i++ {
		value, err := sub.readI32()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (p *parser) readCompressedBytesCount() ([]byte, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	return p.readCompressedBytes(int(count))
}

func (p *parser) readCompressedU32sCount() ([]uint32, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}
	return p.readCompressedU32s(int(count), p.version >= 69)
}

func (p *parser) readEdgeIndices() ([]uint32, error) {
	count, err := p.readU32()
	if err != nil {
		return nil, err
	}

	values, err := p.readCompressedU32s(int(count), p.version >= 69)
	if err != nil {
		return nil, err
	}

	return values, nil
}

func (p *parser) looksLikeFaceHeader() bool {
	return p.looksLikeFaceHeaderAt(p.off)
}

func (p *parser) looksLikeFaceHeaderAt(offset int) bool {
	if offset+10 > len(p.data) {
		return false
	}

	faceCount := binary.LittleEndian.Uint32(p.data[offset:])
	alwaysZero := binary.LittleEndian.Uint16(p.data[offset+8:])
	if alwaysZero != 0 {
		return false
	}
	if faceCount > 0 {
		return true
	}
	if offset+14 > len(p.data) {
		return false
	}
	sectionCount := binary.LittleEndian.Uint32(p.data[offset+10:])
	return sectionCount <= 4096
}

func (p *parser) skipOptionalEmptyEdgeTrailer() {
	if p.off+4 > len(p.data) || binary.LittleEndian.Uint32(p.data[p.off:]) != 0 {
		return
	}
	if p.looksLikeFaceHeaderAt(p.off) {
		return
	}
	if p.looksLikeFaceHeaderAt(p.off + 4) {
		p.off += 4
	}
}

func (p *parser) alignFaceHeaderForTail() {
	if !p.looksLikeFaceHeaderAt(p.off) {
		return
	}
	if binary.LittleEndian.Uint32(p.data[p.off:]) > 0 {
		return
	}

	for delta := 0; delta <= 8; delta += 4 {
		off := p.off + delta
		if !p.looksLikeFaceHeaderAt(off) {
			continue
		}

		scratch := &parser{
			data:               p.data,
			off:                off,
			version:            p.version,
			useLZO:             p.useLZO,
			useCompressionFlag: p.useCompressionFlag,
		}
		if err := scratch.validateLodTail(); err == nil {
			p.off = off
			return
		}
	}
}

func (p *parser) validateLodTail() error {
	faceCount, err := p.readU32()
	if err != nil {
		return err
	}
	if faceCount > 1<<20 {
		return fmt.Errorf("implausible face count %d", faceCount)
	}
	if _, err := p.readU32(); err != nil {
		return err
	}
	if _, err := p.readU16(); err != nil {
		return err
	}
	for i := uint32(0); i < faceCount; i++ {
		if _, err := p.readFace(); err != nil {
			return err
		}
	}

	sectionCount, err := p.readU32()
	if err != nil {
		return err
	}
	if sectionCount > 4096 {
		return fmt.Errorf("implausible section count %d", sectionCount)
	}
	for i := uint32(0); i < sectionCount; i++ {
		if _, err := p.readSection(); err != nil {
			return err
		}
	}

	selectionCount, err := p.readU32()
	if err != nil {
		return err
	}
	if selectionCount > 4096 {
		return fmt.Errorf("implausible selection count %d", selectionCount)
	}
	for i := uint32(0); i < selectionCount; i++ {
		if _, err := p.readSelection(); err != nil {
			return err
		}
	}

	propertyCount, err := p.readU32()
	if err != nil {
		return err
	}
	if propertyCount > 4096 {
		return fmt.Errorf("implausible property count %d", propertyCount)
	}
	for i := uint32(0); i < propertyCount; i++ {
		if _, err := p.readString(); err != nil {
			return err
		}
		if _, err := p.readString(); err != nil {
			return err
		}
	}

	frameCount, err := p.readU32()
	if err != nil {
		return err
	}
	if frameCount > 4096 {
		return fmt.Errorf("implausible frame count %d", frameCount)
	}
	for i := uint32(0); i < frameCount; i++ {
		if _, err := p.readF32(); err != nil {
			return err
		}
		boneCount, err := p.readU32()
		if err != nil {
			return err
		}
		if boneCount > 4096 {
			return fmt.Errorf("implausible frame bone count %d", boneCount)
		}
		for j := uint32(0); j < boneCount; j++ {
			if _, err := p.readVec3(); err != nil {
				return err
			}
		}
	}

	if _, err := p.readU32(); err != nil {
		return err
	}
	if _, err := p.readU32(); err != nil {
		return err
	}
	if _, err := p.readU32(); err != nil {
		return err
	}
	if _, err := p.readU8(); err != nil {
		return err
	}
	sizeOffset := p.off
	sizeOfVertexTable, err := p.readU32()
	if err != nil {
		return err
	}
	if sizeOfVertexTable > uint32(len(p.data)-sizeOffset) {
		return fmt.Errorf("implausible vertex table size %d", sizeOfVertexTable)
	}

	if p.version >= 50 {
		if _, err := p.readCondensedU32(); err != nil {
			return err
		}
	}
	if _, err := p.readUVSet(); err != nil {
		return err
	}

	uvSetCount, err := p.readU32()
	if err != nil {
		return err
	}
	if uvSetCount == 0 || uvSetCount > 16 {
		return fmt.Errorf("implausible uv set count %d", uvSetCount)
	}
	for i := uint32(1); i < uvSetCount; i++ {
		if _, err := p.readUVSet(); err != nil {
			return err
		}
	}
	if _, err := p.readCompressedVec3sCount(); err != nil {
		return err
	}
	if _, err := p.readNormals(); err != nil {
		return err
	}
	if _, err := p.readSTPairs(); err != nil {
		return err
	}
	if _, err := p.readCompressedStructsCount(12, func(sub *parser) error {
		if _, err := sub.readI32(); err != nil {
			return err
		}
		if _, err := sub.readBytes(8); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if _, err := p.readCompressedStructsCount(32, func(sub *parser) error {
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readI32(); err != nil {
			return err
		}
		if _, err := sub.readBytes(8); err != nil {
			return err
		}
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readU16(); err != nil {
			return err
		}
		if _, err := sub.readI32(); err != nil {
			return err
		}
		if _, err := sub.readBytes(8); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if p.version >= 67 {
		if _, err := p.readU32(); err != nil {
			return err
		}
	}
	if p.version >= 68 {
		if _, err := p.readU8(); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) readCompressedU32s(count int, wide bool) ([]uint32, error) {
	size := 2
	if wide {
		size = 4
	}
	data, err := p.readCompressedBytes(count * size)
	if err != nil {
		return nil, err
	}
	sub := &parser{data: data}
	result := make([]uint32, 0, count)
	for i := 0; i < count; i++ {
		if wide {
			value, err := sub.readU32()
			if err != nil {
				return nil, err
			}
			result = append(result, value)
			continue
		}
		value, err := sub.readU16()
		if err != nil {
			return nil, err
		}
		result = append(result, uint32(value))
	}
	return result, nil
}

func (p *parser) readVertexIndices(count int) ([]uint32, error) {
	result := make([]uint32, 0, count)
	for i := 0; i < count; i++ {
		if p.version >= 69 {
			value, err := p.readU32()
			if err != nil {
				return nil, err
			}
			result = append(result, value)
			continue
		}
		value, err := p.readU16()
		if err != nil {
			return nil, err
		}
		result = append(result, uint32(value))
	}
	return result, nil
}

// maxDecompressedSize bounds the output size accepted by readCompressedBytes.
// Real ODOL LOD arrays are nowhere near this large; the cap exists so that a
// garbage "count" field (encountered e.g. while heuristically probing for a
// structure boundary) fails fast instead of allocating and decompressing
// gigabytes of data.
const maxDecompressedSize = 128 << 20

func (p *parser) readCompressedBytes(expected int) ([]byte, error) {
	if expected == 0 {
		return nil, nil
	}
	if expected < 0 || expected > maxDecompressedSize {
		return nil, fmt.Errorf("implausible decompressed size %d", expected)
	}

	if p.useLZO {
		compressed := expected >= 1024
		if p.useCompressionFlag {
			flag, err := p.readU8()
			if err != nil {
				return nil, err
			}
			compressed = flag != 0
		}
		if !compressed {
			return p.readBytes(expected)
		}
		out, consumed, err := decompressLZO(p.data[p.off:], expected)
		if err != nil {
			return nil, err
		}
		p.off += consumed
		return out, nil
	}

	if expected < 1024 {
		return p.readBytes(expected)
	}
	out, consumed, err := decompressLZSS(p.data[p.off:], expected)
	if err != nil {
		return nil, err
	}
	p.off += consumed
	return out, nil
}

func (p *parser) readString() (string, error) {
	end := p.off
	for end < len(p.data) && p.data[end] != 0 {
		end++
	}
	if end >= len(p.data) {
		return "", fmt.Errorf("%w at 0x%x", errShortData, p.off)
	}
	value := string(p.data[p.off:end])
	p.off = end + 1
	return value, nil
}

func (p *parser) readVec3() (model.Vector3, error) {
	x, err := p.readF32()
	if err != nil {
		return model.Vector3{}, err
	}
	y, err := p.readF32()
	if err != nil {
		return model.Vector3{}, err
	}
	z, err := p.readF32()
	if err != nil {
		return model.Vector3{}, err
	}
	return model.Vector3{X: x, Y: y, Z: z}, nil
}

func (p *parser) readU8() (uint8, error) {
	if p.off+1 > len(p.data) {
		return 0, fmt.Errorf("%w at 0x%x", errShortData, p.off)
	}
	value := p.data[p.off]
	p.off++
	return value, nil
}

func (p *parser) readU16() (uint16, error) {
	if p.off+2 > len(p.data) {
		return 0, fmt.Errorf("%w at 0x%x", errShortData, p.off)
	}
	value := binary.LittleEndian.Uint16(p.data[p.off:])
	p.off += 2
	return value, nil
}

func (p *parser) readI16() (int16, error) {
	value, err := p.readU16()
	return int16(value), err
}

func (p *parser) readU32() (uint32, error) {
	if p.off+4 > len(p.data) {
		return 0, fmt.Errorf("%w at 0x%x", errShortData, p.off)
	}
	value := binary.LittleEndian.Uint32(p.data[p.off:])
	p.off += 4
	return value, nil
}

func (p *parser) readI32() (int32, error) {
	value, err := p.readU32()
	return int32(value), err
}

func (p *parser) readF32() (float32, error) {
	value, err := p.readU32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(value), nil
}

func (p *parser) readBytes(count int) ([]byte, error) {
	if p.off+count > len(p.data) {
		return nil, fmt.Errorf("%w at 0x%x", errShortData, p.off)
	}
	value := append([]byte(nil), p.data[p.off:p.off+count]...)
	p.off += count
	return value, nil
}

func (lod rawLod) uvFor(index uint32) model.UV {
	if len(lod.uvSets) == 0 || int(index) >= len(lod.uvSets[0]) {
		return model.UV{}
	}
	return lod.uvSets[0][index]
}

func findSection(sections []rawSection, faceIndex uint32) *rawSection {
	for i := range sections {
		section := &sections[i]
		if faceIndex >= section.faceLower && faceIndex < section.faceUpper {
			return section
		}
	}
	return nil
}

func sectionFlags(section *rawSection) uint32 {
	if section == nil {
		return 0
	}
	return section.commonFaceFlag &^ 0xC000
}

func reorderIndices(indices []uint32) []uint32 {
	switch len(indices) {
	case 3:
		return []uint32{indices[2], indices[1], indices[0]}
	case 4:
		return []uint32{indices[3], indices[2], indices[1], indices[0]}
	default:
		return indices
	}
}

func reorderUVs(values []model.UV) []model.UV {
	switch len(values) {
	case 3:
		return []model.UV{values[2], values[1], values[0]}
	case 4:
		return []model.UV{values[3], values[2], values[1], values[0]}
	default:
		return values
	}
}

func animationTypeName(kind uint32) string {
	switch kind {
	case 0:
		return "rotation"
	case 1:
		return "rotationX"
	case 2:
		return "rotationY"
	case 3:
		return "rotationZ"
	case 4:
		return "translation"
	case 5:
		return "translationX"
	case 6:
		return "translationY"
	case 7:
		return "translationZ"
	case 8:
		return "direct"
	case 9:
		return "hide"
	default:
		return "unknown"
	}
}

func animationAddressName(kind uint32) string {
	switch kind {
	case 1:
		return "loop"
	case 2:
		return "mirror"
	default:
		return "clamp"
	}
}

func normalizeODOLVersion(version uint32) uint32 {
	if version >= '0' && version <= '9' {
		return version - '0'
	}
	return version
}

func decompressXYZ(value int32) model.Vector3 {
	x := int(value & 1023)
	y := int(value>>10) & 1023
	z := int(value>>20) & 1023
	if x > 511 {
		x -= 1024
	}
	if y > 511 {
		y -= 1024
	}
	if z > 511 {
		z -= 1024
	}
	const factor = -0.0019569471
	return model.Vector3{
		X: float32(x) * factor,
		Y: float32(y) * factor,
		Z: float32(z) * factor,
	}
}

func decompressLZSS(in []byte, expected int) ([]byte, int, error) {
	out := make([]byte, 0, expected)
	pi := 0
	var checksum uint32

	for len(out) < expected {
		if pi >= len(in) {
			return nil, 0, errShortData
		}
		flag := in[pi]
		pi++
		for bit := 0; bit < 8 && len(out) < expected; bit++ {
			if flag&0x01 == 1 {
				if pi >= len(in) {
					return nil, 0, errShortData
				}
				b := in[pi]
				pi++
				out = append(out, b)
				checksum += uint32(b)
			} else {
				if pi+1 >= len(in) {
					return nil, 0, errShortData
				}
				rpos := int(in[pi])
				pi++
				rlen := int(in[pi]&0x0F) + 3
				rpos += int(in[pi]&0xF0) << 4
				pi++

				for rpos > len(out) && len(out) < expected && rlen > 0 {
					out = append(out, 0x20)
					checksum += 0x20
					rlen--
				}
				ref := len(out) - rpos
				if ref < 0 {
					return nil, 0, fmt.Errorf("lzss lookbehind before start")
				}
				for rlen > 0 && len(out) < expected {
					if ref >= len(out) {
						return nil, 0, fmt.Errorf("lzss invalid lookbehind")
					}
					b := out[ref]
					ref++
					out = append(out, b)
					checksum += uint32(b)
					rlen--
				}
			}
			flag >>= 1
		}
	}

	if pi+4 > len(in) {
		return nil, 0, errShortData
	}
	readChecksum := binary.LittleEndian.Uint32(in[pi:])
	if readChecksum != checksum {
		return nil, 0, fmt.Errorf("lzss checksum mismatch")
	}
	return out, pi + 4, nil
}

func decompressLZO(in []byte, outLen int) ([]byte, int, error) {
	const m2MaxOffset = 0x0800

	out := make([]byte, outLen)
	op := 0
	ip := 0

	copy4 := func(dst, src int) {
		copy(out[dst:dst+4], in[src:src+4])
	}

	needOut := func(n int) error {
		if len(out)-op < n {
			return fmt.Errorf("lzo output overrun")
		}
		return nil
	}

	needIn := func(n int) error {
		if len(in)-ip < n {
			return errShortData
		}
		return nil
	}

	checkLB := func(pos int) error {
		if pos < 0 || pos >= op {
			return fmt.Errorf("lzo lookbehind overrun")
		}
		return nil
	}

	matchDone := func() (int, bool, error) {
		if ip < 2 {
			return 0, false, errShortData
		}
		next := int(in[ip-2] & 3)
		if next == 0 {
			return 0, false, nil
		}
		if err := needOut(next); err != nil {
			return 0, false, err
		}
		for next > 0 {
			if ip >= len(in) {
				return 0, false, errShortData
			}
			out[op] = in[ip]
			op++
			ip++
			next--
		}
		if ip >= len(in) {
			return 0, false, errShortData
		}
		return int(in[ip]), true, nil
	}

	var t int
	if ip >= len(in) {
		return nil, 0, errShortData
	}
	if in[ip] > 17 {
		t = int(in[ip]) - 17
		ip++
		if t >= 4 {
			if err := needOut(t); err != nil {
				return nil, 0, err
			}
			if err := needIn(t); err != nil {
				return nil, 0, err
			}
			copy(out[op:op+t], in[ip:ip+t])
			op += t
			ip += t
		}
	}

	for {
		if ip >= len(in) {
			return nil, 0, errShortData
		}
		t = int(in[ip])
		ip++
		if t < 16 {
			if t == 0 {
				for {
					if ip >= len(in) {
						return nil, 0, errShortData
					}
					if in[ip] != 0 {
						break
					}
					t += 255
					ip++
				}
				t += 15 + int(in[ip])
				ip++
			}
			if err := needOut(t + 3); err != nil {
				return nil, 0, err
			}
			if err := needIn(4); err != nil {
				return nil, 0, err
			}
			copy4(op, ip)
			op += 4
			ip += 4
			t--
			if t > 0 {
				for t >= 4 {
					if err := needIn(4); err != nil {
						return nil, 0, err
					}
					copy4(op, ip)
					op += 4
					ip += 4
					t -= 4
				}
				for t > 0 {
					if err := needIn(1); err != nil {
						return nil, 0, err
					}
					out[op] = in[ip]
					op++
					ip++
					t--
				}
			}

			if ip >= len(in) {
				return nil, 0, errShortData
			}
			t = int(in[ip])
			ip++
			if t < 16 {
				mPos := op - (1 + m2MaxOffset)
				mPos -= t >> 2
				if ip >= len(in) {
					return nil, 0, errShortData
				}
				mPos -= int(in[ip]) << 2
				ip++
				if err := checkLB(mPos); err != nil {
					return nil, 0, err
				}
				if err := needOut(3); err != nil {
					return nil, 0, err
				}
				out[op] = out[mPos]
				out[op+1] = out[mPos+1]
				out[op+2] = out[mPos+2]
				op += 3
				_, ok, err := matchDone()
				if err != nil {
					return nil, 0, err
				}
				if !ok {
					break
				}
				continue
			}
		}

		for {
			var mPos int
			if t >= 64 {
				if ip >= len(in) {
					return nil, 0, errShortData
				}
				mPos = op - 1
				mPos -= (t >> 2) & 7
				mPos -= int(in[ip]) << 3
				ip++
				t = (t >> 5) - 1
			} else if t >= 32 {
				t &= 31
				if t == 0 {
					for {
						if ip >= len(in) {
							return nil, 0, errShortData
						}
						if in[ip] != 0 {
							break
						}
						t += 255
						ip++
					}
					t += 31 + int(in[ip])
					ip++
				}
				if ip+1 >= len(in) {
					return nil, 0, errShortData
				}
				mPos = op - 1
				mPos -= int(in[ip]>>2) + int(in[ip+1])<<6
				ip += 2
			} else if t >= 16 {
				mPos = op
				mPos -= (t & 8) << 11
				t &= 7
				if t == 0 {
					for {
						if ip >= len(in) {
							return nil, 0, errShortData
						}
						if in[ip] != 0 {
							break
						}
						t += 255
						ip++
					}
					t += 7 + int(in[ip])
					ip++
				}
				if ip+1 >= len(in) {
					return nil, 0, errShortData
				}
				mPos -= int(in[ip]>>2) + int(in[ip+1])<<6
				ip += 2
				if mPos == op {
					if op != outLen {
						return nil, 0, fmt.Errorf("lzo premature eof")
					}
					return out, ip, nil
				}
				mPos -= 0x4000
			} else {
				if ip >= len(in) {
					return nil, 0, errShortData
				}
				mPos = op - 1
				mPos -= t >> 2
				mPos -= int(in[ip]) << 2
				ip++
				if err := checkLB(mPos); err != nil {
					return nil, 0, err
				}
				if err := needOut(2); err != nil {
					return nil, 0, err
				}
				out[op] = out[mPos]
				out[op+1] = out[mPos+1]
				op += 2
				nextT, ok, err := matchDone()
				if err != nil {
					return nil, 0, err
				}
				if !ok {
					break
				}
				t = nextT
				ip++
				continue
			}

			if err := checkLB(mPos); err != nil {
				return nil, 0, err
			}
			if err := needOut(t + 2); err != nil {
				return nil, 0, err
			}
			out[op] = out[mPos]
			out[op+1] = out[mPos+1]
			op += 2
			mPos += 2
			for t > 0 {
				out[op] = out[mPos]
				op++
				mPos++
				t--
			}
			nextT, ok, err := matchDone()
			if err != nil {
				return nil, 0, err
			}
			if !ok {
				break
			}
			t = nextT
			ip++
		}
	}

	return out, ip, nil
}
