# Ghostline Phase 2B Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scoped root CAs, a DoH + LAN DNS server, and Fake SNI (TLS interception with domain fronting) for the local proxy — released as v0.4.0.

**Architecture:** Two new connect phases run after the proxy phase: D (DNS server: LAN CA, firewall, extra `dnsproxy` listeners inside the existing engine) and S (Fake SNI: a RAM-only session CA installed in `LocalMachine\Root`, then an `Issuer` handed to the proxy). Fake SNI is a new branch in `proxy.tunnel` that dials the server with a fake SNI first and falls back to the 2A path when the server refuses. Every system change is written to `state.json` before it happens so the existing four recovery layers undo it.

**Tech Stack:** Go 1.26 standard library (`crypto/x509`, `crypto/tls`, `crypto/ecdsa`), `AdguardTeam/dnsproxy` (already a dependency), `golang.org/x/sys/windows` (crypt32, wlanapi), Wails v3, React + Vitest.

**Spec:** `docs/superpowers/specs/2026-10-05-ghostline-phase2b-design.md` (read it with this plan; section numbers below refer to it).

## Global Constraints

- No new Go module dependencies and no new binaries (spec 13).
- CAs: ECDSA P-256, `ECDSAWithSHA256`, `BasicConstraints{CA:true, MaxPathLen:0, MaxPathLenZero:true}`, `ExtKeyUsage` = `serverAuth` only, Name Constraints marked critical (spec 5.1).
- Subject prefixes are fixed: `Ghostline LAN CA` and `Ghostline Fake SNI` (sweeps match on them).
- CA LAN: 5 years; leaf 90 days, re-issued when the IP set changes or < 14 days remain. Session CA: `NotBefore` = now − 1 h, 30 days, rotated when < 3 days remain; max 1000 domains; key never serialised (spec 5.2, 5.3).
- Leaf cache: LRU, 1000 entries, leaf lifetime 7 days.
- Store: `LocalMachine\Root` only. Install = add + read back by SHA-1 thumbprint; remove of a missing cert = success.
- `state.json` v3 (`certs.session []string`, `firewall.rules []string`); `settings.json` v4 (`dnsServer`, `fakeSni`). v1/v2 state and v1–v3 settings still load.
- Firewall rule names: `Ghostline Proxy`, `Ghostline DNS (TCP)`, `Ghostline DNS (UDP)`, `Ghostline Setup`; all `profile=private remoteip=localsubnet program=<exe>`.
- DoH path `/dns-query`, default port 443; setup page port 8053, closes after 10 minutes; DNS rate limit 100 qps per IP; `ANY` → `REFUSED`; non-private sources → `REFUSED`.
- Fake SNI handshakes: 10 s each side. Server cert must chain to system roots and match the fake SNI **or** the real host (`sni=none`: real host only). Never accept an unverified chain.
- `sni=`/`connect=` only with `domain`, `=domain`, `*.domain`; `connect=` cannot combine with `ip=`; neither combines with `block`.
- Signed lists: ed25519 base64 `.sig` verified with `servers.VerifySigned` and `brand.ServerListPublicKeyHex`.
- No decrypted content, URL, header or body is ever logged or stored.
- Every new UI string exists in both `frontend/src/i18n/vi.json` and `en.json`.
- Commits never include a `Co-Authored-By` trailer (user preference).
- Go tests: `go test ./...`; Windows-only integration tests use build tag `integration`. Frontend: `cd frontend && npm test -- --run`.

## Review Focus

1. **Crash between "thumbprint written" and "cert installed", or between install and `Issuer` set** — recovery must remove whatever is in Root and never leave an orphan; covered in Task 15 (`TestRecover_SessionCertBeforeInstall`) and Task 13 (`TestPhaseS_KillBetweenSteps`).
2. **Rules edited many times quickly while connected** — one rotation per 2 s burst, old CA removed after the new one is live, no window with zero CAs while `Issuer` points at a removed one; Task 14 `TestRotate_Debounced`, `TestRotate_InstallFailsKeepsOld`.
3. **LAN IP changes (Wi-Fi switch, DHCP renew) while phase D runs** — listeners and leaf follow the new set; Task 12 `TestPhaseD_IPChangeRestarts`.
4. **Client offers ALPN the server rejects / server picks nothing** — client side must then negotiate no ALPN, never h2 against an h1 server; Task 8 `TestMITM_ALPNServerChoosesNone`.
5. **A downloaded preset whose signature is missing after it had been valid** — keep the last verified copy, never fall back to the unsigned one; Task 3 `TestSignedList_MissingSigKeepsOld`.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/rules/rule.go`, `text.go`, `compile.go`, `sni.go` (new) | `Connect` action, `sni=`/`connect=` parse + validation, `Compiled.SNIDomains()` |
| `internal/rules/formats/ghostline.go` (new) | `ghostline` list format (rule text, per-line actions) |
| `internal/rules/lists/list.go`, `signed.go` (new), `catalog.json` | `TrustedForSNI`, `Signed`, per-line actions, signature check, "Fake SNI" catalog group |
| `internal/tlsfrag/hello.go` | `ALPN(hello)`, `HasECH(hello)` |
| `internal/certs/ca.go`, `constraints.go`, `leaf.go`, `keyfile.go`, `mobileconfig.go` (new) | CA creation, constraints mapping, leaf issuer + LRU, DPAPI key file, iOS profile |
| `internal/certstore/store.go`, `api_windows.go`, `fake.go` (new) | `Store` interface, Windows crypt32 implementation, test fake, sweep |
| `internal/winutil/firewall.go`, `wlan_windows.go` (new), `dpapi_windows.go` | Named firewall rules, current SSID, machine-scope DPAPI |
| `internal/store/settings.go`, `state.go`, `paths.go` | settings v4, state v3, `LANCACert`/`LANCAKey` paths |
| `internal/proxy/mitm/mitm.go`, `verify.go` (new) | Fake SNI handshake pair |
| `internal/proxy/dialer/dialer.go`, `internal/proxy/server.go`, `stats.go` | `connect=`, `OpenRaw` for MITM, MITM branch with fallback, new outcomes |
| `internal/engine/engine.go`, `serve.go` (new) | DoH + LAN listeners, source filter, ratelimit, ANY refuse |
| `internal/dnsserver/addrs.go`, `setuppage.go`, `setuppage_{vi,en}.html` (new) | Listen address choice, temporary setup page |
| `internal/app/dnsphase.go`, `sniphase.go`, `certsservice.go`, `dnsservice.go`, `fakesniservice.go` (new), `deps.go`, `errors.go`, `status.go`, `orchestrator.go`, `health.go` | Phases D/S, rotation, reasons, bindings |
| `internal/watchdog/recover.go`, `internal/cli/cli.go`, `headless.go` | Recovery order, sweep, `--remove-certs` |
| `internal/shell/certwire.go`, `dnswire.go` (new), `shell.go` | Wiring real implementations |
| `build/windows/nsis/*.nsi` | `--remove-certs`, extra firewall deletes |
| `frontend/src/modes/advanced/pages/DnsServer.tsx`, `FakeSni.tsx`, `FakeSniWarning.tsx` (new), `components/FakeSniBanner.tsx` (new), `Rules*.tsx`, `Lists.tsx`, `Settings.tsx`, `Overview.tsx`, `AdvancedView.tsx`, simple mode stats | UI |
| `lists/fakesni/*.txt` + `.sig`, `lists/embed.go` | Presets and their built-in fallback |
| `README.md`, `README.vi.md`, `docs/user-guide.md`, `docs/huong-dan-su-dung.md`, `docs/release-checklist.md` | Docs, v0.4.0 |

---

### Task 1: Rules — `sni=`, `connect=` and `SNIDomains`

**Files:**
- Modify: `internal/rules/rule.go`, `internal/rules/text.go`
- Create: `internal/rules/sni.go`
- Test: `internal/rules/text_test.go`, `internal/rules/sni_test.go`

**Interfaces:**
- Produces: `Action.Connect string` (json `connect,omitempty`); `Action.SNI` keeps json `sni`, value `"none"` means "send no SNI" (const `SNINone = "none"`); `func (c *Compiled) SNIDomains() []string` — sorted, de-duplicated Name-Constraint strings from enabled user rules with `SNI != ""` (Task 2 extends it to trusted lists); `Decision` gains nothing new here (it embeds `Action`).

- [ ] **Step 1: Write failing tests**

```go
func TestParseText_SNIConnect(t *testing.T) {
	rs, errs := rules.ParseText("youtube.com sni=www.google.com connect=www.google.com\nexample.org sni=none", nil)
	require.Empty(t, errs)
	require.Equal(t, "www.google.com", rs[0].SNI)
	require.Equal(t, "www.google.com", rs[0].Connect)
	require.Equal(t, rules.SNINone, rs[1].SNI)
}

func TestParseText_SNIRejectsNonDomainPatterns(t *testing.T) {
	for _, l := range []string{"~tube sni=a.com", "/yt/ sni=a.com", "10.0.0.0/8 sni=a.com", "~tube connect=a.com"} {
		_, errs := rules.ParseText(l, nil)
		require.Len(t, errs, 1, l)
		require.Contains(t, errs[0].Msg, "only with domain patterns")
	}
}

func TestParseText_ConnectWithIPRejected(t *testing.T) {
	_, errs := rules.ParseText("a.com connect=b.com ip=1.2.3.4", nil)
	require.Len(t, errs, 1)
}

func TestFormatText_RoundTripSNIConnect(t *testing.T) { /* ParseText(FormatText(rs)) == rs for the two lines above */ }

