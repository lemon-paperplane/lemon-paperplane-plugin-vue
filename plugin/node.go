package vue

// 与 nodejs 插件协作的公共代码：读 package.json、读 nodejs 模型、预测 Node 版本、OpenSSL 旧版兼容问题。
// 框架插件之间不共享 Go 代码（各自独立仓库），这份文件在 vue / react / uniapp 里各有一份。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sdk "github.com/lemon-paperplane/lemon-paperplane-main/lemon-paperplane-main-plugin-sdk"
)

// pkgJSON 是框架识别需要的 package.json 字段。
type pkgJSON struct {
	Name            string
	Scripts         map[string]string
	Dependencies    map[string]string
	DevDependencies map[string]string
}

func readPkg(dir string) (*pkgJSON, error) {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	p := &pkgJSON{
		Scripts:         strMap(raw["scripts"]),
		Dependencies:    strMap(raw["dependencies"]),
		DevDependencies: strMap(raw["devDependencies"]),
	}
	_ = json.Unmarshal(raw["name"], &p.Name)
	return p, nil
}

func strMap(r json.RawMessage) map[string]string {
	var m map[string]json.RawMessage
	if len(r) == 0 || json.Unmarshal(r, &m) != nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range m {
		var s string
		if json.Unmarshal(v, &s) == nil {
			out[k] = s
		}
	}
	return out
}

func (p *pkgJSON) has(name string) bool {
	if p == nil {
		return false
	}
	_, a := p.Dependencies[name]
	_, b := p.DevDependencies[name]
	return a || b
}

func (p *pkgJSON) rangeOf(name string) string {
	if v, ok := p.Dependencies[name]; ok {
		return v
	}
	return p.DevDependencies[name]
}

func (p *pkgJSON) hasPrefix(prefix string) bool {
	for _, m := range []map[string]string{p.Dependencies, p.DevDependencies} {
		for k := range m {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
	}
	return false
}

// nodeModel 是 nodejs 插件模型里本插件用到的部分。
type nodeModel struct {
	Versions map[string]string `json:"versions"`
	Node     struct {
		Constraint string `json:"constraint"`
		Preferred  string `json:"preferred"`
		Legacy     bool   `json:"legacy"`
	} `json:"node"`
	LegacyPackage  string `json:"legacyPackage"`
	DefaultScript  string `json:"defaultScript"`
	PackageManager struct {
		Name string `json:"name"`
	} `json:"packageManager"`
}

func loadNodeModel(m *sdk.ModuleInfo) *nodeModel {
	var nm nodeModel
	if m.Model("nodejs", &nm) {
		return &nm
	}
	return &nodeModel{}
}

// depVersion 返回依赖的实际版本：nodejs 模型 → node_modules → 声明范围的下限。
func depVersion(dir string, pkg *pkgJSON, nm *nodeModel, name string) string {
	if nm != nil && nm.Versions[name] != "" {
		return nm.Versions[name]
	}
	if v := installedVersion(dir, name); v != "" {
		return v
	}
	if pkg != nil {
		return cleanRange(pkg.rangeOf(name))
	}
	return ""
}

func installedVersion(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, "node_modules", filepath.FromSlash(name), "package.json"))
	if err != nil {
		return ""
	}
	var v struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(b, &v)
	return v.Version
}

// cleanRange："^6.0.6" → "6.0.6"，"npm:vite@^5" → "5"，取不出返回空。
func cleanRange(r string) string {
	r = strings.TrimSpace(r)
	if i := strings.LastIndex(r, "@"); strings.HasPrefix(r, "npm:") && i > 0 {
		r = r[i+1:]
	}
	for _, bad := range []string{"workspace:", "file:", "link:", "git", "http:", "https:", "github:"} {
		if strings.HasPrefix(r, bad) {
			return ""
		}
	}
	if i := strings.Index(r, "||"); i >= 0 {
		r = r[:i]
	}
	if f := strings.Fields(r); len(f) > 0 {
		r = f[0]
	} else {
		return ""
	}
	r = strings.TrimLeft(r, "^~=>v ")
	for strings.HasSuffix(r, ".x") || strings.HasSuffix(r, ".*") {
		r = r[:len(r)-2]
	}
	if r == "*" || r == "x" || r == "latest" {
		return ""
	}
	return r
}

