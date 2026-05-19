package pokeperms

import (
	"encoding/json"
	"fmt"

	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// SyncChannel define kore amader unique namespaced key jeta Paper server listen korbe
var SyncChannel = proxy.NewChannelIdentifier("pokeperms:sync")

// SyncPacket structures the network message containing security validation token
type SyncPacket struct {
	Token  string `json:"token"`  // token.yml theke asha key, jeta verification validation e lagbe
	Action string `json:"action"` // "USER_UPDATE", "GROUP_UPDATE", "SYNC_ALL"
	Target string `json:"target"` // Affected player UUID ba Group name
}

// MessagingManager handles sending data sync updates to backend servers
type MessagingManager struct {
	proxy *proxy.Proxy
	token string
}

// NewMessagingManager initializes and registers the channel into Gate Proxy network
func NewMessagingManager(p *proxy.Proxy, token string) *MessagingManager {
	// Register the backend channel to proxy network so packets are allowed to flow
	p.ChannelRegistrar().Register(SyncChannel)

	return &MessagingManager{
		proxy: p,
		token: token,
	}
}

// BroadcastUpdate sends a sync request containing token to all registered Paper servers
func (m *MessagingManager) BroadcastUpdate(action string, target string) error {
	// Secure packet bundle with the required token
	packet := SyncPacket{
		Token:  m.token,
		Action: action,
		Target: target,
	}

	// Transform data into simple JSON format for effortless cross-platform Java compatibility
	data, err := json.Marshal(packet)
	if err != nil {
		return fmt.Errorf("failed to encode permission packet: %w", err)
	}

	servers := m.proxy.Servers()
	if len(servers) == 0 {
		return nil // No backend servers online to notify
	}

	// Dispatch packet to all connected downstream servers in the network loop
	for _, server := range servers {
		// Note: Minecraft specifications require at least 1 online player
		// on that specific server to successfully pipe Plugin Messages.
		_ = server.SendPluginMessage(SyncChannel, data)
	}

	return nil
}