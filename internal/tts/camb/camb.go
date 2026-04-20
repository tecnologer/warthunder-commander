package camb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tecnologer/warthunder/internal/tts/player"
	"github.com/tecnologer/warthunder/internal/utils/closer"
)

const (
	baseURL   = "https://client.camb.ai/apis"
	pollPause = 1 * time.Second
	maxPolls  = 120 // 120 s timeout
)

// Speaker synthesises speech via the CAMB.AI API and plays it locally.
type Speaker struct {
	apiKey     string
	voiceID    int
	languageID int
	dir        string
	volume     int
	speed      float64
}

type createRequest struct {
	Text     string `json:"text"`
	VoiceID  int    `json:"voice_id"`
	Language int    `json:"language"`
}

type createResponse struct {
	TaskID string `json:"task_id"`
}

type pollResponse struct {
	Status string `json:"status"`
	RunID  int    `json:"run_id"`
}

type languageEntry struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

// VoiceEntry describes a single CAMB.AI voice.
type VoiceEntry struct {
	ID       int    `json:"id"`
	Name     string `json:"voice_name"`
	Gender   int    `json:"gender"` // 1 = male, 2 = female
	Language int    `json:"language"`
}

// GenderLabel returns "m" or "f".
func (v VoiceEntry) GenderLabel() string {
	if v.Gender == 2 {
		return "f"
	}

	return "m"
}

// ListVoices returns all CAMB.AI voices available for the given locale (e.g. "es-mx").
// Pass an empty language to get all voices regardless of locale.
func ListVoices(ctx context.Context, apiKey, language string) ([]VoiceEntry, error) {
	var langID int

	if language != "" {
		var err error

		langID, err = resolveLanguage(ctx, apiKey, language)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/list-voices", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("X-Api-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, bodyBytes)
	}

	var all []VoiceEntry
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if langID == 0 {
		return all, nil
	}

	filtered := all[:0]
	for _, entry := range all {
		if entry.Language == langID {
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// New constructs a Speaker by resolving the voice and language names to their
// numeric CAMB.AI IDs. Returns an error if either cannot be resolved or if
// the API key is missing.
func New(apiKey, voice, language, dir string, volume int, speed float64) (*Speaker, error) {
	ctx := context.Background()

	langID, err := resolveLanguage(ctx, apiKey, language)
	if err != nil {
		return nil, fmt.Errorf("resolve language %q: %w", language, err)
	}

	voiceID, err := resolveVoice(ctx, apiKey, voice, langID)
	if err != nil {
		return nil, fmt.Errorf("resolve voice %q: %w", voice, err)
	}

	return &Speaker{
		apiKey:     apiKey,
		voiceID:    voiceID,
		languageID: langID,
		dir:        dir,
		volume:     volume,
		speed:      speed,
	}, nil
}

// resolveLanguage queries /source-languages and returns the numeric ID for the
// given short_name (BCP-47 locale code, e.g. "es-es", "en-us"), case-insensitive.
func resolveLanguage(ctx context.Context, apiKey, name string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/source-languages", nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("X-Api-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, bodyBytes)
	}

	var langs []languageEntry
	if err := json.NewDecoder(resp.Body).Decode(&langs); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}

	nameLower := strings.ToLower(name)

	var available []string

	for _, entry := range langs {
		if strings.ToLower(entry.ShortName) == nameLower || strings.ToLower(entry.Name) == nameLower {
			return entry.ID, nil
		}

		available = append(available, entry.ShortName)
	}

	return 0, fmt.Errorf("locale %q not found; available: %s", name, strings.Join(available, ", "))
}

// lookupVoiceByID finds a voice in payload that matches the given numeric ID.
func lookupVoiceByID(payload []VoiceEntry, numID int) (int, bool) {
	for _, entry := range payload {
		if entry.ID == numID {
			return numID, true
		}
	}

	return 0, false
}

// lookupVoiceByName finds a voice in payload by name match.
func lookupVoiceByName(payload []VoiceEntry, voiceName string, languageID int) (int, []string, bool) {
	voiceLower := strings.ToLower(voiceName)

	var available []string

	for _, entry := range payload {
		if strings.ToLower(entry.Name) == voiceLower {
			return entry.ID, nil, true
		}

		if entry.Language == languageID {
			gender := "m"
			if entry.Gender == 2 {
				gender = "f"
			}

			available = append(available, fmt.Sprintf("%q (id=%d, %s)", entry.Name, entry.ID, gender))
		}
	}

	return 0, available, false
}