func TestSNIDomains(t *testing.T) {
	user := []rules.Rule{
		{Pattern: "youtube.com", Action: rules.Action{SNI: "www.google.com"}, Enabled: true},
		{Pattern: "*.googlevideo.com", Action: rules.Action{SNI: "www.google.com"}, Enabled: true},
		{Pattern: "=example.com", Action: rules.Action{SNI: rules.SNINone}, Enabled: true},
		{Pattern: "off.com", Action: rules.Action{SNI: "x.com"}, Enabled: false},
	}
	c, err := rules.Compile(user, nil)
	require.NoError(t, err)
	require.Equal(t, []string{".googlevideo.com", "example.com", "youtube.com"}, c.SNIDomains())
}
```

- [ ] **Step 2: Run** `go test ./internal/rules/ -run 'SNI|Connect'` — Expected: FAIL (unknown field `Connect`, `SNIDomains` undefined).

- [ ] **Step 3: Implement**
  - `parseLine`: `connect=<host>` normalised with `NormalizeHost`; `sni=none` stored as `SNINone`, otherwise `NormalizeHost`.
  - `validateAction(a Action, kind PatternKind) error` (add the kind parameter, update both callers): `SNI`/`Connect` with `KindKeyword|KindRegexp|KindCIDR` → error text `"sni= and connect= work only with domain patterns"`; `Connect != "" && len(IPs) > 0` → `"connect= cannot be combined with ip="`; `Block` with `Connect` → existing block error. `Empty()` also checks `Connect`.
  - `FormatText` writes ` connect=` after ` sni=`.
  - `sni.go`: `SNIDomains` maps `KindDomain`→`v`, `KindSubOnly`→`"."+v`, `KindExact`→`v`; collects from `c.user` enabled rules with `SNI != ""`.

- [ ] **Step 4: Run** `go test ./internal/rules/...` — Expected: PASS (existing tests too; `TestParseText_SNIAccepted` keeps passing).

- [ ] **Step 5: Commit** — `git commit -m "feat(rules): sni= and connect= for Fake SNI, domain patterns only"`

---

### Task 2: Lists — `ghostline` format, per-line actions, `trustedForSNI`

**Files:**
- Create: `internal/rules/formats/ghostline.go`, `internal/rules/formats/testdata/ghostline.txt`
- Modify: `internal/rules/formats/formats.go`, `detect.go`, `internal/rules/compile.go`, `internal/rules/match.go`, `internal/rules/lists/list.go`
- Test: `internal/rules/formats/formats_test.go`, `internal/rules/match_test.go`, `internal/rules/lists/list_test.go` (create if absent)

**Interfaces:**
- Consumes: `rules.ParseText` (Task 1).
- Produces: `formats.Ghostline Format = "ghostline"`; `rules.Entry.Action *Action` (nil = use the set's action); `rules.ListSet.TrustedForSNI bool`; `SNIDomains()` also collects entries of trusted sets whose `Action.SNI != ""`; `lists.List.TrustedForSNI bool` (json `trustedForSNI`), `lists.List.Signed bool` (json `signed`), `lists.List.SignatureOK bool` (json `signatureOk`); `ToListSet` accepts `l.Action == "perLine"` (required when `Detected == "ghostline"`), copies `TrustedForSNI`.
- Matching rule: when a set is not `TrustedForSNI`, `Match` clears `SNI` and `Connect` from that set's decision but keeps the other actions; `Decision.SNIIgnored bool` (json `sniIgnored`) is set so Explain can say why.

- [ ] **Step 1: Write failing tests**
  - `TestDetect_Ghostline`: data starting with `# ghostline-rules v1` → `Ghostline`, regardless of file name.
  - `TestParse_Ghostline`: three lines (`youtube.com sni=www.google.com connect=www.google.com`, `ads.com block`, `~kw sni=a.com`) → 2 entries with `Action` set, `Skipped == 1`, `Counts["domain"] == 2`.
  - `TestMatch_ListPerLineActions`: a set with `Entry.Action` set uses the entry action.
  - `TestMatch_UntrustedListDropsSNI`: same set with `TrustedForSNI=false` → `dec.SNI == ""`, `dec.Connect == ""`, `dec.SNIIgnored == true`, `dec.Block` unaffected for a block line.
  - `TestSNIDomains_TrustedListsOnly`: a trusted set with `vercel.com sni=nextjs.org` and an untrusted set with `evil.com sni=x.com` → result contains `vercel.com`, not `evil.com`.
  - `TestToListSet_PerLine`: `List{Action:"perLine", TrustedForSNI:true}` → `ListSet.TrustedForSNI`; `Action:"perLine"` with a non-ghostline result → error.

- [ ] **Step 2: Run** `go test ./internal/rules/...` — Expected: FAIL.
- [ ] **Step 3: Implement** — `ghostline` parser calls `rules.ParseText` line by line on the raw text (so the format shares one grammar), converts each `Rule` to an `Entry` via `rules.ParsePattern`; lines failing `validateAction` (keyword + sni) go to `skip`. Add `Ghostline` first in `All` so detection wins on the header. Disallow `perLine` for other formats.
- [ ] **Step 4: Run** `go test ./internal/rules/...` — Expected: PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(lists): ghostline rule-text lists with per-line actions and trustedForSNI"`

---

### Task 3: Signed lists and the "Fake SNI" catalog group

**Files:**
- Create: `internal/rules/lists/signed.go`
- Modify: `internal/rules/lists/fetch.go`, `internal/rules/lists/catalog.go`, `internal/rules/lists/catalog.json`
- Test: `internal/rules/lists/signed_test.go`, `internal/rules/lists/fetch_test.go`

**Interfaces:**
- Consumes: `servers.VerifySigned(data, sig []byte, pub ed25519.PublicKey) error`.
- Produces: `lists.Fetcher` gains field `SigKey ed25519.PublicKey`; for a list with `Signed: true` the fetcher downloads `<url>.sig`, verifies, and only then replaces `lists/<id>.txt` and writes `lists/<id>.txt.sig`; on failure it keeps the old file and sets `LastError = "LIST_SIGNATURE_INVALID"`, `SignatureOK = false`. `func VerifyCached(path string, pub ed25519.PublicKey) bool` re-checks a cached file at load. Catalog items gain `group: "fakesni"`, `signed: true`, `trustedForSNI: true`, `action: "perLine"`.
- Error code constant `app.CodeListSignatureInvalid = "LIST_SIGNATURE_INVALID"` is added in Task 12; here use the string via a `lists.CodeSignatureInvalid` const and map it there.

