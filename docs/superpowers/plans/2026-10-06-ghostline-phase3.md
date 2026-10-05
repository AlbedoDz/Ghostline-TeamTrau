# Ghostline Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A "Tools" page (DNS Lookup, Advanced DNS Scanner, Cloudflare clean-IP scan, STAMP tool) and settings import/export — released as v0.5.0.

**Architecture:** Five pure-Go packages (`lookup`, `scanner/advanced`, `cfscan`, `stamps`, `backup`) that know nothing about `app`, Wails or Win32; each takes its network access as a function (`Build` upstream, `Dial`). `app` adds bound methods on the existing `*Service` that run each tool outside `Orchestrator.opMu`, one job per tool at a time, with progress over events. The frontend adds one Advanced-mode page with four tabs plus a backup section in Settings.

**Tech Stack:** Go 1.26 (`net/netip`, `crypto/tls`, `embed`), `miekg/dns`, `AdguardTeam/dnsproxy/upstream`, `ameshkov/dnsstamps` (all already dependencies), Wails v3, React + Vitest.

**Spec:** `docs/superpowers/specs/2026-10-06-ghostline-phase3-design.md` (read it with this plan; section numbers below refer to it).

## Global Constraints

- No new Go module dependencies, no new npm dependencies, no new binaries (spec 14).
- Tools never take `Orchestrator.opMu`; Connect/Disconnect neither wait for nor cancel them. One running job per tool (`advanced`, `cfscan`); a second start → `TOOL_BUSY{tool}`.
- Plain DNS is sent **only** by Lookup source kind `isp` (UDP 53). Scanner input goes through `servers.ParseImport`, which rejects plain.
- Lookup: types `A AAAA CNAME MX TXT NS SOA HTTPS CAA PTR`; DO bit set; 5 s per source; compare mode 2–6 sources.
- Advanced scan: rounds 3–20 (default 5), workers 4–32 (8), timeout 1000–10000 ms (3000), max 500 servers, whole scan ≤ 10 min, ≤ 10 progress events/s.
- Clean IP: IPv4 only, from embedded `internal/cfscan/ranges_v4.txt`; 1 random IP per /24; maxIps 200–10000 (2000); want 0–1000 (50); concurrency 8–128 (64); ≤ 200 new TCP connections/s; timeout 1000–5000 ms (2000); port 443 only; speed test top 10, `speedBytes` 102400–26214400 (1048576), 10 s per IP, sequential; cache keeps top 100 per network.
- Default probe host `speed.cloudflare.com`; server certificate must verify against system roots for that host. Never `InsecureSkipVerify` without a full verify.
- `settings.json` v5 adds `tools` (spec 11). v1–v4 still load.
- Backup file: `format: "ghostline-backup"`, `formatVersion: 1`, max 8 MB, default name `ghostline-<yyyy-mm-dd>.ghostline.json`.
- Never exported: `adapterGuids`, `advancedWindow`, `dnsServer.iosSsid`, `proxy.upstreams[].passEnc`, state, caches, CA files, list caches, logs.
- On import always: `fakeSni.enabled=false`, `dnsServer.enabled=false`, `dnsServer.shareLan=false`, `proxy.shareLan=false`, `startWithWindows=false`, `fakeSni.ackVersion` kept from this machine, `trustedForSNI=false` except lists with `signed: true`; refused while not disconnected.
- Error codes (spec 12): `TOOL_BUSY`, `LOOKUP_NOT_CONNECTED`, `LOOKUP_BAD_NAME`, `SCAN_TOO_MANY`, `CFSCAN_NO_NETWORK`, `CFSCAN_HOST_INVALID`, `STAMP_INVALID`, `IMPORT_INVALID`, `IMPORT_WHILE_CONNECTED`, `IMPORT_EXPIRED`, `IMPORT_WRITE_FAILED`, `EXPORT_WRITE_FAILED`.
- Every new UI string exists in both `frontend/src/i18n/vi.json` and `en.json` (`parity.test.ts`).
- Commits never include a `Co-Authored-By` trailer (user preference).
- Go tests: `go test ./...` (with `-race` for `app`, `cfscan`, `scanner/...`); network tests use build tag `integration`. Frontend: `cd frontend && npm test -- --run`.

**Deviation from spec 4.2:** the tool methods live on the existing `*Service` (in `toolsservice.go`, `cfservice.go`, `backupservice.go`) rather than a second Wails service, because the frontend has a single generated binding module (`frontend/src/app/api.ts`). Task 20 updates the spec line.

## Review Focus

1. **Lookup name input with a trailing dot, upper case, IDN (`bücher.de`), an IP for a non-PTR type, or a URL pasted from the browser** — normalise (`https://x.com/a` → `x.com`, IDN → punycode, lower case) or reject with `LOOKUP_BAD_NAME`, never send garbage; Task 5 `TestQueryName_Normalises`.
2. **Clean-IP scan on a network where port 443 to Cloudflare is blackholed (every dial times out)** — must end with `CFSCAN_NO_NETWORK` after the first 200 attempts, not run 2000 × 2 s; Task 9 `TestScan_NoNetworkStopsEarly`.
3. **Import of a file exported by v0.5 on a machine whose data dir is missing `rules.json` or `servers-custom.json`** — apply must create them, and rollback after a later failure must delete them again (no `.bak-import` exists to restore); Task 12 `TestApply_RollbackRemovesNewFiles`.
4. **Starting a tool, then Disconnect / Connect / closing to tray mid-scan** — tool keeps running or cancels cleanly, never deadlocks Connect; Task 14 `TestTools_DoNotBlockConnect`.
5. **Rule creation from clean IPs with a pattern the rule parser rejects (`*.*.com`, a regexp, a CIDR) or a pattern that already has an `ip=` rule** — line errors shown, nothing partial written; duplicates skipped; Task 2 `TestAppendUserRules_RejectsBadAndDedups`.

---

## File Structure

```
internal/store/settings.go          + ToolsSettings, ValidateTools, MigrateSettings, v5
internal/store/paths.go             + CFScanCache
internal/rules/append.go            AppendUserRules
internal/scanner/query.go           Exchange (shared one-query helper)
internal/stamps/{decode,encode}.go  Fields, Decode, Encode, FromURL
internal/lookup/{query,dig,compare,cdn}.go
internal/scanner/advanced/{checks,scan}.go
internal/cfscan/{ranges.go,ranges_v4.txt,sample.go,probe.go,limiter.go,scan.go,speed.go,cache.go}
internal/backup/{format,build,plan,apply}.go
internal/app/{toolsservice,advservice,cfservice,backupservice}.go  + errors.go, events.go, service.go (deps)
internal/shell/toolswire.go          deps for the above
internal/cli/cli.go, headless.go     --export
tools/gencfranges/main.go
frontend/src/modes/advanced/pages/tools/{Tools,Lookup,Scanner,CfScan,Stamp}.tsx (+ tests)
frontend/src/modes/advanced/pages/settings/Backup.tsx (+ test)
```

---

### Task 1: Store — settings v5 (`tools`), `MigrateSettings`, cache path

**Files:**
- Modify: `internal/store/settings.go`, `internal/store/paths.go`
- Test: `internal/store/v5_test.go`

**Interfaces:**
- Produces:

