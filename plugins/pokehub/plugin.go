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

// ADDED: default config content written automatically when the file does not exist.
const defaultConfig = `target-server: "lobby"

messages:
  player-only: "&cOnly players can use this command."
  target-missing: "&cThe configured server &e%server%&c is not available."
  already-there: "&eYou are already on &b%server%&e."
  transferring: "&aSending you to &e%server%&a..."
  transferred: "&aSuccessfully moved to &e%server%&a."
  failed: "&cFailed to connect to &e%server%&c."
`

// ADDED: returns all possible config locations we want to support.
func configCandidates() []string {
	candidates := []string{
		filepath.Join("plugins", "pokehub", "config.yml"),
	}

	// ADDED: also try next to the executable, because some hosts run the binary from another cwd.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "plugins", "pokehub", "config.yml"))
	}

	return candidates
}

// ADDED: create folder + config automatically if missing, then load the first usable config.
func LoadConfig() (*Config, error) {
	candidates := configCandidates()

	var lastErr error

	for _, configPath := range candidates {
		dir := filepath.Dir(configPath)

		// ADDED: create the plugin folder automatically.
		if err := os.MkdirAll(dir, 0755); err != nil {
			lastErr = fmt.Errorf("failed to create config directory %q: %w", dir, err)
			continue
		}

		// ADDED: create default config if missing.
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
				lastErr = fmt.Errorf("failed to create default config %q: %w", configPath, err)
				continue
			}
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			lastErr = fmt.Errorf("failed to read config %q: %w", configPath, err)
			continue
		}

		cfg := &Config{}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			lastErr = fmt.Errorf("failed to parse config %q: %w", configPath, err)
			continue
		}

		cfg.TargetServer = strings.TrimSpace(cfg.TargetServer)
		return cfg, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("unable to locate a writable config path")
	}
	return nil, lastErr
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
								strings.ReplaceAll(cfg.Messages.TargetMissing, "%server%", cfg.TargetServer),
							),
						})
					}

					// ADDED: do nothing if the player is already on the target server.
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