// majorMinor："3.5.13" → "3.5"；预发布或只有主版本时只给主版本。
func majorMinor(v string) string {
	pv, ok := sdk.ParseVersion(v)
	if !ok {
		return ""
	}
	if pv.N >= 2 {
		return fmt.Sprintf("%d.%d", pv.Parts[0], pv.Parts[1])
	}
	return fmt.Sprint(pv.Parts[0])
}

func major(v string) int {
	pv, _ := sdk.ParseVersion(v)
	return pv.Major()
}

// predictNode 预测内核会给模块选的 Node：已选定的优先；否则在已知工具链里按 nodejs 的需求选（偏好 → 满足约束的最高）。
func predictNode(ctx *sdk.Context, p *sdk.AnalyzeParams, nm *nodeModel) (sdk.Version, bool) {
	if tc, ok := p.Toolchains["node"]; ok {
		return sdk.ParseVersion(tc.Version)
	}
	var tcs []sdk.Toolchain
	if ctx.Host != nil {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		tcs, _ = ctx.Host.Toolchains(c, "node")
		cancel()
	}
	var vs []string
	for _, t := range tcs {
		vs = append(vs, t.Version)
	}
	if i := sdk.BestVersion(vs, nm.Node.Constraint, nm.Node.Preferred); i >= 0 {
		return sdk.ParseVersion(vs[i])
	}
	if nm.Node.Preferred != "" {
		return sdk.ParseVersion(nm.Node.Preferred)
	}
	return sdk.Version{}, false
}

// legacyProblem 生成「Node 17+ 需要 OpenSSL 旧版兼容」的问题与修复（不等运行报错）。
func legacyProblem(ctx *sdk.Context, what, nodeVersion string) sdk.Diagnosis {
	return sdk.Diagnosis{
		RuleID:   "openssl-legacy.static",
		Severity: "warning",
		Title:    ctx.T("problem.legacy.title"),
		Message:  ctx.T("problem.legacy.message", what, nodeVersion),
		Fixes: []sdk.Fix{
			{ID: "legacy", Title: ctx.T("fix.opensslLegacy"), Kind: sdk.FixAppendEnv, Params: map[string]string{"key": "NODE_OPTIONS", "value": "--openssl-legacy-provider", "scope": "project"}, Recommended: true},
			{ID: "node16", Title: ctx.T("fix.useNode16"), Kind: sdk.FixSetToolchain, Params: map[string]string{"kind": "node", "constraint": "16"}},
		},
	}
}

// legacyEnabled 判断工程环境（或系统环境）的 NODE_OPTIONS 里是否已开启 OpenSSL 旧版兼容。
func legacyEnabled(env map[string]string) bool {
	v, ok := env["NODE_OPTIONS"]
	if !ok {
		v = os.Getenv("NODE_OPTIONS")
	}
	return strings.Contains(v, "--openssl-legacy-provider")
}

// legacyNeeded 判断模块会不会在 Node 17+ 上遇到 0308010C：nodejs 推断需要旧版兼容、工程还没开启旧版兼容、
// 且预测的 Node ≥ 17（用户选了 Node 16 时内核在 Toolchains 里传来 16，不会提示）。
func legacyNeeded(ctx *sdk.Context, p *sdk.AnalyzeParams, nm *nodeModel) (string, bool) {
	if !nm.Node.Legacy {
		return "", false
	}
	if legacyEnabled(p.Env) {
		return "", false
	}
	v, ok := predictNode(ctx, p, nm)
	if !ok || v.Major() < 17 {
		return "", false
	}
	return v.String(), true
}

// scriptsMatching 返回命令匹配 pred 的脚本名（按名字排序，dev/serve/start 优先）。
func scriptsMatching(pkg *pkgJSON, pred func(cmd string) bool) []string {
	var out []string
	for _, name := range []string{"dev", "serve", "start"} {
		if cmd, ok := pkg.Scripts[name]; ok && pred(cmd) {
			out = append(out, name)
		}
	}
	var rest []string
	for name, cmd := range pkg.Scripts {
		if name != "dev" && name != "serve" && name != "start" && pred(cmd) {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}