```go
type ToolsSettings struct {
    Scanner ScannerTool `json:"scanner"`
    CFScan  CFScanTool  `json:"cfscan"`
}
type ScannerTool struct { Rounds int `json:"rounds"`; Workers int `json:"workers"`; TimeoutMs int `json:"timeoutMs"` }
type CFScanTool struct {
    Host string `json:"host"`; MaxIPs int `json:"maxIps"`; Want int `json:"want"`
    Concurrency int `json:"concurrency"`; TimeoutMs int `json:"timeoutMs"`
    SpeedTest bool `json:"speedTest"`; SpeedBytes int `json:"speedBytes"`
}
// Settings gains: Tools ToolsSettings `json:"tools"`
func ValidateTools(t ToolsSettings) error
func MigrateSettings(b []byte) (Settings, error) // BOM-tolerant; error only for invalid JSON
// Paths gains CFScanCache = <DataDir>\cfscan-cache.json
```

- [ ] **Step 1: Write failing tests** — `TestSettingsV4ToV5` (a v4 file loads with `Version==5`, `Tools` equal to the spec 11 defaults, every v4 field unchanged); `TestMigrateSettings_V1ToV5` (reuses a v1 fixture from `store_test.go`: engine becomes goodbyedpi, version 5); `TestMigrateSettings_BadJSON` (error, no panic); `TestValidateTools` table: rounds 2/21, workers 3/33, scanner timeout 999/10001, maxIps 199/10001, want -1/1001, concurrency 7/129, cf timeout 999/5001, speedBytes 102399/26214401, host `""`, `1.1.1.1`, `a..b`, 254-char name → error; defaults → nil.
- [ ] **Step 2: Run** `go test ./internal/store/` — FAIL (undefined `ToolsSettings`).
- [ ] **Step 3: Implement.** Move the body of `LoadSettings` after the read into `MigrateSettings`; `LoadSettings` keeps the rename-to-`.bak` on error. Fill a zero `Tools` block (and zero sub-fields) with defaults during migration. `DefaultSettings().Version = 5`. Host check: `net.ParseIP(host)==nil`, labels 1–63 chars `[a-z0-9-]`, not starting/ending with `-`, total ≤ 253.
- [ ] **Step 4: Run** `go test ./internal/store/` — PASS (including v2/v3/v4 tests, now asserting version 5).
- [ ] **Step 5: Wire validation** — call `store.ValidateTools(n.Tools)` in `Service.saveSettings` (`internal/app/service.go:162`) next to `ValidateProxy`; add `TestSaveSettings_RejectsBadTools` in `internal/app/service_test.go`; run `go test ./internal/app/`.
- [ ] **Step 6: Commit** — `git commit -m "feat(store): settings v5 with tool options; shared settings migration"`

---

### Task 2: Rules — `AppendUserRules`

**Files:**
- Create: `internal/rules/append.go`
- Test: `internal/rules/append_test.go`

**Interfaces:**
- Produces: `func AppendUserRules(existing, add []Rule) (out []Rule, added int, errs []LineError)` — validates each of `add` with the same checks the table editor uses (1-based index in `add` as `Line`); if any error, returns `existing` unchanged, `added=0`; otherwise appends rules not already present (same `Pattern` and equal `Action`), keeps order, fails with one `LineError{Line:0, Msg:"too many rules"}` when the result would exceed `MaxUserRules`.

- [ ] **Step 1: Write failing tests** — `TestAppendUserRules_AppendsInOrder`; `TestAppendUserRules_RejectsBadAndDedups` (`*.*.com`, `/re/`, `10.0.0.0/8` with `ip=` → errors and nothing appended; identical rule twice → added once; same pattern different IPs → both kept); `TestAppendUserRules_Cap`.
- [ ] **Step 2: Run** `go test ./internal/rules/ -run Append` — FAIL.
- [ ] **Step 3: Implement** using the existing per-rule validator in `internal/rules` (the one `SaveRulesTable` relies on; compare actions with `reflect.DeepEqual` or a field compare including `IPs`).
- [ ] **Step 4: Run** `go test ./internal/rules/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(rules): append user rules with validation and de-duplication"`

---

### Task 3: Scanner — shared `Exchange`

**Files:**
- Create: `internal/scanner/query.go`
- Modify: `internal/scanner/scan.go` (`DNSChecker.Check` uses `Exchange`)
- Test: `internal/scanner/query_test.go`

**Interfaces:**
- Produces: `func Exchange(ctx context.Context, u upstream.Upstream, name string, qtype uint16, do bool) (*dns.Msg, time.Duration, error)` — builds `SetQuestion(dns.Fqdn(name), qtype)`, `SetEdns0(1232, do)` when `do`, RD set; returns duration of the exchange. `func Classify(err error, ctxErr error) string` returns `"bootstrap" | "timeout" | "error"` (moved out of `Check`).

- [ ] **Step 1: Write failing test** — `TestExchange_SetsDO` with a fake `upstream.Upstream` capturing the message: `do=true` → OPT record with DO bit; `do=false` → no OPT. `TestClassify` for a bootstrap error string, `context.DeadlineExceeded`, a `net.Error` timeout, other.
- [ ] **Step 2: Run** `go test ./internal/scanner/` — FAIL.
- [ ] **Step 3: Implement**; refactor `Check` to call `Exchange`/`Classify` with identical behaviour.
- [ ] **Step 4: Run** `go test -race ./internal/scanner/` — PASS (old tests unchanged).
- [ ] **Step 5: Commit** — `git commit -m "refactor(scanner): shared single-query helper"`

---

### Task 4: `internal/stamps`

**Files:**
- Create: `internal/stamps/decode.go`, `encode.go`
- Modify: `internal/servers/stamp.go` (`FromStamp` reads protocol/IP/props from `stamps.Decode`)
- Test: `internal/stamps/stamps_test.go`, `internal/stamps/fuzz_test.go`

**Interfaces:**
- Produces:

```go
type Fields struct {
    Proto        string   `json:"proto"`  // doh dot doq dnscrypt plain odoh-target odoh-relay dnscrypt-relay doh-relay
    Addr         string   `json:"addr"`   // "ip" or "ip:port", may be empty for doh/dot/doq
    Host         string   `json:"host"`   // doh/dot/doq/odoh hostname (may include :port)
    Path         string   `json:"path"`
    ProviderName string   `json:"providerName"` // dnscrypt
    PublicKey    string   `json:"publicKey"`    // dnscrypt, hex
    Hashes       []string `json:"hashes"`       // hex SHA-256 of TBS certs
    DNSSEC       bool     `json:"dnssec"`
    NoLog        bool     `json:"noLog"`
    NoFilter     bool     `json:"noFilter"`
    Stamp        string   `json:"stamp"`  // normalised sdns:// string (output)
    Usable       bool     `json:"usable"` // Ghostline can add it (doh dot doq dnscrypt)
}
func Decode(s string) (Fields, error)
func Encode(f Fields) (string, error)           // validates, encodes, decodes again and compares; mismatch → error
func FromURL(raw, ip string) (Fields, error)    // https://, tls://, quic://
var ErrInvalid = errors.New("stamps: invalid")  // wrapped with the field name
```

- [ ] **Step 1: Write failing tests** — `TestRoundTrip` table for doh (with hashes and path), dot, doq, dnscrypt (provider `2.dnscrypt-cert.example`, 32-byte key), plain: `Decode(Encode(f))` equals `f` except `Stamp`/`Usable`; `TestDecode_AllShippedStamps` iterates every `sdns://` in `lists/` fixtures and `internal/servers` testdata and the embedded `servers.json` → no error; `TestDecode_RelayNotUsable`; `TestEncode_Rejects` (hash 31 bytes, key 31 bytes, provider without `2.dnscrypt-cert.` prefix, `Addr` `300.1.1.1`, doh without host) → `errors.Is(err, ErrInvalid)` and message names the field; `TestFromURL` (`https://dns.google/dns-query` + `8.8.8.8` → doh, host `dns.google`, path `/dns-query`, addr `8.8.8.8`; `tls://one.one.one.one`; `quic://dns.adguard-dns.com`; `http://x` → error). `FuzzDecode` must never panic.
- [ ] **Step 2: Run** `go test ./internal/stamps/` — FAIL.
- [ ] **Step 3: Implement** over `dnsstamps.ServerStamp` (`NewServerStampFromString`, `.String()`); props map to `ServerInformalPropertyDNSSEC/NoLog/NoFilter`.
- [ ] **Step 4: Switch `servers.FromStamp`** to `stamps.Decode`; run `go test ./internal/servers/ ./internal/stamps/` — PASS, server tests unchanged.
- [ ] **Step 5: Commit** — `git commit -m "feat(stamps): decode and encode DNS stamps"`

