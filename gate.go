package main

import (
	"context"
	"log"
	"path/filepath"

	// Existing plugins from your template
	"github.com/minekube/gate-plugin-template/plugins/cloversecurity"
	"github.com/minekube/gate-plugin-template/plugins/pokehub"
	"github.com/minekube/gate-plugin-template/plugins/pokeskins"

	// IMPORT YOUR PACKAGE HERE: 
	// Make sure the path matches your actual module structure
	"github.com/minekube/gate-plugin-template/plugins/PokePermsGate" 

	// Core Gate Framework imports
	"go.minekube.com/gate/cmd/gate"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// PokePermsPlugin holds the internal state of our permissions ecosystem
type PokePermsPlugin struct {
	config  *PokePermsGate.Config
	storage *PokePermsGate.Storage
}

func main() {
	// 1. Initializing our PokePerms instance wrapper
	pp := &PokePermsPlugin{}

	// 2. Creating Gate Lifecycle Hook for initialization
	pokePermsHook := proxy.Plugin{
		Name: "PokePermsGate",
		Init: func(ctx context.Context, p *proxy.Proxy) error {
			log.Println("[PokePerms] Initializing Gate proxy permissions engine...")

			// Load configurations from standard directory path
			dataDir := filepath.Join("plugins", "PokePerms")
			cfg, err := PokePermsGate.LoadConfig(dataDir)
			if err != nil {
				log.Fatalf("[PokePerms] Critical error loading config.yml: %v", err)
				return err
			}
			pp.config = cfg

			// Connect to targeted Storage layer
			storage, err := PokePermsGate.NewStorage(cfg)
			if err != nil {
				log.Fatalf("[PokePerms] Critical error establishing database pipeline: %v", err)
				return err
			}
			pp.storage = storage

			// Register dynamic command structures
			PokePermsGate.RegisterCommands(p, pp.storage, pp.config)

			log.Println("[PokePerms] Gate Proxy component successfully linked and active!")
			return nil
		},
	}

	// 3. Appending our plugin (pokegate removed)
	proxy.Plugins = append(proxy.Plugins,
		cloversecurity.Plugin,
		pokehub.Plugin,
		pokeskins.Plugin,
		pokePermsHook, // Injected PokePerms core gate engine
	)

	// 4. Fire up the Minekube Gate Engine
	gate.Execute()
}