# Fake SNI presets

Each `*.txt` file here is a signed list in Ghostline's rule format (first line
`# ghostline-rules v1`). The app downloads it from
`https://raw.githubusercontent.com/hashcott/ghostline/main/lists/fakesni/<name>.txt`,
verifies `<name>.txt.sig`, and falls back to the copy built into the binary
(also verified). An unsigned or tampered file is never used.

Preset content is decided by testing, not by guesswork: CDNs change what they
accept, so a group that worked last month may not work now.

## Testing a group

Use a network that blocks the sites in the group (with any DPI bypass off).

1. Build and run Ghostline, Connect.
2. Proxy page: turn on the proxy and "use for this PC". DPI page: turn both
   engines off.
3. Fake SNI page: read and confirm the warning, turn Fake SNI on, turn the
   group on.
4. In Chrome, open each site of the group. Keep the group only if pages load
   and media plays (for YouTube: a full video, not only the page).
5. Fake SNI page counters: "decrypted" must grow; "fell back to fragment" and
   "bad server certificate" should stay near zero. Lines that always fall back
   are removed from the file.
6. Record the result below (date, network, sites, outcome).

## Signing

Only the maintainer holds the key (the same one as `lists/servers.json`):

```
go run ./tools/genservers -sign-file lists/fakesni/google.txt -sign-env SERVERLIST_SIGNING_KEY
```

This writes `google.txt.sig`. Commit both files. `GHOSTLINE_RELEASE=1 go test ./lists/`
fails while any preset is unsigned.

## Results

| Date | Network | Group | Outcome |
|---|---|---|---|
| — | — | Google & YouTube | not tested yet |
