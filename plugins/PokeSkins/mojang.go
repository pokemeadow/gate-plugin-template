package pokeskins

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	mojangUsernameURL = "https://api.mojang.com/users/profiles/minecraft/%s"
	mojangProfileURL  = "https://sessionserver.mojang.com/session/minecraft/profile/%s?unsigned=false"

	profileCacheTTL = time.Hour
	uuidCacheTTL    = 6 * time.Hour
	httpTimeout     = 3 * time.Second
)

type mojangProfileResp struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Properties []mojangProperty `json:"properties"`
	Legacy     bool             `json:"legacy,omitempty"`
}

type mojangProperty struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature"`
}

type mojangUsernameResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type skinFetcher struct {
	client       *http.Client
	profileCache *ttlcache.Cache[string, []profile.Property]
	uuidCache    *ttlcache.Cache[string, uuid.UUID]
}

func newSkinFetcher() *skinFetcher {
	f := &skinFetcher{
		client: &http.Client{Timeout: httpTimeout},
		profileCache: ttlcache.New[string, []profile.Property](
			ttlcache.WithTTL[string, []profile.Property](profileCacheTTL),
		),
		uuidCache: ttlcache.New[string, uuid.UUID](
			ttlcache.WithTTL[string, uuid.UUID](uuidCacheTTL),
		),
	}
	// start cache goroutines (ttlcache default starter)
	go f.profileCache.Start()
	go f.uuidCache.Start()
	return f
}

// resolveUUID returns the premium (Mojang) UUID for the given username.
// It caches results. Returns uuid.Nil if the username has no premium profile.
func (f *skinFetcher) resolveUUID(username string) (uuid.UUID, error) {
	key := strings.ToLower(strings.TrimSpace(username))
	if key == "" {
		return uuid.Nil, fmt.Errorf("empty username")
	}
	if item := f.uuidCache.Get(key); item != nil && !item.IsExpired() {
		return item.Value(), nil
	}
	uid, err := f.fetchUsernameToUUIDWithFallback(username)
	if err != nil {
		return uuid.Nil, err
	}
	f.uuidCache.Set(key, uid, uuidCacheTTL)
	return uid, nil
}

// fetchUsernameToUUIDWithFallback will be implemented in providers.go
// but we declare it here as a method placeholder; actual implementation uses providers.
// We'll call the version that includes fallback.
func (f *skinFetcher) fetchUsernameToUUIDWithFallback(username string) (uuid.UUID, error) {
	// This will be overridden or implemented in providers.go
	return f.fetchUsernameToUUID(username)
}

// fetchUsernameToUUID is a wrapper to call Mojang directly (will be extended in providers).
func (f *skinFetcher) fetchUsernameToUUID(username string) (uuid.UUID, error) {
	return f.fetchUUIDMojang(username)
}

// texturesForUUID returns the "textures" profile properties for the given UUID.
// Uses cache. Returns nil if the profile has no textures or on error.
func (f *skinFetcher) texturesForUUID(uid uuid.UUID) ([]profile.Property, error) {
	key := uid.Undashed()
	if item := f.profileCache.Get(key); item != nil && !item.IsExpired() {
		return item.Value(), nil
	}
	props, err := f.fetchProfileByUUIDWithFallback(uid)
	if err != nil {
		return nil, err
	}
	if len(props) != 0 {
		f.profileCache.Set(key, props, profileCacheTTL)
	}
	return props, nil
}

// fetchProfileByUUIDWithFallback will be implemented in providers.go.
func (f *skinFetcher) fetchProfileByUUIDWithFallback(uid uuid.UUID) ([]profile.Property, error) {
	return f.fetchProfileByUUID(uid)
}

func (f *skinFetcher) fetchProfileByUUID(uid uuid.UUID) ([]profile.Property, error) {
	return f.fetchProfileMojang(uid)
}

// fetchUUIDMojang calls Mojang API to get UUID from username.
func (f *skinFetcher) fetchUUIDMojang(username string) (uuid.UUID, error) {
	apiURL := fmt.Sprintf(mojangUsernameURL, url.PathEscape(strings.TrimSpace(username)))
	resp, err := f.client.Get(apiURL)
	if err != nil {
		return uuid.Nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return uuid.Nil, errRateLimited // errRateLimited defined in providers.go
	}
	if resp.StatusCode == http.StatusNoContent {
		return uuid.Nil, fmt.Errorf("no premium profile for username %q", username)
	}
	if resp.StatusCode != http.StatusOK {
		return uuid.Nil, fmt.Errorf("mojang username API: %s", resp.Status)
	}
	var v mojangUsernameResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(v.ID)
}

// fetchProfileMojang fetches profile (textures) from Mojang by UUID.
func (f *skinFetcher) fetchProfileMojang(uid uuid.UUID) ([]profile.Property, error) {
	apiURL := fmt.Sprintf(mojangProfileURL, uid.Undashed())
	resp, err := f.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errRateLimited
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mojang profile API: %s", resp.Status)
	}
	var v mojangProfileResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	for _, p := range v.Properties {
		if p.Name == "textures" {
			return []profile.Property{{Name: p.Name, Value: p.Value, Signature: p.Signature}}, nil
		}
	}
	return nil, nil
}