---

### Task 5: `internal/lookup` — query and `dig` text

**Files:**
- Create: `internal/lookup/query.go`, `internal/lookup/dig.go`
- Test: `internal/lookup/query_test.go`, `internal/lookup/testdata/dig_*.golden`

**Interfaces:**
- Consumes: `scanner.Exchange`, `scanner.Classify` (Task 3).
- Produces:

```go
type Source struct { Kind string `json:"kind"`; Ref string `json:"ref"`; Label string `json:"label"` } // kind: ghostline | server | address | isp
type Record struct { Name string `json:"name"`; Type string `json:"type"`; TTL uint32 `json:"ttl"`; Data string `json:"data"` }
type Answer struct {
    Source Source `json:"source"`; OK bool `json:"ok"`; Error string `json:"error,omitempty"` // bootstrap|timeout|error
    Rcode string `json:"rcode"`; Records []Record `json:"records"`
    AD bool `json:"ad"`; TC bool `json:"tc"`; RA bool `json:"ra"`
    LatencyMs int64 `json:"latencyMs"`; Dig string `json:"dig"`
}
var Types = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "HTTPS", "CAA", "PTR"}
var ErrBadName = errors.New("lookup: bad name")
func QueryName(input, qtype string) (string, error)
func Query(ctx context.Context, u upstream.Upstream, src Source, name, qtype string) Answer // 5 s deadline inside, DO set
func Dig(m *dns.Msg, latency time.Duration, src Source) string
```

- [ ] **Step 1: Write failing tests** — a `miekg/dns` server on `127.0.0.1:0` (helper `startDNS(t, handler)`) and `upstream.AddressToUpstream("udp://"+addr, nil)`:
  - `TestQuery_Types` — each of the 10 types returns its record and `Records[0].Type` matches; TTL preserved.
  - `TestQuery_Flags` — handler sets AD/TC → flags true; `OK` true for NXDOMAIN with `Rcode=="NXDOMAIN"`.
  - `TestQuery_Timeout` — handler sleeps 6 s → `Error=="timeout"` within 5.5 s.
  - `TestQueryName_Normalises` — `"Example.COM."`→`example.com`; `"https://x.com/a?b"`→`x.com`; `"bücher.de"`→`xn--bcher-kva.de`; PTR `"8.8.4.4"`→`4.4.8.8.in-addr.arpa`; PTR `"2001:db8::1"`→ip6.arpa form; `"1.2.3.4"` with type A → `ErrBadName`; `""`, `"a b"`, 254 chars → `ErrBadName`.
  - `TestDig_Golden` — fixed message → matches `testdata/dig_a.golden` (sections HEADER/flags/QUESTION/ANSWER/AUTHORITY/ADDITIONAL, `;; Query time: N msec`, `;; SERVER: <label>`).
- [ ] **Step 2: Run** `go test ./internal/lookup/` — FAIL.
- [ ] **Step 3: Implement.** IDN via `golang.org/x/net/idna` (`golang.org/x/net` is already a direct dependency in `go.mod`). `Dig` builds on `m.String()` sections with the extra header/footer lines.
- [ ] **Step 4: Run** `go test ./internal/lookup/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(lookup): single DNS query with dig-style output"`

---

### Task 6: `internal/lookup` — comparison verdict and CDN table

**Files:**
- Create: `internal/lookup/compare.go`, `internal/lookup/cdn.go`
- Test: `internal/lookup/compare_test.go`

**Interfaces:**
- Consumes: `scanner.IsPublicIP`.
- Produces:

```go
type Verdict string
const (VerdictPoisoned Verdict = "poisoned"; VerdictDiffers = "differs"; VerdictMatch = "match"; VerdictFailed = "failed"; VerdictNone = "") // None: non-A/AAAA types
func Compare(qtype string, as []Answer) (overall Verdict, per []Verdict)
func CDNOf(a netip.Addr) string // "cloudflare" | "google" | "akamai" | "fastly" | "cloudfront" | ""
```

- [ ] **Step 1: Write failing tests** — `TestCompare` table (spec 5.2): one source with `10.10.34.35` → that source `poisoned`, overall `poisoned`; NXDOMAIN while another has addresses → `poisoned`; `{104.16.1.1}` vs `{104.16.2.2}` → `match` (both cloudflare); `{1.1.1.1}` vs `{9.9.9.9}` → `differs`; same set in different order and TTL → `match`; one source failed + two matching → that one `failed`, overall `match`; all failed → overall `failed`; type MX → overall `""`. `TestCDNOf` — one address per provider plus a non-CDN address.
- [ ] **Step 2: Run** `go test ./internal/lookup/ -run 'Compare|CDN'` — FAIL.
- [ ] **Step 3: Implement.** CDN prefixes are a small literal table in `cdn.go` (Cloudflare from the same list Task 8 embeds — import `cfscan.Ranges()` is not allowed here to keep `lookup` independent; copy the ~15 Cloudflare v4 prefixes plus `2606:4700::/32`, Google `142.250.0.0/15`, `172.217.0.0/16`, `216.58.192.0/19`, Akamai `23.32.0.0/11`, `104.64.0.0/10`, Fastly `151.101.0.0/16`, CloudFront `13.32.0.0/15`, `18.64.0.0/14`, `52.84.0.0/15`).
- [ ] **Step 4: Run** `go test ./internal/lookup/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(lookup): compare answers across sources"`

---

### Task 7: `internal/scanner/advanced`

**Files:**
- Create: `internal/scanner/advanced/checks.go`, `internal/scanner/advanced/scan.go`
- Test: `internal/scanner/advanced/advanced_test.go`

**Interfaces:**
- Consumes: `scanner.DNSChecker`, `scanner.Exchange`, `scanner.IsPublicIP`.
- Produces:

```go
type Tri string // "yes" | "no" | "partial" | "unknown"
type Options struct {
    Rounds int; Timeout time.Duration; TestDomain string; PoisonDomains []string
    Label func() string             // random 8-char label; tests inject
    Sleep func(context.Context, time.Duration) error // 100 ms between rounds; tests inject
}
type Result struct {
    ServerID string `json:"serverId"`; Reach scanner.Result `json:"reach"`
    MinMs int64 `json:"minMs"`; MedianMs int64 `json:"medianMs"`; P90Ms int64 `json:"p90Ms"`
    JitterMs float64 `json:"jitterMs"`; Loss float64 `json:"loss"` // 0..1
    DNSSEC Tri `json:"dnssec"`; AdFilter Tri `json:"adFilter"`; Poisoned []string `json:"poisoned"`
}
type Checker struct { Build func(model.Server) (upstream.Upstream, error); Opt Options }
func (c Checker) Run(ctx context.Context, s model.Server) Result
func Sort(rs []Result)
var ErrTooMany = errors.New("advanced: more than 500 servers")
const MaxServers = 500
func Scan(ctx context.Context, list []model.Server, c Checker, workers int, onProgress func(done, total int, r Result)) ([]Result, error)
```

