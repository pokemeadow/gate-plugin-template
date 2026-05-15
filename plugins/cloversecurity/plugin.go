package cloversecurity

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AuthServer             string   `yaml:"auth-server"`
	LobbyServer            string   `yaml:"lobby-server"`
	FallbackServers        []string `yaml:"fallback-servers"`
	AuthKickTimeoutSeconds  int     `yaml:"auth-kick-timeout-seconds"`
	Messages               struct {
		AuthOffline  string `yaml:"auth-offline"`
		NoAuthAccess string `yaml:"no-auth-access"`
		AuthBlocked  string `yaml:"auth-blocked"`
		Transferred  string `yaml:"transferred"`
		AuthKick     string `yaml:"auth-kick"`
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
	auth map[uuid.UUID]bool
}

func NewAuthManager() *AuthManager {
	return &AuthManager{auth: make(map[uuid.UUID]bool)}
}

func (m *AuthManager) IsAuthenticated(id uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.auth[id]
}

func (m *AuthManager) SetAuthenticated(id uuid.UUID, val bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if val {
		m.auth[id] = true
	} else {
		delete(m.auth, id)
	}
}

func (m *AuthManager) Remove(id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.auth, id)
}

// ADDED: custom channel identifier that matches Gate's ChannelIdentifier interface.
type authChannelID string

// ADDED: current Gate API expects an ID() method on channel identifiers.
func (a authChannelID) ID() string { return string(a) }

// ADDED: String() is extra-safe if your Gate build also uses String() internally.
func (a authChannelID) String() string { return string(a) }

// readUTF reads a Java DataOutputStream.writeUTF string from the byte slice at the given offset.
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

		// ADDED: Gate docs recommend logger from context.
		log := logr.FromContextOrDiscard(ctx).WithName("CloverSecurity")

		authMgr := NewAuthManager()
		kickDelay := time.Duration(cfg.AuthKickTimeoutSeconds) * time.Second

		// ADDED: register our plugin channel with the proxy.
		authChannel := authChannelID("clover:auth")
		prx.ChannelRegistrar().Register(authChannel)

		event.Subscribe(prx.Event(), 0, func(e *proxy.LoginEvent) {
			authSrv := prx.Server(cfg.AuthServer)
			if authSrv == nil {
				e.Deny(&component.Text{Content: replaceColor(cfg.Messages.AuthOffline)})
				return
			}

			// CHANGED: track auth by UUID, not username.
			authMgr.SetAuthenticated(e.Player().ID(), false)
			e.Allow()
		})

		event.Subscribe(prx.Event(), 0, func(e *proxy.ServerPreConnectEvent) {
			player := e.Player()
			target := e.Server()
			authSrv := prx.Server(cfg.AuthServer)

			if target == nil || authSrv == nil {
				return
			}

			targetName := target.ServerInfo().Name()
			authName := authSrv.ServerInfo().Name()

			// CHANGED: UUID-based auth check.
			if !authMgr.IsAuthenticated(player.ID()) {
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

		event.Subscribe(prx.Event(), 0, func(e *proxy.PluginMessageEvent) {
			// CHANGED: use ID(), not String().
			if e.Identifier().ID() != "clover:auth" {
				return
			}

			data := e.Data()
			if len(data) < 2 {
				return
			}

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
				log.Error(err, "failed to read UUID from auth_success message")
				return
			}

			playerUUID, err := uuid.Parse(playerUUIDStr)
			if err != nil {
				log.Error(err, "failed to parse player UUID")
				return
			}

			player := prx.Player(playerUUID)
			if player == nil {
				log.Info("auth_success received but player is not online", "uuid", playerUUIDStr)
				return
			}

			// CHANGED: store auth by UUID.
			authMgr.SetAuthenticated(playerUUID, true)
			log.Info("player authenticated", "player", player.Username(), "uuid", playerUUIDStr)

			// ADDED: kick timer only if the player is still stuck on auth.
			time.AfterFunc(kickDelay, func() {
				p := prx.Player(playerUUID)
				if p != nil && p.CurrentServer() != nil && p.CurrentServer().Server() != nil {
					if p.CurrentServer().Server().ServerInfo().Name() == cfg.AuthServer {
						p.Disconnect(&component.Text{Content: replaceColor(cfg.Messages.AuthKick)})
					}
				}
			})

			targets := make([]proxy.RegisteredServer, 0, 1+len(cfg.FallbackServers))

			if lobby := prx.Server(cfg.LobbyServer); lobby != nil {
				targets = append(targets, lobby)
			} else {
				log.Info("lobby server not found", "name", cfg.LobbyServer)
			}

			for _, name := range cfg.FallbackServers {
				if srv := prx.Server(name); srv != nil {
					targets = append(targets, srv)
				} else {
					log.Info("fallback server not found", "name", name)
				}
			}

			if len(targets) == 0 {
				_ = player.SendMessage(&component.Text{Content: "§cNo lobby or fallback server is available."})
				return
			}

			// ADDED: use player context so the transfer cancels if the player disconnects.
			switchCtx := player.Context()

			for _, target := range targets {
				if player.CreateConnectionRequest(target).ConnectWithIndication(switchCtx) {
					_ = player.SendMessage(&component.Text{Content: replaceColor(cfg.Messages.Transferred)})
					return
				}
			}

			_ = player.SendMessage(&component.Text{Content: "§cFailed to connect to lobby or fallback servers."})
		})

		event.Subscribe(prx.Event(), 0, func(e *proxy.DisconnectEvent) {
			authMgr.Remove(e.Player().ID())
		})

		return nil
	},
}

func replaceColor(s string) string {
	return strings.ReplaceAll(s, "&", "§")
}