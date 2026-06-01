package modelcfg

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/jmhobbs/odol/internal/model"
)

var cfgTemplate = template.Must(template.New("model.cfg").Funcs(template.FuncMap{
	"isLast": func(i, n int) bool { return i == n-1 },
	"parentOr": func(s, def string) string {
		if s == "" {
			return def
		}
		return s
	},
	"derefBool":  func(b *bool) bool { return *b },
	"derefFloat": func(f *float32) float32 { return *f },
	"formatFloat": func(v float32) string {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
	},
}).Parse(`class CfgSkeletons
{
{{- range .Skeletons}}
	class {{.Name}} : {{parentOr .Parent "Default"}}
	{
		skeletonInherit = {{printf "%q" .SkeletonInherit}};
{{- if .IsDiscrete}}
		isDiscrete = {{if derefBool .IsDiscrete}}1{{else}}0{{end}};
{{- end}}
		SkeletonBones[] =
		{
{{- $n := len .Bones}}
{{- range $i, $bone := .Bones}}
			{{printf "%q" $bone.Name}}, {{printf "%q" $bone.Parent}}{{if not (isLast $i $n)}},{{end}}
{{- end}}
		};
{{- if .PivotsModel}}
		pivotsModel = {{printf "%q" .PivotsModel}};
{{- end}}
	};
{{- end}}
};
class CfgModels
{
	class Default
	{
		sections[] = {};
		sectionsInherit = "";
		skeletonName = "";
	};
{{- range .Models}}
	class {{.Name}} : {{parentOr .Parent "Default"}}
	{
		sectionsInherit = {{printf "%q" .SectionsInherit}};
		sections[] =
		{
{{- $n := len .Sections}}
{{- range $i, $section := .Sections}}
			{{printf "%q" $section}}{{if not (isLast $i $n)}},{{end}}
{{- end}}
		};
		skeletonName = {{printf "%q" .SkeletonName}};
		class Animations
		{
{{- range .Animations}}
			class {{.ClassName}}
			{
				type = {{printf "%q" .Type}};
				source = {{printf "%q" .Source}};
{{- if .Selection}}
				selection = {{printf "%q" .Selection}};
{{- end}}
{{- if .Axis}}
				axis = {{printf "%q" .Axis}};
{{- end}}
{{- if .Memory}}
				memory = {{if derefBool .Memory}}true{{else}}false{{end}};
{{- end}}
{{- if .SourceAddress}}
				sourceAddress = {{printf "%q" .SourceAddress}};
{{- end}}
				minValue = {{formatFloat .MinValue}};
				maxValue = {{formatFloat .MaxValue}};
{{- if .Angle0}}
				angle0 = {{formatFloat (derefFloat .Angle0)}};
{{- end}}
{{- if .Angle1}}
				angle1 = {{formatFloat (derefFloat .Angle1)}};
{{- end}}
{{- if .Offset0}}
				offset0 = {{formatFloat (derefFloat .Offset0)}};
{{- end}}
{{- if .Offset1}}
				offset1 = {{formatFloat (derefFloat .Offset1)}};
{{- end}}
{{- if .HideValue}}
				hideValue = {{formatFloat (derefFloat .HideValue)}};
{{- end}}
			};
{{- end}}
		};
	};
{{- end}}
};
`))

func Render(cfg model.Config) (string, error) {
	var b bytes.Buffer
	if err := cfgTemplate.Execute(&b, cfg); err != nil {
		return "", err
	}
	return b.String(), nil
}