- [ ] **Step 1: Write failing tests** — fake upstream (`fakeUp` with a per-name/type handler and injected delays/errors):
  - `TestRun_LatencyStats` — 5 rounds with delays 10,20,30,40,50 ms (fake via handler + measured wall time is flaky; instead inject latencies through a fake `Exchange` seam: `Checker.exchange` field defaulting to `scanner.Exchange`) → min 10, median 30, p90 50, jitter ≈ 14.14, loss 0; random labels → each round queries `<label>.<testDomain>`.
  - `TestRun_Loss` — 2 of 5 rounds time out → loss 0.4; NXDOMAIN rounds count as success.
  - `TestRun_DNSSEC` — `dnssec-failed.org` SERVFAIL → `yes`; has address → `no`; error → `unknown`.
  - `TestRun_AdFilter` — both blocked (`0.0.0.0` / NXDOMAIN / `10.0.0.1`) → `yes`; one → `partial`; none → `no`.
  - `TestRun_Poisoned` — poison domain answered with `10.10.34.35` → `Poisoned==["youtube.com"]`.
  - `TestRun_ReachFailStops` — reach fails → no further queries issued.
  - `TestSort` — unreachable and poisoned last; then loss rounded to 10% ascending; then median ascending.
  - `TestScan_TooMany` (501 → `ErrTooMany`), `TestScan_CancelFast` (cancel mid-scan returns within 1 s with partial results), `TestScan_TenMinuteCap` (via a short injected cap field `maxDuration`).
- [ ] **Step 2: Run** `go test ./internal/scanner/advanced/` — FAIL.
- [ ] **Step 3: Implement.** Own worker pool (same shape as `scanner.Scan`, results unsorted until `Sort`). Ad domains `doubleclick.net`, `googleadservices.com`. DNSSEC confirm query `cloudflare.com` A with DO: AD flag set upgrades `unknown` to `yes` when the failed-domain query errored. Progress throttled to ≤ 10 calls/s except the final one.
- [ ] **Step 4: Run** `go test -race ./internal/scanner/advanced/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(scanner): advanced checks — latency, loss, DNSSEC, ad filtering, poisoning"`

---

### Task 8: `internal/cfscan` — ranges, sampling, `gencfranges`

**Files:**
- Create: `internal/cfscan/ranges.go`, `internal/cfscan/ranges_v4.txt`, `internal/cfscan/sample.go`, `tools/gencfranges/main.go`
- Test: `internal/cfscan/sample_test.go`, `tools/gencfranges/main_test.go`

**Interfaces:**
- Produces:

```go
func Ranges() []netip.Prefix                                  // parsed go:embed file, panics at init on a bad line (caught by test)
func Contains(rs []netip.Prefix, a netip.Addr) bool
func Sample(rs []netip.Prefix, max int, r *rand.Rand) []netip.Addr // one random host per /24, shuffled, first max
```
`tools/gencfranges`: `go run ./tools/gencfranges [-out internal/cfscan/ranges_v4.txt]` fetches `https://www.cloudflare.com/ips-v4`, `func parse(body []byte) ([]netip.Prefix, error)` rejects non-IPv4, non-CIDR, prefixes shorter than /8 or empty input, writes sorted lines with a header comment `# source: https://www.cloudflare.com/ips-v4 (fetched YYYY-MM-DD)`.

- [ ] **Step 1: Write failing tests** — `TestRanges_Parse` (non-empty, all IPv4, no overlap); `TestSample_OnePer24` (no two samples share a /24; never network `.0` or broadcast `.255`); `TestSample_AlwaysInRanges` (`testing/quick` over seeds: every sample `Contains`); `TestSample_Max`; `TestGencfranges_Parse` (good body, IPv6 line, garbage, `/4`, empty).
- [ ] **Step 2: Run** `go test ./internal/cfscan/ ./tools/gencfranges/` — FAIL.
- [ ] **Step 3: Implement**; generate `ranges_v4.txt` by running the tool once (`go run ./tools/gencfranges`) and commit its output.
- [ ] **Step 4: Run** the tests — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(cfscan): embedded Cloudflare IPv4 ranges and per-/24 sampling"`

---

### Task 9: `internal/cfscan` — probe, rate limit, scan

**Files:**
- Create: `internal/cfscan/probe.go`, `internal/cfscan/limiter.go`, `internal/cfscan/scan.go`
- Test: `internal/cfscan/probe_test.go`, `internal/cfscan/scan_test.go`, `internal/cfscan/integration_test.go` (`//go:build integration`)

**Interfaces:**
- Consumes: `Sample`, `Contains` (Task 8).
- Produces:

```go
type Result struct {
    IP string `json:"ip"`; OK bool `json:"ok"`; Reason string `json:"reason,omitempty"`
    LatencyMs int64 `json:"latencyMs"`; Colo string `json:"colo"`; Mbps float64 `json:"mbps"`; CheckedAt time.Time `json:"checkedAt"`
}
type Prober struct {
    Dial    func(ctx context.Context, network, addr string) (net.Conn, error)
    Host    string
    Timeout time.Duration
    Roots   *x509.CertPool // nil = system roots
    Now     func() time.Time
}
func (p Prober) Probe(ctx context.Context, ip netip.Addr) Result
type Limiter interface { Wait(ctx context.Context) error }
func NewLimiter(perSec int) Limiter // ticker-based token source
type Options struct {
    Concurrency, Want int; Limiter Limiter
    OnProgress func(tried, ok int, r Result)
}
var ErrNoNetwork = errors.New("cfscan: no network")
func Scan(ctx context.Context, ips []netip.Addr, p Prober, o Options) ([]Result, error)
func Sort(rs []Result) // OK first; Mbps desc when both have it; then latency asc
```

Reasons (spec 7.2): `tcp_timeout`, `tcp_refused`, `tls_timeout`, `tls_reset`, `tls_verify`, `http_status`, `bad_trace`.

- [ ] **Step 1: Write failing tests** — test TLS server from `httptest.NewUnstartedServer` with `StartTLS`, its cert for `speed.cloudflare.com` via a test CA passed as `Roots`; `Dial` ignores the target and dials the test server:
  - `TestProbe_OK` — `/cdn-cgi/trace` returns `ip=1.2.3.4\ncolo=SIN\n` → OK, `Colo=="SIN"`, request `Host` header is the probe host, SNI is the probe host.
  - `TestProbe_Reasons` — 403 → `http_status`; body without `colo=` → `bad_trace`; cert for `other.com` → `tls_verify`; server closes after accept → `tls_reset`; dial returns `ECONNREFUSED` → `tcp_refused`; dial blocks past `Timeout` → `tcp_timeout`; server never sends ServerHello → `tls_timeout`.
  - `TestScan_WantStopsEarly` — 100 IPs, all OK, want 10 → 10 OK results, fewer than 100 probes.
  - `TestScan_NoNetworkStopsEarly` — every dial times out → `ErrNoNetwork` after exactly 200 attempts (counted), not 2000.
  - `TestScan_Limiter` — fake limiter counting `Wait` calls equals probes started.
  - `TestScan_CancelClosesConns` — cancel with 64 dials blocked → returns ≤ 1 s, every fake conn `Close`d.
  - `TestScan_RejectsOutOfRange` — an IP outside `Ranges()` passed in → skipped, never dialed.
  - `TestNewLimiter_Rate` — 50 `Wait`s at 200/s take ≥ 240 ms.
  - Integration `TestIntegration_RealScan` — 200 sampled IPs with the real dialer → at least one OK.