- [ ] **Step 1: Write failing tests** (sign with a test key via `servers.Sign`)
  - `TestSignedList_ValidReplaces`
  - `TestSignedList_BadSigKeepsOld` — old content unchanged, `LastError == "LIST_SIGNATURE_INVALID"`.
  - `TestSignedList_MissingSigKeepsOld` — `.sig` 404 after an earlier valid fetch → old content kept, never the unsigned new one.
  - `TestCatalog_FakeSNIGroup` — catalog has ≥ 1 item with `group == "fakesni"`, each `signed && trustedForSNI && action == "perLine"`, URL under `raw.githubusercontent.com/hashcott/ghostline/main/lists/fakesni/`.
- [ ] **Step 2: Run** `go test ./internal/rules/lists/` — Expected: FAIL.
- [ ] **Step 3: Implement** as above; jsDelivr fallback applies to the `.sig` too (same `github.go` rewrite).
- [ ] **Step 4: Run** — Expected: PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(lists): signed lists and the Fake SNI catalog group"`

---

### Task 4: `tlsfrag.ALPN` and `HasECH`

**Files:**
- Modify: `internal/tlsfrag/hello.go`
- Test: `internal/tlsfrag/hello_test.go`, fuzz in the same file

**Interfaces:**
- Produces: `func ALPN(hello []byte) []string` (nil when absent/malformed); `func HasECH(hello []byte) bool` (extension `0xfe0d` present).

- [ ] **Step 1: Write failing tests** — build hellos with `tls.Client` writing into a capture conn (pattern already used by existing `SNI` tests): `NextProtos: {"h2","http/1.1"}` → `ALPN` equals that; no ALPN → nil; truncated hello → nil, no panic; `FuzzALPN` seeded with the captured hello. `HasECH` false for Go's default hello; true for a hand-patched hello carrying extension `0xfe0d`.
- [ ] **Step 2: Run** `go test ./internal/tlsfrag/ -run 'ALPN|ECH'` — FAIL.
- [ ] **Step 3: Implement** on the existing extension walker used by `SNI`.
- [ ] **Step 4: Run** `go test ./internal/tlsfrag/ && go test ./internal/tlsfrag/ -fuzz FuzzALPN -fuzztime 20s` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(tlsfrag): read ALPN and detect ECH in a ClientHello"`

---

### Task 5: `internal/certs` — CAs, constraints, leaf issuer

**Files:**
- Create: `internal/certs/ca.go`, `constraints.go`, `leaf.go`
- Test: `internal/certs/ca_test.go`, `leaf_test.go`

**Interfaces:**
- Produces:

```go
type CA struct {
	Cert *x509.Certificate
	DER  []byte
	Key  *ecdsa.PrivateKey
}
func NewLANCA(host string, now time.Time) (*CA, error)                  // subject "Ghostline LAN CA — <host> <4 random [A-Z0-9]>"
func NewSessionCA(domains []string, now time.Time) (*CA, error)         // subject "Ghostline Fake SNI — phiên <now RFC3339>"; ErrTooManyDomains if > 1000
var ErrTooManyDomains = errors.New("certs: too many Fake SNI domains")
func (c *CA) Thumbprint() string                                         // SHA-1 hex lower-case of DER (Windows store key)
func (c *CA) Fingerprint() string                                        // SHA-256 hex, "AA:BB:…" upper-case (shown to users)
func (c *CA) IssueServer(names []string, ips []netip.Addr, life time.Duration, now time.Time) (*tls.Certificate, error)
type Issuer struct { /* CA + LRU(1000) keyed by host */ }
func NewIssuer(ca *CA, now func() time.Time) *Issuer
func (i *Issuer) Leaf(host string) (*tls.Certificate, error)             // 7-day leaf, cached
func (i *Issuer) CA() *CA
const (LANPrefix = "Ghostline LAN CA"; SessionPrefix = "Ghostline Fake SNI")
var LANPermittedCIDRs = []string{"10.0.0.0/8","172.16.0.0/12","192.168.0.0/16","127.0.0.0/8","169.254.0.0/16","fc00::/7","fe80::/10","::1/128"}
const LANDomain = "ghostline.lan"
```

- [ ] **Step 1: Write failing tests** (verify with `x509.Verify` against a pool holding only the CA, `KeyUsages: serverAuth`):
  - `TestSessionCA_AllowsListedDomains` — domains `{"youtube.com", ".googlevideo.com"}`: leaf for `youtube.com`, `m.youtube.com`, `r1.googlevideo.com` verify.
  - `TestSessionCA_RejectsOthers` — leaf for `vietcombank.com.vn`, `googlevideo.com` (exact, sub-only constraint), and an IP-SAN leaf for `1.1.1.1` fail verification.
  - `TestSessionCA_TooMany` — 1001 domains → `ErrTooManyDomains`.
  - `TestLANCA_Constraints` — leaf with `192.168.1.5`, `127.0.0.1`, `::1`, `dns.ghostline.lan` verifies; leaf with `8.8.8.8` or `example.com` fails.
  - `TestCA_Basics` — `IsCA`, `MaxPathLenZero`, EKU only serverAuth, name constraints critical, LAN validity 5 y, session `NotBefore == now-1h` and 30 d; a client-auth leaf fails verification.
  - `TestIssuer_CachesAndEvicts` — same host twice → same pointer; 1001 hosts → first evicted.
- [ ] **Step 2: Run** `go test ./internal/certs/` — FAIL.
- [ ] **Step 3: Implement** — `x509.CreateCertificate` with `PermittedDNSDomains`, `PermittedIPRanges`, `ExcludedIPRanges` (`0.0.0.0/0`, `::/0` for session), `PermittedDNSDomainsCritical: true`; serials 128-bit random; LRU via `container/list`.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(certs): name-constrained LAN and session CAs with a leaf issuer"`

---

### Task 6: CA LAN key file and the iOS profile

**Files:**
- Create: `internal/certs/keyfile.go`, `internal/certs/mobileconfig.go`
- Modify: `internal/winutil/dpapi_windows.go` (add machine scope)
- Test: `internal/certs/keyfile_test.go`, `mobileconfig_test.go`, `internal/winutil/dpapi_windows_test.go`

**Interfaces:**
- Consumes: `NewLANCA` (Task 5).
- Produces:

```go
// winutil
func ProtectMachine(b []byte) ([]byte, error)   // CryptProtectData with CRYPTPROTECT_LOCAL_MACHINE
func UnprotectMachine(b []byte) ([]byte, error)
// certs
type Protector interface { Protect([]byte) ([]byte, error); Unprotect([]byte) ([]byte, error) }
func SaveLANCA(certPath, keyPath string, ca *CA, p Protector) error   // DER cert; PKCS#8 key → Protect; files written 0600 then ACL left to the data dir
func LoadLANCA(certPath, keyPath string, p Protector) (*CA, error)    // ErrKeyUnreadable when Unprotect fails; os.ErrNotExist passes through
var ErrKeyUnreadable = errors.New("certs: LAN CA key unreadable")
type ProfileInput struct { CA *CA; Addrs []netip.Addr; Port int; SSID string; Lang string }
func MobileConfig(in ProfileInput) ([]byte, error)                    // ErrNoSSID when SSID == ""
```

- [ ] **Step 1: Write failing tests**
  - `TestLANCA_SaveLoadRoundTrip` with an XOR fake `Protector`; key bytes on disk differ from PKCS#8.
  - `TestLANCA_UnprotectFails` → `ErrKeyUnreadable`.
  - `TestMobileConfig` — check the output is well-formed with `xml.NewDecoder` (read all tokens without error) and assert with `bytes.Contains`: contains `com.apple.security.root`, `com.apple.dnsSettings.managed`, `<string>HTTPS</string>`, `https://dns.ghostline.lan/dns-query` (port 443 omitted; 8443 → `:8443`), each address in `ServerAddresses`, `OnDemandRules` with `SSIDMatch` = SSID and a trailing `Disconnect`; SSID `a&b<c` is escaped; `SSID == ""` → `ErrNoSSID`; payload UUIDs stable for the same CA (derive from thumbprint) so re-installing replaces rather than duplicates.
  - `TestProtectMachine_RoundTrip` (Windows, `integration` not needed — crypt32 is always present).
