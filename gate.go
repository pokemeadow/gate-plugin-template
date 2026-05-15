package main

import (
	"github.com/minekube/gate-plugin-template/plugins/cloversecurity"
	"go.minekube.com/gate/cmd/gate"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func main() {
	proxy.Plugins = append(proxy.Plugins,
		cloversecurity.Plugin,
	)
	gate.Execute()
}