- [ ] **Step 2: Run** `go test ./internal/cfscan/` — FAIL.
- [ ] **Step 3: Implement.** One deadline (`Timeout`) for the whole chain; TLS `MinVersion: tls.VersionTLS12`, `NextProtos: []string{"http/1.1"}`, `ServerName: Host`, `RootCAs: Roots`; write the request by hand (`GET /cdn-cgi/trace HTTP/1.1\r\nHost: …\r\nConnection: close\r\n\r\n`) and read with `http.ReadResponse`, body capped at 4 KB. `ErrNoNetwork`: the first 200 finished results all have `tcp_*` reasons. Track open conns in a set so cancel can close them.
- [ ] **Step 4: Run** `go test -race ./internal/cfscan/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(cfscan): probe Cloudflare IPs over TCP, TLS and /cdn-cgi/trace"`

---

### Task 10: `internal/cfscan` — speed test and cache

**Files:**
- Create: `internal/cfscan/speed.go`, `internal/cfscan/cache.go`
- Test: `internal/cfscan/speed_test.go`, `internal/cfscan/cache_test.go`

**Interfaces:**
- Produces:

```go
func (p Prober) Speed(ctx context.Context, ip netip.Addr, bytes int, limit time.Duration) (mbps float64, err error) // GET /__down?bytes=N
var ErrNoSpeedEndpoint = errors.New("cfscan: host has no /__down")
func SpeedTop(ctx context.Context, rs []Result, p Prober, top, bytes int, limit time.Duration, onEach func(Result)) error // sequential, fills Mbps on the best `top` OK results
type CacheEntry struct { ScannedAt time.Time `json:"scannedAt"`; Host string `json:"host"`; Results []Result `json:"results"` }
type Cache struct { Entries map[string]CacheEntry `json:"entries"` }
func (c *Cache) Put(key string, now time.Time, host string, rs []Result) // sorts, keeps top 100 OK
func (c *Cache) Get(key string) (CacheEntry, bool)
func LoadCache(path string) *Cache   // missing/corrupt → empty
func SaveCache(path string, c *Cache) error
```

- [ ] **Step 1: Write failing tests** — `TestSpeed_Mbps` (test server serves 1 MB; result > 0 and `bytes` query matches); `TestSpeed_404` → `ErrNoSpeedEndpoint`; `TestSpeed_Limit` (server trickles; returns at the limit with the rate so far, no error); `TestSpeedTop_OnlyTopOK` (12 results, top 10 OK get `Mbps`, failures untouched, sequential — max one in flight); `TestSpeedTop_StopsOnNoEndpoint` (first 404 → returns `ErrNoSpeedEndpoint`, no further requests); `TestCache_PutKeeps100`, `TestCache_PerNetwork`, `TestLoadCache_Corrupt`.
- [ ] **Step 2: Run** `go test ./internal/cfscan/` — FAIL.
- [ ] **Step 3: Implement** reusing `Probe`'s dial + TLS code (extract `p.connect(ctx, ip) (*tls.Conn, error)`); Mbps = bytes×8 / seconds / 1e6.
- [ ] **Step 4: Run** `go test -race ./internal/cfscan/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(cfscan): download speed for the best IPs and a per-network cache"`

---

### Task 11: `internal/backup` — format and `Build`

**Files:**
- Create: `internal/backup/format.go`, `internal/backup/build.go`
- Test: `internal/backup/build_test.go`

**Interfaces:**
- Produces:

```go
const Format = "ghostline-backup"; const FormatVersion = 1; const MaxSize = 8 << 20
var AllSections = []string{"settings", "rules", "customServers", "dpiBlacklist", "dpiAutoHostlist"}
type File struct {
    Format string `json:"format"`; FormatVersion int `json:"formatVersion"`
    AppVersion string `json:"appVersion"`; CreatedAt time.Time `json:"createdAt"`
    Sections Sections `json:"sections"`
}
type Sections struct {
    Settings        json.RawMessage  `json:"settings,omitempty"`
    Rules           *store.RulesFile `json:"rules,omitempty"`
    CustomServers   []model.Server   `json:"customServers,omitempty"`
    DPIBlacklist    *string          `json:"dpiBlacklist,omitempty"`
    DPIAutoHostlist []string         `json:"dpiAutoHostlist,omitempty"`
}
type Data struct { // everything a backup can hold, as live values
    Settings store.Settings; Rules store.RulesFile; Custom []model.Server; Blacklist string; AutoHostlist []string
}
func Build(d Data, sections []string, appVersion string, now time.Time) ([]byte, error) // indented JSON
func FileName(now time.Time) string // ghostline-2026-10-06.ghostline.json
```

- [ ] **Step 1: Write failing tests** — `TestBuild_Redacts`: settings with `AdapterGUIDs`, `AdvancedWindow`, `DNSServer.IOSSSID="HomeWiFi"`, an upstream proxy with `PassEnc="secret"` → output bytes contain none of `adapterGuids`, `advancedWindow`, `HomeWiFi`, `secret`, `iosSsid`; `passEnc` is `""`. `TestBuild_ListsWithoutCache`: rules file lists keep `id/url/format/action/...` metadata; no list body text is read or written. `TestBuild_PickSections` (only `rules` → other keys absent); `TestBuild_UnknownSection` → error; `TestFileName`.
- [ ] **Step 2: Run** `go test ./internal/backup/` — FAIL.
- [ ] **Step 3: Implement.** Redaction marshals `Settings` to a `map[string]any` and deletes keys (`adapterGuids`, `advancedWindow`, `dnsServer.iosSsid`), blanks every `proxy.upstreams[].passEnc`. Strip per-list runtime fields that `lists.List` carries (`lastUpdated`, `lastError`, `count`, `signatureOk` — check the struct and drop every field that is not user configuration).
- [ ] **Step 4: Run** `go test ./internal/backup/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(backup): export file format with redaction"`

---

### Task 12: `internal/backup` — `Parse` (preview + safety) and `Apply` (atomic with rollback)

**Files:**
- Create: `internal/backup/plan.go`, `internal/backup/apply.go`
- Test: `internal/backup/plan_test.go`, `internal/backup/apply_test.go`

**Interfaces:**
- Consumes: `store.MigrateSettings` (Task 1), `rules.AppendUserRules` (Task 2), `servers.ParseImport`.
- Produces:

```go
type Validators struct {
    Settings func(store.Settings) error
    List     func(lists.List) error
}
type Warning struct { Code string `json:"code"`; Detail string `json:"detail"` } // flag_off, trusted_reset, http_list
type SectionPreview struct { Name string `json:"name"`; New int `json:"new"`; Replaced int `json:"replaced"`; Errors []string `json:"errors"` }
type Preview struct { Sections []SectionPreview `json:"sections"`; Warnings []Warning `json:"warnings"`; SNIRules []rules.Rule `json:"sniRules"`; AppVersion string `json:"appVersion"`; CreatedAt time.Time `json:"createdAt"` }
type Plan struct { Preview Preview; /* unexported: parsed + sanitised sections */ }
var ErrInvalid = errors.New("backup: invalid file")   // wrapped with detail
var ErrNewer = errors.New("backup: made by a newer Ghostline")
func Parse(b []byte, current Data, v Validators) (*Plan, error)
type Choices struct { Sections []string `json:"sections"`; Merge bool `json:"merge"`; SNIRules string `json:"sniRules"` } // "accept" | "drop"
var ErrSNIUnconfirmed = errors.New("backup: rules with sni= or connect= need confirmation")
func (p *Plan) Result(current Data, c Choices) (Data, []string, error) // target data and the section names that change
type Write struct { Path string; Data []byte }
func Apply(ws []Write) error // .bak-import backups, atomic writes, full rollback
```

