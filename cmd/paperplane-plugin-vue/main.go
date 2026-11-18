// paperplane-plugin-vue 是 vue 插件的独立可执行入口。
package main

import (
	sdk "github.com/lemon-paperplane/lemon-paperplane-main/lemon-paperplane-main-plugin-sdk"
	vue "github.com/lemon-paperplane/lemon-paperplane-plugin-vue/plugin"
)

func main() { sdk.Serve(vue.New()) }
