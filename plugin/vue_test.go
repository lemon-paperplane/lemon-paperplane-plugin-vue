package vue

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/lemon-paperplane/lemon-paperplane-main/lemon-paperplane-main-plugin-sdk"
)

const viteVue = `{"name":"console","scripts":{"serve":"vite preview","dev":"vite --host"},"dependencies":{"vue":"^3.5.13"},"devDependencies":{"vite":"^6.0.6","@vitejs/plugin-vue":"^5.2.1"}}`
const vueCLI4 = `{"name":"dash","scripts":{"serve":"vue-cli-service serve","build":"vue-cli-service build"},"dependencies":{"vue":"^2.6.11"},"devDependencies":{"@vue/cli-service":"~4.5.0"}}`
const nuxt3 = `{"name":"site","scripts":{"dev":"nuxt dev","build":"nuxt build"},"dependencies":{"nuxt":"^3.12.0","vue":"^3.4.0"}}`
const uniVue2 = `{"name":"uni","scripts":{"serve":"npm run dev:h5","dev:h5":"cross-env UNI_PLATFORM=h5 vue-cli-service uni-serve"},"dependencies":{"vue":"^2.6.11","@dcloudio/uni-h5":"^2.0.0"},"devDependencies":{"@vue/cli-service":"^4.2.0","@dcloudio/vue-cli-plugin-uni":"^2.0.0"}}`

