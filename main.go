// Last.fm Scrobble Exclusion Plugin for Navidrome
//
// This plugin scrobbles plays to Last.fm exactly like Navidrome's native Last.fm
// integration, EXCEPT it silently skips any track whose Navidrome track ID appears
// in a configurable blacklist. Blacklisted tracks are never sent to Last.fm (neither
// "now playing" nor a completed scrobble); every other track is scrobbled normally.
//
// Because a Navidrome scrobbler plugin is ADDITIVE (it cannot veto the built-in
// Last.fm agent), you must disable Navidrome's native Last.fm scrobbler and let this
// plugin be the sole path to Last.fm. See README.md.
//
// Capabilities: Scrobbler
// Host services: HTTP (ws.audioscrobbler.com)
package main

import (
	"encoding/json"
	"fmt"

	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
	"github.com/navidrome/navidrome/plugins/pdk/go/scrobbler"
)

// Configuration keys (must match the lowercase property names in manifest.json).
const (
	apiKeyKey          = "apikey"
	apiSecretKey       = "apisecret"
	usersKey           = "users"
	blacklistKey       = "blacklist"       // by Navidrome track ID
	artistBlacklistKey = "artistblacklist" // by artist name
	titleBlacklistKey  = "titleblacklist"  // by title substring
)

// userSession is one entry of the "users" config array: a Navidrome username mapped
// to that user's Last.fm session key (obtained once via `make session-key`).
type userSession struct {
	Username   string `json:"username"`
	SessionKey string `json:"sessionkey"`
}

// idEntry is one entry of the "blacklist" config array (match by track ID).
type idEntry struct {
	TrackID string `json:"trackid"`
	Note    string `json:"note"`
}

// artistEntry is one entry of the "artistblacklist" config array (match by artist).
type artistEntry struct {
	Artist string `json:"artist"`
	Note   string `json:"note"`
}

// titleEntry is one entry of the "titleblacklist" config array (match by title substring).
type titleEntry struct {
	Text string `json:"text"`
	Note string `json:"note"`
}

// exclusionPlugin implements the Scrobbler capability.
type exclusionPlugin struct{}

func init() {
	scrobbler.Register(&exclusionPlugin{})
}

// main is required by the WASM runtime but is never called by Navidrome.
func main() {}

// getConfig loads and validates the plugin configuration.
func getConfig() (cfg lastfmConfig, users map[string]string, bl blacklists, err error) {
	apiKey, ok := pdk.GetConfig(apiKeyKey)
	if !ok || apiKey == "" {
		return cfg, nil, bl, fmt.Errorf("missing %q in configuration", apiKeyKey)
	}
	apiSecret, ok := pdk.GetConfig(apiSecretKey)
	if !ok || apiSecret == "" {
		return cfg, nil, bl, fmt.Errorf("missing %q in configuration", apiSecretKey)
	}
	cfg = lastfmConfig{APIKey: apiKey, APISecret: apiSecret}

	// Users -> session keys.
	users = make(map[string]string)
	if usersJSON, ok := pdk.GetConfig(usersKey); ok && usersJSON != "" {
		var entries []userSession
		if err := json.Unmarshal([]byte(usersJSON), &entries); err != nil {
			return cfg, nil, bl, fmt.Errorf("failed to parse %q config: %w", usersKey, err)
		}
		for _, e := range entries {
			// Match usernames case-insensitively and tolerant of stray whitespace,
			// since Navidrome logins are not case-sensitive.
			if key := normalizeText(e.Username); key != "" && e.SessionKey != "" {
				users[key] = e.SessionKey
			}
		}
	}

	bl = blacklists{
		ids:     make(map[string]struct{}),
		artists: make(map[string]struct{}),
	}

	// Blacklist by Navidrome track ID.
	if raw, ok := pdk.GetConfig(blacklistKey); ok && raw != "" {
		var entries []idEntry
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return cfg, nil, bl, fmt.Errorf("failed to parse %q config: %w", blacklistKey, err)
		}
		for _, e := range entries {
			if id := normalizeText(e.TrackID); id != "" {
				bl.ids[id] = struct{}{}
			}
		}
	}

	// Blacklist by artist name.
	if raw, ok := pdk.GetConfig(artistBlacklistKey); ok && raw != "" {
		var entries []artistEntry
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return cfg, nil, bl, fmt.Errorf("failed to parse %q config: %w", artistBlacklistKey, err)
		}
		for _, e := range entries {
			if a := normalizeText(e.Artist); a != "" {
				bl.artists[a] = struct{}{}
			}
		}
	}

	// Blacklist by title substring.
	if raw, ok := pdk.GetConfig(titleBlacklistKey); ok && raw != "" {
		var entries []titleEntry
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return cfg, nil, bl, fmt.Errorf("failed to parse %q config: %w", titleBlacklistKey, err)
		}
		for _, e := range entries {
			if t := normalizeText(e.Text); t != "" {
				bl.titleSubstrings = append(bl.titleSubstrings, t)
			}
		}
	}

	return cfg, users, bl, nil
}

