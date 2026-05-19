package pokeperms

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TokenConfig structures the token.yml layout
type TokenConfig struct {
	Token string `yaml:"sync-token"`
}

// LoadOrGenerateToken reads the existing token or creates a new secure one
func LoadOrGenerateToken(dir string) (string, error) {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "token.yml")

	// If file exists, try reading the valid token
	if data, err := os.ReadFile(path); err == nil {
		var tc TokenConfig
		if err := yaml.Unmarshal(data, &tc); err == nil {
			token := strings.TrimSpace(tc.Token)
			if token != "" && token != "REPLACE_ME" {
				return token, nil
			}
		}
	}

	// Generate a completely secure 32-character Hex key
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	newToken := hex.EncodeToString(bytes)

	// Save the new token to token.yml
	tc := TokenConfig{Token: newToken}
	d, err := yaml.Marshal(tc)
	if err == nil {
		// Add an instructional comment at the top for admins
		comment := "# Copy this sync-token and paste it into your PokePerms Paper verify_token.yml file\n"
		_ = os.WriteFile(path, []byte(comment+string(d)), 0644)
	}

	return newToken, nil
}