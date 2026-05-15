package cloversecurity

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/robinbraemer/event"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"gopkg.in/yaml.v3"
	"go.minekube.com/gate/pkg/util/uuid"
)

type Config struct {
	AuthServer      string   `yaml:"auth-server"`
	LobbyServer     string   `yaml:"lobby-server"`
	FallbackServers []string `yaml:"fallback-servers"`
	Messages        struct {
		AuthOffline  string `yaml:"auth-offline"`
		NoAuthAccess string `yaml:"no-auth-access"`
		AuthBlocked  string `yaml:"auth-blocked"`
		Transferred  string `yaml:"transferred"`
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
	return cfg, nil
}

type AuthManager struct {
	mu   sync.RWMutex
	auth map[string]bool
}

func NewAuthManager() *AuthManager {
	return &AuthManager{auth: make(map[string]bool)}
}

func (m *AuthManager) IsAuthenticated(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.auth[name]
}

func (m *AuthManager) SetAuthenticated(name string, val bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if val {
		m.auth[name] = true
	} else {
		delete(m.auth, name)
	}
}

func (m *AuthManager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.auth, name)
}

// readUTF reads a Java modified UTF-8 string from the byte slice at the given offset.
// Returns the string and the new offset (number of bytes consumed).
func readUTF(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", offset, fmt.Errorf("not enough bytes for length")
	}
	length := int(data[offset])<<8 | int(data[offset+1])
	offset += 2
	if offset+length > len(data) {
		return "", offset, fmt.Errorf("not enough bytes for string data")
	}
	str := string(data[offset : offset+length])
	offset += length
	return str, offset, nil
}

var Plugin = proxy.Plugin{
	Name: "CloverSecurity",
	Init: func(ctx context.Context, prx *proxy.Proxy) error {
		cfg, err := LoadConfig("plugins/cloversecurity/config.yml")
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		authMgr := NewAuthManager()

		// LoginEvent – Auth সার্ভার চেক
		event.Subscribe(prx.Event(), 0, func(e *proxy.LoginEvent) {
			authSrv := prx.Server(cfg.AuthServer)
			if authSrv == nil {
				e.Deny(&component.Text{Content: replaceColor(cfg.Messages.AuthOffline)})
				return
			}
			authMgr.SetAuthenticated(e.Player().Username(), false)
			e.Allow()
		})

		// ServerPreConnectEvent – আনঅথ প্লেয়ারকে Auth-এ ফোর্স, অথ প্লেয়ারকে Auth-এ ব্লক
		event.Subscribe(prx.Event(), 0, func(e *proxy.ServerPreConnectEvent) {
			player := e.Player()
			target := e.Server()
			authSrv := prx.Server(cfg.AuthServer)

			if target == nil || authSrv == nil {
				return
			}

			targetName := target.ServerInfo().Name()
			authName := authSrv.ServerInfo().Name()

			if !authMgr.IsAuthenticated(player.Username()) {
				if targetName != authName {
					e.Allow(authSrv)
					_ = player.SendMessage(&component.Text{Content: replaceColor(cfg.Messages.NoAuthAccess)})
				}
				return
			}

			if targetName == authName {
				e.Deny()
				_ = player.SendMessage(&component.Text{Content: replaceColor(cfg.Messages.AuthBlocked)})
			}
		})

		// PluginMessageEvent – Spigot থেকে auth_success মেসেজ ধরা
		event.Subscribe(prx.Event(), 0, func(e *proxy.PluginMessageEvent) {
			// সঠিক চ্যানেল চেক
			if !e.Identifier().Equals(message.MinecraftChannelIdentifier.Create("clover", "auth")) {
				return
			}

			data := e.Data()
			if len(data) < 2 {
				return
			}

			// Spigot ফরম্যাট: writeUTF("auth_success"), writeUTF(playerUUID)
			offset := 0
			subChannel, newOffset, err := readUTF(data, offset)
			if err != nil {
				return
			}
			offset = newOffset

			if subChannel != "auth_success" {
				return
			}

			playerUUIDStr, _, err := readUTF(data, offset)
			if err != nil {
				return
			}

			playerUUID, err := uuid.Parse(playerUUIDStr)
			if err != nil {
				return
			}

			player := prx.PlayerByUUID(playerUUID)
			if player == nil {
				return
			}

			authMgr.SetAuthenticated(player.Username(), true)

			// লবি বা ফলব্যাকে পাঠানো
			targets := make([]proxy.RegisteredServer, 0, 1+len(cfg.FallbackServers))
			if lobby := prx.Server(cfg.LobbyServer); lobby != nil {
				targets = append(targets, lobby)
			}
			for _, name := range cfg.FallbackServers {
				if srv := prx.Server(name); srv != nil {
					targets = append(targets, srv)
				}
			}

			for _, target := range targets {
				if player.CreateConnectionRequest(target).ConnectWithIndication(ctx) {
					_ = player.SendMessage(&component.Text{Content: replaceColor(cfg.Messages.Transferred)})
					return
				}
			}
			_ = player.SendMessage(&component.Text{Content: "§cNo lobby or fallback server is available."})
		})

		// DisconnectEvent – ক্লিনআপ
		event.Subscribe(prx.Event(), 0, func(e *proxy.DisconnectEvent) {
			authMgr.Remove(e.Player().Username())
		})

		// /csv reload কমান্ড
		cmd := brigodier.Literal("csv").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return c.Source.HasPermission("cloversecurity.reload")
			})).
			Executes(command.Command(func(c *command.Context) error {
				newCfg, err := LoadConfig("plugins/cloversecurity/config.yml")
				if err != nil {
					_ = c.Source.SendMessage(&component.Text{Content: "§cFailed to reload configuration!"})
					return err
				}
				cfg = newCfg
				_ = c.Source.SendMessage(&component.Text{Content: "§aConfiguration reloaded successfully."})
				return nil
			}))
		prx.Command().Register(cmd)

		return nil
	},
}

func replaceColor(s string) string {
	return strings.ReplaceAll(s, "&", "§")
}