// Package vue 是 Lemon Paperplane 的 Vue.js 插件：识别 Vue 与 Vite / Vue CLI / Nuxt，
// 给出关键信息、默认脚本偏好，以及旧版 Vue CLI（webpack 4）在新 Node 上的预先提示与修复。
package vue

import (
	"regexp"

	sdk "github.com/lemon-paperplane/lemon-paperplane-main/lemon-paperplane-main-plugin-sdk"
)

// PluginID 是插件 id。
const PluginID = "vue"

// 构建工具。
const (
	BuildVite   = "vite"
	BuildVueCLI = "vue-cli"
	BuildNuxt   = "nuxt"
	BuildOther  = ""
)

// Plugin 实现 vue 插件。
type Plugin struct{}

// New 返回插件实例。
func New() any { return &Plugin{} }

// Detection 是从 package.json 得到的识别结果。
type Detection struct {
	Vue      bool
	Build    string
	Nuxt     bool
	UniApp   bool // uni-app 工程（Vue 是它的底座，脚本与旧版兼容提示交给 uniapp 插件）
	Prefer   []string
	VueRange string
}

var (
	vueCLIServeRe = regexp.MustCompile(`vue-cli-service\s+serve\b`)
	nuxtDevRe     = regexp.MustCompile(`\b(?:nuxt|nuxi)(?:\s+dev\b|\s*$)`)
)

// DetectVue 判断 package.json 是否是 Vue 工程。
func DetectVue(pkg *pkgJSON) Detection {
	d := Detection{}
	if pkg == nil {
		return d
	}
	d.Nuxt = pkg.has("nuxt") || pkg.has("nuxt3") || pkg.has("nuxt-edge")
	d.Vue = pkg.has("vue") || d.Nuxt || pkg.has("@vue/cli-service") || pkg.has("@vitejs/plugin-vue")
	if !d.Vue {
		return d
	}
	d.VueRange = pkg.rangeOf("vue")
	d.UniApp = pkg.hasPrefix("@dcloudio/")
	switch {
	case d.Nuxt:
		d.Build = BuildNuxt
		d.Prefer = scriptsMatching(pkg, nuxtDevRe.MatchString)
	case pkg.has("@vue/cli-service"):
		d.Build = BuildVueCLI
		d.Prefer = scriptsMatching(pkg, vueCLIServeRe.MatchString)
	case pkg.has("vite") || pkg.has("@vitejs/plugin-vue"):
		d.Build = BuildVite
	}
	if d.UniApp {
		d.Prefer = nil
	}
	return d
}

// Detect 在 nodejs facet 出现的目录里识别 Vue。
func (pl *Plugin) Detect(ctx *sdk.Context, p *sdk.DetectParams) (*sdk.DetectResult, error) {
	if !sdk.HasFile(p.Files, "package.json") {
		return &sdk.DetectResult{}, nil
	}
	pkg, err := readPkg(p.Dir)
	if err != nil {
		return &sdk.DetectResult{}, nil
	}
	d := DetectVue(pkg)
	if !d.Vue {
		return &sdk.DetectResult{}, nil
	}
	data := map[string]any{"build": d.Build}
	if mj := major(cleanRange(d.VueRange)); mj > 0 {
		data["vue"] = mj
	}
	if len(d.Prefer) > 0 {
		data[sdk.FacetDataPreferScripts] = d.Prefer
	}
	res := &sdk.DetectResult{Facets: []sdk.Facet{{ID: "vue", Data: data}}}
	if d.Nuxt {
		res.Facets = append(res.Facets, sdk.Facet{ID: "nuxt"})
	}
	return res, nil
}

// Analyze 给出 Vue / 构建工具版本，旧版 Vue CLI、Nuxt 2 在 Node 17+ 上预先提示。
func (pl *Plugin) Analyze(ctx *sdk.Context, p *sdk.AnalyzeParams) (*sdk.AnalyzeResult, error) {
	dir := p.Module.Dir
	pkg, err := readPkg(dir)
	if err != nil {
		return &sdk.AnalyzeResult{}, nil
	}
	d := DetectVue(pkg)
	if !d.Vue {
		return &sdk.AnalyzeResult{}, nil
	}
	nm := loadNodeModel(&p.Module)
	res := &sdk.AnalyzeResult{}
	if v := depVersion(dir, pkg, nm, "vue"); v != "" {
		res.Info = append(res.Info, sdk.InfoItem{Label: "Vue", Value: majorMinor(v), Icon: "icons/vue.svg"})
	}
	var buildPkg, buildLabel, buildIcon string
	switch d.Build {
	case BuildNuxt:
		buildPkg, buildLabel, buildIcon = "nuxt", "Nuxt", "icons/nuxt.svg"
		if !pkg.has("nuxt") {
			buildPkg = "nuxt3"
		}
	case BuildVueCLI:
		buildPkg, buildLabel, buildIcon = "@vue/cli-service", "Vue CLI", "icons/vue.svg"
	case BuildVite:
		buildPkg, buildLabel, buildIcon = "vite", "Vite", "icons/vite.svg"
	}
	buildVer := ""
	if buildPkg != "" {
		buildVer = depVersion(dir, pkg, nm, buildPkg)
		if !d.UniApp {
			res.Info = append(res.Info, sdk.InfoItem{Label: buildLabel, Value: majorMinor(buildVer), Icon: buildIcon})
		}
	}
	// webpack 4 时代的 Vue CLI / Nuxt 2：在 Node 17+ 上会报 0308010C，不等运行就提示
	if !d.UniApp && !p.Module.HasFacet("uniapp") && (nm.LegacyPackage == "@vue/cli-service" || nm.LegacyPackage == "nuxt") {
		if nv, ok := legacyNeeded(ctx, p, nm); ok {
			what := buildLabel
			if mm := majorMinor(buildVer); mm != "" {
				what += " " + mm
			}
			res.Problems = append(res.Problems, legacyProblem(ctx, what, nv))
		}
	}
	return res, nil
}