- [ ] **Step 1: Write failing tests (`plan_test.go`)** —
  - `TestParse_Rejects`: > 8 MB, bad JSON, `format` other, `formatVersion` 2 → `ErrNewer`, missing `format` → `ErrInvalid`.
  - `TestParse_OldSettings`: a v4 settings section → migrated to v5.
  - `TestParse_SafetyFlags`: file with `fakeSni.enabled`, `dnsServer.enabled`, `dnsServer.shareLan`, `proxy.shareLan`, `startWithWindows` all true and `fakeSni.ackVersion: 9` → result settings has them false and `ackVersion` equal to `current`'s; five `flag_off` warnings.
  - `TestParse_TrustedReset`: list `trustedForSNI:true, signed:false` → false + `trusted_reset` warning; `signed:true` keeps true.
  - `TestParse_HTTPList` → `http_list` warning, still importable.
  - `TestParse_SNIRules`: rules with `sni=` and `connect=` listed in `Preview.SNIRules`; `Result` with `SNIRules:""` → `ErrSNIUnconfirmed`; `"drop"` → those rules absent, others present; `"accept"` → present.
  - `TestParse_Counts`: replace vs merge counts for rules, custom servers (`servers.ParseImport` on each address; invalid ones appear in `Errors`), blacklist lines.
  - `TestResult_MergeNoDuplicates`; `TestResult_ReplaceKeepsMachineFields` (target settings keep current `AdapterGUIDs`, `AdvancedWindow`, `IOSSSID`; upstream proxies whose `PassEnc` was blank keep the current `PassEnc` when the ID matches).
- [ ] **Step 2: Write failing tests (`apply_test.go`)** — temp dir: `TestApply_WritesAll`; `TestApply_RollbackRestores` (second path is a directory → error; first file content back to original, no `.bak-import` left); `TestApply_RollbackRemovesNewFiles` (first target did not exist → removed after rollback); `TestApply_BackupKeptOnSuccess` (`.bak-import` exists for files that existed).
- [ ] **Step 3: Run** `go test ./internal/backup/` — FAIL.
- [ ] **Step 4: Implement.** Writes go through `store.WriteJSONAtomic`-style temp + rename (`os.WriteFile` to `<path>.tmp` then `os.Rename`) so text files (blacklist) work too.
- [ ] **Step 5: Run** `go test ./internal/backup/` — PASS.
- [ ] **Step 6: Commit** — `git commit -m "feat(backup): import preview with safety resets and atomic apply with rollback"`

---

### Task 13: App — Lookup and STAMP bindings, tool error codes

**Files:**
- Create: `internal/app/toolsservice.go`
- Modify: `internal/app/errors.go` (codes from Global Constraints), `internal/app/service.go` (`ServiceDeps`)
- Test: `internal/app/toolsservice_test.go`

**Interfaces:**
- Consumes: `lookup.*` (Tasks 5–6), `stamps.*` (Task 4).
- Produces — `ServiceDeps` gains:

```go
BuildUpstream func(model.Server) (upstream.Upstream, error)
PlainUpstream func(addr string) (upstream.Upstream, error) // "ip" → udp://ip:53; used for kind ghostline (127.0.0.1) and isp
ISPResolvers  func() []string // pre-connect DNS of the active adapters (state snapshot when connected)
```

Bound methods:

```go
type LookupResult struct { Overall lookup.Verdict `json:"overall"`; Answers []lookup.Answer `json:"answers"`; Verdicts []lookup.Verdict `json:"verdicts"` }
func (s *Service) LookupTypes() []string
func (s *Service) DefaultLookupSources() []lookup.Source // ghostline (or best server) + 2 fastest OK servers from scan cache
func (s *Service) ISPResolvers() []string
func (s *Service) Lookup(name, qtype string, sources []lookup.Source) (LookupResult, error)
type StampCard struct { Line string `json:"line"`; Fields *stamps.Fields `json:"fields,omitempty"`; Error string `json:"error,omitempty"` }
func (s *Service) DecodeStamps(text string) []StampCard
func (s *Service) EncodeStamp(f stamps.Fields) (string, error)
func (s *Service) StampFromURL(raw, ip string) (stamps.Fields, error)
```

- [ ] **Step 1: Write failing tests** — with fake `BuildUpstream`/`PlainUpstream` returning an in-memory upstream:
  - `TestLookup_GhostlineNeedsConnect` → `LOOKUP_NOT_CONNECTED` when status is not protected/degraded.
  - `TestLookup_BadName` → `LOOKUP_BAD_NAME`.
  - `TestLookup_SourceCount` — 1 or 7 sources → error; 2..6 OK; sources run in parallel (total time < sum of fake delays).
  - `TestLookup_ISPOnlyWhenAsked` — `DefaultLookupSources` never contains kind `isp`.
  - `TestLookup_AddressSource` — kind `address` with a plain `udp://` URL → rejected (only `isp` may be plain).
  - `TestDecodeStamps_PerLine` (good + bad line), `TestEncodeStamp_InvalidCode` → `STAMP_INVALID` with field param.
- [ ] **Step 2: Run** `go test ./internal/app/ -run 'Lookup|Stamp'` — FAIL.
- [ ] **Step 3: Implement.** Source resolution: `ghostline` → `PlainUpstream("127.0.0.1")`; `server` → catalog lookup by ID + `BuildUpstream`; `address` → `servers.FromAddress(ref, model.SourceCustom)` + `BuildUpstream`; `isp` → ref must be in `ISPResolvers()` or a valid IP the user typed, then `PlainUpstream`. Close every upstream after use.
- [ ] **Step 4: Run** `go test -race ./internal/app/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): DNS lookup and stamp tool bindings"`

---

### Task 14: App — Advanced scan bindings

**Files:**
- Create: `internal/app/advservice.go`
- Modify: `internal/app/events.go` (`EventToolsScan = "tools:scan"`)
- Test: `internal/app/advservice_test.go`

**Interfaces:**
- Consumes: `advanced.*` (Task 7), `servers.ParseImport`, `Service.SetPinnedMany`, `Service.AddServers`, `ServiceDeps.SaveFile`.
- Produces:

```go
type ServerFilter struct { Protocols []string `json:"protocols"`; Tags []string `json:"tags"`; Sources []string `json:"sources"`; PinnedOnly bool `json:"pinnedOnly"` }
type AdvScanRequest struct { Filter *ServerFilter `json:"filter,omitempty"`; Pasted string `json:"pasted"`; PoisonDomains []string `json:"poisonDomains"` }
type AdvScanStart struct { Total int `json:"total"`; Bad []string `json:"bad"` }
type AdvScanProgress struct { Done int `json:"done"`; Total int `json:"total"`; Result *advanced.Result `json:"result,omitempty"`; Running bool `json:"running"` }
func (s *Service) StartAdvancedScan(req AdvScanRequest) (AdvScanStart, error) // TOOL_BUSY, SCAN_TOO_MANY
func (s *Service) CancelAdvancedScan()
func (s *Service) AdvancedResults() []AdvRow // AdvRow{Result advanced.Result; Server model.Server; Pasted bool; Pinned bool}
func (s *Service) AddScannedServers(ids []string) (int, error) // pasted ones only → custom list
func (s *Service) ExportAdvancedCSV() error                      // UTF-8 BOM, header row, via SaveFile
```

