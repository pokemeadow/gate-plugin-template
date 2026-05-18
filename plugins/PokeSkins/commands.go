package pokeskins

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// registerCommands builds and registers all commands using Gate's command API.
func registerCommands(p *proxy.Proxy, log logr.Logger, cfg *Config, storage *SkinStorage, fetcher *skinFetcher, mineskin *MineSkin) {
	handler := &commandHandler{
		log:      log,
		cfg:      cfg,
		storage:  storage,
		fetcher:  fetcher,
		mineskin: mineskin,
		proxy:    p,
	}

	root := command.Literal("pokeskin").
		Executes(handler.handleHelp) // /pokeskin alone shows help

	// Aliases from config
	for _, alias := range cfg.Commands.Aliases {
		root = root.Alias(alias)
	}

	// Subcommand: set <username>
	setPremium := command.Literal("set").
		Then(
			command.Argument("username", command.String).
				Executes(handler.handleSetPremium),
		)
	root = root.Then(setPremium)

	// Subcommand: set url <url>
	setURL := command.Literal("set").
		Then(
			command.Literal("url").
				Then(
					command.Argument("url", command.String).
						Executes(handler.handleSetURL),
				),
		)
	root = root.Then(setURL)

	// Subcommand: reset
	reset := command.Literal("reset").
		Requires(handler.requirePermission("pokeskins.command.reset")).
		Executes(handler.handleReset)
	root = root.Then(reset)

	// Subcommand: clear (alias of reset)
	clear := command.Literal("clear").
		Requires(handler.requirePermission("pokeskins.command.reset")).
		Executes(handler.handleReset)
	root = root.Then(clear)

	// Subcommand: info
	info := command.Literal("info").
		Requires(handler.requirePermission("pokeskins.command.info")).
		Executes(handler.handleInfo)
	root = root.Then(info)

	// Subcommand: reload
	reload := command.Literal("reload").
		Requires(handler.requirePermission("pokeskins.admin")).
		Executes(handler.handleReload)
	root = root.Then(reload)

	// Subcommand: help
	help := command.Literal("help").
		Executes(handler.handleHelp)
	root = root.Then(help)

	p.Command().Register(root)
}

type commandHandler struct {
	log      logr.Logger
	cfg      *Config
	storage  *SkinStorage
	fetcher  *skinFetcher
	mineskin *MineSkin
	proxy    *proxy.Proxy
}

func (h *commandHandler) handleHelp(ctx *command.Context) error {
	return h.sendHelp(ctx)
}

