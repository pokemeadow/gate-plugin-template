package pokeskins

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

var errRateLimited = errors.New("rate limited (429)")

const userAgent = "PokeSkins/1.0 (Gate-Proxy; +https://github.com/minekube/gate)"

const playerdbURL = "https://playerdb.co/api/player/minecraft/%s"

// doReq performs HTTP request with proper user agent.
func (f *skinFetcher) doReq(apiURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return f.client.Do(req)
}

// --- Username → UUID with fallback ---

// fetchUUIDPlayerDB gets UUID from PlayerDB.
func (f *skinFetcher) fetchUUIDPlayerDB(username string) (uuid.UUID, error) {
	apiURL := fmt.Sprintf(playerdbURL, url.PathEscape(strings.TrimSpace(username)))
	resp, err := f.doReq(apiURL)
	if err != nil {
		return uuid.Nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return uuid.Nil, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return uuid.Nil, fmt.Errorf("playerdb API: %s", resp.Status)
	}
	var v playerDBResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return uuid.Nil, err
	}
	if !v.Success || v.Data.Player.ID == "" {
		return uuid.Nil, fmt.Errorf("no premium profile for username %q", username)
	}
	return uuid.Parse(v.Data.Player.ID)
}

// fetchUsernameToUUIDWithFallback tries Mojang, then PlayerDB.
func (f *skinFetcher) fetchUsernameToUUIDWithFallback(username string) (uuid.UUID, error) {
	uid, err := f.fetchUUIDMojang(username)
	if err == nil {
		return uid, nil
	}
	if !errors.Is(err, errRateLimited) {
		return uuid.Nil, err
	}
	return f.fetchUUIDPlayerDB(username)
}

// --- UUID → Profile (textures) with fallback ---

type playerDBResp struct {
	Success bool `json:"success"`
	Data    struct {
		Player struct {
			ID         string           `json:"id"`
			Properties []mojangProperty `json:"properties"`
		} `json:"player"`
	} `json:"data"`
}

// fetchProfilePlayerDB fetches profile by UUID from PlayerDB (accepts UUID or username).
func (f *skinFetcher) fetchProfilePlayerDB(uid uuid.UUID) ([]profile.Property, error) {
	apiURL := fmt.Sprintf(playerdbURL, uid.Undashed())
	resp, err := f.doReq(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playerdb API: %s", resp.Status)
	}
	var v playerDBResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	if !v.Success || v.Data.Player.Properties == nil {
		return nil, nil
	}
	for _, p := range v.Data.Player.Properties {
		if p.Name == "textures" {
			return []profile.Property{{Name: p.Name, Value: p.Value, Signature: p.Signature}}, nil
		}
	}
	return nil, nil
}

// fetchProfileByUUIDWithFallback tries Mojang, then PlayerDB.
func (f *skinFetcher) fetchProfileByUUIDWithFallback(uid uuid.UUID) ([]profile.Property, error) {
	props, err := f.fetchProfileMojang(uid)
	if err == nil {
		return props, nil
	}
	if !errors.Is(err, errRateLimited) {
		return nil, err
	}
	return f.fetchProfilePlayerDB(uid)
}