- [ ] **Step 2: Run** `go test ./internal/certs/ ./internal/winutil/ -run 'LANCA|MobileConfig|ProtectMachine'` — FAIL.
- [ ] **Step 3: Implement** — profile from a `text/template` with `xml.EscapeText` for values; certificate payload is base64 DER in `<data>`.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(certs): DPAPI-protected LAN CA key and the iOS DoH profile"`

---

### Task 7: `internal/certstore`

**Files:**
- Create: `internal/certstore/store.go`, `api_windows.go`, `fake.go`, `store_integration_test.go` (tag `integration`)
- Test: `internal/certstore/store_test.go`

**Interfaces:**
- Produces:

```go
type Cert struct { Thumbprint, Subject string; NotAfter time.Time }
type Store interface {
	Install(der []byte) error           // add (replace existing) then read back by thumbprint; ErrNotInstalled if read-back misses
	Remove(thumbprint string) error     // missing = nil
	List(subjectPrefix string) ([]Cert, error)
}
var ErrNotInstalled = errors.New("certstore: certificate not found after install")
func NewWindows(location Location) Store  // Location: LocalMachine (prod) | CurrentUser (integration test only)
func Sweep(s Store, prefix string, keep []string) (removed []string, err error)  // remove every cert with prefix not in keep
type Fake struct { /* map + hooks: FailInstall, FailRemove error */ }
func NewFake() *Fake
func Thumbprint(der []byte) string       // same as certs.CA.Thumbprint, used by callers holding only DER
```

- [ ] **Step 1: Write failing tests** (against `Fake`, plus a tiny `Sweep` table test): `TestSweep_KeepsCurrent` (3 certs with `Ghostline Fake SNI`, 1 `Ghostline LAN CA`, keep one session thumbprint → removes exactly 2), `TestSweep_RemoveErrorReported`. Integration (`-tags integration`): `TestWindows_InstallListRemove` on `CurrentUser\Root` using a throwaway constrained CA — note: the CurrentUser Root add shows a Windows dialog; run it manually once, it is excluded from CI.
- [ ] **Step 2: Run** `go test ./internal/certstore/` — FAIL.
- [ ] **Step 3: Implement** `api_windows.go` with `windows.CertOpenStore(CERT_STORE_PROV_SYSTEM_W, 0, 0, CERT_SYSTEM_STORE_LOCAL_MACHINE, "ROOT")`, `CertAddEncodedCertificateToStore(..., CERT_STORE_ADD_REPLACE_EXISTING)`, find by `CERT_FIND_SHA1_HASH`, `CertDeleteCertificateFromStore`, enumerate with `CertEnumCertificatesInStore` and match `x509.ParseCertificate(...).Subject.CommonName` prefix. Use `golang.org/x/sys/windows` (declare missing procs with `windows.NewLazySystemDLL("crypt32.dll")`).
- [ ] **Step 4: Run** `go test ./internal/certstore/` — PASS; `go vet ./internal/certstore/`.
- [ ] **Step 5: Commit** — `git commit -m "feat(certstore): install, list and remove Ghostline roots in LocalMachine\\Root"`

---

### Task 8: `internal/proxy/mitm`

**Files:**
- Create: `internal/proxy/mitm/mitm.go`, `verify.go`
- Test: `internal/proxy/mitm/mitm_test.go`

**Interfaces:**
- Consumes: `tlsfrag.ALPN` (Task 4).
- Produces:

```go
type LeafSource interface { Leaf(host string) (*tls.Certificate, error) }   // *certs.Issuer satisfies it
type Params struct {
	Host      string          // real host (ClientHello SNI)
	FakeSNI   string          // "" = send no SNI (rule sni=none)
	Hello     []byte          // the client's buffered ClientHello
	Roots     *x509.CertPool  // nil = system pool
	Timeout   time.Duration   // 10 s
	Now       func() time.Time
}
var (ErrServerRejected = errors.New("mitm: server refused the fake SNI"); ErrVerifyFailed = errors.New("mitm: server certificate not valid for the fake SNI or host"); ErrClientRejected = errors.New("mitm: client refused the local certificate"))
// DialServer completes TLS to raw (already connected, nothing written) with the fake SNI and the client's ALPN offer.
func DialServer(ctx context.Context, raw net.Conn, p Params) (*tls.Conn, error)     // ErrServerRejected | ErrVerifyFailed
// AcceptClient completes TLS with the client, replaying p.Hello first, offering only server's negotiated protocol.
func AcceptClient(ctx context.Context, client net.Conn, br *bufio.Reader, p Params, leaf LeafSource, negotiated string) (*tls.Conn, error) // ErrClientRejected
```

- [ ] **Step 1: Write failing tests** — a test CA from `certs.NewSessionCA` for the client side and a separate test "public" CA (in `Params.Roots`) for the server side; `httptest`-style TLS listener on loopback.
  - `TestMITM_SendsFakeSNI` — server `GetConfigForClient` records SNI == `"www.google.com"`; with `FakeSNI:""` the recorded SNI is empty.
  - `TestMITM_VerifyAcceptsFakeOrReal` — server cert for `www.google.com` passes; for `youtube.com` passes; for `other.com` → `ErrVerifyFailed`; expired → `ErrVerifyFailed`; self-signed not in Roots → `ErrVerifyFailed`; with `FakeSNI:""` a cert only for `www.google.com` → `ErrVerifyFailed`.
  - `TestMITM_ServerAlert` — server aborts handshake → `ErrServerRejected`.
  - `TestMITM_ALPN` — client offers `h2,http/1.1`, server supports only `http/1.1` → client side negotiates `http/1.1`; server supports both → `h2`.
  - `TestMITM_ALPNServerChoosesNone` — server sets no `NextProtos` → client handshake negotiates `""` even though the client offered `h2`.
  - `TestMITM_ClientRejects` — client pool lacks the session CA → `ErrClientRejected`.
  - `TestMITM_EndToEndHTTP2` — `http.Client` with `ForceAttemptHTTP2`, through `DialServer`+`AcceptClient`+`io.Copy` pair → body read correctly.
- [ ] **Step 2: Run** `go test ./internal/proxy/mitm/` — FAIL.
- [ ] **Step 3: Implement** — server side `tls.Config{ServerName: FakeSNI, NextProtos: ALPN(Hello), MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: verify}`; `verify` builds `x509.VerifyOptions{Roots, Intermediates from cs.PeerCertificates[1:], DNSName: name, KeyUsages: serverAuth, CurrentTime: Now()}` and tries `FakeSNI` then `Host` (only `Host` when `FakeSNI==""`). Distinguish verify errors by wrapping in `VerifyConnection`. Client side: a `net.Conn` wrapper whose `Read` drains `Hello` then `br`, `tls.Server` with `GetCertificate: leaf.Leaf(hello.ServerName)`, `NextProtos: []string{negotiated}` or nil.
- [ ] **Step 4: Run** — PASS, with `-race`.
- [ ] **Step 5: Commit** — `git commit -m "feat(proxy/mitm): Fake SNI handshake pair with strict server verification"`

---

### Task 9: Dialer and proxy — `connect=`, MITM branch, fallback

**Files:**
- Modify: `internal/proxy/dialer/dialer.go`, `internal/proxy/server.go`, `internal/proxy/stats.go`
- Test: `internal/proxy/dialer/dialer_test.go`, `internal/proxy/server_test.go`

