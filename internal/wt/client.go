package wt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "http://localhost:8111"

// Client fetches data from the War Thunder local API.
type Client struct {
	http *http.Client
}

// NewClient returns a Client with a short timeout.
func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// MapObjects fetches and parses /map_obj.json.
// Returns nil, nil when the game is not running or in hangar.
func (c *Client) MapObjects() ([]MapObject, error) {
	var objs []MapObject
	if err := c.get("/map_obj.json", &objs); err != nil {
		return nil, err
	}

	return objs, nil
}

// stateResponse covers the fields we care about from /state.
type stateResponse struct {
	Valid    bool   `json:"valid"`
	GameMode string `json:"game_mode"` // e.g. "arcade", "realistic", "simulator"
	Type     string `json:"type"`      // alternate field seen in some API versions
}

// indicatorsResponse covers the fields we care about from /indicators.
type indicatorsResponse struct {
	GameMode string `json:"game_mode"`
	Type     string `json:"type"`
}

// GameMode returns the current battle mode by querying /state, then /indicators
// as fallback. Returns GameModeArcade when the game is not running or the mode
// cannot be determined.
func (c *Client) GameMode() GameMode {
	var state stateResponse
	if err := c.get("/state", &state); err == nil {
		if mode := modeFromFields(state.GameMode, state.Type); mode != GameModeArcade {
			return mode
		}
	}

	var ind indicatorsResponse
	if err := c.get("/indicators", &ind); err == nil {
		if mode := modeFromFields(ind.GameMode, ind.Type); mode != GameModeArcade {
			return mode
		}
	}

	return GameModeArcade
}

// modeFromFields tries each candidate string in order and returns the first
// non-Arcade result, or GameModeArcade if none match.
func modeFromFields(candidates ...string) GameMode {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		if mode := ParseGameMode(candidate); mode != GameModeArcade {
			return mode
		}
	}

	return GameModeArcade
}

// MapInfo fetches and parses /map_info.json.
func (c *Client) MapInfo() (*MapInfo, error) {
	var info MapInfo
	if err := c.get("/map_info.json", &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func (c *Client) get(path string, dst any) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("%s%s", baseURL, path), nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", path, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http get %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode response for %s: %w", path, err)
	}

	return nil
}