func detect(t *testing.T, pkg string) *sdk.DetectResult {
	t.Helper()
	dir := writeFiles(t, map[string]string{"package.json": pkg})
	r, err := (&Plugin{}).Detect(testCtx(), &sdk.DetectParams{Dir: dir, Files: []string{"package.json"}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDetect(t *testing.T) {
	r := detect(t, viteVue)
	if len(r.Facets) != 1 || r.Facets[0].ID != "vue" || r.Facets[0].Data["build"] != BuildVite || r.Facets[0].Data["vue"] != 3 {
		t.Errorf("vite: %+v", r)
	}
	if _, ok := r.Facets[0].Data["preferScripts"]; ok {
		t.Error("Vite 工程交给 nodejs 的通用规则选默认脚本")
	}
	r = detect(t, vueCLI4)
	if r.Facets[0].Data["build"] != BuildVueCLI || strings.Join(r.Facets[0].Data["preferScripts"].([]string), ",") != "serve" {
		t.Errorf("vue cli: %+v", r)
	}
	r = detect(t, nuxt3)
	if len(r.Facets) != 2 || r.Facets[1].ID != "nuxt" || strings.Join(r.Facets[0].Data["preferScripts"].([]string), ",") != "dev" {
		t.Errorf("nuxt: %+v", r)
	}
	r = detect(t, uniVue2)
	if len(r.Facets) != 1 || r.Facets[0].Data["preferScripts"] != nil {
		t.Errorf("uni-app 的脚本偏好交给 uniapp 插件：%+v", r)
	}
	if r = detect(t, `{"name":"x","dependencies":{"react":"^18.0.0"}}`); len(r.Facets) != 0 {
		t.Errorf("react 工程不是 vue：%+v", r)
	}
}

func analyze(t *testing.T, pkg string, nodeVer string, model json.RawMessage, facets ...string) *sdk.AnalyzeResult {
	t.Helper()
	dir := writeFiles(t, map[string]string{"package.json": pkg})
	mod := sdk.ModuleInfo{Dir: dir, Models: map[string]json.RawMessage{"nodejs": model}}
	for _, f := range facets {
		mod.Facets = append(mod.Facets, sdk.Facet{ID: f})
	}
	p := &sdk.AnalyzeParams{Module: mod, Toolchains: map[string]sdk.Toolchain{}}
	if nodeVer != "" {
		p.Toolchains["node"] = sdk.Toolchain{Kind: "node", Version: nodeVer}
	}
	r, err := (&Plugin{}).Analyze(testCtx(), p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func infoText(r *sdk.AnalyzeResult) string {
	var s []string
	for _, i := range r.Info {
		s = append(s, i.Label+" "+i.Value)
	}
	return strings.Join(s, " · ")
}

func TestAnalyze(t *testing.T) {
	m := nodeModelJSON(map[string]string{"vue": "3.5.26", "vite": "6.4.1"}, "18 || 20 || >=22", "26", false, "")
	r := analyze(t, viteVue, "22.20.0", m, "nodejs", "vue")
	if infoText(r) != "Vue 3.5 · Vite 6.4" || len(r.Problems) != 0 {
		t.Errorf("vite: %s %+v", infoText(r), r.Problems)
	}
	// Vue CLI 4 在 Node 22 上：预先给出问题与两个修复
	m = nodeModelJSON(map[string]string{"vue": "2.6.14", "@vue/cli-service": "4.5.19"}, "", "16", true, "@vue/cli-service")
	r = analyze(t, vueCLI4, "22.20.0", m, "nodejs", "vue")
	if infoText(r) != "Vue 2.6 · Vue CLI 4.5" || len(r.Problems) != 1 {
		t.Fatalf("vue cli 4 / node 22: %s %+v", infoText(r), r.Problems)
	}
	fx := r.Problems[0].Fixes
	if len(fx) != 2 || fx[0].Kind != sdk.FixAppendEnv || fx[0].Params["value"] != "--openssl-legacy-provider" || !fx[0].Recommended || fx[1].Params["constraint"] != "16" {
		t.Errorf("fixes %+v", fx)
	}
	// 工程环境里已开启旧版兼容（fix.apply 之后）不再提示
	dir := writeFiles(t, map[string]string{"package.json": vueCLI4})
	pr := &sdk.AnalyzeParams{Module: sdk.ModuleInfo{Dir: dir, Models: map[string]json.RawMessage{"nodejs": m}},
		Toolchains: map[string]sdk.Toolchain{"node": {Kind: "node", Version: "22.20.0"}},
		Env:        map[string]string{"NODE_OPTIONS": "--max-old-space-size=4096 --openssl-legacy-provider"}}
	if r, _ := (&Plugin{}).Analyze(testCtx(), pr); len(r.Problems) != 0 {
		t.Errorf("已开启旧版兼容仍提示：%+v", r.Problems)
	}
	// Node 16 上没有问题
	if r = analyze(t, vueCLI4, "16.17.1", m, "nodejs", "vue"); len(r.Problems) != 0 {
		t.Errorf("node 16 不该有问题：%+v", r.Problems)
	}
	// uni-app 工程交给 uniapp 插件提示，也不重复显示构建工具
	m = nodeModelJSON(map[string]string{"vue": "2.7.16"}, "", "16", true, "@vue/cli-service")
	if r = analyze(t, uniVue2, "22.20.0", m, "nodejs", "vue", "uniapp"); len(r.Problems) != 0 || infoText(r) != "Vue 2.7" {
		t.Errorf("uni-app: %s %+v", infoText(r), r.Problems)
	}
	// 没有 nodejs 模型时退回声明范围
	if r = analyze(t, nuxt3, "", nil, "nodejs", "vue", "nuxt"); infoText(r) != "Vue 3.4 · Nuxt 3.12" {
		t.Errorf("nuxt: %s", infoText(r))
	}
}

func TestRules(t *testing.T) {
	if ok, _ := ruleMatch(t, "vue.version-mismatch", "Error: Vue packages version mismatch:", ""); !ok {
		t.Error("version mismatch")
	}
	if ok, _ := ruleMatch(t, "vue.preprocessor-missing", `[vite] Internal server error: Preprocessor dependency "sass" not found. Did you install it?`, ""); !ok {
		t.Error("preprocessor")
	}
}

func TestCleanRange(t *testing.T) {
	for in, want := range map[string]string{"^3.5.13": "3.5.13", "~4.5.0": "4.5.0", "3.x": "3", "npm:vue@^3.4": "3.4", "latest": "", "workspace:*": "", ">=2.6 <3": "2.6", "2.x || 3.x": "2"} {
		if got := cleanRange(in); got != want {
			t.Errorf("cleanRange(%q) = %q want %q", in, got, want)
		}
	}
}
