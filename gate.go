package main

import (
	"github.com/minekube/gate-plugin-template/plugins/cloversecurity"
	"github.com/minekube/gate-plugin-template/plugins/pokehub"
	"github.com/minekube/gate-plugin-template/plugins/pokeskins"

	"go.minekube.com/gate/cmd/gate"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func main() {
	proxy.Plugins = append(proxy.Plugins,
		cloversecurity.Plugin,
		pokehub.Plugin,
		pokeskins.Plugin,
	)

	gate.Execute()
}