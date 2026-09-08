package main

import (
	"testing"

	"github.com/navidrome/navidrome/plugins/pdk/go/scrobbler"
)

func TestSign(t *testing.T) {
	// Reference value computed independently:
	//   md5("api_key" + "abc" + "method" + "auth.getToken" + "xyz")
	// Params must be sorted by name and concatenated as name+value, then the shared
	// secret appended. format/api_sig must be excluded from the signature.
	const want = "2a379a844d6cae900cab08529c2a183c"

	params := map[string]string{
		"method":  "auth.getToken",
		"api_key": "abc",
		"format":  "json",        // must be ignored
		"api_sig": "stale-value", // must be ignored
	}
	if got := sign(params, "xyz"); got != want {
		t.Fatalf("sign() = %q, want %q", got, want)
	}
}

func TestMapLastfmError(t *testing.T) {
	cases := []struct {
		name   string
		code   int
		status int
		want   error
	}{
		{"invalid session key", 9, 200, scrobbler.ScrobblerErrorNotAuthorized},
		{"invalid api key", 10, 200, scrobbler.ScrobblerErrorUnrecoverable},
		{"service offline", 11, 200, scrobbler.ScrobblerErrorRetryLater},
		{"rate limited", 29, 200, scrobbler.ScrobblerErrorRetryLater},
		{"http 401", 0, 401, scrobbler.ScrobblerErrorNotAuthorized},
		{"http 503", 0, 503, scrobbler.ScrobblerErrorRetryLater},
		{"unknown 400", 0, 400, scrobbler.ScrobblerErrorUnrecoverable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapLastfmError(tc.code, tc.status, ""); got != tc.want {
				t.Errorf("mapLastfmError(%d, %d) = %v, want %v", tc.code, tc.status, got, tc.want)
			}
		})
	}
}

func TestPrimaryArtist(t *testing.T) {
	tests := []struct {
		name  string
		track scrobbler.TrackInfo
		want  string
	}{
		{
			name:  "prefers first structured artist over display string",
			track: scrobbler.TrackInfo{Artist: "A • B", Artists: []scrobbler.ArtistRef{{Name: "A"}, {Name: "B"}}},
			want:  "A",
		},
		{
			name:  "falls back to display Artist when no structured artists",
			track: scrobbler.TrackInfo{Artist: "Solo Artist"},
			want:  "Solo Artist",
		},
		{
			name:  "skips empty structured names",
			track: scrobbler.TrackInfo{Artist: "Fallback", Artists: []scrobbler.ArtistRef{{Name: "  "}}},
			want:  "Fallback",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := primaryArtist(tc.track); got != tc.want {
				t.Errorf("primaryArtist() = %q, want %q", got, tc.want)
			}
		})
	}
}