**Interfaces:**
- Consumes: `mitm.DialServer`, `mitm.AcceptClient`, `mitm.LeafSource` (Task 8); `tlsfrag.SNI`, `tlsfrag.HasECH` (Task 4); `Decision.SNI/Connect` (Task 1).
- Produces:
  - Outcomes: `OutcomeFakeSNI = "fakesni"`, `OutcomeFakeSNIFallback = "fakesni_fallback"`, `OutcomeFakeSNIClientRejected = "fakesni_client_rejected"`, `OutcomeFakeSNIVerifyFailed = "fakesni_verify_failed"` (the last is recorded together with the fallback outcome).
  - `func (d *Dialer) Plan(t wire.Target, hello []byte) (dec rules.Decision, host string, ok bool)` — `ok` when hello has a non-IP SNI, no ECH, and the decision (matched on SNI) has `SNI != ""`.
  - `func (d *Dialer) OpenRaw(ctx context.Context, client netip.Addr, t wire.Target, dec rules.Decision) (net.Conn, error)` — rules/SSRF/upstream like `Open` but writes nothing; resolves `dec.Connect` when set; applies a fragment-on-first-write wrapper only when `dec.Fragment == FragOn`.
  - `proxy.Config.MITM func() mitm.LeafSource` — nil func or nil result = Fake SNI inactive.
  - `proxy.Stats` gains `FakeSNI, FakeSNIFallback, FakeSNIClientRejected, FakeSNIVerifyFailed uint64` (json camelCase).

- [ ] **Step 1: Write failing tests**
  - Dialer: `TestOpenRaw_ConnectResolvesOtherHost` (resolver fake records it was asked `www.google.com`, not `youtube.com`); `TestOpenRaw_NoSystemResolver` (existing trap resolver pattern); `TestPlan_SkipsECHAndIP`.
  - Server: `TestTunnel_FakeSNI` — fake TLS server (test roots injected through a `Params.Roots` hook on the server config: add `proxy.Config.Roots *x509.CertPool` for tests, nil in prod) requires SNI `www.google.com`; client through the proxy with the session CA trusted receives the body; stats `FakeSNI == 1`.
  - `TestTunnel_FakeSNIFallback` — server rejects the fake SNI → connection still succeeds through the 2A path with the original hello (client trusts only the real server cert) and stats `FakeSNIFallback == 1`.
  - `TestTunnel_FakeSNIInactive` — `MITM` returns nil → 2A path, no interception (client sees the real server cert).
  - `TestTunnel_ClientRejects` — client without the session CA → connection closed, `FakeSNIClientRejected == 1`.
- [ ] **Step 2: Run** `go test ./internal/proxy/...` — FAIL.
- [ ] **Step 3: Implement** in `tunnel` after `ReadHello`: if `s.cfg.MITM != nil`, `leaf := s.cfg.MITM()`, `dec, host, ok := Plan(...)`; on ok → `OpenRaw` → `mitm.DialServer`; error → close raw, record outcome, continue to the existing `Open` call with the untouched hello; success → `mitm.AcceptClient` → `relay(clientTLS, bufio.NewReader(clientTLS), serverTLS)` (relay already takes `net.Conn`). Panics are already recovered per connection.
- [ ] **Step 4: Run** `go test -race ./internal/proxy/...` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(proxy): Fake SNI branch with connect= and automatic fallback to the 2A path"`

---

### Task 10: Engine — DoH and LAN listeners

**Files:**
- Modify: `internal/engine/engine.go`
- Create: `internal/engine/serve.go`
- Test: `internal/engine/serve_test.go`

**Interfaces:**
- Produces:

```go
type ServeConfig struct {
	DoH       []netip.AddrPort          // loopback (+ LAN IPs when sharing)
	Plain     []netip.AddrPort          // LAN IPs :53, UDP+TCP
	Cert      func() *tls.Certificate   // current DoH leaf
}
type ServeResult struct { Bound []netip.AddrPort; Skipped map[netip.AddrPort]string }  // per-address bind error text
func (e *Engine) Serve(ctx context.Context, sc ServeConfig) (ServeResult, error)       // error only when no DoH loopback address bound
func (e *Engine) StopServe(ctx context.Context) error
func (e *Engine) ServeStats() ServeStats   // {Queries uint64; Clients10m int; ClientIPs []string}
```

Source filtering uses `winutil.IsPrivateOrLocal(a netip.Addr) bool` (new in `internal/winutil/lanip.go`, also used by Task 11): loopback, RFC 1918, `fc00::/7`, link-local.

The extra listeners run in a second `dnsproxy` instance that shares the engine's upstream config, `handle` and rules (construct with the same `RequestHandler: proxy.HandlerFunc(e.handle)`), with `HTTPSListenAddr`, `TLSConfig{GetCertificate: …, MinVersion: TLS12, NextProtos: h2,http/1.1}`, `Ratelimit: 100`, and `UDPListenAddr/TCPListenAddr` for `Plain`. Each address is bound on its own instance attempt so one failing IP is skipped, not fatal. `Swap` also swaps the serve instance's upstreams. `handle` answers `REFUSED` for `!winutil.IsPrivateOrLocal(d.Addr.Addr())` and for `Qtype == ANY`.

- [ ] **Step 1: Write failing tests** (upstream = existing fake upstream helper; ports 0 on 127.0.0.1)
  - `TestServe_DoHGetPost` — RFC 8484 GET (`?dns=` base64url) and POST return the fake upstream's answer; HTTP/2 negotiated.
  - `TestServe_PlainUDPTCP` — `miekg/dns` client over UDP and TCP.
  - `TestServe_RefusesPublicSource` — call `e.handle` with a `DNSContext` whose `Addr` is `8.8.8.8:5353` → `Rcode == REFUSED`, upstream not called.
  - `TestServe_RefusesANY`.
  - `TestServe_SkipsBusyAddr` — pre-bind one address; `Skipped` names it, `Bound` has the rest.
  - `TestServe_CertRotates` — swap the cert func result; a new TLS connection sees the new leaf.
  - `TestServe_NoLeak` — resolver trap: no system resolver calls during the above.
- [ ] **Step 2: Run** `go test ./internal/engine/ -run Serve` — FAIL.
- [ ] **Step 3: Implement** as described.
- [ ] **Step 4: Run** `go test -race ./internal/engine/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(engine): DoH and LAN DNS listeners sharing the encrypted engine"`

---

### Task 11: `internal/dnsserver`, winutil firewall rules and SSID

**Files:**
- Create: `internal/dnsserver/addrs.go`, `setuppage.go`, `setuppage_vi.html`, `setuppage_en.html`, `internal/winutil/wlan_windows.go`
- Modify: `internal/winutil/firewall.go`, `firewall_windows.go`
- Test: `internal/dnsserver/addrs_test.go`, `setuppage_test.go`, `internal/winutil/firewall_test.go`

**Interfaces:**
- Produces:

```go
// dnsserver
func ListenPlan(lan []netip.Addr, dohPort int, share bool) (doh, plain []netip.AddrPort)  // loopback v4/v6 always for DoH; LAN only when share
type SetupFiles struct { CRT []byte; MobileConfig []byte /* nil when no SSID */; Fingerprint string }
type SetupPage struct{ /* … */ }
func NewSetupPage(files SetupFiles, now func() time.Time) *SetupPage
func (p *SetupPage) Start(ctx context.Context, addrs []netip.AddrPort, life time.Duration) error  // life = 10 min; serves /, /ghostline-lan-ca.crt, /ghostline.mobileconfig
func (p *SetupPage) Stop() error
func (p *SetupPage) Remaining() time.Duration
// winutil
type FirewallRule struct { Name, Protocol string; Ports []int }
var (RuleDNSTCP = "Ghostline DNS (TCP)"; RuleDNSUDP = "Ghostline DNS (UDP)"; RuleSetup = "Ghostline Setup")
func FirewallRuleArgs(r FirewallRule, exe string) []string
func AddNamedRule(r FirewallRule, exe string) error   // delete-then-add like AddFirewallRule
func DeleteNamedRule(name string) error               // idempotent like DeleteFirewallRule
func CurrentSSID() (string, error)                    // WlanOpenHandle/WlanEnumInterfaces/WlanQueryInterface(wlan_intf_opcode_current_connection); "" when not on Wi-Fi
```

