package pokeskins

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// commandHandler holds dependencies for commands.
type commandHandler struct {
	log      logr.Logger
	cfg      *Config
	storage  *SkinStorage
	fetcher  *skinFetcher
	mineskin *MineSkin
}

// newCommands creates and registers all commands.
func newCommands(log logr.Logger, cfg *Config, storage *SkinStorage, fetcher *skinFetcher, mineskin *MineSkin) *commandHandler {
	return &commandHandler{
		log:      log,
		cfg:      cfg,
		storage:  storage,
		fetcher:  fetcher,
		mineskin: mineskin,
	}
}

// register registers the root command and its subcommands.
func (h *commandHandler) register(reg command.Registry) {
	if !h.cfg.Commands.Enabled {
		return
	}
	root := command.RootCommand("pokeskin", command.Description("Manage your skin"))
	// Add aliases from config
	for _, alias := range h.cfg.Commands.Aliases {
		root = root.Alias(alias)
	}

	// Subcommands
	root.Subcommand(command.SubCommand("set", command.Description("Set a premium or URL skin").
		Arg("type", command.ArgTypeString). // "premium" or "url"? Actually we parse as username or "url" keyword
		Arg("target", command.ArgTypeString).
		Handler(h.handleSet)))

	// For easier usage: /pokeskin set <username>  (type inferred)
	// We'll also accept explicit "set url <url>"
	// We'll parse in handler.

	root.Subcommand(command.SubCommand("reset", command.Description("Remove your custom skin").Handler(h.handleReset)))
	root.Subcommand(command.SubCommand("clear", command.Description("Alias for reset").Handler(h.handleReset)))
	root.Subcommand(command.SubCommand("info", command.Description("Show current skin info").Handler(h.handleInfo)))
	root.Subcommand(command.SubCommand("reload", command.Description("Reload config").Handler(h.handleReload).
		Permission("pokeskins.admin")))

	// Default help
	root.Subcommand(command.SubCommand("help", command.Description("Show help").Handler(h.handleHelp)))

	reg.Register(root)
}

// handleSet processes /pokeskin set <username> or /pokeskin set url <url>
func (h *commandHandler) handleSet(c *command.Context) error {
	player, ok := c.Sender().(proxy.Player)
	if !ok {
		return c.SendMessage("Only players can use this command.")
	}
	args := c.Args()
	if len(args) < 1 {
		return h.sendHelp(c)
	}
	// Check if first arg is "url"
	if strings.EqualFold(args[0], "url") {
		if len(args) < 2 {
			return c.SendMessage(h.msg("skin_invalid_url"))
		}
		return h.setURLSkin(c, player, args[1])
	}
	// Otherwise treat as premium username
	username := args[0]
	return h.setPremiumSkin(c, player, username)
}

