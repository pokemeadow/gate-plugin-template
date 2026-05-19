package pokeperms

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// colorText parses legacy color codes (&a, &c, etc.) into Gate's component system
func colorText(s string) component.Component {
	leg := legacy.Legacy{
		Char:    legacy.AmpersandChar,
		HexChar: legacy.HexChar,
	}
	comp, err := leg.Unmarshal([]byte(s))
	if err != nil {
		return &component.Text{Content: s}
	}
	return comp
}

// RegisterCommands initializes the entire /pokeperms command tree
func RegisterCommands(p *proxy.Proxy, storage *Storage, cfg *Config) {
	h := &commandHandler{
		proxy:   p,
		storage: storage,
		cfg:     cfg,
	}

	// Root Command: /pokeperms
	root := brigodier.Literal("pokeperms").
		Requires(command.Requires(func(c *command.RequiresContext) bool {
			// 1. Console Bypass Logic
			// Jodi executor player na hoy (mane console theke ashe), tahole direct true (Bypass)
			_, isPlayer := c.Source.(proxy.Player)
			if !isPlayer {
				return true
			}

			// 2. Admin Permission Logic (Gate Specific)
			hasPerm := c.Source.HasPermission("pokeperms.gate.admin")
			if !hasPerm {
				// Send custom unauthorized translation message from config
				c.Source.SendMessage(colorText(cfg.Messages.Prefix + cfg.Messages.NoPermission))
			}
			return hasPerm
		})).
		Executes(command.Command(func(c *command.Context) error {
			c.Source.SendMessage(colorText(cfg.Messages.Prefix + "&7Version 1.0.0 - Use /pp user or /pp group"))
			return nil
		}))

	// --- 1. USER COMMANDS FULL TREE ---
	userNode := brigodier.Literal("user").
		Then(brigodier.Argument("player", brigodier.String).
			Then(brigodier.Literal("info").Executes(command.Command(h.userInfo))).
			Then(brigodier.Literal("clear").Executes(command.Command(h.userClear))).
			Then(brigodier.Literal("parent").
				Then(brigodier.Literal("set").
					Then(brigodier.Argument("group", brigodier.String).
						Executes(command.Command(h.userParentSet)))).
				Then(brigodier.Literal("addtemp").
					Then(brigodier.Argument("group", brigodier.String).
						Then(brigodier.Argument("duration", brigodier.String).
							Executes(command.Command(h.userParentAddTemp)))))).
			Then(brigodier.Literal("permission").
				Then(brigodier.Literal("set").
					Then(brigodier.Argument("node", brigodier.String).
						Then(brigodier.Argument("value", brigodier.Bool).
							Executes(command.Command(h.userPermSet)).
							Then(brigodier.Argument("server", brigodier.String).
								Executes(command.Command(h.userPermSet)))))).
				Then(brigodier.Literal("unset").
					Then(brigodier.Argument("node", brigodier.String).
						Executes(command.Command(h.userPermUnset)).
						Then(brigodier.Argument("server", brigodier.String).
							Executes(command.Command(h.userPermUnset)))))))

	// --- 2. GROUP COMMANDS FULL TREE ---
	groupNode := brigodier.Literal("group").
		Then(brigodier.Argument("group_name", brigodier.String).
			Then(brigodier.Literal("create").Executes(command.Command(h.groupCreate))).
			Then(brigodier.Literal("delete").Executes(command.Command(h.groupDelete))).
			Then(brigodier.Literal("info").Executes(command.Command(h.groupInfo))).
			Then(brigodier.Literal("prefix").
				Then(brigodier.Literal("set").
					Then(brigodier.Argument("prefix_str", brigodier.String).
						Executes(command.Command(h.groupPrefix))))).
			Then(brigodier.Literal("setweight").
				Then(brigodier.Argument("weight", brigodier.Integer).
					Executes(command.Command(h.groupWeight)))).
			Then(brigodier.Literal("parent").
				Then(brigodier.Literal("add").
					Then(brigodier.Argument("parent_name", brigodier.String).Executes(command.Command(h.groupParentAdd)))).
				Then(brigodier.Literal("remove").
					Then(brigodier.Argument("parent_name", brigodier.String).Executes(command.Command(h.groupParentRemove))))).
			Then(brigodier.Literal("permission").
				Then(brigodier.Literal("set").
					Then(brigodier.Argument("node", brigodier.String).
						Then(brigodier.Argument("value", brigodier.Bool).
							Executes(command.Command(h.groupPermSet)).
							Then(brigodier.Argument("server", brigodier.String).
								Executes(command.Command(h.groupPermSet)))))).
				Then(brigodier.Literal("unset").
					Then(brigodier.Argument("node", brigodier.String).
						Executes(command.Command(h.groupPermUnset)).
						Then(brigodier.Argument("server", brigodier.String).
							Executes(command.Command(h.groupPermUnset)))))))

	// --- 3. UTILITY COMMANDS FULL TREE ---
	reloadNode := brigodier.Literal("reload").Executes(command.Command(h.handleReload))
	syncNode := brigodier.Literal("sync").Executes(command.Command(h.handleSync))

	// Attach all branches to root
	cmd := root.Then(userNode).Then(groupNode).Then(reloadNode).Then(syncNode)

	// Register into proxy command manager with aliases from config
	p.Command().RegisterWithAliases(cmd, cfg.Aliases...)
}

// commandHandler structure holds runtime dependencies for commands
type commandHandler struct {
	proxy   *proxy.Proxy
	storage *Storage
	cfg     *Config
}

// msg appends the custom prefix from config
func (h *commandHandler) msg(s string) string {
	return h.cfg.Messages.Prefix + s
}

