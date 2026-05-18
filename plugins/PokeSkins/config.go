package pokeskins

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all plugin settings.
type Config struct {
	Commands struct {
		Enabled bool     `yaml:"enabled"`
		Aliases []string `yaml:"aliases"`
	} `yaml:"commands"`

	StorageTTL int `yaml:"storageTTL"` // days

	DefaultSkin     string `yaml:"defaultSkin"`     // "steve" or "alex"
	ForceCustomSkin bool   `yaml:"forceCustomSkin"` // if true, always override even if player has textures
	LogLevel        int    `yaml:"logLevel"`        // 0=debug,1=info,2=warn

	MineSkin struct {
		Enabled       bool          `yaml:"enabled"`
		APIKey        string        `yaml:"apiKey"`
		UploadTimeout time.Duration `yaml:"uploadTimeout"`
	} `yaml:"mineskin"`

	Messages map[string]string `yaml:"messages"`
}

// Default messages (English). Users can override in config.yml.
var defaultMessages = map[string]string{
	"prefix":                   "&7[&6PokeSkins&7]&r ",
	"skin_set_success":         "&aSkin set successfully! Use /pokeskin info to see.",
	"skin_set_fail":            "&cFailed to set skin: %s",
	"skin_set_username_fetching": "&eFetching premium skin for %s...",
	"skin_set_url_fetching":    "&eUploading skin from URL...",
	"skin_reset_success":       "&aYour custom skin has been removed. Your original skin will be used.",
	"skin_not_found":           "&cNo premium profile found for that username.",
	"skin_invalid_url":         "&cInvalid image URL. Must be a direct image link (png/jpg).",
	"skin_api_error":           "&cSkin service error. Please try again later.",
	"skin_info_title":          "&6Your current skin:",
	"skin_info_custom":         "&7- Custom skin (%s: %s)",
	"skin_info_premium":        "&7- Premium/Mojang skin",
	"skin_info_default":        "&7- Default skin (Steve/Alex)",
	"skin_no_permission":       "&cYou don't have permission to use this command.",
	"reload_success":           "&aConfiguration reloaded successfully.",
	"unknown_command":          "&cUnknown subcommand. Use /pokeskin help",
	"help_header":              "&6PokeSkins Commands:",
	"help_set":                 "&e/pokeskin set <username> &7- Set premium skin",
	"help_set_url":             "&e/pokeskin set url <url> &7- Set custom skin from URL",
	"help_reset":               "&e/pokeskin reset &7- Remove custom skin",
	"help_info":                "&e/pokeskin info &7- Show current skin info",
	"help_reload":              "&e/pokeskin reload &7- Reload config (admin only)",
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	cfg := &Config{
		StorageTTL:      30,
		DefaultSkin:     "steve",
		ForceCustomSkin: false,
		LogLevel:        1,
		Messages:        make(map[string]string),
	}
	cfg.Commands.Enabled = true
	cfg.Commands.Aliases = []string{"pokeskins", "pskins"}
	cfg.MineSkin.Enabled = false
	cfg.MineSkin.APIKey = ""
	cfg.MineSkin.UploadTimeout = 10 * time.Second

	// copy default messages
	for k, v := range defaultMessages {
		cfg.Messages[k] = v
	}
	return cfg
}

// LoadConfig reads the YAML file at path. If file does not exist, it writes
// the default config and returns it. Returns an error if parsing fails.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// create default config file
			cfg := DefaultConfig()
			if err := saveConfig(cfg, path); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, err
	}
	cfg := DefaultConfig() // start with defaults, then override
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// ensure messages map has all default keys (in case user omitted some)
	for k, v := range defaultMessages {
		if _, ok := cfg.Messages[k]; !ok {
			cfg.Messages[k] = v
		}
	}
	return cfg, nil
}

// ReloadConfig reloads the config from disk and returns a new Config.
// Useful for /pokeskin reload command.
func ReloadConfig(path string) (*Config, error) {
	return LoadConfig(path)
}

// saveConfig writes the config to the given path (used for initial creation).
func saveConfig(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}