func (h *commandHandler) setPremiumSkin(c *command.Context, player proxy.Player, username string) error {
	if !c.Sender().HasPermission("pokeskins.command.set") {
		return c.SendMessage(h.msg("skin_no_permission"))
	}
	c.SendMessage(h.msgf("skin_set_username_fetching", username))
	// Resolve UUID from username (Mojang + fallback)
	uid, err := h.fetcher.resolveUUID(username)
	if err != nil || uid == uuid.Nil {
		c.SendMessage(h.msgf("skin_set_fail", h.msg("skin_not_found")))
		return nil
	}
	// Fetch textures for that UUID
	textures, err := h.fetcher.texturesForUUID(uid)
	if err != nil {
		c.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	if len(textures) == 0 {
		c.SendMessage(h.msgf("skin_set_fail", h.msg("skin_not_found")))
		return nil
	}
	// Store preference
	pref := SkinPreference{
		Type:   "premium",
		Target: username,
	}
	if err := h.storage.Set(player.ID().Undashed(), pref); err != nil {
		c.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	c.SendMessage(h.msg("skin_set_success"))
	// Optionally force immediate update? The skin will apply on next join/event.
	// For now, just notify.
	return nil
}

func (h *commandHandler) setURLSkin(c *command.Context, player proxy.Player, urlStr string) error {
	if !c.Sender().HasPermission("pokeskins.command.set") {
		return c.SendMessage(h.msg("skin_no_permission"))
	}
	if h.mineskin == nil {
		return c.SendMessage(h.msg("skin_api_error"))
	}
	c.SendMessage(h.msg("skin_set_url_fetching"))
	prop, err := h.mineskin.UploadSkinFromURL(urlStr)
	if err != nil {
		c.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	// Store preference (URL skin)
	pref := SkinPreference{
		Type:   "url",
		Target: urlStr,
	}
	if err := h.storage.Set(player.ID().Undashed(), pref); err != nil {
		c.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	// Optionally cache the texture? Not needed because we re-upload each join; but we could store texture in storage?
	// For now, store preference only; on each join we will upload again (cached via MineSkin's own CDN? Not ideal).
	// Better: store the texture property as well? But we avoided bloat. So we accept re-upload on join.
	c.SendMessage(h.msg("skin_set_success"))
	return nil
}

func (h *commandHandler) handleReset(c *command.Context) error {
	player, ok := c.Sender().(proxy.Player)
	if !ok {
		return c.SendMessage("Only players can use this command.")
	}
	if !c.Sender().HasPermission("pokeskins.command.reset") {
		return c.SendMessage(h.msg("skin_no_permission"))
	}
	if err := h.storage.Delete(player.ID().Undashed()); err != nil {
		return c.SendMessage(h.msgf("skin_set_fail", err.Error()))
	}
	c.SendMessage(h.msg("skin_reset_success"))
	return nil
}

func (h *commandHandler) handleInfo(c *command.Context) error {
	player, ok := c.Sender().(proxy.Player)
	if !ok {
		return c.SendMessage("Only players can use this command.")
	}
	pref, exists := h.storage.Get(player.ID().Undashed())
	c.SendMessage(h.msg("skin_info_title"))
	if !exists {
		c.SendMessage(h.msg("skin_info_default"))
	} else if pref.Type == "premium" {
		c.SendMessage(h.msgf("skin_info_custom", "premium", pref.Target))
	} else if pref.Type == "url" {
		c.SendMessage(h.msgf("skin_info_custom", "URL", pref.Target))
	} else {
		c.SendMessage(h.msg("skin_info_default"))
	}
	return nil
}

func (h *commandHandler) handleReload(c *command.Context) error {
	if !c.Sender().HasPermission("pokeskins.admin") {
		return c.SendMessage(h.msg("skin_no_permission"))
	}
	// Reload config from disk
	newCfg, err := ReloadConfig("plugins/PokeSkins/config.yml")
	if err != nil {
		return c.SendMessage(fmt.Sprintf("&cFailed to reload config: %v", err))
	}
	h.cfg = newCfg
	c.SendMessage(h.msg("reload_success"))
	return nil
}

func (h *commandHandler) handleHelp(c *command.Context) error {
	return h.sendHelp(c)
}

func (h *commandHandler) sendHelp(c *command.Context) error {
	c.SendMessage(h.msg("help_header"))
	c.SendMessage(h.msg("help_set"))
	if h.mineskin != nil && h.cfg.MineSkin.Enabled {
		c.SendMessage(h.msg("help_set_url"))
	}
	c.SendMessage(h.msg("help_reset"))
	c.SendMessage(h.msg("help_info"))
	if c.Sender().HasPermission("pokeskins.admin") {
		c.SendMessage(h.msg("help_reload"))
	}
	return nil
}

// msg returns a formatted message from config messages.
func (h *commandHandler) msg(key string) string {
	prefix := h.cfg.Messages["prefix"]
	msg := h.cfg.Messages[key]
	if msg == "" {
		msg = key
	}
	return prefix + msg
}

// msgf returns formatted message with fmt.Sprintf.
func (h *commandHandler) msgf(key string, args ...interface{}) string {
	prefix := h.cfg.Messages["prefix"]
	msg := h.cfg.Messages[key]
	if msg == "" {
		msg = key
	}
	return prefix + fmt.Sprintf(msg, args...)
}