// resolveVoice resolves voice to a numeric ID. If voice is a plain integer
// string it is validated against the voices list; otherwise matched by name.
func resolveVoice(ctx context.Context, apiKey, voice string, languageID int) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/list-voices", nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("X-Api-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, bodyBytes)
	}

	var payload []VoiceEntry
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}

	// Try numeric ID match first.
	if numID, parseErr := strconv.Atoi(voice); parseErr == nil {
		if voiceID, ok := lookupVoiceByID(payload, numID); ok {
			return voiceID, nil
		}
	}

	// Try name match.
	voiceID, available, ok := lookupVoiceByName(payload, voice, languageID)
	if ok {
		return voiceID, nil
	}

	if len(available) == 0 {
		return 0, fmt.Errorf("voice %q not found and no voices available for language id %d", voice, languageID)
	}

	return 0, fmt.Errorf("voice %q not found; voices for this language: %s", voice, strings.Join(available, ", "))
}

// Speak synthesises msg and plays it, blocking until done.
func (c *Speaker) Speak(msg string) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("camb: mkdir: %w", err)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%d:%d:%s", c.voiceID, c.languageID, msg)))
	path := filepath.Join(c.dir, "camb-"+hash+".mp3")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := c.fetch(msg, path); err != nil {
			return err
		}
	}

	if err := player.PlayFile(path, c.volume, c.speed); err != nil {
		return fmt.Errorf("camb: play: %w", err)
	}

	return nil
}

func (c *Speaker) fetch(msg, dest string) error {
	taskID, err := c.createJob(msg)
	if err != nil {
		return err
	}

	runID, err := c.pollJob(taskID)
	if err != nil {
		return err
	}

	return c.downloadAudio(baseURL+"/tts-result/"+runID, dest)
}

func (c *Speaker) createJob(msg string) (string, error) {
	body, err := json.Marshal(createRequest{
		Text:     msg,
		VoiceID:  c.voiceID,
		Language: c.languageID,
	})
	if err != nil {
		return "", fmt.Errorf("camb: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/tts", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("camb: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("camb: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return "", fmt.Errorf("camb: create status %d: %s", resp.StatusCode, bodyBytes)
	}

	var createResp createResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", fmt.Errorf("camb: decode create response: %w", err)
	}

	if createResp.TaskID == "" {
		return "", fmt.Errorf("camb: empty task_id in response")
	}

	return createResp.TaskID, nil
}

func (c *Speaker) pollJob(runID string) (string, error) {
	url := baseURL + "/tts/" + runID

	for range maxPolls {
		time.Sleep(pollPause)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return "", fmt.Errorf("camb: poll build request: %w", err)
		}

		req.Header.Set("X-Api-Key", c.apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("camb: poll http: %w", err)
		}

		var poll pollResponse

		decErr := json.NewDecoder(resp.Body).Decode(&poll)
		resp.Body.Close()

		if decErr != nil {
			return "", fmt.Errorf("camb: decode poll response: %w", decErr)
		}

		switch poll.Status {
		case "SUCCESS":
			if poll.RunID == 0 {
				return "", fmt.Errorf("camb: SUCCESS status but zero run_id")
			}

			return strconv.Itoa(poll.RunID), nil
		case "FAILED", "ERROR":
			return "", fmt.Errorf("camb: task %s failed with status %q", runID, poll.Status)
		}
		// IN_PROGRESS or similar — keep polling
	}

	return "", fmt.Errorf("camb: job %s timed out after %d polls", runID, maxPolls)
}

func (c *Speaker) downloadAudio(audioURL, dest string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, audioURL, nil)
	if err != nil {
		return fmt.Errorf("camb: download build request: %w", err)
	}

	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("camb: download audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("camb: download status %d: %s", resp.StatusCode, bodyBytes)
	}

	audioFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("camb: create %s: %w", dest, err)
	}
	defer closer.Close(audioFile)

	if _, err := io.Copy(audioFile, resp.Body); err != nil {
		os.Remove(dest)

		return fmt.Errorf("camb: write audio: %w", err)
	}

	return nil
}