// IsAuthorized reports whether the given user has a Last.fm session key configured.
// Navidrome only routes scrobbles for users assigned to this plugin, but it still
// checks authorization first.
func (p *exclusionPlugin) IsAuthorized(input scrobbler.IsAuthorizedRequest) (bool, error) {
	_, users, _, err := getConfig()
	if err != nil {
		pdk.Log(pdk.LogError, fmt.Sprintf("IsAuthorized: %v", err))
		return false, nil
	}
	_, ok := users[normalizeText(input.Username)]
	if !ok {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("IsAuthorized(user=%q) -> false; configured usernames: %v "+
			"(the left field of each User Session Keys row must be the Navidrome username)",
			input.Username, configuredNames(users)))
	} else {
		pdk.Log(pdk.LogInfo, fmt.Sprintf("IsAuthorized(user=%q) -> true", input.Username))
	}
	return ok, nil
}

// configuredNames returns the normalized usernames present in the config, for
// diagnostics only (session keys are never logged).
func configuredNames(users map[string]string) []string {
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	return names
}

// NowPlaying sends a "now playing" update to Last.fm, unless the track is blacklisted.
func (p *exclusionPlugin) NowPlaying(input scrobbler.NowPlayingRequest) error {
	pdk.Log(pdk.LogInfo, fmt.Sprintf("NowPlaying(user=%q, id=%s, %q)",
		input.Username, input.Track.ID, input.Track.Title))
	cfg, sessionKey, skip, _, err := resolve(input.Username, input.Track)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	if err := cfg.updateNowPlaying(sessionKey, input.Track); err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("NowPlaying failed for %q: %v", input.Track.Title, err))
		return err
	}
	return nil
}

// Scrobble submits a completed scrobble to Last.fm, unless the track is blacklisted.
func (p *exclusionPlugin) Scrobble(input scrobbler.ScrobbleRequest) error {
	pdk.Log(pdk.LogInfo, fmt.Sprintf("Scrobble(user=%q, id=%s, %q)",
		input.Username, input.Track.ID, input.Track.Title))
	cfg, sessionKey, skip, reason, err := resolve(input.Username, input.Track)
	if err != nil {
		return err
	}
	if skip {
		if reason != "" {
			pdk.Log(pdk.LogInfo, fmt.Sprintf("skipping blacklisted track (%s - %s): %s",
				primaryArtist(input.Track), input.Track.Title, reason))
		}
		return nil
	}
	if err := cfg.scrobble(sessionKey, input.Track, input.Timestamp); err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("Scrobble failed for %q: %v", input.Track.Title, err))
		return err
	}
	return nil
}

// PlaybackReport is not needed for Last.fm scrobbling (Last.fm uses NowPlaying +
// Scrobble). It is a required interface method, so it is a no-op.
func (p *exclusionPlugin) PlaybackReport(_ scrobbler.PlaybackReportRequest) error {
	return nil
}

// resolve loads config and works out, for a given user+track, the Last.fm config,
// the user's session key, whether the track should be skipped, and (when skipped for a
// blacklist reason) a short reason string. It returns a NotAuthorized error only when
// the user has no session key, so Navidrome can surface a re-auth prompt.
func resolve(username string, track scrobbler.TrackInfo) (cfg lastfmConfig, sessionKey string, skip bool, reason string, err error) {
	cfg, users, bl, err := getConfig()
	if err != nil {
		pdk.Log(pdk.LogError, err.Error())
		// Configuration problems are transient (admin can fix them); ask Navidrome to retry.
		return cfg, "", false, "", scrobbler.ScrobblerErrorRetryLater
	}

	sessionKey, ok := users[normalizeText(username)]
	if !ok {
		return cfg, "", true, "", scrobbler.ScrobblerErrorNotAuthorized
	}

	if blacklisted, why := bl.match(track); blacklisted {
		return cfg, sessionKey, true, why, nil
	}
	return cfg, sessionKey, false, "", nil
}