func (h *commandHandler) handleSetPremium(ctx *command.Context) error {
	player, ok := ctx.Sender().(proxy.Player)
	if !ok {
		return ctx.SendMessage("Only players can use this command.")
	}
	if !player.HasPermission("pokeskins.command.set") {
		return ctx.SendMessage(h.msg("skin_no_permission"))
	}
	username := ctx.Arg("username").String()
	if username == "" {
		return h.sendHelp(ctx)
	}
	ctx.SendMessage(h.msgf("skin_set_username_fetching", username))
	uid, err := h.fetcher.resolveUUID(username)
	if err != nil || uid == uuid.Nil {
		ctx.SendMessage(h.msgf("skin_set_fail", h.msg("skin_not_found")))
		return nil
	}
	textures, err := h.fetcher.texturesForUUID(uid)
	if err != nil {
		ctx.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	if len(textures) == 0 {
		ctx.SendMessage(h.msgf("skin_set_fail", h.msg("skin_not_found")))
		return nil
	}
	pref := SkinPreference{
		Type:   "premium",
		Target: username,
	}
	if err := h.storage.Set(player.ID().Undashed(), pref); err != nil {
		ctx.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	ctx.SendMessage(h.msg("skin_set_success"))
	return nil
}

func (h *commandHandler) handleSetURL(ctx *command.Context) error {
	player, ok := ctx.Sender().(proxy.Player)
	if !ok {
		return ctx.SendMessage("Only players can use this command.")
	}
	if !player.HasPermission("pokeskins.command.set") {
		return ctx.SendMessage(h.msg("skin_no_permission"))
	}
	if h.mineskin == nil {
		return ctx.SendMessage(h.msg("skin_api_error"))
	}
	urlStr := ctx.Arg("url").String()
	if urlStr == "" {
		return h.sendHelp(ctx)
	}
	ctx.SendMessage(h.msg("skin_set_url_fetching"))
	prop, err := h.mineskin.UploadSkinFromURL(urlStr)
	if err != nil {
		ctx.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	_ = prop // we don't store the property, will re-upload on each join (acceptable for simplicity)
	pref := SkinPreference{
		Type:   "url",
		Target: urlStr,
	}
	if err := h.storage.Set(player.ID().Undashed(), pref); err != nil {
		ctx.SendMessage(h.msgf("skin_set_fail", err.Error()))
		return nil
	}
	ctx.SendMessage(h.msg("skin_set_success"))
	return nil
}

func (h *commandHandler) handleReset(ctx *command.Context) error {
	player, ok := ctx.Sender().(proxy.Player)
	if !ok {
		return ctx.SendMessage("Only players can use this command.")
	}
	if !player.HasPermission("pokeskins.command.reset") {
		return ctx.SendMessage(h.msg("skin_no_permission"))
	}
	if err := h.storage.Delete(player.ID().Undashed()); err != nil {
		return ctx.SendMessage(h.msgf("skin_set_fail", err.Error()))
	}
	ctx.SendMessage(h.msg("skin_reset_success"))
	return nil
}

func (h *commandHandler) handleInfo(ctx *command.Context) error {
	player, ok := ctx.Sender().(proxy.Player)
	if !ok {
		return ctx.SendMessage("Only players can use this command.")
	}
	pref, exists := h.storage.Get(player.ID().Undashed())
	ctx.SendMessage(h.msg("skin_info_title"))
	if !exists {
		ctx.SendMessage(h.msg("skin_info_default"))
	} else if pref.Type == "premium" {
		ctx.SendMessage(h.msgf("skin_info_custom", "premium", pref.Target))
	} else if pref.Type == "url" {
		ctx.SendMessage(h.msgf("skin_info_custom", "URL", pref.Target))
	} else {
		ctx.SendMessage(h.msg("skin_info_default"))
	}
	return nil
}

func (h *commandHandler) handleReload(ctx *command.Context) error {
	if !ctx.Sender().HasPermission("pokeskins.admin") {
		return ctx.SendMessage(h.msg("skin_no_permission"))
	}
	newCfg, err := ReloadConfig("plugins/PokeSkins/config.yml")
	if err != nil {
		return ctx.SendMessage(fmt.Sprintf("&cFailed to reload config: %v", err))
	}
	h.cfg = newCfg
	ctx.SendMessage(h.msg("reload_success"))
	return nil
}

func (h *commandHandler) sendHelp(ctx *command.Context) error {
	ctx.SendMessage(h.msg("help_header"))
	ctx.SendMessage(h.msg("help_set"))
	if h.mineskin != nil && h.cfg.MineSkin.Enabled {
		ctx.SendMessage(h.msg("help_set_url"))
	}
	ctx.SendMessage(h.msg("help_reset"))
	ctx.SendMessage(h.msg("help_info"))
	if ctx.Sender().HasPermission("pokeskins.admin") {
		ctx.SendMessage(h.msg("help_reload"))
	}
	return nil
}

func (h *commandHandler) requirePermission(perm string) command.Requirement {
	return func(ctx *command.Context) bool {
		return ctx.Sender().HasPermission(perm)
	}
}

func (h *commandHandler) msg(key string) string {
	prefix := h.cfg.Messages["prefix"]
	msg := h.cfg.Messages[key]
	if msg == "" {
		msg = key
	}
	return prefix + msg
}

func (h *commandHandler) msgf(key string, args ...interface{}) string {
	prefix := h.cfg.Messages["prefix"]
	msg := h.cfg.Messages[key]
	if msg == "" {
		msg = key
	}
	return prefix + fmt.Sprintf(msg, args...)
}