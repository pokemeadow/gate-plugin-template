package pokeskins

import (
	"fmt"

	"github.com/go-logr/logr"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// legacyText converts a string with legacy '&' color codes into a component.
func legacyText(s string) component.Component {
	leg := legacy.Legacy{
		Char:    legacy.AmpersandChar, // Use '&' -> '§' conversion.
		HexChar: legacy.HexChar,       // Handle hex colors like "#FF5555".
	}
	comp, err := leg.Unmarshal([]byte(s))
	if err != nil {
		return &component.Text{Content: s}
	}
	return comp
}

func registerCommands(p *proxy.Proxy, log logr.Logger, cfg *Config, storage *SkinStorage, fetcher *skinFetcher, mineskin *MineSkin) {
	handler := &commandHandler{
		log:      log,
		cfg:      cfg,
		storage:  storage,
		fetcher:  fetcher,
		mineskin: mineskin,
		proxy:    p,
	}

	// Root command: /pokeskin
	root := brigodier.Literal("pokeskin").
		Executes(command.Command(handler.handleRootHelp()))

	// Combined Subcommand: set
	// This solves the Brigadier Node Collision by branching both URL and Premium under a single "set" literal.
	setNode := brigodier.Literal("set").
		Then(brigodier.Literal("url").
			Then(brigodier.Argument("url", brigodier.String).
				Executes(command.Command(handler.handleSetURL())))).
		Then(brigodier.Argument("username", brigodier.String).
			Executes(command.Command(handler.handleSetPremium())))

	// Subcommand: reset
	reset := brigodier.Literal("reset").
		Executes(command.Command(handler.handleReset()))

	// Subcommand: info
	info := brigodier.Literal("info").
		Executes(command.Command(handler.handleInfo()))

	// Subcommand: reload
	reload := brigodier.Literal("reload").
		Executes(command.Command(handler.handleReload()))

	// Assemble command tree safely
	cmd := root.
		Then(setNode).
		Then(reset).
		Then(info).
		Then(reload)

	// Register with aliases (e.g., pokeskins, pskins)
	p.Command().RegisterWithAliases(cmd, cfg.Commands.Aliases...)
}

type commandHandler struct {
	log      logr.Logger
	cfg      *Config
	storage  *SkinStorage
	fetcher  *skinFetcher
	mineskin *MineSkin
	proxy    *proxy.Proxy
}

// handleRootHelp shows the help menu when only /pokeskin is typed.
func (h *commandHandler) handleRootHelp() func(*command.Context) error {
	return func(c *command.Context) error {
		h.sendHelp(c)
		return nil
	}
}

// handleSetPremium processes "/pokeskin set <username>"
func (h *commandHandler) handleSetPremium() func(*command.Context) error {
	return func(c *command.Context) error {
		player, ok := c.Source.(proxy.Player)
		if !ok {
			c.Source.SendMessage(legacyText("Only players can use this command."))
			return nil
		}
		if !player.HasPermission("pokeskins.command.set") {
			c.Source.SendMessage(legacyText(h.msg("skin_no_permission")))
			return nil
		}
		username := c.String("username")
		if username == "" {
			h.sendHelp(c)
			return nil
		}
		c.Source.SendMessage(legacyText(h.msgf("skin_set_username_fetching", username)))
		uid, err := h.fetcher.resolveUUID(username)
		if err != nil || uid == uuid.Nil {
			c.Source.SendMessage(legacyText(h.msgf("skin_set_fail", h.msg("skin_not_found"))))
			return nil
		}
		textures, err := h.fetcher.texturesForUUID(uid)
		if err != nil {
			c.Source.SendMessage(legacyText(h.msgf("skin_set_fail", err.Error())))
			return nil
		}
		if len(textures) == 0 {
			c.Source.SendMessage(legacyText(h.msgf("skin_set_fail", h.msg("skin_not_found"))))
			return nil
		}
		pref := SkinPreference{
			Type:   "premium",
			Target: username,
		}
		if err := h.storage.Set(player.ID().Undashed(), pref); err != nil {
			c.Source.SendMessage(legacyText(h.msgf("skin_set_fail", err.Error())))
			return nil
		}
		c.Source.SendMessage(legacyText(h.msg("skin_set_success")))
		return nil
	}
}

// handleSetURL processes "/pokeskin set url <url>"
func (h *commandHandler) handleSetURL() func(*command.Context) error {
	return func(c *command.Context) error {
		player, ok := c.Source.(proxy.Player)
		if !ok {
			c.Source.SendMessage(legacyText("Only players can use this command."))
			return nil
		}
		if !player.HasPermission("pokeskins.command.set") {
			c.Source.SendMessage(legacyText(h.msg("skin_no_permission")))
			return nil
		}
		if h.mineskin == nil {
			c.Source.SendMessage(legacyText(h.msg("skin_api_error")))
			return nil
		}
		urlStr := c.String("url")
		if urlStr == "" {
			h.sendHelp(c)
			return nil
		}
		c.Source.SendMessage(legacyText(h.msg("skin_set_url_fetching")))
		_, err := h.mineskin.UploadSkinFromURL(urlStr)
		if err != nil {
			c.Source.SendMessage(legacyText(h.msgf("skin_set_fail", err.Error())))
			return nil
		}
		pref := SkinPreference{
			Type:   "url",
			Target: urlStr,
		}
		if err := h.storage.Set(player.ID().Undashed(), pref); err != nil {
			c.Source.SendMessage(legacyText(h.msgf("skin_set_fail", err.Error())))
			return nil
		}
		c.Source.SendMessage(legacyText(h.msg("skin_set_success")))
		return nil
	}
}

// handleReset processes "/pokeskin reset"
func (h *commandHandler) handleReset() func(*command.Context) error {
	return func(c *command.Context) error {
		player, ok := c.Source.(proxy.Player)
		if !ok {
			c.Source.SendMessage(legacyText("Only players can use this command."))
			return nil
		}
		if !player.HasPermission("pokeskins.command.reset") {
			c.Source.SendMessage(legacyText(h.msg("skin_no_permission")))
			return nil
		}
		if err := h.storage.Delete(player.ID().Undashed()); err != nil {
			c.Source.SendMessage(legacyText(h.msgf("skin_set_fail", err.Error())))
			return nil
		}
		c.Source.SendMessage(legacyText(h.msg("skin_reset_success")))
		return nil
	}
}

// handleInfo processes "/pokeskin info"
func (h *commandHandler) handleInfo() func(*command.Context) error {
	return func(c *command.Context) error {
		player, ok := c.Source.(proxy.Player)
		if !ok {
			c.Source.SendMessage(legacyText("Only players can use this command."))
			return nil
		}
		if !player.HasPermission("pokeskins.command.info") {
			c.Source.SendMessage(legacyText(h.msg("skin_no_permission")))
			return nil
		}
		pref, exists := h.storage.Get(player.ID().Undashed())
		c.Source.SendMessage(legacyText(h.msg("skin_info_title")))
		if !exists {
			c.Source.SendMessage(legacyText(h.msg("skin_info_default")))
		} else if pref.Type == "premium" {
			c.Source.SendMessage(legacyText(h.msgf("skin_info_custom", "premium", pref.Target)))
		} else if pref.Type == "url" {
			c.Source.SendMessage(legacyText(h.msgf("skin_info_custom", "URL", pref.Target)))
		} else {
			c.Source.SendMessage(legacyText(h.msg("skin_info_default")))
		}
		return nil
	}
}

// handleReload processes "/pokeskin reload"
func (h *commandHandler) handleReload() func(*command.Context) error {
	return func(c *command.Context) error {
		if !c.Source.HasPermission("pokeskins.admin") {
			c.Source.SendMessage(legacyText(h.msg("skin_no_permission")))
			return nil
		}
		newCfg, err := ReloadConfig("plugins/PokeSkins/config.yml")
		if err != nil {
			c.Source.SendMessage(legacyText(fmt.Sprintf("&cFailed to reload config: %v", err)))
			return nil
		}
		h.cfg = newCfg
		c.Source.SendMessage(legacyText(h.msg("reload_success")))
		return nil
	}
}

// sendHelp displays the help menu.
func (h *commandHandler) sendHelp(c *command.Context) {
	c.Source.SendMessage(legacyText(h.msg("help_header")))
	c.Source.SendMessage(legacyText(h.msg("help_set")))
	if h.mineskin != nil && h.cfg.MineSkin.Enabled {
		c.Source.SendMessage(legacyText(h.msg("help_set_url")))
	}
	c.Source.SendMessage(legacyText(h.msg("help_reset")))
	c.Source.SendMessage(legacyText(h.msg("help_info")))
	if c.Source.HasPermission("pokeskins.admin") {
		c.Source.SendMessage(legacyText(h.msg("help_reload")))
	}
}

// msg returns a raw string message from config (still with & codes).
func (h *commandHandler) msg(key string) string {
	prefix := h.cfg.Messages["prefix"]
	msg := h.cfg.Messages[key]
	if msg == "" {
		msg = key
	}
	return prefix + msg
}

// msgf returns a formatted raw string.
func (h *commandHandler) msgf(key string, args ...interface{}) string {
	return fmt.Sprintf(h.msg(key), args...)
}