- [ ] **Step 1: Write failing tests**
  - `TestListenPlan` — share=false → only `127.0.0.1:443`, `[::1]:443`, no plain; share=true adds each LAN IP for DoH and `:53` for plain.
  - `TestSetupPage_ServesFiles` — `Content-Type` `application/x-x509-ca-cert` and `application/x-apple-aspen-config`; `/` contains the fingerprint; `Accept-Language: vi` returns Vietnamese text.
  - `TestSetupPage_PrivateOnly` — request with `RemoteAddr` `8.8.8.8:1` → 403.
  - `TestSetupPage_ClosesAfterLife` — fake clock past 10 min → listener closed, `Remaining() == 0`.
  - `TestSetupPage_NoMobileConfigWithoutSSID` → 404 for `.mobileconfig`, page text explains.
  - `TestFirewallRuleArgs` — exact argv for DNS TCP (`localport=53,443`), DNS UDP, Setup (`8053`); `profile=private`, `remoteip=localsubnet`, `program=<exe>`.
- [ ] **Step 2: Run** `go test ./internal/dnsserver/ ./internal/winutil/` — FAIL.
- [ ] **Step 3: Implement** — HTML pages embedded with `embed`, rendered with `html/template`; source filtering with `winutil.IsPrivateOrLocal` (Task 10).
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(dnsserver): listen plan, temporary phone setup page; named firewall rules and SSID"`

---

### Task 12: Store v3/v4 and phase D

**Files:**
- Modify: `internal/store/settings.go`, `state.go`, `paths.go`, `internal/app/deps.go`, `errors.go`, `status.go`, `orchestrator.go`, `health.go`, `proxyphase.go` (firewall state now a list)
- Create: `internal/app/dnsphase.go`
- Test: `internal/store/settings_test.go`, `state_test.go`, `internal/app/dnsphase_test.go`, update `internal/app/fakes_test.go`

**Interfaces:**
- Consumes: Tasks 5–7, 10, 11.
- Produces:
  - `store.Settings.DNSServer DNSServerSettings{Enabled, ShareLAN bool; DoHPort int; IOSSSID string}` (json `dnsServer{enabled,shareLan,dohPort,iosSsid}`), `store.Settings.FakeSNI FakeSNISettings{Enabled bool; AckVersion int}` (json `fakeSni{enabled,ackVersion}`); version 4; validation per spec 10 (`dohPort` 1–65535, ≠53, ≠8053, ≠`proxy.port`; SSID ≤ 32 bytes).
  - `store.State` version 3: `Firewall *FirewallState{Rules []string}` — v2 `{"rule":"X"}` loads as `Rules: ["X"]`; `Certs *CertsState{Session []string}`. `CleanState()` returns version 3.
  - `store.Paths.LANCACert = j("lan-ca.crt")`, `LANCAKey = j("lan-ca.key")`.
  - app deps:

```go
type DNSServer interface {
	Serve(ctx context.Context, sc engine.ServeConfig) (engine.ServeResult, error)
	StopServe(ctx context.Context) error
}
type Certs interface {   // shell implements with certs + certstore + paths
	LANCA(ctx context.Context) (*certs.CA, error)            // load or create + install in Root (read back)
	ResetLANCA(ctx context.Context) (*certs.CA, error)
	RemoveLANCA(ctx context.Context) error
	InstallSession(der []byte) error
	RemoveSession(thumbprint string) error
	List() ([]certstore.Cert, error)
}
// Firewall gains:
	AddNamed(r winutil.FirewallRule) error
	DeleteNamed(name string) error
// Deps gains: DNSServer DNSServer; Certs Certs; LANAddrs func() []netip.Addr
```

  - Reason `reasonDNSServer = "dnsserver"`; codes `CodeDNSServerPortInUse = "DNSSERVER_PORT_IN_USE"`, `CodeDNSServerFirewall = "DNSSERVER_FIREWALL"`, `CodeCertInstallFailed = "CERT_INSTALL_FAILED"`, `CodeCertRemoveFailed = "CERT_REMOVE_FAILED"`, `CodeCertKeyUnreadable = "CERT_KEY_UNREADABLE"`, `CodeListSignatureInvalid = "LIST_SIGNATURE_INVALID"`, `CodeSetupPageFailed = "SETUP_PAGE_FAILED"`.
  - `Snapshot.DNSServer DNSServerStatus{Running bool; Addrs []string; Skipped map[string]string; Error *AppError}`.
  - `func (o *Orchestrator) startDNSPhase(ctx) error`, `stopDNSPhase(ctx)`, `ReapplyDNSServer(ctx) error`, `checkDNSHealth(ctx)` (IP set change → stop+start).

- [ ] **Step 1: Write failing tests**
  - Store: `TestSettings_V3toV4Defaults` (dnsServer off, dohPort 443, fakeSni off, ack 0; DPI engine fields untouched); `TestSettings_DoHPortValidation` (53, 8053, proxy port rejected); `TestState_V2FirewallRuleUpgrades`; `TestState_V3RoundTrip`.
  - App (`fakes_test.go` gets `fakeDNSServer`, `fakeCerts`, fake firewall named rules):
    - `TestPhaseD_StepsInOrder` — firewall names written to `state.json` before `AddNamed`; LAN CA requested before `Serve`.
    - `TestPhaseD_NoLoopbackBind` → undo (firewall deleted, state cleared), reason `dnsserver`, `DNSSERVER_PORT_IN_USE`, DNS still protected.
    - `TestPhaseD_PartialBind` — skipped addresses reported, phase OK.
    - `TestPhaseD_KeyUnreadable` → `CERT_KEY_UNREADABLE`, degraded.
    - `TestPhaseD_IPChangeRestarts` — `LANAddrs` changes between health ticks → `StopServe` then `Serve` with new addrs.
    - `TestDisconnect_OrderWithDNSServer` — StopServe before proxy stop, before DNS restore.
- [ ] **Step 2: Run** `go test ./internal/store/ ./internal/app/` — FAIL.
- [ ] **Step 3: Implement** — mirror `startProxyPhase` (`runSteps`, `setSysProxyState` generalised to `setState(fn)`); phase D runs after `startProxyPhase` in `Connect` (`orchestrator.go:212`) and is stopped in `disconnectLocked` per spec 6.3 order. Update `proxyphase.go` to append/remove `winutil.FirewallRuleName` in `Firewall.Rules` instead of setting `Rule`.
- [ ] **Step 4: Run** `go test -race ./internal/store/ ./internal/app/` — PASS (all existing app tests too).
- [ ] **Step 5: Commit** — `git commit -m "feat(app): phase D — LAN CA, DNS firewall rules and DoH/LAN listeners"`

---

### Task 13: Phase S — session CA and Fake SNI activation

**Files:**
- Create: `internal/app/sniphase.go`
- Modify: `internal/app/deps.go`, `errors.go`, `status.go`, `orchestrator.go`, `proxyphase.go`
- Test: `internal/app/sniphase_test.go`

**Interfaces:**
- Consumes: `certs.NewSessionCA`, `certs.NewIssuer`, `Compiled.SNIDomains()`, `Deps.Certs`.
- Produces:
  - `Deps.SetMITM func(mitm.LeafSource)` (shell stores it in an `atomic.Pointer` read by `proxy.Config.MITM`); `Deps.MITMSelfTest func(ctx) error` (S4).
  - `const FakeSNIWarningVersion = 1`; reason `reasonFakeSNI = "fakesni"`; codes `CodeFakeSNINeedsProxy = "FAKESNI_NEEDS_PROXY"`, `CodeFakeSNITooMany = "FAKESNI_TOO_MANY"`, `CodeFakeSNISelfTest = "FAKESNI_SELFTEST_FAILED"`.
  - `Snapshot.FakeSNI FakeSNIStatus{Active bool; Domains int; Thumbprint string; NotAfter time.Time; Error *AppError; NeedsProxy bool}`.
  - `startSNIPhase(ctx) error`, `stopSNIPhase(ctx)`; `ReapplyFakeSNI(ctx) error`. `startProxyPhase` callers re-run phase S after it (ReapplyProxy, health restart).

