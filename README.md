# Lemon Paperplane Vue.js 插件

依赖 nodejs 插件，在 Node 模块上识别 Vue：

- facet `vue`（Nuxt 工程另有 `nuxt`），关键信息如「Vue 3.5 · Vite 6.4」「Vue 2.6 · Vue CLI 4.5」。
- Vue CLI / Nuxt 的开发脚本作为默认脚本偏好（经 facet Data 的 `preferScripts` 交给 nodejs）。
- Vue CLI 4（webpack 4）、Nuxt 2 在 Node 17+ 上会报 0308010C：分析时就给出问题和两个修复（启用 OpenSSL 旧版兼容 / 使用 Node 16），不用等运行报错。
- uni-app 工程只显示 Vue 版本，脚本与兼容提示交给 uniapp 插件。

```sh
./build.sh
go test ./...
```
