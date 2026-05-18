package pokeskins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.minekube.com/gate/pkg/edition/java/profile"
)

// MineSkin API endpoints
const (
	mineskinAPIURL = "https://api.mineskin.org/generate/url"
)

// MineSkin handles uploading images from URLs to get texture properties.
type MineSkin struct {
	apiKey     string
	httpClient *http.Client
}

// NewMineSkin creates a new MineSkin client.
func NewMineSkin(apiKey string, timeout time.Duration) *MineSkin {
	return &MineSkin{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// mineskinRequest represents the request body for MineSkin API.
type mineskinRequest struct {
	URL        string `json:"url"`
	Visibility int    `json:"visibility"` // 0 = public, 1 = private, 2 = unlisted
}

// mineskinResponse represents the response from MineSkin API.
type mineskinResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Texture struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"texture"`
	} `json:"data"`
	Error string `json:"error"`
}

// UploadSkinFromURL uploads a skin from an image URL and returns the texture property.
// The URL must point to a direct PNG image (MineSkin supports PNG, JPG, and some others).
func (m *MineSkin) UploadSkinFromURL(imageURL string) (profile.Property, error) {
	if m == nil {
		return profile.Property{}, fmt.Errorf("mineskin not enabled")
	}

	reqBody := mineskinRequest{
		URL:        imageURL,
		Visibility: 0, // public - can be changed if needed
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return profile.Property{}, err
	}

	req, err := http.NewRequest(http.MethodPost, mineskinAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return profile.Property{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return profile.Property{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return profile.Property{}, err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return profile.Property{}, errRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return profile.Property{}, fmt.Errorf("mineskin API error: %s (body: %s)", resp.Status, string(body))
	}

	var msResp mineskinResponse
	if err := json.Unmarshal(body, &msResp); err != nil {
		return profile.Property{}, err
	}
	if !msResp.Success {
		return profile.Property{}, fmt.Errorf("mineskin upload failed: %s", msResp.Error)
	}
	if msResp.Data.Texture.Value == "" {
		return profile.Property{}, fmt.Errorf("mineskin returned empty texture")
	}

	return profile.Property{
		Name:      "textures",
		Value:     msResp.Data.Texture.Value,
		Signature: msResp.Data.Texture.Signature,
	}, nil
}