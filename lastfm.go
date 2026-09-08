package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
	"github.com/navidrome/navidrome/plugins/pdk/go/scrobbler"
)

// lastfmAPIURL is the Last.fm 2.0 API endpoint. The host must be listed in the
// manifest's http.requiredHosts.
const lastfmAPIURL = "https://ws.audioscrobbler.com/2.0/"

// lastfmConfig holds the application-level Last.fm credentials (not per-user).
type lastfmConfig struct {
	APIKey    string
	APISecret string
}

// updateNowPlaying calls the Last.fm track.updateNowPlaying method.
func (c lastfmConfig) updateNowPlaying(sessionKey string, track scrobbler.TrackInfo) error {
	params := map[string]string{
		"method": "track.updateNowPlaying",
		"artist": primaryArtist(track),
		"track":  track.Title,
	}
	if track.Album != "" {
		params["album"] = track.Album
	}
	if track.Duration > 0 {
		params["duration"] = strconv.Itoa(int(track.Duration))
	}
	return c.call(sessionKey, params)
}

// scrobble calls the Last.fm track.scrobble method for a completed play. timestamp is
// the Unix time (seconds) at which the track started playing.
func (c lastfmConfig) scrobble(sessionKey string, track scrobbler.TrackInfo, timestamp int64) error {
	params := map[string]string{
		"method":    "track.scrobble",
		"artist":    primaryArtist(track),
		"track":     track.Title,
		"timestamp": strconv.FormatInt(timestamp, 10),
	}
	if track.Album != "" {
		params["album"] = track.Album
	}
	if track.AlbumArtist != "" {
		params["albumArtist"] = track.AlbumArtist
	}
	if track.TrackNumber > 0 {
		params["trackNumber"] = strconv.Itoa(int(track.TrackNumber))
	}
	if track.MBZRecordingID != "" {
		params["mbid"] = track.MBZRecordingID
	}
	return c.call(sessionKey, params)
}

// call signs and POSTs a request to the Last.fm API and interprets the response,
// mapping Last.fm error codes to the scrobbler error types Navidrome understands.
func (c lastfmConfig) call(sessionKey string, params map[string]string) error {
	params["api_key"] = c.APIKey
	params["sk"] = sessionKey

	// api_sig must be computed over every parameter EXCEPT format/callback, then the
	// format is added only to the transmitted body.
	params["api_sig"] = sign(params, c.APISecret)
	params["format"] = "json"

	body := url.Values{}
	for k, v := range params {
		body.Set(k, v)
	}

	resp, err := host.HTTPSend(host.HTTPRequest{
		Method:  "POST",
		URL:     lastfmAPIURL,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Body:    []byte(body.Encode()),
	})
	if err != nil {
		// Network/permission failure: transient, let Navidrome retry.
		pdk.Log(pdk.LogWarn, fmt.Sprintf("lastfm %s: HTTP send error: %v", params["method"], err))
		return scrobbler.ScrobblerErrorRetryLater
	}

	// Last.fm returns HTTP 200 with a JSON error body for API-level errors, and 4xx
	// for some auth failures. Parse the body either way to extract the error code.
	var parsed struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(resp.Body, &parsed)

	if parsed.Error == 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		pdk.Log(pdk.LogInfo, fmt.Sprintf("lastfm %s: OK (HTTP %d)", params["method"], resp.StatusCode))
		return nil
	}
	pdk.Log(pdk.LogWarn, fmt.Sprintf("lastfm %s: HTTP %d, error=%d msg=%q body=%s",
		params["method"], resp.StatusCode, parsed.Error, parsed.Message, snippet(resp.Body)))
	return mapLastfmError(parsed.Error, int(resp.StatusCode), parsed.Message)
}

// snippet trims a response body to a loggable length.
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// mapLastfmError translates a Last.fm error code (or HTTP status) into the scrobbler
// error the host expects. See https://www.last.fm/api/errorcodes.
func mapLastfmError(code int, status int, message string) error {
	switch code {
	case 9: // Invalid session key - user must re-authenticate.
		return scrobbler.ScrobblerErrorNotAuthorized
	case 4, 10, 13, 26: // Auth/key/signature problems - not fixable by retrying.
		return scrobbler.ScrobblerErrorUnrecoverable
	case 11, 16: // Service offline / temporarily unavailable.
		return scrobbler.ScrobblerErrorRetryLater
	case 29: // Rate limit exceeded.
		return scrobbler.ScrobblerErrorRetryLater
	}
	if status == 401 || status == 403 {
		return scrobbler.ScrobblerErrorNotAuthorized
	}
	if status >= 500 {
		return scrobbler.ScrobblerErrorRetryLater
	}
	// Unknown 4xx or error code: treat as unrecoverable so we don't retry forever.
	return scrobbler.ScrobblerErrorUnrecoverable
}

// sign computes the Last.fm api_sig: sort the parameters by name, concatenate
// name+value for each (excluding format and callback), append the shared secret, and
// take the lowercase hex MD5. This matches the native integration's signing.
func sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "format" || k == "callback" || k == "api_sig" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(secret)

	sum := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
