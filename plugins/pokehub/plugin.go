package pokehub

import (
	"context"
	"fmt"
	"os"
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

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
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
		cfg, err := LoadConfig("plugins/pokehub/config.yml")
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// ADDED: Gate logger from context.
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
								strings.ReplaceAll(cfg.Messages.TargetMissing, "%server%", cfg.TargetServer),
							),
						})
					}

					if player.CurrentServer() != nil &&
						player.CurrentServer().Server() != nil &&
						player.CurrentServer().Server().ServerInfo().Name() == target.ServerInfo().Name() {
						return player.SendMessage(&component.Text{
							Content: replaceColor(
								strings.ReplaceAll(cfg.Messages.AlreadyThere, "%server%", cfg.TargetServer),
							),
						})
					}

					_ = player.SendMessage(&component.Text{
						Content: replaceColor(
							strings.ReplaceAll(cfg.Messages.Transferring, "%server%", cfg.TargetServer),
						),
					})

					ok = player.CreateConnectionRequest(target).ConnectWithIndication(player.Context())
					if ok {
						_ = player.SendMessage(&component.Text{
							Content: replaceColor(
								strings.ReplaceAll(cfg.Messages.Transferred, "%server%", cfg.TargetServer),
							),
						})
						return nil
					}

					return player.SendMessage(&component.Text{
						Content: replaceColor(
							strings.ReplaceAll(cfg.Messages.Failed, "%server%", cfg.TargetServer),
						),
					})
				})),
			)
		}

		registerHubCommand("hub")
		registerHubCommand("lobby")

		log.Info("PokeHub loaded", "target-server", cfg.TargetServer)
		return nil
	},
}

func replaceColor(s string) string {
	return strings.ReplaceAll(s, "&", "§")
}