- [ ] **Step 1: Write failing tests** — `TestAdvScan_Busy` (second start → `TOOL_BUSY` with `tool=advanced`); `TestAdvScan_TooMany` (`SCAN_TOO_MANY{count:501}`); `TestAdvScan_FilterAndPasted` (filter by protocol `doh` + tag `no-log`; pasted with one bad line → `Bad` has it); `TestAdvScan_EventsThrottled`; `TestTools_DoNotBlockConnect` (scan with a blocking fake checker; `Connect()` and `Disconnect()` both return within 2 s while it runs, scan still running afterwards); `TestAdvScan_DoesNotTouchScanCache` (scan-cache file unchanged); `TestExportAdvancedCSV_BOM` (bytes start with `EF BB BF`, header `server,protocol,ok,median_ms,p90_ms,jitter_ms,loss,dnssec,ad_filter,poisoned`).
- [ ] **Step 2: Run** `go test ./internal/app/ -run 'Adv|Tools'` — FAIL.
- [ ] **Step 3: Implement** with `s.mu`-guarded `advCancel`, settings from `Tools.Scanner`, `TestDomain` from settings, default poison domains = `ProbeSites`. Pasted servers get IDs from `servers.ParseImport` and live only in the job until added.
- [ ] **Step 4: Run** `go test -race ./internal/app/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): advanced DNS scanner bindings"`

---

### Task 15: App — Clean-IP scan bindings and rule creation

**Files:**
- Create: `internal/app/cfservice.go`
- Modify: `internal/app/events.go` (`EventToolsCFScan = "tools:cfscan"`), `internal/app/service.go` (`ServiceDeps.DialDirect`), `internal/app/rulesservice.go` (use `rules.AppendUserRules`)
- Test: `internal/app/cfservice_test.go`

**Interfaces:**
- Consumes: `cfscan.*` (Tasks 8–10), `rules.AppendUserRules`, `ServiceDeps.NetKey`, `Paths.CFScanCache`.
- Produces:

```go
// ServiceDeps gains: DialDirect func(ctx context.Context, network, addr string) (net.Conn, error)
type CFProgress struct { Phase string `json:"phase"`; Tried int `json:"tried"`; OK int `json:"ok"`; Total int `json:"total"`; Result *cfscan.Result `json:"result,omitempty"`; Running bool `json:"running"`; Note string `json:"note,omitempty"` } // phase: probe | speed; note: no_speed_endpoint
type CFView struct { ScannedAt time.Time `json:"scannedAt"`; Host string `json:"host"`; Results []cfscan.Result `json:"results"`; Running bool `json:"running"` }
func (s *Service) StartCFScan() error              // TOOL_BUSY, CFSCAN_HOST_INVALID, CFSCAN_NO_NETWORK (as final event error)
func (s *Service) CancelCFScan()
func (s *Service) GetCFView() CFView               // running job or cache for current network
func (s *Service) RecheckCF(ips []string) error     // probe only these; IPs outside ranges rejected
func (s *Service) CFSuggestDomains() []string       // patterns from user rules + lists whose catalog group is Cloudflare
func (s *Service) CreateCFRules(patterns []string, ips []string) []rules.LineError // 1–4 IPs, each in ranges
```

- [ ] **Step 1: Write failing tests** — fake `DialDirect` + test TLS server with test roots injected through an unexported `cfRoots` field: `TestCFScan_SavesCache` (top results stored under the current `NetKey`); `TestCFScan_Busy`; `TestCFScan_NoNetworkEvent` (final event carries error code `CFSCAN_NO_NETWORK`); `TestCFScan_NoSpeedEndpointNote`; `TestCreateCFRules` (`["example.com","*.example.com"]` + 2 IPs → two rules `ip=a,b` appended and rules recompiled; 5 IPs → error; IP outside Cloudflare → error; bad pattern → `LineError`, nothing written); `TestRecheckCF_RejectsOutOfRange`.
- [ ] **Step 2: Run** `go test ./internal/app/ -run CF` — FAIL.
- [ ] **Step 3: Implement.** Options from `Tools.CFScan`; `Sample(Ranges(), MaxIPs, rand.New(rand.NewPCG(seed…)))`; `NewLimiter(200)`. `CreateCFRules` builds `rules.Rule{Pattern, Action{IPs}, Enabled:true, Comment:"cloudflare clean ip"}` and goes through the same save/recompile path as `SaveRulesTable` (`storeRules`).
- [ ] **Step 4: Run** `go test -race ./internal/app/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): Cloudflare clean-IP scan bindings and ip= rule creation"`

---

### Task 16: App — Backup bindings and `--export`

**Files:**
- Create: `internal/app/backupservice.go`
- Modify: `internal/app/service.go` (`ServiceDeps.OpenFile func(title string) (string, error)`), `internal/cli/cli.go`, `headless.go`
- Test: `internal/app/backupservice_test.go`, `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `backup.*` (Tasks 11–12), `Service.saveSettings`, rules/custom-server/blacklist load and save paths already in `Service`.
- Produces:

```go
func (s *Service) ExportSettings(sections []string) error // SaveFile(backup.FileName(now), bytes); EXPORT_WRITE_FAILED
type ImportPreview struct { Token string `json:"token"`; Path string `json:"path"`; backup.Preview }
func (s *Service) PreviewImport() (ImportPreview, error)   // OpenFile; IMPORT_INVALID{detail}
func (s *Service) ApplyImport(token string, c backup.Choices) error // IMPORT_WHILE_CONNECTED, IMPORT_EXPIRED, IMPORT_WRITE_FAILED{file}
func ExportTo(s *Service, path string) error               // used by --export
```

- [ ] **Step 1: Write failing tests** — `TestExport_RoundTrip` (export from service A's temp data dir, import into service B's → same rules, custom servers, pins, DPI, proxy (minus `passEnc`); B's `fakeSni`, `dnsServer.enabled/shareLan`, `proxy.shareLan` false); `TestApplyImport_WhileConnected` for every status except `disconnected` and `error`; `TestApplyImport_TokenExpires` (injected clock, 10 min + 1 s); `TestApplyImport_ReloadsEverything` (settings box, `rules.Holder` recompiled, list refresh queued for subscribed lists, `OnSettingsChanged` called); `TestApplyImport_WriteFailRollsBack` (rules path made unwritable → `IMPORT_WRITE_FAILED{file:"rules.json"}`, settings file unchanged); `TestCLI_Export` (`--export out.json` writes a valid file and exits 0 without starting the UI).
- [ ] **Step 2: Run** `go test ./internal/app/ ./internal/cli/ -run 'Export|Import'` — FAIL.
- [ ] **Step 3: Implement.** Tokens: `crypto/rand` 16 bytes hex, map guarded by `s.mu`, one live token (a new preview replaces the old). Writes in order settings → rules → custom servers → blacklist → autohostlist, one `backup.Apply` call.
- [ ] **Step 4: Run** `go test -race ./internal/app/ ./internal/cli/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): settings export and import bindings; --export"`

---

### Task 17: Shell wiring

**Files:**
- Create: `internal/shell/toolswire.go`
- Modify: `internal/shell/shell.go` (fill the new `ServiceDeps` fields)
- Test: `internal/shell/toolswire_test.go`

**Interfaces:**
- Consumes: `ServiceDeps.BuildUpstream`, `PlainUpstream`, `ISPResolvers`, `DialDirect`, `OpenFile`.
- Produces: `func ispResolvers(st store.State, adapters func() ([]sysdns.Adapter, error)) []string` — when `st.Phase == "dns_set"`, the IPv4/IPv6 DNS servers from `st.Snapshot` (pre-connect values); otherwise the active adapters' current DNS; loopback and duplicates removed. `BuildUpstream` = the existing `upstreams.Factory.Build`. `PlainUpstream(ip)` = `upstream.AddressToUpstream("udp://"+net.JoinHostPort(ip,"53"), &upstream.Options{Timeout: 5*time.Second})`. `DialDirect` = `(&net.Dialer{}).DialContext`. `OpenFile` = `wapp.Dialog.OpenFile().AddFilter("Ghostline", "*.ghostline.json;*.json").PromptForSingleSelection()`.