- [ ] **Step 1: Write failing tests**
  - `TestPhaseS_Skipped` — disabled, ack < version, or zero domains → nothing installed, no reason.
  - `TestPhaseS_NeedsProxy` — proxy not running → `NeedsProxy`, warning code, **not** degraded.
  - `TestPhaseS_ThumbprintBeforeInstall` — fake `InstallSession` asserts `state.json` already lists the thumbprint.
  - `TestPhaseS_InstallFails` — undo removes the thumbprint, reason `fakesni`, `CERT_INSTALL_FAILED`, `SetMITM(nil)`.
  - `TestPhaseS_SelfTestFails` — CA removed, thumbprint removed.
  - `TestPhaseS_TooMany` — 1001 domains → `FAKESNI_TOO_MANY`.
  - `TestPhaseS_KillBetweenSteps` — simulate crash after S2 (stop orchestrator without undo) then run `watchdog.RestoreIfOrphaned` with fake store → session cert removed (pairs with Task 15).
  - `TestDisconnect_SNIFirst` — `SetMITM(nil)` and `RemoveSession` happen before sysproxy restore.
  - `TestReapplyProxy_RerunsSNI`.
- [ ] **Step 2: Run** `go test ./internal/app/ -run 'PhaseS|SNIFirst|RerunsSNI'` — FAIL.
- [ ] **Step 3: Implement** per spec 6.2 table.
- [ ] **Step 4: Run** `go test -race ./internal/app/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): phase S — RAM-only session CA installed before Fake SNI is enabled"`

---

### Task 14: Session CA rotation

**Files:**
- Modify: `internal/app/sniphase.go`, `internal/app/rulesservice.go` (rules compiled hook), `internal/app/health.go`
- Test: `internal/app/sniphase_test.go`

**Interfaces:**
- Produces: `func (o *Orchestrator) OnRulesCompiled()` (called wherever `rules:compiled` is emitted) → debounced 2 s → `rotateSession(ctx, reason string)`; health check calls `rotateSession` when `NotAfter - now < 72h`.

- [ ] **Step 1: Write failing tests** (fake clock + fake ticker)
  - `TestRotate_OnDomainChange` — new CA installed, thumbprint list grows to 2 then shrinks to 1, old CA removed **after** `SetMITM(new)`.
  - `TestRotate_SameDomainsNoop`.
  - `TestRotate_Debounced` — 5 compiled events within 2 s → one rotation.
  - `TestRotate_InstallFailsKeepsOld` — old CA and issuer still active, `CERT_INSTALL_FAILED` warning.
  - `TestRotate_NearExpiry` — clock at 27 days 1 h → rotation.
  - `TestRotate_RemoveOldFails` — `CERT_REMOVE_FAILED` warning, old thumbprint stays in `state.json`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement** the five-step order of spec 5.3. **Step 4: Run** `go test -race ./internal/app/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): rotate the Fake SNI CA on rule changes and before expiry"`

---

### Task 15: Recovery, sweep and `--remove-certs`

**Files:**
- Modify: `internal/watchdog/recover.go`, `internal/cli/cli.go`, `headless.go`, `internal/shell/shell.go` (startup sweep), `build/windows/nsis/*.nsi`
- Test: `internal/watchdog/recover_test.go`, `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `certstore.Store`, `certstore.Sweep`, `certs.SessionPrefix`, `certs.LANPrefix`, `winutil.DeleteNamedRule`.
- Produces: `watchdog.Deps.RemoveCert func(thumbprint string) error`, `watchdog.Deps.DeleteRule func(name string) error` (replaces `DeleteFirewall`, keep a shim for the corrupt-state path that deletes all four names), `watchdog.Deps.SweepSession func(keep []string) error`. CLI flag `--remove-certs` → `cli.Options.RemoveCerts bool`; headless runs `--restore` logic then removes every `Ghostline Fake SNI` and `Ghostline LAN CA` cert and deletes `lan-ca.*`.

- [ ] **Step 1: Write failing tests**
  - `TestRecover_OrderSessionCertFirst` — call log: `RemoveCert` → sysproxy → `DeleteRule`×N → DNS → DPI.
  - `TestRecover_SessionCertBeforeInstall` — thumbprint in state but cert absent in fake store → success, state clean.
  - `TestRecover_RemoveCertFailsKeepsState` — state not cleaned, thumbprint kept.
  - `TestRecover_CorruptStateDeletesAllRules` — all four rule names deleted.
  - `TestRecover_SweepAfterClean`.
  - `TestParse_RemoveCerts`.
- [ ] **Step 2: Run** `go test ./internal/watchdog/ ./internal/cli/` — FAIL.
- [ ] **Step 3: Implement**; NSIS uninstall section: after `--restore` add `ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --remove-certs'` and `nsExec::Exec` deletes for `Ghostline DNS (TCP)`, `Ghostline DNS (UDP)`, `Ghostline Setup`.
- [ ] **Step 4: Run** `go test -race ./internal/watchdog/ ./internal/cli/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(watchdog): remove session CAs first, sweep leftovers, --remove-certs for the uninstaller"`

---

### Task 16: Service bindings

**Files:**
- Create: `internal/app/dnsservice.go`, `fakesniservice.go`, `certsservice.go`
- Modify: `internal/app/service.go` (`ServiceDeps`), `events.go`
- Test: `internal/app/dnsservice_test.go`, `fakesniservice_test.go`, `certsservice_test.go`

**Interfaces:**
- Produces (Wails-bound methods on `*Service`):

```go
type DeviceInfo struct { DNSAddrs []string; DoHURLs []string; Fingerprint string; SSID string; Public bool; SetupURL string; SetupRemainingSec int }
func (s *Service) GetDeviceInfo() DeviceInfo
func (s *Service) SetDNSServer(enabled, shareLAN bool, dohPort int) error      // save + ReapplyDNSServer
func (s *Service) SetIOSSSID(ssid string) error
func (s *Service) OpenSetupPage() (string, error)                              // URL for the QR; adds RuleSetup to state + firewall; auto-close removes it
func (s *Service) CloseSetupPage() error
func (s *Service) SaveDeviceFiles() error                                      // native save dialog via ServiceDeps.SaveFile
func (s *Service) ResetLANCA() error
func (s *Service) RemoveLANCA() error                                          // turns DNS server off first
func (s *Service) AckFakeSNIWarning() error                                    // ackVersion = FakeSNIWarningVersion
func (s *Service) SetFakeSNI(enabled bool) error
func (s *Service) GetFakeSNIView() FakeSNIView                                 // {Ack bool; Enabled bool; Rules []rules.Rule (effective sni rules); Presets []lists.List; Stats proxy.Stats}
func (s *Service) SetListTrustedForSNI(id string, trusted bool) error
func (s *Service) ListCerts() ([]certstore.Cert, error)
func (s *Service) RemoveAllCerts() error                                       // stops phase S and D first
func (s *Service) RetryCertRemoval() error
```

  Events: `dnsserver:stats`, `fakesni:stats`, `certs:changed`, `setup:countdown` (once per second while open).

- [ ] **Step 1: Write failing tests** — one per method's contract: `SetDNSServer` rejects 8053; `OpenSetupPage` writes `Ghostline Setup` into `state.json` before adding the rule and removes both on close/expiry; `AckFakeSNIWarning` stores version 1; `SetFakeSNI(true)` without ack → error `FAKESNI_NOT_ACKED`; `RemoveAllCerts` order (phase S stop → phase D stop → remove); `GetDeviceInfo` with no SSID returns `SSID == ""` and the profile URL empty.
- [ ] **Step 2–4:** run `go test ./internal/app/`, implement, run with `-race`.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): DNS server, Fake SNI and certificate bindings"`

---

### Task 17: Shell wiring

**Files:**
- Create: `internal/shell/certwire.go`, `internal/shell/dnswire.go`
- Modify: `internal/shell/shell.go`, `proxywire.go`
- Test: `internal/shell/certwire_test.go` (logic with `certstore.Fake`), `wiring_test.go` in app if present

