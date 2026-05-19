package pokeperms

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// PluginName and Version metadata variables
const (
	PluginName    = "PokePermsGate"
	PluginVersion = "1.0.0"
)

// PokePermsGate represents the main core instance structure of the plugin
type PokePermsGate struct {
	proxy    *proxy.Proxy
	config   *Config
	storage  *Storage
	msgMgr   *MessagingManager
	token    string
}

// NewPlugin is the standard registration factory hook that Gate looks for
func NewPlugin(p *proxy.Proxy) *PokePermsGate {
	return &PokePermsGate{
		proxy: p,
	}
}

// Init runs automatically when the Gate Proxy boots up and initializes the plugin lifecycle
func (p *PokePermsGate) Init(ctx context.Context) error {
	// Define the exact administrative plugin data directory paths
	dir := fmt.Sprintf("plugins/%s", PluginName)

	// 1. Initialize Configuration (Auto folder generation is built-in)
	cfg, err := LoadConfig(dir)
	if err != nil {
		return fmt.Errorf("[%s] Failed to load config.yml: %w", PluginName, err)
	}
	p.config = cfg

	// 2. Load or Auto-Generate Security Pre-Shared Key (Token)
	token, err := LoadOrGenerateToken(dir)
	if err != nil {
		return fmt.Errorf("[%s] Failed to load/generate token.yml: %w", PluginName, err)
	}
	p.token = token

	// 3. Establish Database Connection Cache Engine (MySQL / SQLite Wrapper Router)
	storage, err := NewStorage(dir, cfg)
	if err != nil {
		return fmt.Errorf("[%s] Failed to initialize database driver storage: %w", PluginName, err)
	}
	p.storage = storage

	// 4. Fire Up Networking Channel Messaging Manager
	p.msgMgr = NewMessagingManager(p.proxy, p.token)

	// 5. Build and Inject Brigadier Dynamic Tree Node Commands
	RegisterCommands(p.proxy, p.storage, p.config)

	// Print successful operational logs to the master proxy console terminal
	fmt.Printf("[%s] Successfully loaded version %s! Database Driver Connected: %s\n",
		PluginName, PluginVersion, cfg.StorageType)
	fmt.Printf("[%s] Security Verification Token initialized. Ready to sync with PokePerms Paper!\n", PluginName)

	return nil
}

// Disable handles graceful shutdown when proxy stops or reloads
func (p *PokePermsGate) Disable() {
	if p.storage != nil {
		p.storage.Close()
		fmt.Printf("[%s] Database connections successfully closed and safely locked.\n", PluginName)
	}
	fmt.Printf("[%s] Plugin successfully disabled.\n", PluginName)
}