// resolvePlayerUUID attempts to find online player UUID, falls back to offline UUID generation
func (h *commandHandler) resolvePlayerUUID(username string) string {
	player := h.proxy.PlayerByName(username)
	if player != nil {
		return player.ID().Undashed()
	}
	return uuid.OfflinePlayerUUID(username).Undashed()
}

// parseDuration converts strings like "30d" or "12h" into Go time.Duration
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// optionalServer gets the server context for permissions, defaults to "global"
func optionalServer(c *command.Context) string {
	val := c.String("server")
	if val == "" {
		return "global"
	}
	return val
}

// ---------------------------------------------------------
// User Execution Handlers
// ---------------------------------------------------------

func (h *commandHandler) userInfo(c *command.Context) error {
	username := c.String("player")
	c.Source.SendMessage(colorText(h.msg("&aFetching profile for &e" + username)))
	return nil
}

func (h *commandHandler) userClear(c *command.Context) error {
	username := c.String("player")
	uid := h.resolvePlayerUUID(username)

	if err := h.storage.ClearUser(uid); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.UserCleared, username))))
	return nil
}

func (h *commandHandler) userParentSet(c *command.Context) error {
	username := c.String("player")
	group := c.String("group")
	uid := h.resolvePlayerUUID(username)

	h.storage.EnsureUser(uid, username)
	if err := h.storage.SetUserGroup(uid, group); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.UserParentSet, username, group))))
	return nil
}

func (h *commandHandler) userParentAddTemp(c *command.Context) error {
	username := c.String("player")
	group := c.String("group")
	durStr := c.String("duration")
	uid := h.resolvePlayerUUID(username)

	duration, err := parseDuration(durStr)
	if err != nil {
		c.Source.SendMessage(colorText(h.msg(h.cfg.Messages.InvalidDuration)))
		return nil
	}

	h.storage.EnsureUser(uid, username)
	if err := h.storage.AddUserTempGroup(uid, group, duration); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.UserParentAddTemp, group, username, durStr))))
	return nil
}

func (h *commandHandler) userPermSet(c *command.Context) error {
	username := c.String("player")
	node := c.String("node")
	val := c.Bool("value")
	server := optionalServer(c)
	uid := h.resolvePlayerUUID(username)

	h.storage.EnsureUser(uid, username)
	if err := h.storage.SetUserPermission(uid, node, val, server); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.UserPermSet, node, val, username, server))))
	return nil
}

func (h *commandHandler) userPermUnset(c *command.Context) error {
	username := c.String("player")
	node := c.String("node")
	server := optionalServer(c)
	uid := h.resolvePlayerUUID(username)

	if err := h.storage.UnsetUserPermission(uid, node, server); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.UserPermUnset, node, username, server))))
	return nil
}

// ---------------------------------------------------------
// Group Execution Handlers
// ---------------------------------------------------------

func (h *commandHandler) groupCreate(c *command.Context) error {
	group := c.String("group_name")
	if err := h.storage.CreateGroup(group); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupCreated, group))))
	return nil
}

func (h *commandHandler) groupDelete(c *command.Context) error {
	group := c.String("group_name")
	if err := h.storage.DeleteGroup(group); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupDeleted, group))))
	return nil
}

func (h *commandHandler) groupInfo(c *command.Context) error {
	group := c.String("group_name")
	c.Source.SendMessage(colorText(h.msg("&aFetching info block for group &b" + group)))
	return nil
}

func (h *commandHandler) groupPrefix(c *command.Context) error {
	group := c.String("group_name")
	prefix := c.String("prefix_str")
	if err := h.storage.SetGroupPrefix(group, prefix); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupPrefixSet, group, prefix))))
	return nil
}

func (h *commandHandler) groupWeight(c *command.Context) error {
	group := c.String("group_name")
	weight := c.Int("weight")
	if err := h.storage.SetGroupWeight(group, weight); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupWeightSet, group, weight))))
	return nil
}

func (h *commandHandler) groupParentAdd(c *command.Context) error {
	group := c.String("group_name")
	parent := c.String("parent_name")
	if err := h.storage.AddGroupParent(group, parent); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupParentAdded, group, parent))))
	return nil
}

func (h *commandHandler) groupParentRemove(c *command.Context) error {
	group := c.String("group_name")
	parent := c.String("parent_name")
	if err := h.storage.RemoveGroupParent(group, parent); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupParentRemoved, group, parent))))
	return nil
}

func (h *commandHandler) groupPermSet(c *command.Context) error {
	group := c.String("group_name")
	node := c.String("node")
	val := c.Bool("value")
	server := optionalServer(c)

	if err := h.storage.SetGroupPermission(group, node, val, server); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupPermSet, node, val, group, server))))
	return nil
}

func (h *commandHandler) groupPermUnset(c *command.Context) error {
	group := c.String("group_name")
	node := c.String("node")
	server := optionalServer(c)

	if err := h.storage.UnsetGroupPermission(group, node, server); err != nil {
		c.Source.SendMessage(colorText(h.msg("&cError: " + err.Error())))
		return nil
	}
	c.Source.SendMessage(colorText(h.msg(fmt.Sprintf(h.cfg.Messages.GroupPermUnset, node, group, server))))
	return nil
}

// ---------------------------------------------------------
// Utility Execution Handlers
// ---------------------------------------------------------

func (h *commandHandler) handleReload(c *command.Context) error {
	c.Source.SendMessage(colorText(h.msg("&eReloading configurations and messages...")))
	// Trigger configuration reload mechanism here
	return nil
}

func (h *commandHandler) handleSync(c *command.Context) error {
	c.Source.SendMessage(colorText(h.msg("&eSyncing active database structures to all backend servers...")))
	// Trigger network payload dispatcher here
	return nil
}