**Interfaces:**
- Consumes: everything above.
- Produces: `certWiring` implementing `app.Certs` (paths `LANCACert/LANCAKey`, `winutil.ProtectMachine`, `certstore.NewWindows(LocalMachine)`); `dnsWiring` implementing `app.DNSServer` over the engine and building the leaf from the LAN CA for the current `LANAddrs`; `proxyWiring` passes `MITM: func() mitm.LeafSource { return w.mitm.Load() }`; MITM self-test (S4) uses a loopback TLS server with a throwaway "public" CA passed as `Roots`. `lists.Fetcher.SigKey = serverListKey()`. Startup sweep runs after `Recover`.

- [ ] **Step 1: Write failing tests** — `TestCertWiring_LANCAInstalledOnce` (second call does not reinstall when present), `TestCertWiring_LeafReissuedOnIPChange`, `TestCertWiring_LeafReissuedNearExpiry` (< 14 days).
- [ ] **Step 2–4:** run `go test ./internal/shell/`, implement, run; then `go build ./...` and `go vet ./...`.
- [ ] **Step 5: Commit** — `git commit -m "feat(shell): wire certificates, DNS server and Fake SNI"`

---

### Task 18: Frontend — DNS server page

**Files:**
- Create: `frontend/src/modes/advanced/pages/DnsServer.tsx`, `dnsserver.test.tsx`
- Modify: `AdvancedView.tsx` (sidebar order: …Rules · DNS server · Fake SNI · Logs · Settings), `Overview.tsx`, simple-mode stats panel, `frontend/src/app/api.ts` (export `DeviceInfo`, `FakeSNIView`, `Cert`), `i18n/vi.json`, `en.json`
- Regenerate bindings: `wails3 generate bindings` (same command the project already uses; check `build/Taskfile.yml`).

- [ ] **Step 1: Write failing tests** (mock `Service` like `proxy.test.tsx`)
  - renders toggles and DoH port; saving port 8053 shows the validation message.
  - "Open phone setup page" shows QR (`QRCode` component) and a countdown that reaches 0 and hides the QR.
  - iOS profile button disabled with the explanation when `SSID` is empty.
  - Public network hint shown when `public`.
  - skipped addresses listed with their reason.
  - Overview shows the DNS server card; simple mode shows "DNS cho LAN: 192.168.1.5" when running with `shareLan`.
- [ ] **Step 2: Run** `cd frontend && npm test -- --run dnsserver` — FAIL.
- [ ] **Step 3: Implement**, reusing `QRCode` and existing card/toggle components from `Proxy.tsx`.
- [ ] **Step 4: Run** `npm test -- --run` (all, including i18n parity) — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): DNS server page with phone setup QR and LAN CA card"`

---

### Task 19: Frontend — Fake SNI page, warning, banner, rules and certificates

**Files:**
- Create: `pages/FakeSni.tsx`, `pages/FakeSniWarning.tsx`, `components/FakeSniBanner.tsx`, `fakesni.test.tsx`
- Modify: `App.tsx` (banner above both modes), `Overview.tsx` (Fake SNI card), `internal/shell/traytext.go` + `traytext_test.go` (tooltip line "Fake SNI: bật/on" when `snapshot.FakeSNI.Active`), `RulesTable.tsx` (SNI, Connect columns; drop "needs phase 2B" label), `Rules.tsx` (tester text), `Lists.tsx` (trusted-for-SNI toggle with confirm dialog, signature state), `Settings.tsx` (certificates section), `i18n/*.json`

- [ ] **Step 1: Write failing tests**
  - Warning: "Continue" disabled until scrolled to bottom **and** checkbox ticked; click calls `AckFakeSNIWarning`.
  - After ack, the page shows the main switch; with proxy off it shows "Needs Proxy" with an enable button calling `SetProxyEnabled(true)`.
  - Banner renders in simple and advanced modes when `snapshot.fakeSni.active`, shows the domain count, has no close button, "Turn off" calls `SetFakeSNI(false)`.
  - Rules table shows `sni=`/`connect=` values; the old "needs 2B" label is gone; tester shows "will decrypt with Fake SNI (sni=…, connect=…)" and the `sniIgnored` reason.
  - Lists: turning on "Trust for Fake SNI" opens a confirmation listing the domain count; cancel leaves it off.
  - Overview shows the Fake SNI card (on/off, domain count); Go test `TestTrayText_FakeSNI` for the tooltip line.
  - Settings: certificate list renders rows; "Remove all Ghostline certificates" asks for confirmation then calls `RemoveAllCerts`.
- [ ] **Step 2: Run** `npm test -- --run fakesni` — FAIL.
- [ ] **Step 3: Implement**; warning copy (VI + EN) covers every point of spec 9.1 verbatim in meaning.
- [ ] **Step 4: Run** `npm test -- --run` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): Fake SNI page, mandatory warning, persistent banner and certificate list"`

---

### Task 20: Presets — verify which fronting pairs work, ship signed lists

This task starts with a manual spike on a real blocked network; its outcome decides the list content.

**Files:**
- Create: `lists/fakesni/google.txt` (+ others that pass), their `.sig`, `lists/fakesni/README.md` (how to test and sign)
- Modify: `lists/embed.go` (embed `fakesni/*.txt` and `.sig` as the built-in fallback), `internal/rules/lists/catalog.json`
- Test: `lists/embed_test.go` (every embedded preset parses as `ghostline`, has `# ghostline-rules v1`, verifies against `brand.ServerListPublicKeyHex`, only domain patterns carry `sni=`)

- [ ] **Step 1: Spike (manual, record results in `lists/fakesni/README.md`)** — with a dev build connected, proxy + system proxy on, both DPI engines off, try candidate pairs from spec 8.2 (`youtube.com`/`*.googlevideo.com`/`*.ytimg.com` → `sni=www.google.com connect=www.google.com`; Fastly via `www.python.org`; Vercel via `nextjs.org`; Netlify via `kubernetes.io`) against a site from each CDN that is blocked on the test network. Keep a group only if pages load and video plays.
- [ ] **Step 2: Write failing test** `TestEmbeddedFakeSNIPresets` (as above). Run `go test ./lists/` — FAIL.
- [ ] **Step 3: Write the lists, sign each** — `go run ./tools/genservers -sign-file lists/fakesni/google.txt -sign-env SERVERLIST_SIGNING_KEY` (key held by the maintainer; ask the user to run this step if the key is not available in the environment).
- [ ] **Step 4: Run** `go test ./lists/ ./internal/rules/lists/` — PASS.
- [ ] **Step 5: Commit** — `git commit -m "feat(lists): signed Fake SNI presets verified on a blocked network"`

---

### Task 21: Docs and release v0.4.0

**Files:**
- Modify: `README.md`, `README.vi.md`, `docs/user-guide.md`, `docs/huong-dan-su-dung.md`, `docs/release-checklist.md`, version files used by the previous `chore(release)` commit (`git show c51a733 --stat` lists them)

- [ ] **Step 1:** Guides: DNS server (this PC, LAN, iOS profile + full trust + OnDemand SSID, Android static DNS, Steam Deck per-network DNS, router/TV), Fake SNI (what it decrypts, this-PC only, pinning, Firefox `security.enterprise_roots.enabled`), removing certificates by hand with `certlm.msc`.
- [ ] **Step 2:** Release checklist: every manual item of spec 12 (iPhone at home/4G/PC off, Android, Steam Deck Game Mode vs Desktop mode, router, each preset, `certlm.msc` after Disconnect/`taskkill /F`/reboot/uninstall, Chrome/Edge/Firefox, Wireshark no plain DNS) and "re-verify presets".
- [ ] **Step 3:** Run full verification: `go vet ./... && go test -race ./... && cd frontend && npm test -- --run && npm run build` — all PASS.
- [ ] **Step 4:** Bump version to 0.4.0 in the same files as `c51a733`.
- [ ] **Step 5: Commit** — `git commit -m "chore(release): v0.4.0"` (tagging and publishing are left to the user).