- [ ] **Step 1: Write failing tests** — `TestISPResolvers_FromSnapshotWhenConnected` (state with snapshot `["203.162.4.191"]` and adapters reporting `127.0.0.1` → `["203.162.4.191"]`); `TestISPResolvers_FromAdaptersWhenClean`; `TestISPResolvers_DropsLoopbackAndDupes`.
- [ ] **Step 2: Run** `go test ./internal/shell/` — FAIL.
- [ ] **Step 3: Implement**, then `go build ./... && go vet ./...`.
- [ ] **Step 4: Run** `go test ./internal/shell/` — PASS; run `task common:generate:bindings` (defined in `build/Taskfile.yml`) and commit the regenerated `frontend/bindings`.
- [ ] **Step 5: Commit** — `git commit -m "feat(shell): wire tool dependencies and regenerate bindings"`

---

### Task 18: Frontend — Tools page, Lookup and STAMP tabs

**Files:**
- Create: `frontend/src/modes/advanced/pages/tools/Tools.tsx`, `Lookup.tsx`, `Stamp.tsx`, `tools.module.css`, `lookup.test.tsx`, `stamp.test.tsx`
- Modify: `frontend/src/app/store.ts` (`Page` adds `"tools"`; `toolsTab: "lookup" | "scanner" | "cfscan" | "stamp"` + setter), `frontend/src/app/api.ts` (export new types: `LookupResult`, `Answer`, `Source as LookupSource`, `Fields as StampFields`, `StampCard`, `AdvRow`, `AdvScanProgress`, `CFView`, `CFProgress`, `CfResult`, `ImportPreview`, `Choices as ImportChoices`), `frontend/src/modes/advanced/AdvancedView.tsx` (`pages` adds `"tools"` between `"fakesni"` and `"logs"`), `frontend/src/i18n/vi.json`, `en.json`

**Interfaces:**
- Consumes: bound methods from Tasks 13–16 via `Service` in `api.ts`.
- Produces: `<Tools />` rendering a tab bar and `<Lookup />`, `<Scanner />` (Task 19), `<CfScan />` (Task 19), `<Stamp />`; i18n namespace `tools.*`.

- [ ] **Step 1: Write failing tests** (mock `Service` like `proxy.test.tsx`):
  - `lookup.test.tsx`: `ISP source unchecked by default` (the ISP option exists after `ISPResolvers` resolves, is not checked, and carries the "unencrypted" label); `shows verdict card` for each of `poisoned`/`differs`/`match` with the localized title; `details toggles dig text`; `LOOKUP_NOT_CONNECTED shows connect hint`; `poisoned row offers "use another source" and opens Rules with pattern prefilled` (asserts `setPage("rules")` and the store's rule draft).
  - `stamp.test.tsx`: decode two lines → two cards, one error; relay card has "not usable" label and no add button; build from URL fills fields and shows stamp; invalid hash shows the field error from `STAMP_INVALID`; "Add to servers" calls `AddServers(stamp)`.
  - Tab state survives navigating away and back.
- [ ] **Step 2: Run** `cd frontend && npm test -- --run tools` — FAIL.
- [ ] **Step 3: Implement** using the existing neon components and page layout of `Proxy.tsx`; add every string to both locale files.
- [ ] **Step 4: Run** `cd frontend && npm test -- --run` — PASS (including `parity.test.ts`).
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): Tools page with DNS lookup and stamp tool"`

---

### Task 19: Frontend — Scanner and Cloudflare tabs, Settings backup section

**Files:**
- Create: `frontend/src/modes/advanced/pages/tools/Scanner.tsx`, `CfScan.tsx`, `scanner.test.tsx`, `cfscan.test.tsx`, `frontend/src/modes/advanced/pages/settings/Backup.tsx`, `backup.test.tsx`
- Modify: `frontend/src/modes/advanced/pages/Settings.tsx` (render `<Backup />`), locale files

**Interfaces:**
- Consumes: Tasks 14–16 methods; events `tools:scan`, `tools:cfscan` through the existing event subscription helper in `bridge.ts`.

- [ ] **Step 1: Write failing tests** —
  - `scanner.test.tsx`: start → progress bar from `tools:scan` events; Cancel calls `CancelAdvancedScan`; column filters (DNSSEC yes only) hide rows; bulk pin calls `SetPinnedMany`; "Use only this server" calls `UseOnlyServer`; pasted rows show "Add" calling `AddScannedServers`; `SCAN_TOO_MANY` message shows the count; Export calls `ExportAdvancedCSV`; the collapsed options panel saves `tools.scanner` through `SaveSettings` and shows the range error from the server.
  - `cfscan.test.tsx`: cached view shows "scanned <time>"; progress from `tools:cfscan` (probe then speed); `no_speed_endpoint` note shown; Copy writes selected IPs joined by `\n` to the clipboard; "Create rule" dialog: suggestions from `CFSuggestDomains`, more than 4 IPs disables submit, `LineError`s shown inline, success closes and shows a toast; the "only works behind Cloudflare" note is visible in the dialog; the options panel (host, maxIps, want, concurrency, timeout, speed test) saves `tools.cfscan` through `SaveSettings`, and "Restore default" resets the host after `CFSCAN_HOST_INVALID`.
  - `backup.test.tsx`: Export dialog has all five sections checked; Import disabled with reason while status is `protected`; preview lists warnings (`flag_off` etc.) in localized text; with `sniRules` present the Import button stays disabled until the checkbox is ticked or "drop" chosen; `IMPORT_EXPIRED` shows "choose the file again"; success shows the "press Connect" message.
- [ ] **Step 2: Run** `cd frontend && npm test -- --run` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `cd frontend && npm test -- --run && npm run build` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): advanced scanner, Cloudflare clean-IP tab and settings backup"`

---

### Task 20: Docs, manual checks and release v0.5.0

**Files:**
- Modify: `README.md`, `README.vi.md`, `docs/user-guide.md`, `docs/huong-dan-su-dung.md`, `docs/release-checklist.md`, `docs/superpowers/specs/2026-10-06-ghostline-phase3-design.md` (spec 4.2: methods on `*Service`, not a separate service), version files bumped the same way as commit `e38e93f` (`chore(release): v0.4.1`)

- [ ] **Step 1:** Write the Tools section (Lookup and reading verdicts, Scanner, Cloudflare clean IPs + creating rules + limits, STAMP) and "Backup and move to another PC" (what is never exported, what import turns off) in all four docs, EN and VI.
- [ ] **Step 2:** `release-checklist.md` — add `go run ./tools/gencfranges` + review diff, and the manual checks from spec 13 (Viettel/VNPT/FPT poisoned lookup, ≥ 10 clean IPs in ≤ 60 s and a working `ip=` rule, export A → import B with Fake SNI and LAN DNS still off, no firewall/AV alert during a default clean-IP scan).
- [ ] **Step 3:** Run the full suite: `go test -race ./... && go vet ./... && cd frontend && npm test -- --run && npm run build`. All PASS.
- [ ] **Step 4:** Bump the version to 0.5.0 following `git show e38e93f --stat`.
- [ ] **Step 5: Commit** — `git commit -m "docs: tools and backup guides"` then `git commit -m "chore(release): v0.5.0"` (tagging and pushing only when the user asks).
