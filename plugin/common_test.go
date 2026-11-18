package vue

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdk "github.com/lemon-paperplane/lemon-paperplane-main/lemon-paperplane-main-plugin-sdk"
)

func testCtx() *sdk.Context { return &sdk.Context{Context: context.Background()} }

// writeFiles 在临时目录里按 路径 → 内容 生成夹具。
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// nodeModelJSON 构造 nodejs 插件的模型（框架插件从 Module.Models["nodejs"] 读取）。
func nodeModelJSON(versions map[string]string, constraint, preferred string, legacy bool, legacyPkg string) json.RawMessage {
	m := map[string]any{
		"versions":       versions,
		"node":           map[string]any{"constraint": constraint, "preferred": preferred, "legacy": legacy},
		"legacyPackage":  legacyPkg,
		"packageManager": map[string]any{"name": "yarn"},
	}
	b, _ := json.Marshal(m)
	return b
}

// TestL10nCoverage：manifest 的 %key% 与代码里 ctx.T 的 key 在两份文案里都要有，且两份文案键一致。
func TestL10nCoverage(t *testing.T) {
	keys := map[string]bool{}
	b, _ := os.ReadFile("../plugin.json")
	for _, m := range regexp.MustCompile(`%([\w.\-:]+)%`).FindAllStringSubmatch(string(b), -1) {
		keys[m[1]] = true
	}
	files, _ := filepath.Glob("*.go")
	tre := regexp.MustCompile(`ctx\.T\("([^"]+)"`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		for _, m := range tre.FindAllStringSubmatch(string(src), -1) {
			if !strings.HasSuffix(m[1], ".") {
				keys[m[1]] = true
			}
		}
	}
	bundles := map[string]map[string]string{}
	for _, loc := range []string{"zh-Hans", "en"} {
		var bundle map[string]string
		raw, err := os.ReadFile(filepath.Join("..", "l10n", loc+".json"))
		if err != nil || json.Unmarshal(raw, &bundle) != nil {
			t.Fatalf("l10n/%s.json: %v", loc, err)
		}
		bundles[loc] = bundle
		for k := range keys {
			if bundle[k] == "" {
				t.Errorf("%s 缺少 %s", loc, k)
			}
		}
	}
	for k := range bundles["zh-Hans"] {
		if _, ok := bundles["en"][k]; !ok {
			t.Errorf("en 缺少 %s", k)
		}
	}
	if _, err := sdk.LoadManifest(".."); err != nil {
		t.Fatal(err)
	}
}

// ruleMatch 用 manifest 里的规则匹配一行日志，返回第一个修复里 param 替换捕获组后的值。
func ruleMatch(t *testing.T, id, line, param string) (bool, string) {
	t.Helper()
	m, err := sdk.LoadManifest("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Contributes.Diagnostics {
		if r.ID != id {
			continue
		}
		if !strings.Contains(line, r.Hint) {
			return false, ""
		}
		g := regexp.MustCompile(r.Pattern).FindStringSubmatch(line)
		if g == nil {
			return false, ""
		}
		v := ""
		if param != "" && len(r.Fixes) > 0 {
			v = r.Fixes[0].Params[param]
			for i := len(g) - 1; i >= 1; i-- {
				v = strings.ReplaceAll(v, "{"+string(rune('0'+i))+"}", g[i])
			}
		}
		return true, v
	}
	t.Fatalf("没有规则 %s", id)
	return false, ""
}
