package pokeskins

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

var Plugin = proxy.Plugin{
	Name: "PokeSkins",
	Init: func(ctx context.Context, p *proxy.Proxy) error {
		log := logr.FromContextOrDiscard(ctx)

		// create plugin folder: ./plugins/PokeSkins
		pluginDir := "plugins/PokeSkins"
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			log.Error(err, "Failed to create plugin folder")
			return err
		}

		// load config
		cfg, err := LoadConfig(filepath.Join(pluginDir, "config.yml"))
		if err != nil {
			log.Error(err, "Failed to load config")
			return err
		}

		// init storage (preferences only)
		storage, err := NewSkinStorage(filepath.Join(pluginDir, "skins.json"), cfg.StorageTTL)
		if err != nil {
			log.Error(err, "Failed to init storage")
			return err
		}

		// init skin fetcher (Mojang + PlayerDB fallback)
		fetcher := newSkinFetcher()

		// init MineSkin if enabled (for URL skins)
		var mineskin *MineSkin
		if cfg.MineSkin.Enabled {
			mineskin = NewMineSkin(cfg.MineSkin.APIKey, cfg.MineSkin.UploadTimeout)
		}

		// register commands (using Gate's Brigadier API)
		registerCommands(p, log, cfg, storage, fetcher, mineskin)

		// subscribe to event for automatic skin application
		event.Subscribe(p.Event(), 0, func(e *proxy.GameProfileRequestEvent) {
			onGameProfile(log, cfg, storage, fetcher, mineskin, e)
		})

		log.Info("PokeSkins plugin loaded",
			"storageTTL", cfg.StorageTTL,
			"mineskinEnabled", cfg.MineSkin.Enabled,
			"forceCustomSkin", cfg.ForceCustomSkin)

		return nil
	},
}

// onGameProfile applies custom/premium skin based on stored preferences.
func onGameProfile(log logr.Logger, cfg *Config, storage *SkinStorage, fetcher *skinFetcher, mineskin *MineSkin, e *proxy.GameProfileRequestEvent) {
	orig := e.Original()
	playerUUID := orig.ID.Undashed()
	name := strings.TrimSpace(orig.Name)
	if name == "" {
		return
	}

	// check for stored preference
	pref, hasPref := storage.Get(playerUUID)
	if hasPref {
		// apply custom skin based on preference type
		textures, err := resolveSkinByPreference(log, pref, fetcher, mineskin)
		if err != nil {
			log.V(1).Info("Failed to apply custom skin from preference", "player", name, "type", pref.Type, "error", err)
			// fall through to default logic
		} else if len(textures) > 0 {
			merged := mergeTextureProperties(orig.Properties, textures)
			if len(merged) > 0 {
				e.SetGameProfile(profile.GameProfile{
					ID:         orig.ID,
					Name:       orig.Name,
					Properties: merged,
				})
				log.V(1).Info("Applied custom skin", "player", name, "type", pref.Type)
				return
			}
		}
	}

	// no valid preference or fallback: use original Mojang logic
	if e.OnlineMode() {
		// online mode: only restore if original lacks textures (auth didn't provide)
		if hasTextures(orig.Properties) {
			return
		}
		premiumUUID := orig.ID
		textures, err := fetcher.texturesForUUID(premiumUUID)
		if err != nil {
			log.V(1).Info("Failed to fetch textures for online-mode player", "player", name, "error", err)
			return
		}
		if len(textures) > 0 {
			merged := mergeTextureProperties(orig.Properties, textures)
			e.SetGameProfile(profile.GameProfile{ID: orig.ID, Name: orig.Name, Properties: merged})
			log.V(1).Info("Restored premium skin for online-mode player", "player", name)
		}
		return
	}

	// offline mode: resolve username -> premium UUID and fetch textures
	if orig.ID != uuid.OfflinePlayerUUID(name) {
		return // already has a non-offline UUID? shouldn't happen, but guard
	}
	premiumUUID, err := fetcher.resolveUUID(name)
	if err != nil || premiumUUID == uuid.Nil {
		log.V(1).Info("No premium profile for offline player", "player", name, "error", err)
		return
	}
	textures, err := fetcher.texturesForUUID(premiumUUID)
	if err != nil {
		log.V(1).Info("Failed to fetch premium textures for offline player", "player", name, "error", err)
		return
	}
	if len(textures) > 0 {
		merged := mergeTextureProperties(orig.Properties, textures)
		e.SetGameProfile(profile.GameProfile{ID: orig.ID, Name: orig.Name, Properties: merged})
		log.V(1).Info("Restored premium skin for offline-mode player", "player", name)
	}
}

// resolveSkinByPreference returns textures based on stored preference.
func resolveSkinByPreference(log logr.Logger, pref SkinPreference, fetcher *skinFetcher, mineskin *MineSkin) ([]profile.Property, error) {
	switch pref.Type {
	case "premium":
		uid, err := fetcher.resolveUUID(pref.Target)
		if err != nil || uid == uuid.Nil {
			return nil, err
		}
		return fetcher.texturesForUUID(uid)
	case "url":
		if mineskin == nil {
			return nil, nil
		}
		prop, err := mineskin.UploadSkinFromURL(pref.Target)
		if err != nil {
			return nil, err
		}
		return []profile.Property{prop}, nil
	default:
		return nil, nil
	}
}

// hasTextures checks if the property slice contains a "textures" entry.
func hasTextures(props []profile.Property) bool {
	for _, p := range props {
		if p.Name == "textures" {
			return true
		}
	}
	return false
}

// mergeTextureProperties removes any existing "textures" and appends the new ones.
func mergeTextureProperties(orig []profile.Property, textures []profile.Property) []profile.Property {
	out := make([]profile.Property, 0, len(orig)+len(textures))
	for _, p := range orig {
		if p.Name != "textures" {
			out = append(out, p)
		}
	}
	out = append(out, textures...)
	return out
}