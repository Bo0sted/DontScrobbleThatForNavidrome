# Navidrome Last.fm Scrobble Exclusion Plugin

A [Navidrome](https://www.navidrome.org/) WebAssembly plugin that scrobbles your
plays to **Last.fm** exactly like the native integration — **except it silently skips
any track whose Navidrome track ID is on a blacklist.** Blacklisted tracks are never
sent to Last.fm (no "now playing", no scrobble); everything else scrobbles normally.

Structured after the
[discord-rich-presence-plugin](https://github.com/Bo0sted/discord-rich-presence-plugin).

---

## Why it works this way (important)

A Navidrome scrobbler plugin is **additive**: Navidrome sends every play to *all*
registered scrobblers independently, and a plugin **cannot veto or filter** the
built-in Last.fm agent. There is no "intercept and drop" hook.

So to get a blacklist, this plugin has to **become** your Last.fm scrobbler and
re-implement the Last.fm Scrobbling API itself (the signed `track.updateNowPlaying` /
`track.scrobble` calls). It then simply returns early, without calling Last.fm, when a
track is blacklisted.

That means two things:

1. **You must disable Navidrome's native Last.fm scrobbler** (below), or every track —
   including blacklisted ones — will still be scrobbled by the built-in agent.
2. **Authentication is per user.** An API key alone is not enough: Last.fm requires a
   per-user **session key**. Navidrome stores its own session keys internally and a
   plugin can't read them, so each user mints a session key for the plugin once using
   the included [`session-key` helper](#3-get-a-session-key-per-user).

---

## Requirements

- Navidrome with the plugin system enabled.
- A Last.fm API account: <https://www.last.fm/api/account/create> (gives you an **API
  key** and a **shared secret**).
- To build: [Go 1.25+](https://go.dev/) and [TinyGo](https://tinygo.org/) (recommended).

---

## Build

```bash
make package
```

This produces `navidrome-lastfm-exclusion.ndp` (a zip of `plugin.wasm` +
`manifest.json`). If TinyGo isn't installed, the Makefile falls back to the standard
Go WASM compiler.

## Install

1. Copy `navidrome-lastfm-exclusion.ndp` into your Navidrome plugins folder.
2. Enable plugins in your Navidrome config:

   ```toml
   [Plugins]
   Enabled = true
   Folder  = "/path/to/plugins"
   ```

3. Restart Navidrome. The plugin appears under **Settings → Plugins → Last.fm Scrobble
   Exclusion**.

---

## Configuration

### 1. Disable the native Last.fm scrobbler

In your Navidrome config, turn off the built-in Last.fm integration so this plugin is
the *only* thing scrobbling to Last.fm:

```toml
[LastFM]
Enabled = false
```

(Or via env: `ND_LASTFM_ENABLED=false`.) Restart Navidrome.

> If you skip this step, blacklisted tracks will still be scrobbled by the native
> agent and the blacklist will appear to do nothing.

### 2. Enter your API credentials

In the plugin settings, fill in:

- **Last.fm API Key**
- **Last.fm Shared Secret**

### 3. Get a session key (per user)

Run the helper once for each Navidrome user that should scrobble:

```bash
make session-key APIKEY=<your_api_key> APISECRET=<your_shared_secret>
```

It prints a URL — open it in your browser, click **"Yes, allow access"**, come back
and press Enter. The helper prints a **session key**. Add an entry under **User Session
Keys** in the plugin config mapping the **Navidrome username** to that **session key**.

> Repeat per user. Each user's session key ties scrobbles to *their* Last.fm account.

### 4. Assign the plugin to users

Scrobbler plugins only receive events for users assigned to them. Assign the plugin to
the relevant users in Navidrome's plugin/user configuration (same as any scrobbler
plugin).

### 5. Configure the blacklist

There are **three independent filters**. A track is excluded if it matches **any** of
them (they're OR'd together), and every track is checked against all three before it
would be scrobbled:

| Filter | Config field | Matches when… | Example |
|--------|--------------|---------------|---------|
| **Track ID** | Blacklisted Track IDs | the track's Navidrome ID equals an entry | `2a4f9c…` |
| **Artist** | Blacklisted Artists | *any* artist on the track (including featured artists) equals an entry | `Rick Astley` |
| **Title word** | Blacklisted Title Words | the track title *contains* an entry as a substring | `interlude`, `skit`, `intro` |

All matching is case-insensitive. Artist matching is exact per-artist name — a track
credited to "A feat. B" is excluded by a blacklist entry for either `A` **or** `B`.
Title matching is a substring, so `skit` also excludes "Album Skit" and "Skit 2".

**Finding a track's ID:** a track's ID is the `id` returned by Navidrome's Subsonic
API (e.g. `search3`) or visible in the track's share URL.

---

## How it behaves

| Event | Blacklisted track | Normal track |
|-------|-------------------|--------------|
| Now playing | not sent | `track.updateNowPlaying` |
| Completed play | not sent (logged as skipped) | `track.scrobble` |

Last.fm/API errors are mapped to Navidrome's scrobbler retry semantics: invalid
session key → *not authorized* (re-auth), rate limit / service down → *retry later*,
bad key/signature → *unrecoverable*.

---

## Development

```bash
make test     # go test -race ./...
make build    # compile plugin.wasm only
make package  # build + zip into .ndp
make clean
```

Source layout:

| File | Purpose |
|------|---------|
| `main.go` | Plugin registration, config loading, Scrobbler methods, blacklist gate |
| `lastfm.go` | Last.fm API client: request signing, `updateNowPlaying`, `scrobble`, error mapping |
| `blacklist.go` | The three-filter matcher (ID / artist / title), normalization, primary-artist selection |
| `manifest.json` | Plugin metadata, permissions, and config UI schema |
| `cmd/session-key/` | Standalone helper to mint a per-user Last.fm session key |

---

## License

[MIT](LICENSE)
