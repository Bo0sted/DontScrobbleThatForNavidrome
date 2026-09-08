package main

import (
	"strings"

	"github.com/navidrome/navidrome/plugins/pdk/go/scrobbler"
)

// blacklists holds the three independent exclusion filters. A track is skipped if it
// matches ANY of them:
//
//   - ids: the Navidrome track ID matches exactly (case-insensitive).
//   - artists: any of the track's artists matches an entry exactly (case-insensitive),
//     regardless of how many other artists the track also has.
//   - titleSubstrings: the track title contains an entry as a substring (case-insensitive).
type blacklists struct {
	ids             map[string]struct{}
	artists         map[string]struct{}
	titleSubstrings []string
}

// match reports whether a track should be excluded and, if so, a short human-readable
// reason for logging.
func (b blacklists) match(track scrobbler.TrackInfo) (bool, string) {
	if _, ok := b.ids[normalizeText(track.ID)]; ok {
		return true, "track id " + track.ID
	}

	for _, name := range trackArtistNames(track) {
		if _, ok := b.artists[normalizeText(name)]; ok {
			return true, "artist " + name
		}
	}

	title := normalizeText(track.Title)
	for _, sub := range b.titleSubstrings {
		if strings.Contains(title, sub) {
			return true, "title contains \"" + sub + "\""
		}
	}

	return false, ""
}

// normalizeText canonicalizes a value for comparison: trimmed and lowercased. This
// makes matching case-insensitive and forgiving of stray whitespace.
func normalizeText(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// trackArtistNames returns every distinct artist name associated with a track:
// structured track artists and album artists, plus the display strings as a fallback
// for tracks that lack structured artist data.
func trackArtistNames(track scrobbler.TrackInfo) []string {
	var names []string
	add := func(n string) {
		if strings.TrimSpace(n) != "" {
			names = append(names, n)
		}
	}
	for _, a := range track.Artists {
		add(a.Name)
	}
	for _, a := range track.AlbumArtists {
		add(a.Name)
	}
	add(track.Artist)
	add(track.AlbumArtist)
	return names
}

// primaryArtist returns the single best artist name to send to Last.fm. The display
// Artist field joins multiple artists with " • ", which Last.fm should not receive,
// so prefer the first structured artist when available.
func primaryArtist(track scrobbler.TrackInfo) string {
	for _, a := range track.Artists {
		if strings.TrimSpace(a.Name) != "" {
			return a.Name
		}
	}
	return track.Artist
}
