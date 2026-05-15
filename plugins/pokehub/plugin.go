package pokehub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"gopkg.in/yaml.v3"
)

type Config struct {
	TargetServer string `yaml:"target-server"`
	Messages     struct {
		PlayerOnly    string `yaml:"player-only"`
		TargetMissing string `yaml:"target-missing"`
		AlreadyThere  string `yaml:"already-there"`
		Transferring  string `yaml:"transferring"`
		Transferred   string `yaml:"transferred"`
		Failed        string `yaml:"failed"`
	} `yaml:"messages"`
}

// ADDED: default config content that will be auto-created.
const defaultConfig = `target-server: "lobby"

messages:
  player-only: "&cOnly players can use this command."
  target-missing: "&cThe configured server &e%server%&c is not available."
  already-there: "&eYou are already on &b%server%&e."
  transferring: "&aSending you to &e%server%&a..."
  transferred: "&aSuccessfully moved to &e%server%&a."
  failed: "&cFailed to connect to &e%server%&c."
`

// ADDED: automatically creates plugin folder + config.yml if missing.
func LoadConfig() (*Config, error) {
	pluginDir := "plugins/pokehub"
	configPath := filepath.Join(pluginDir, "config.yml")

	// ADDED: auto-create plugin directory.
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	// ADDED: auto-create config.yml if it doesn't exist.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	cfg.TargetServer = strings.TrimSpace(cfg.TargetServer)

	return cfg, nil
}

var Plugin = proxy.Plugin{
	Name: "PokeHub",

	Init: func(ctx context.Context, prx *proxy.Proxy) error {
		cfg, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		log := logr.FromContextOrDiscard(ctx).WithName("PokeHub")

		registerHubCommand := func(name string) {
			prx.Command().Register(
				brigodier.Literal(name).Executes(command.Command(func(c *command.Context) error {

					player, ok := c.Source.(proxy.Player)
					if !ok {
						return c.Source.SendMessage(&component.Text{
							Content: replaceColor(cfg.Messages.PlayerOnly),
						})
					}

					if cfg.TargetServer == "" {
						return player.SendMessage(&component.Text{
							Content: "§cTarget server is not configured.",
						})
					}

					target := prx.Server(cfg.TargetServer)
					if target == nil {
						return player.SendMessage(&component.Text{
							Content: replaceColor(
								strings.ReplaceAll(
									cfg.Messages.TargetMissing,
									"%server%",
									cfg.TargetServer,
								),
							),
						})
					}

					// ADDED: prevent reconnecting to same server.
					if player.CurrentServer() != nil &&
						player.CurrentServer().Server() != nil &&
						player.CurrentServer().Server().ServerInfo().Name() == target.ServerInfo().Name() {

						return player.SendMessage(&component.Text{
							Content: replaceColor(
								strings.ReplaceAll(
									cfg.Messages.AlreadyThere,
									"%server%",
									cfg.TargetServer,
								),
							),
						})
					}

					_ = player.SendMessage(&component.Text{
						Content: replaceColor(
							strings.ReplaceAll(
								cfg.Messages.Transferring,
								"%server%",
								cfg.TargetServer,
							),
						),
					})

					// ADDED: switch player to configured server.
					ok = player.CreateConnectionRequest(target).
						ConnectWithIndication(player.Context())

					if ok {
						_ = player.SendMessage(&component.Text{
							Content: replaceColor(
								strings.ReplaceAll(
									cfg.Messages.Transferred,
									"%server%",
									cfg.TargetServer,
								),
							),
						})
						return nil
					}

					return player.SendMessage(&component.Text{
						Content: replaceColor(
							strings.ReplaceAll(
								cfg.Messages.Failed,
								"%server%",
								cfg.TargetServer,
							),
						),
					})
				})),
			)
		}

		// ADDED: register both commands.
		registerHubCommand("hub")
		registerHubCommand("lobby")

		log.Info("PokeHub loaded", "target-server", cfg.TargetServer)

		return nil
	},
}

func replaceColor(s string) string {
	return strings.ReplaceAll(s, "&", "§")
}