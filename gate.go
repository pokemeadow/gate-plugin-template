package main

import (
	"github.com/minekube/gate-plugin-template/plugins/cloversecurity"
	"github.com/minekube/gate-plugin-template/plugins/pokehub"

	"go.minekube.com/gate/cmd/gate"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func main() {
	proxy.Plugins = append(proxy.Plugins,
		cloversecurity.Plugin,
		pokehub.Plugin,
	)

	gate.Execute()
}