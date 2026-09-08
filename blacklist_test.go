package main

import (
	"testing"

	"github.com/navidrome/navidrome/plugins/pdk/go/scrobbler"
)

func TestNormalizeText(t *testing.T) {
	tests := map[string]string{
		"abc123":     "abc123",
		"  abc123  ": "abc123",
		"ABC123":     "abc123",
		"\tAbC123\n": "abc123",
		"":           "",
		"   ":        "",
	}
	for in, want := range tests {
		if got := normalizeText(in); got != want {
			t.Errorf("normalizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

// artist references two artists, used across match tests.
func trackFixture() scrobbler.TrackInfo {
	return scrobbler.TrackInfo{
		ID:     "Track-One",
		Title:  "Live at the Interlude (Skit)",
		Artist: "Main Artist • Featured Guest",
		Artists: []scrobbler.ArtistRef{
			{Name: "Main Artist"},
			{Name: "Featured Guest"},
		},
		AlbumArtist: "Main Artist",
	}
}

func TestMatchByID(t *testing.T) {
	bl := blacklists{ids: map[string]struct{}{normalizeText("track-one"): {}}}
	ok, reason := bl.match(trackFixture())
	if !ok {
		t.Fatalf("expected ID match, got none")
	}
	if reason == "" {
		t.Errorf("expected a reason for ID match")
	}
}

func TestMatchByArtist_FeaturedCounts(t *testing.T) {
	// Blacklisting a featured (non-primary) artist should still match.
	bl := blacklists{artists: map[string]struct{}{normalizeText("featured guest"): {}}}
	if ok, _ := bl.match(trackFixture()); !ok {
		t.Errorf("expected featured artist to match")
	}
}

func TestMatchByTitleSubstring(t *testing.T) {
	bl := blacklists{titleSubstrings: []string{normalizeText("skit")}}
	if ok, _ := bl.match(trackFixture()); !ok {
		t.Errorf("expected title substring to match")
	}
}

// artistsOf builds a TrackInfo whose structured Artists are the given names.
func artistsOf(names ...string) scrobbler.TrackInfo {
	t := scrobbler.TrackInfo{Title: "some title"}
	for _, n := range names {
		t.Artists = append(t.Artists, scrobbler.ArtistRef{Name: n})
	}
	return t
}

// TestUserExamples encodes the exact cases from the spec discussion.
func TestUserExamples(t *testing.T) {
	// Title filter = case-insensitive SUBSTRING of "apple".
	titleBL := blacklists{titleSubstrings: []string{normalizeText("apple")}}
	titleCases := map[string]bool{
		"Apple 123":          true,  // substring
		"ApPlE123":           true,  // substring, different case
		"Grape Apple Orange": true,  // substring in the middle
		"Banana Split":       false, // no "apple"
	}
	for title, want := range titleCases {
		tr := scrobbler.TrackInfo{Title: title}
		if got, _ := titleBL.match(tr); got != want {
			t.Errorf("title %q: got %v, want %v", title, got, want)
		}
	}

	// Artist filter = case-insensitive EXACT per-artist, entries {"orange123","orange"}.
	artistBL := blacklists{artists: map[string]struct{}{
		normalizeText("orange123"): {},
		normalizeText("orange"):    {},
	}}
	artistCases := []struct {
		names []string
		want  bool
	}{
		{[]string{"Apple", "Orange123", "Grape"}, true}, // Orange123 credited among others
		{[]string{"Orange123"}, true},                   // exact
		{[]string{"Orange"}, true},                      // exact
		{[]string{"ORANGE123"}, true},                   // case-insensitive
		{[]string{"Apple", "Grape"}, false},             // neither listed artist present
	}
	for _, c := range artistCases {
		if got, _ := artistBL.match(artistsOf(c.names...)); got != c.want {
			t.Errorf("artists %v: got %v, want %v", c.names, got, c.want)
		}
	}
}

func TestNoMatch(t *testing.T) {
	bl := blacklists{
		ids:             map[string]struct{}{"other-id": {}},
		artists:         map[string]struct{}{"someone else": {}},
		titleSubstrings: []string{"remix"},
	}
	if ok, reason := bl.match(trackFixture()); ok {
		t.Errorf("expected no match, got true (%s)", reason)
	}
}
