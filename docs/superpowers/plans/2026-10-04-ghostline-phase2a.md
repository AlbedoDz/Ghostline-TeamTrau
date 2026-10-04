# Ghostline Giai đoạn 2A — Kế hoạch triển khai

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Mục tiêu:** Thêm proxy HTTP/HTTPS/SOCKS chạy cùng Connect (dùng cho máy này qua System Proxy và chia sẻ LAN), Fragment tự động cho lưu lượng web, và rules theo domain/keyword/regexp/CIDR kèm danh sách cộng đồng tải từ GitHub.

**Kiến trúc:** Package mới một nhiệm vụ: `tlsfrag` (cắt ClientHello), `rules` (parse, định dạng cộng đồng, biên dịch, khớp, tải), `proxy` + `proxy/dialer` (proxy tự viết), `sysproxy` (WinINET), `qr`. `engine` thêm `Resolve` và áp dụng rules. `app` chạy "pha P" (P1–P4) sau saga DNS 01–07; pha P lỗi chỉ đưa về SUY_GIẢM. `watchdog` khôi phục system proxy và firewall trước DNS.

**Tech stack:** Go 1.26 (module hiện có) · `github.com/miekg/dns` · `golang.org/x/sys/windows` · `golang.org/x/net` (`idna`; `proxy` chỉ trong test) — nâng từ indirect lên direct · testify · test-only: `github.com/makiuchi-d/gozxing` (giải mã QR) · React 18 + TS + Vite · Zustand · i18next · Vitest + Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-04-ghostline-phase2a-design.md` (dựa trên spec giai đoạn 1). Người thực thi đọc spec trước. Kế hoạch và spec mâu thuẫn thì spec thắng; ghi chú mâu thuẫn trong báo cáo task.

## Ràng buộc chung

- Mọi ràng buộc chung của kế hoạch giai đoạn 1 (`docs/superpowers/plans/2026-10-04-ghostline-phase1.md`, mục "Ràng buộc chung") vẫn áp dụng, **trừ** dòng về trailer commit: **commit KHÔNG có dòng `Co-Authored-By`** (yêu cầu của người dùng).
- **Proxy không bao giờ dùng DNS hệ thống.** Trong `internal/proxy/...` cấm `net.DefaultResolver`, `net.LookupHost`, `net.LookupIP`, `net.Dial*` với tên miền. Mọi kết nối ra ngoài dial bằng `netip.AddrPort`. `golangci-lint` thêm luật `forbidigo` cho các tên trên trong `internal/proxy/`.
- **Không ghi tên miền hoặc IP đích xuống đĩa** (log file chỉ ghi mã và bộ đếm). Danh sách kết nối gần đây: tối đa 500, chỉ RAM.
- Không thêm dependency cho lõi ngoài Tech stack. `gozxing` chỉ được import trong file `_test.go`.
- Giá trị cố định từ spec: cổng mặc định `8080`; bắt tay 10 s, header 8 KB; 64 kết nối/IP, 1024 tổng; đọc ClientHello chờ 5 s / tối đa 16 KB; `autoTimeoutMs` 3000; idle 5 phút; dừng chờ 2 s; tự kiểm tra P1 2 s; thử IPv6 sau 3 s; `chunks` 5, `delayMs` 5, `cacheDays` 7; rule viết tay ≤ 10.000; regexp ≤ 1.000/danh sách; file ≤ 50 MB sau giải nén; tổng ≤ 2.000.000 mục; tải timeout 30 s; lịch: trễ 30 s lúc mở app, kiểm tra 15 phút, jitter 0–10 phút; `include:` sâu ≤ 5; nhận diện 200 dòng, ngưỡng 60 %; TTL câu trả lời rules 60 s; tra < 5 µs, < 250 MB cho 2 triệu mục.
- Tên cố định: luật firewall `Ghostline Proxy`; mã lỗi đúng như spec mục 10; sự kiện Wails `proxy:stats`, `proxy:conn`, `rules:compiled`, `lists:progress`.
- `state.json` và `settings.json` lên **version 2**; đọc được version 1.
- Mọi chuỗi giao diện mới có trong `vi.json` và `en.json` (test parity hiện có phải đạt).

## Trọng tâm review

Những đầu vào spec ngầm đòi hỏi mà dễ bị bỏ sót nhất, đã gắn test vào task sở hữu:

1. **Client gửi ClientHello lớn, bị TCP cắt thành nhiều lần `Read`** (Chrome với post-quantum key share ~1.8 KB, thường > 1 MSS) → proxy phải đọc đủ một bản ghi TLS theo độ dài trong header trước khi cắt, không cắt nửa bản ghi. Test `TestReadHello_AssemblesRecordAcrossReads` (Task 9).
2. **Trình duyệt dùng HTTP forward keep-alive tới nhiều host khác nhau trên cùng một kết nối proxy** → mỗi host mới mở kết nối upstream mới, rules được xét lại cho từng host. Test `TestHTTPForward_KeepAliveSwitchesHost` (Task 10).
3. **Người dùng nhập tên miền có dấu hoặc in hoa, có dấu chấm cuối** (`Bánh.VN.`) trong rule, còn trình duyệt gửi punycode chữ thường → vẫn khớp. Test `TestMatch_IDNAndCaseAndTrailingDot` (Task 3).
4. **File danh sách có BOM UTF-8, dòng kết thúc CRLF, hoặc chú thích cuối dòng** (`ads.com # tracker`) → đọc đúng, không tính là dòng bỏ qua. Test `TestFormats_BOMCRLFAndInlineComments` (Task 4).
5. **Bấm Disconnect đúng lúc đang chạy lại pha P sau khi đổi cài đặt** → không để system proxy trỏ vào proxy đã tắt; thao tác được tuần tự hoá bằng `opMu`. Test `TestProxyPhase_ReapplyRacesDisconnect` (Task 15).

---

## Bản đồ file

```
internal/tlsfrag/{hello,split,record}.go          ClientHello: nhận diện, SNI, cắt tcp/record/both
internal/fragdoh/{split.go→xoá, conn.go}           dùng tlsfrag
internal/rules/rule.go                             Rule, Pattern, Action, Decision
internal/rules/text.go                             ParseText / FormatText
internal/rules/compile.go, match.go, explain.go    Compiled, Holder, Match, Explain
internal/rules/formats/*.go                        10 parser + detect
internal/rules/lists/{list,fetch,github,schedule,catalog}.go + catalog.json   tải, chuẩn hoá link, lịch, danh mục (package lists: import rules + formats, tránh vòng import)
internal/store/{settings,state,rulesfile,fragcache,jsonfile,paths}.go
internal/engine/engine.go                          Resolve + rules trong handle
internal/proxy/wire/{request,socks4,socks5,http}.go đọc request (package wire, dùng chung cho proxy và dialer, tránh vòng import)
internal/proxy/{server,relay,limits,stats}.go      listener, chuyển dữ liệu, giới hạn, thống kê
internal/proxy/dialer/{dialer,autofrag,upstream,ssrf,hello}.go
internal/sysproxy/{sysproxy.go,api_windows.go,watch_windows.go}
internal/winutil/{firewall_windows,netprofile_windows,lanip,dpapi_windows}.go
internal/qr/qr.go
internal/watchdog/recover.go
internal/app/{proxyphase,proxydeps,proxyservice,rulesservice}.go + sửa orchestrator.go, health.go, status.go, errors.go, deps.go
internal/shell/{shell.go,proxywire.go,ui.go}
build/windows/nsis/project.nsi
frontend/src/app/{api.ts,store.ts}, i18n/{vi,en}.json
frontend/src/components/neon/QRCode.tsx
frontend/src/modes/advanced/pages/{Proxy.tsx,Rules.tsx,RulesTable.tsx,RulesText.tsx,Lists.tsx}
README.md, README.vi.md, docs/{user-guide,huong-dan-su-dung,release-checklist}.md
```

---

### Task 1: `tlsfrag` — cắt ClientHello dùng chung

**Files:**
- Create: `internal/tlsfrag/hello.go`, `internal/tlsfrag/split.go`, `internal/tlsfrag/record.go`, `internal/tlsfrag/tlsfrag_test.go`
- Delete: `internal/fragdoh/split.go` (chuyển logic `sniRange` sang `tlsfrag/hello.go`)
- Modify: `internal/fragdoh/conn.go` (gọi `tlsfrag.Split`), test hiện có của `fragdoh` phải vẫn đạt

**Interfaces:**
- Produces:
  ```go
  type Method string // "tcp" | "record" | "both"
  const (MethodTCP Method = "tcp"; MethodRecord Method = "record"; MethodBoth Method = "both")
  func IsClientHello(rec []byte) bool
  func SNI(rec []byte) (string, bool)
  func RecordLen(hdr []byte) (int, bool)          // tổng độ dài bản ghi (5 + length) từ 5 byte header
  // Split trả về các segment cần ghi lần lượt (mỗi phần tử = một lần Write, nghỉ delay giữa chúng).
  func Split(rec []byte, m Method, chunks int) [][]byte
  ```
  `Split` với dữ liệu không phải ClientHello có SNI trả `[][]byte{rec}`.

- [ ] **Step 1: Viết test thất bại** trong `tlsfrag_test.go`, dùng ClientHello thật sinh bằng `crypto/tls` (client ghi vào `net.Pipe`, đọc bản ghi đầu):
  - `TestSplit_TCPJoinsBack`: `bytes.Join(Split(rec, MethodTCP, 5), nil) == rec`; có đúng `5+2` segment; segment 1 kết thúc ngay trước SNI.
  - `TestSplit_RecordIsValidTLS`: `Split(rec, MethodRecord, 4)` trả **1** segment; đọc nó thành chuỗi bản ghi: mỗi bản ghi type `0x16`, version giống bản gốc, length khớp; ghép payload các bản ghi == payload bản ghi gốc; có đúng 4 bản ghi; ranh giới bản ghi đầu nằm trong khoảng byte của SNI.
  - `TestSplit_BothIsRecordsAsSegments`: `Split(rec, MethodBoth, 4)` trả 4 segment, mỗi segment là một bản ghi TLS hoàn chỉnh; ghép lại == segment duy nhất của `MethodRecord`.
  - `TestSplit_NotHelloUnchanged`: `[]byte("GET / HTTP/1.1\r\n")` và bản ghi `0x17` → trả nguyên.
  - `TestSNI`: trả đúng `"example.com"`; `IsClientHello` đúng/sai.
  - `FuzzSplit`: với mọi đầu vào và mọi method, `Split` không panic; với `MethodTCP` ghép lại == đầu vào.
- [ ] **Step 2: Chạy** `go test ./internal/tlsfrag/` — Expected: FAIL (package chưa có).
- [ ] **Step 3: Cài đặt.** `record`: cắt **payload** handshake (sau header 5 byte) tại các điểm: điểm đầu = giữa SNI, phần còn lại chia đều cho `chunks-1` bản ghi; mỗi bản ghi = `0x16, ver[0], ver[1], len>>8, len&0xff` + phần payload. `both`: kết quả `record` tách thành từng bản ghi.
- [ ] **Step 4: Sửa `fragdoh/conn.go`** dùng `tlsfrag.Split(p, tlsfrag.MethodTCP, c.chunks)`, xoá `fragdoh/split.go`, chuyển test cũ của split (nếu có) sang `tlsfrag`.
- [ ] **Step 5: Chạy** `go test ./internal/tlsfrag/ ./internal/fragdoh/ && go test -fuzz=FuzzSplit -fuzztime=30s ./internal/tlsfrag/` — Expected: PASS.
- [ ] **Step 6: Commit** `refactor(tlsfrag): shared ClientHello splitting with TLS record mode`

---

### Task 2: `rules` — model và định dạng text

**Files:**
- Create: `internal/rules/rule.go`, `internal/rules/text.go`, `internal/rules/text_test.go`
- Modify: `go.mod` (nâng `golang.org/x/net` lên direct)

**Interfaces:**
- Produces:
  ```go
  type PatternKind uint8 // KindDomain (domain+sub), KindExact (=), KindSubOnly (*.), KindKeyword (~), KindRegexp (/../), KindCIDR
  type Pattern struct { Kind PatternKind; Value string; Prefix netip.Prefix } // Value đã chuẩn hoá (punycode, chữ thường, không dấu chấm cuối)
  type Frag string // "", "auto", "on", "off"
  type Action struct {
      Block, Allow bool
      IPs      []netip.Addr `json:"ips,omitempty"`
      Fragment Frag         `json:"fragment,omitempty"`
      Upstream string       `json:"upstream,omitempty"`
      SNI      string       `json:"sni,omitempty"` // chừa cho 2B, 2A bỏ qua
  }
  type Rule struct {
      Pattern string `json:"pattern"` // dạng text gốc, ví dụ "*.ads.com"
      Action
      Enabled bool   `json:"enabled"`
      Comment string `json:"comment,omitempty"`
  }
  type LineError struct { Line int; Msg string }
  func NormalizeHost(s string) (string, error)       // idna.Lookup.ToASCII + lower + bỏ "."
  func ParsePattern(s string) (Pattern, error)
  func ParseText(text string, upstreamIDs []string) ([]Rule, []LineError)
  func FormatText(rs []Rule) string
  const MaxUserRules = 10000
  ```

- [ ] **Step 1: Viết test thất bại** `text_test.go` (dạng bảng):
  - `TestParseText_Valid`: mọi dòng ví dụ trong spec mục 7.1 parse ra đúng `Rule` (ví dụ `example.org ip=1.2.3.4 fragment=off` → `IPs=[1.2.3.4]`, `Fragment="off"`); `ip=` lặp lại cho cả v4 và v6; chú thích cuối dòng thành `Comment`.
  - `TestParseText_Errors`: `block allow` cùng dòng; `block fragment=on`; `ip=abc`; `fragment=maybe`; `upstream=nope` (không có trong `upstreamIDs`); regexp không biên dịch được; CIDR sai; dòng thứ 10.001 → mỗi lỗi có `Line` đúng (1-based).
  - `TestParseText_DisabledRoundTrip`: `#! youtube.com fragment=on` → `Enabled=false`; `ParseText(FormatText(rs))` == `rs` cho một bộ rule hỗn hợp.
  - `TestParseText_SNIAccepted`: `x.com sni=y.com` parse đạt, `SNI="y.com"`.
  - `TestNormalizeHost`: `"Bánh.VN."` → `"xn--bnh-hoa.vn"`; `"EXAMPLE.com"` → `"example.com"`.
- [ ] **Step 2: Chạy** `go test ./internal/rules/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** theo Interfaces. Một dòng = mẫu + các token `key=value` hoặc `block`/`allow`, tách bằng khoảng trắng; `#` không nằm trong `/regexp/` bắt đầu chú thích; dòng `#!` là rule bị tắt.
- [ ] **Step 4: Chạy** `go test ./internal/rules/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(rules): rule model and text format`

---

### Task 3: `rules` — biên dịch, khớp, Explain, đổi nóng

**Files:**
- Create: `internal/rules/compile.go`, `internal/rules/match.go`, `internal/rules/explain.go`, `internal/rules/match_test.go`

**Interfaces:**
- Consumes: Task 2.
- Produces:
  ```go
  // Entry là một mục của danh sách (Task 4 tạo ra). Action chỉ dùng khi danh sách có action "fromFile".
  type Entry struct { Pattern Pattern; Except bool; IPs []netip.Addr; Line int }
  type ListSet struct { ID string; Action Action; FromFile bool; Entries []Entry }
  type Source struct { Kind string; Index int; ListID string; Line int } // Kind: "rule" | "list" | ""
  type Decision struct { Action; Source Source }
  type Compiled struct { /* unexported */ }
  func Compile(user []Rule, lists []ListSet) (*Compiled, error)
  func (c *Compiled) Match(host string, ip netip.Addr) Decision // host "" hoặc ip zero khi không có
  func (c *Compiled) Explain(host string) Decision              // = Match(NormalizeHost(host), zero)
  func (c *Compiled) Count() int
  type Holder struct { /* atomic.Pointer[Compiled] */ }
  func (h *Holder) Load() *Compiled   // không bao giờ nil (rỗng nếu chưa Store)
  func (h *Holder) Store(c *Compiled)
  ```
  Ngữ nghĩa theo spec 7.4: rule viết tay đang bật, rule đầu tiên khớp thắng; sau đó danh sách theo thứ tự, danh sách đầu tiên khớp thắng; `Except` trong một danh sách làm danh sách đó không khớp; `Allow` dừng. Mẫu CIDR chỉ điền các trường chưa đặt sau khi đã khớp domain.

- [ ] **Step 1: Viết test thất bại** `match_test.go`:
  - `TestMatch_PatternKinds`: `ads.com` khớp `ads.com` và `x.ads.com`, không khớp `badads.com`; `=ads.com` không khớp `x.ads.com`; `*.ads.com` không khớp `ads.com`; `~adservice` khớp `pagead.adservice.google.com`; `/^ad[0-9]+\./` khớp `ad12.x.com`; `10.0.0.0/8` khớp ip `10.1.2.3`.
  - `TestMatch_Order`: rule 2 và rule 5 cùng khớp → `Source.Index==2`; rule thắng danh sách; danh sách 1 thắng danh sách 2; `bank.vn allow` thắng danh sách chặn `bank.vn`.
  - `TestMatch_ListException`: danh sách có `||x.com^` và `@@||ok.x.com^` → `ok.x.com` không khớp danh sách đó nhưng vẫn khớp danh sách sau.
  - `TestMatch_CIDRFillsUnset`: rule `youtube.com fragment=on` + `142.250.0.0/15 upstream=corp` → `Match("youtube.com", 142.250.1.1)` có `Fragment=on` **và** `Upstream=corp`; rule domain `block` thì CIDR không được xét.
  - `TestMatch_IDNAndCaseAndTrailingDot` (Trọng tâm review 3): rule `Bánh.VN.` khớp host `xn--bnh-hoa.vn` và `WWW.xn--bnh-hoa.vn`.
  - `TestHolder_ConcurrentSwap`: 8 goroutine `Load().Match` trong khi một goroutine `Store` 1.000 lần; chạy với `-race`.
  - `BenchmarkMatch2M`: 2.000.000 domain ngẫu nhiên trong một danh sách; mục tiêu < 5 µs/op (ghi kết quả vào báo cáo task, không assert cứng); `testing.AllocsPerRun` của `Match` không có regexp == 0.
- [ ] **Step 2: Chạy** `go test ./internal/rules/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt.** Domain/Exact/SubOnly: `map[string][]ref` theo tên chuẩn hoá, tra từ tên đầy đủ lên từng hậu tố (với hậu tố ≠ tên đầy đủ chỉ nhận `KindDomain`/`KindSubOnly`; với tên đầy đủ chỉ nhận `KindDomain`/`KindExact`). Keyword và regexp duyệt tuần tự. CIDR: cây tiền tố nhị phân riêng v4 và v6. Mỗi `ListSet` biên dịch thành một bộ con.
- [ ] **Step 4: Chạy** `go test -race ./internal/rules/ && go test -bench Match2M -run x ./internal/rules/` — Expected: PASS, benchmark in ra số liệu.
- [ ] **Step 5: Commit** `feat(rules): compiled matcher with explain and hot swap`

---

### Task 4: `rules/formats` — đọc 10 họ định dạng cộng đồng

**Files:**
- Create: `internal/rules/formats/{detect,hosts,domains,adblock,dnsmasq,unbound,rpz,clash,v2fly,singbox,cidr}.go`, `internal/rules/formats/formats_test.go`, `internal/rules/formats/testdata/*` (đoạn trích ngắn ≤ 30 dòng mỗi file, dòng đầu ghi nguồn và license)

**Interfaces:**
- Consumes: `rules.Entry`, `rules.ParsePattern`, `rules.NormalizeHost`.
- Produces:
  ```go
  type Format string // "hosts","domains","adblock","dnsmasq","unbound","rpz","clash","v2fly","singbox","cidr"
  type Result struct {
      Format   Format
      Entries  []rules.Entry
      Includes []string        // chỉ v2fly: tên trong include:
      Counts   map[string]int  // theo loại khớp: "domain","exact","suffix","keyword","regexp","cidr"
      Skipped  int
      Samples  []string        // tối đa 5 dòng bị bỏ qua
  }
  var ErrUnsupported = errors.New("formats: unsupported or binary content")
  func Detect(name string, data []byte) (Format, error)     // name: tên file/đường dẫn URL để xét phần mở rộng
  func Parse(f Format, data []byte) (Result, error)
  const MaxRegexpPerList = 1000
  ```
  Ánh xạ từng định dạng đúng bảng spec 7.3. IP sinkhole: `0.0.0.0`, `127.0.0.1`, `::`, `::1` → `Entry` chặn (IPs rỗng); IP khác → `Entry.IPs`. Clash YAML đọc tay theo dòng (`payload:`, `- 'DOMAIN-SUFFIX,x'`, `- '+.x'`, `- '.x'`), **không** thêm thư viện YAML. sing-box dùng `encoding/json`.

- [ ] **Step 1: Viết test thất bại** `formats_test.go`:
  - `TestDetect_Testdata`: mỗi file trong `testdata/` (ít nhất một file cho mỗi định dạng, tên file chỉ rõ định dạng) → `Detect` đúng; nội dung có byte `0x00` trong 512 byte đầu → `ErrUnsupported`; nội dung văn xuôi → `ErrUnsupported`.
  - `TestParse_Mapping` (dạng bảng, mỗi định dạng ≥ 3 dòng mẫu từ bảng spec) → `Entries` với `Pattern.Kind` đúng; `Counts` và `Skipped` đúng; `Samples` ≤ 5.
  - `TestParse_AdblockModifiers`: `||x.com^$important` nhận; `||x.com^$third-party` bỏ qua; `example.com##.ad` bỏ qua; `@@||ok.com^` → `Except=true`.
  - `TestParse_ClashSkipsUnknown`: `PROCESS-NAME,x`, `GEOIP,CN` bỏ qua; `IP-CIDR,1.0.0.0/8,no-resolve` nhận.
  - `TestParse_V2flyIncludeAndAttrs`: `include:google` vào `Includes`; `x.com @cn` → domain `x.com`.
  - `TestParse_RegexpCap`: 1.001 dòng `regexp:` → 1.000 entry, `Skipped==1`.
  - `TestFormats_BOMCRLFAndInlineComments` (Trọng tâm review 4): file domains có BOM, CRLF, `ads.com # tracker` → 1 entry, `Skipped==0`.
- [ ] **Step 2: Chạy** `go test ./internal/rules/formats/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** từng parser (một file một định dạng) và `Detect` (phần mở rộng trước; sau đó chấm điểm 200 dòng có nghĩa đầu, bỏ dòng bắt đầu `#`, `!`, `;`; chọn điểm cao nhất nếu ≥ 60 %).
- [ ] **Step 4: Chạy** `go test ./internal/rules/formats/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(rules): parse community list formats with auto-detection`

---

### Task 5: `rules/lists` — tải danh sách, link GitHub, lịch, danh mục

**Files:**
- Create: `internal/rules/lists/list.go`, `github.go`, `fetch.go`, `schedule.go`, `catalog.go`, `catalog.json`, `fetch_test.go` (package `lists`; `rules` và `formats` không import `lists`)

**Interfaces:**
- Consumes: Task 3 (`rules.ListSet`, `rules.Action`), Task 4 (`formats.Detect`, `formats.Parse`).
- Produces (package `lists`):
  ```go
  func NormalizeURL(raw string) (primary string, fallback string, err error) // fallback "" nếu không phải GitHub
  type List struct { // lưu trong rules.json (Task 6 dùng lại struct này)
      ID, Name, Source, URL, Path, Format, Action string // Source: "file"|"url"; Action: "block"|"allow"|"fragment=on"|"upstream=<id>"|"fromFile"
      Enabled     bool
      UpdateHours int
      LastUpdated time.Time
      ETag, LastModified, Detected string
      Counts      map[string]int
      Skipped     int
      SkippedSamples []string
      LastError   string
  }
  type Fetcher struct {
      Client  *http.Client
      Dir     string // <data>\lists
      Now     func() time.Time
      WriteFile func(path string, data []byte) error // shell truyền store.WriteFileAtomic; lists không import store
  }
  // Fetch tải (hoặc đọc file), giải nén, nhận diện, parse, ghi cache lists/<id>.txt nguyên tử và cập nhật metadata của l.
  // Lỗi: giữ cache cũ, điền l.LastError, trả lỗi.
  func (f *Fetcher) Fetch(ctx context.Context, l *List) (formats.Result, error)
  func (f *Fetcher) LoadCached(l List) (formats.Result, error)
  func ToListSet(l List, r formats.Result) (rules.ListSet, error) // ánh xạ Action → rules.Action
  type Scheduler struct {
      Lists   func() []List
      Due     func(l List, now time.Time) bool
      Run     func(ctx context.Context, id string) error
      Now     func() time.Time
      After   func(time.Duration) <-chan time.Time
      Jitter  func() time.Duration // 0..10 phút
  }
  func (s *Scheduler) Loop(ctx context.Context)
  type CatalogItem struct { ID, Name, Description, Repo, License, URL, Format, Action string }
  func Catalog() []CatalogItem // go:embed catalog.json
  var ErrTooLarge = errors.New("rules: list too large")
  ```

- [ ] **Step 1: Viết test thất bại** `fetch_test.go`:
  - `TestNormalizeURL` (dạng bảng): `github.com/u/r/blob/main/a/b.txt` → primary `https://raw.githubusercontent.com/u/r/main/a/b.txt`, fallback `https://cdn.jsdelivr.net/gh/u/r@main/a/b.txt`; `github.com/u/r/raw/main/x` → như trên; `cdn.jsdelivr.net/gh/u/r@v1/x` → primary raw, fallback jsDelivr; `gist.github.com/u/abc` → `.../raw`, fallback ""; `http://...` → lỗi (chỉ HTTPS); `https://example.com/x.txt` → nguyên, fallback "".
  - `TestFetch_ETag304`: server `httptest` TLS trả `ETag`, lần 2 nhận `If-None-Match` và trả 304 → `LastUpdated` đổi, cache giữ nguyên.
  - `TestFetch_GzipAndZipBomb`: `.gz` hợp lệ được giải nén; gz giải nén > 50 MB → `ErrTooLarge`, cache cũ còn nguyên.
  - `TestFetch_FallbackJsDelivr`: primary trả lỗi kết nối → dùng fallback (inject `Client` có `Transport` định tuyến host về server test).
  - `TestFetch_ErrorKeepsCache`: lần 2 trả 500 → trả lỗi, `LastError` có nội dung, `LoadCached` vẫn ra dữ liệu lần 1.
  - `TestFetch_V2flyInclude`: file `a` có `include:b`, `b` có `include:a` → không lặp vô hạn; độ sâu 6 bị cắt ở 5; include chỉ cùng host và cùng thư mục.
  - `TestScheduler_DueAndJitter`: đồng hồ giả; danh sách quá hạn được chạy sau 30 s; chạy tuần tự; `UpdateHours=0` không bao giờ chạy.
  - `TestCatalog_Valid`: mọi mục có URL HTTPS, `NormalizeURL` đạt, `License` và `Repo` không rỗng, gồm đủ 6 mục spec.
- [ ] **Step 2: Chạy** `go test ./internal/rules/lists/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt.** `http.Client` timeout 30 s; `io.LimitReader(50<<20 + 1)` sau giải nén; ghi cache qua `Fetcher.WriteFile` (test dùng helper ghi file tạm rồi rename). `catalog.json` gồm hagezi Light, hagezi Pro, StevenBlack Unified, OISD Small, AdGuard DNS filter, hostsVN với URL raw chính thức của từng repo.
- [ ] **Step 4: Chạy** `go test ./internal/rules/...` — Expected: PASS; `go list -deps ./internal/rules ./internal/rules/formats` không chứa `internal/store` hay `internal/rules/lists`.
- [ ] **Step 5: Commit** `feat(rules): fetch community lists with GitHub normalization and schedule`

---

### Task 6: `store` — settings v2, state v2, rules.json, frag-cache

**Files:**
- Modify: `internal/store/settings.go`, `internal/store/state.go`, `internal/store/jsonfile.go`, `internal/store/paths.go`
- Create: `internal/store/rulesfile.go`, `internal/store/fragcache.go`, test tương ứng trong `internal/store/`

**Interfaces:**
- Produces:
  ```go
  // settings.go
  type ProxySettings struct {
      Enabled     bool              `json:"enabled"`
      Port        int               `json:"port"`
      SystemProxy bool              `json:"systemProxy"`
      ShareLAN    bool              `json:"shareLan"`
      Fragment    WebFragment       `json:"fragment"`
      Upstreams   []UpstreamProxy   `json:"upstreams"`
  }
  type WebFragment struct { Mode, Method string; Chunks, DelayMs, AutoTimeoutMs, CacheDays int } // json: mode, method, chunks, delayMs, autoTimeoutMs, cacheDays
  type UpstreamProxy struct { ID, Type, Addr, User, PassEnc string } // json: id, type, addr, user, passEnc
  // Settings thêm: Proxy ProxySettings `json:"proxy"`; DNSBlockMode string `json:"dnsBlockMode"`
  func ValidateProxy(p ProxySettings) error // port 1024–65535 và ≠53; chunks 2–64; delayMs 0–100; autoTimeoutMs 1000–10000; cacheDays 1–90; mode/method hợp lệ; id upstream ^[a-z0-9-]+$ và duy nhất
  // state.go
  type SysProxySnapshot struct { Flags uint32; Server, Bypass, AutoconfigURL string } // json: flags, server, bypass, autoconfigUrl
  type SysProxyState struct { Set, TakenOver bool; Ours string; Snapshot *SysProxySnapshot } // json: set, takenOver, ours, snapshot
  type FirewallState struct { Rule string `json:"rule"` }
  // State thêm: SysProxy *SysProxyState `json:"sysproxy,omitempty"`; Firewall *FirewallState `json:"firewall,omitempty"`
  func CleanState() State // Version 2, Phase clean (thay cleanState; cập nhật mọi chỗ đang tạo State{Version:1, Phase: clean})
  // jsonfile.go
  func WriteFileAtomic(path string, data []byte) error
  // rulesfile.go
  type RulesFile struct { Version int; Rules []rules.Rule; Lists []lists.List } // json: version, rules, lists
  func LoadRules(path string) (RulesFile, bool /*recovered*/, error) // hỏng → rules.json.bak, trả rỗng
  func SaveRules(path string, f RulesFile) error
  // fragcache.go
  type FragCache struct { /* mu + map[netKey]map[host]time.Time */ }
  func LoadFragCache(path string, now time.Time) (*FragCache, error) // dọn mục hết hạn
  func (c *FragCache) Has(netKey, host string, now time.Time) bool
  func (c *FragCache) Add(netKey, host string, until time.Time)
  func (c *FragCache) List(netKey string) []string
  func (c *FragCache) Remove(netKey, host string) // host "" = xoá hết của mạng đó
  func (c *FragCache) Save(path string) error
  // paths.go: Paths thêm Rules ("rules.json"), FragCache ("frag-cache.json"), ListsDir ("lists")
  ```
  `DefaultSettings()` thêm khối mặc định spec mục 9 và `Version: 2`, `DNSBlockMode: "zero"`.

- [ ] **Step 1: Viết test thất bại:**
  - `TestLoadSettings_V1Upgrades`: file v1 của giai đoạn 1 → `Version==2`, `Proxy` bằng mặc định, các trường cũ giữ nguyên.
  - `TestValidateProxy` (bảng): port 53, 80, 70000; chunks 1; delayMs 101; autoTimeoutMs 999; cacheDays 0; mode "x"; id upstream trùng / có chữ hoa → lỗi; mặc định → đạt.
  - `TestState_V1ReadsWithoutProxy`: JSON v1 → `SysProxy==nil`, `Firewall==nil`; `CleanState().Version==2`.
  - `TestRulesFile_CorruptRecovers` và vòng ghi/đọc.
  - `TestFragCache_ExpiryAndPerNetwork`: thêm ở mạng A không thấy ở mạng B; hết hạn bị dọn khi nạp; `Remove("A","")` xoá hết.
  - `TestWriteFileAtomic`.
- [ ] **Step 2: Chạy** `go test ./internal/store/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt**; thay mọi `store.State{Version: 1, Phase: store.PhaseClean}` trong `internal/app` và `internal/watchdog` bằng `store.CleanState()`, và `st.Version = 1` ở bước snapshot bằng `2`.
- [ ] **Step 4: Chạy** `go test ./...` — Expected: PASS (test giai đoạn 1 vẫn đạt).
- [ ] **Step 5: Commit** `feat(store): settings and state v2, rules.json and fragment cache`

---

### Task 7: `engine` — `Resolve` và áp dụng rules

**Files:**
- Modify: `internal/engine/engine.go`
- Test: `internal/engine/rules_test.go`

**Interfaces:**
- Consumes: `rules.Holder`, `rules.Decision`.
- Produces:
  ```go
  // Config thêm:
  //   Rules     func() *rules.Compiled // nil = không có rules
  //   BlockMode string                 // "zero" | "nxdomain"
  func (e *Engine) Resolve(ctx context.Context, host string) ([]netip.Addr, error)
  ```
  `Resolve` gửi A và AAAA song song tới chính listener của engine (`e.addr`, như `SelfTest`) bằng `dns.Client` timeout 3 s, trả IPv4 trước rồi IPv6; NXDOMAIN/không có bản ghi → lỗi `ErrNoAddress`. Không bao giờ chạm resolver hệ thống.
  `QueryEvent` thêm `Action string` (`""`, `"blocked"`, `"rewritten"`).

- [ ] **Step 1: Viết test thất bại** `rules_test.go` (engine trên cổng ngẫu nhiên, upstream DoH giả như test hiện có):
  - `TestRules_BlockZero`: rule `ads.com block` → A `0.0.0.0`, AAAA `::`, MX NODATA, TTL 60; upstream giả **không** nhận truy vấn.
  - `TestRules_BlockNXDomain`: `BlockMode:"nxdomain"` → rcode NXDOMAIN.
  - `TestRules_Rewrite`: `x.lan ip=192.168.1.1` → A đúng; AAAA NODATA; `ip=` có cả v6 → AAAA đúng.
  - `TestRules_VerifyBypasses`: rule `ghostline.test block` vẫn để `<nonce>.verify.ghostline.test` trả `192.0.2.1`.
  - `TestRules_AllowAndFragmentIgnored`: `allow`, `fragment=on`, `upstream=x` → đi upstream bình thường.
  - `TestResolve`: trả IP từ upstream giả; tên bị chặn trả `0.0.0.0`; tên không tồn tại → `ErrNoAddress`.
- [ ] **Step 2: Chạy** `go test ./internal/engine/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** trong `handle` sau nhánh verify: `d := cfg.Rules().Match(strings.TrimSuffix(name,"."), netip.Addr{})`; câu trả lời rules đặt `d.Res` và ghi `QueryEvent` với `Action`, không gọi `p.Resolve`.
- [ ] **Step 4: Chạy** `go test ./internal/engine/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(engine): in-process Resolve and DNS rules`

---

### Task 8: `proxy/wire` — đọc request SOCKS4/4a/5 và HTTP

**Files:**
- Create: `internal/proxy/wire/request.go`, `socks4.go`, `socks5.go`, `http.go`, `request_test.go` (package `wire`; không import `proxy` hay `dialer`)

**Interfaces:**
- Produces (package `wire`):
  ```go
  type Proto uint8 // ProtoSOCKS4, ProtoSOCKS5, ProtoHTTPConnect, ProtoHTTPForward
  type Target struct { Host string; IP netip.Addr; Port uint16 } // đúng một trong Host/IP
  type Request struct {
      Proto  Proto
      Target Target
      // Chỉ HTTP forward: request đầu tiên đã viết lại sang dạng đường dẫn, bỏ header hop-by-hop.
      HTTPHead []byte
  }
  type Reply uint8 // ReplyOK, ReplyBlocked, ReplyUnreachable, ReplyBadCommand, ReplyBadAddress, ReplyFailure
  // ReadRequest đọc bắt tay từ br (đã bufio), tối đa 8 KB header; trả lỗi ErrUnsupported kèm Reply phù hợp.
  func ReadRequest(br *bufio.Reader, w io.Writer) (Request, error)
  func WriteReply(w io.Writer, p Proto, r Reply) error // SOCKS4 0x5A/0x5B; SOCKS5 0x00/0x02/0x04/0x07/0x08/0x01; HTTP 200 "Connection established"/403/502/405/400/500
  func RewriteForward(req *http.Request) ([]byte, Target, error) // dùng cho request forward tiếp theo trên cùng kết nối
  var ErrUnsupported = errors.New("proxy: unsupported request")
  const MaxHeader = 8 << 10
  ```
  SOCKS5: chỉ phương thức `0x00`; không có thì trả `0xFF`. Lệnh khác `CONNECT` → `0x07`.

- [ ] **Step 1: Viết test thất bại** `request_test.go` (bảng, mỗi ca gồm byte vào, `Request` mong đợi hoặc byte trả lời mong đợi):
  - SOCKS4 IP; SOCKS4a tên miền (`0.0.0.x` + userid + host); SOCKS5 IPv4 / IPv6 / tên miền; SOCKS5 không có phương thức `0x00` → ghi `05 FF`; SOCKS5 `BIND` → `05 07 …`; ATYP lạ → `05 08 …`.
  - HTTP `CONNECT example.com:443 HTTP/1.1`; `GET http://example.com/a?b HTTP/1.1` + `Proxy-Connection: keep-alive` → `HTTPHead` bắt đầu `GET /a?b HTTP/1.1`, không còn `Proxy-Connection`; `GET /relative` → 400; `TRACE` → 405.
  - Header > 8 KB → lỗi; SOCKS5 domain dài 255 hợp lệ.
  - `FuzzReadRequest`: không panic, luôn trả trong giới hạn byte đã đọc ≤ `MaxHeader + 512`.
- [ ] **Step 2: Chạy** `go test ./internal/proxy/wire/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt.** `ReadRequest` peek byte đầu: `0x04` → socks4, `0x05` → socks5, còn lại → `http.ReadRequest` trên `io.LimitReader`.
- [ ] **Step 4: Chạy** `go test ./internal/proxy/wire/ && go test -fuzz=FuzzReadRequest -fuzztime=60s ./internal/proxy/wire/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(proxy): parse SOCKS4/4a/5 and HTTP proxy requests`

---

### Task 9: `proxy/dialer` — rules, phân giải, SSRF, upstream, fragment tự động

**Files:**
- Create: `internal/proxy/dialer/{dialer,hello,autofrag,upstream,ssrf}.go`, `internal/proxy/dialer/dialer_test.go`, `internal/proxy/dialer/dpisim_test.go`

**Interfaces:**
- Consumes: `wire.Target` (Task 8), `rules.Decision` (Task 3), `tlsfrag` (Task 1).
- Produces:
  ```go
  type Resolver interface { Resolve(ctx context.Context, host string) ([]netip.Addr, error) }
  type Matcher interface { Match(host string, ip netip.Addr) rules.Decision }
  type FragCache interface { Has(host string) bool; Add(host string) }
  type Upstream struct { ID, Type, Addr, User, Pass string } // Pass đã giải mã
  type FragConfig struct { Mode string; Method tlsfrag.Method; Chunks int; Delay, AutoTimeout time.Duration }
  type Config struct {
      Resolver  Resolver
      Matcher   func() Matcher           // đọc mỗi kết nối (đổi nóng)
      Cache     FragCache
      Frag      func() FragConfig
      Upstreams func(id string) (Upstream, bool)
      SelfAddrs func() []netip.Addr      // IP của máy này
      Forbidden []netip.AddrPort         // cổng proxy + cổng 53 engine
      Dial      func(ctx context.Context, ap netip.AddrPort) (net.Conn, error) // mặc định net.Dialer TCP, timeout 10 s
  }
  type Outcome string // "direct","fragmented","upstream","blocked","blockedEvenFragmented","failed"
  type Result struct { Conn net.Conn; Outcome Outcome; Source rules.Source; FirstServerBytes []byte }
  var ErrBlocked, ErrForbidden, ErrUnreachable error
  type Dialer struct { /* Config */ }
  func New(c Config) *Dialer
  // Open: client là nguồn (để biết loopback hay LAN), hello là bản ghi đầu tiên của client
  // (nil nếu client chưa gửi gì, ví dụ HTTP forward thì là HTTPHead). Kết quả Conn đã được ghi hello.
  func (d *Dialer) Open(ctx context.Context, client netip.Addr, t wire.Target, hello []byte) (Result, error)
  // ReadHello đọc đúng một bản ghi TLS (theo độ dài header) hoặc những byte đầu không phải TLS, chờ tối đa 5 s, tối đa 16 KB.
  func ReadHello(r io.Reader, timeout time.Duration) ([]byte, error)
  ```
  Luồng `Open` theo spec 5.3–5.6. Rule `upstream=` không đặt `fragment` → coi như `off`. Host dùng cho `FragCache` = SNI nếu có, ngược lại `t.Host`.

- [ ] **Step 1: Viết test thất bại.** `dpisim_test.go` dựng **DPI giả**: listener TCP đứng trước một server `crypto/tls`; nếu một `Read` đầu tiên chứa nguyên chuỗi SNI bị chặn thì đóng bằng RST (`SetLinger(0)`) — biến thể "im lặng" thì không trả gì. `dialer_test.go`:
  - `TestOpen_AutoRetriesWithFragment`: DPI reset → `Outcome=="fragmented"`, `Cache.Add("blocked.test")` được gọi, bắt tay TLS phía client hoàn tất.
  - `TestOpen_AutoUsesCache`: `Cache.Has` true → chỉ **một** lần dial, đã cắt ngay.
  - `TestOpen_SilentDPITimesOut`: biến thể im lặng, `AutoTimeout` 200 ms → thử lại có fragment thành công.
  - `TestOpen_FragmentAlsoFails`: DPI chặn cả bản đã cắt → `Outcome=="blockedEvenFragmented"`, lỗi.
  - `TestOpen_ModesNeverAlways` và rule `fragment=off` thắng `always`.
  - `TestOpen_RuleBlockAndRewrite`: `block` → `ErrBlocked` không dial; `ip=` → dial đúng IP, `Resolver` không bị gọi.
  - `TestOpen_UpstreamSOCKS5AndHTTP`: upstream giả (SOCKS5 có user/pass; HTTP CONNECT) nhận **tên miền** chứ không phải IP; `Resolver` không bị gọi cho host đích.
  - `TestOpen_SSRF`: client `192.168.1.9` tới `127.0.0.1:80`, `169.254.1.1`, IP trong `SelfAddrs` → `ErrForbidden`; client loopback tới `127.0.0.1:80` → được; mọi client tới `Forbidden` → `ErrForbidden`.
  - `TestOpen_IPv6AfterV4Fails`: Resolver trả v4 không kết nối được + v6 được → dùng v6.
  - `TestOpen_NeverSystemDNS`: `Resolver` giả kèm bẫy; đồng thời gán `net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(...) { t.Fatal("system DNS used") }}` trong test (khôi phục bằng `t.Cleanup`).
  - `TestReadHello_AssemblesRecordAcrossReads` (Trọng tâm review 1): ClientHello 1.900 byte gửi qua 4 lần ghi cách nhau 20 ms → `ReadHello` trả đủ 1.900 byte; dữ liệu không phải TLS trả ngay những byte đã có.
- [ ] **Step 2: Chạy** `go test ./internal/proxy/dialer/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** theo Interfaces. Fragment ghi từng segment của `tlsfrag.Split` với `SetNoDelay(true)` và nghỉ `Delay` giữa các segment. Thử nguyên bản: ghi hello, `SetReadDeadline(AutoTimeout)`, đọc ≥ 1 byte vào `FirstServerBytes`.
- [ ] **Step 4: Chạy** `go test -race ./internal/proxy/dialer/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(proxy): dialer with rules, SSRF guard, upstreams and auto fragment`

---

### Task 10: `proxy` — server, giới hạn, chuyển dữ liệu, thống kê

**Files:**
- Create: `internal/proxy/{server,relay,limits,stats}.go`, `internal/proxy/server_test.go`
- Modify: `.golangci.yml` (thêm `forbidigo` cho `internal/proxy/`, xem Ràng buộc chung)

**Interfaces:**
- Consumes: Task 8 (`wire.ReadRequest`, `wire.WriteReply`, `wire.RewriteForward`), Task 9 (`dialer.Dialer`, `dialer.ReadHello`).
- Produces:
  ```go
  type Config struct {
      Listen   []netip.AddrPort  // loopback, hoặc 0.0.0.0/[::] khi chia sẻ LAN
      ShareLAN bool
      Dialer   *dialer.Dialer
      OnConn   func(ConnEvent)   // nil = không ghi (chỉ gọi khi bật hiện truy vấn)
  }
  type ConnEvent struct { Time time.Time; Client, Target, Outcome string; Source rules.Source }
  type Stats struct { Open, LANClients int; BytesIn, BytesOut uint64; ByOutcome map[string]uint64 }
  type Server struct { /* unexported */ }
  func New(c Config) *Server
  func (s *Server) Start(ctx context.Context) error   // lỗi bind trả nguyên *net.OpError để app phân loại PROXY_PORT_BUSY
  func (s *Server) Stop(ctx context.Context) error    // đóng listener, chờ ≤ 2 s, rồi đóng hết
  func (s *Server) SelfTest(ctx context.Context) error // listener echo tạm trên 127.0.0.1:0, CONNECT qua proxy, kiểm tra echo, ≤ 2 s
  func (s *Server) Alive() bool
  func (s *Server) Stats() Stats
  func AllowedSource(a netip.Addr, shareLAN bool) bool
  ```

- [ ] **Step 1: Viết test thất bại** `server_test.go`:
  - `TestEndToEnd_SOCKS5AndHTTP`: `golang.org/x/net/proxy.SOCKS5` và `http.Transport{Proxy: http.ProxyURL(...)}` tải `https://` từ `httptest.NewTLSServer` qua proxy (Dialer dùng Resolver giả map `test.local` → IP của server).
  - `TestAllowedSource` (bảng): loopback luôn được; `192.168.1.5`, `10.x`, `172.16.x`, `169.254.x`, `fd00::1`, `fe80::1` chỉ được khi `shareLAN`; `8.8.8.8` không bao giờ.
  - `TestLimits_PerIPAndTotal`: kết nối thứ 65 từ cùng IP bị đóng; tổng 1.025 bị đóng (dùng hằng số có thể ghi đè trong test).
  - `TestHandshakeTimeout`: client mở rồi im lặng → đóng sau 10 s (hằng số ghi đè được, test dùng 100 ms).
  - `TestHTTPForward_KeepAliveSwitchesHost` (Trọng tâm review 2): một kết nối gửi `GET http://a.test/` rồi `GET http://b.test/` → hai server khác nhau nhận đúng request; `Matcher` được gọi cho cả hai host.
  - `TestRelay_HalfCloseAndIdle`: client `CloseWrite` → server nhận EOF nhưng vẫn gửi được dữ liệu về; idle (ghi đè 100 ms) → đóng.
  - `TestStop_DrainsThenCloses` và `TestSelfTest`.
  - `TestPanicInConnIsContained`: Dialer giả panic → chỉ kết nối đó đóng, server vẫn nhận kết nối mới.
- [ ] **Step 2: Chạy** `go test ./internal/proxy/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt.** Mỗi kết nối: kiểm nguồn → giới hạn → `ReadRequest` (deadline bắt tay) → với CONNECT/SOCKS: `WriteReply(OK)` trước rồi `ReadHello` → `Dialer.Open` (khi `Open` lỗi sau khi đã trả OK thì đóng kết nối) → ghi `FirstServerBytes` cho client → relay. Lưu ý: SOCKS/CONNECT phải trả OK **trước** khi đọc hello vì client chỉ gửi ClientHello sau khi nhận OK; lỗi rules `block` được biết trước khi trả lời nên kiểm `Matcher` trước khi `WriteReply` để trả mã chặn đúng chuẩn.
- [ ] **Step 4: Chạy** `go test -race ./internal/proxy/... && golangci-lint run ./internal/proxy/...` — Expected: PASS, không lỗi lint.
- [ ] **Step 5: Commit** `feat(proxy): local proxy server with limits, relay and stats`

---

### Task 11: `sysproxy` — chụp, đặt, khôi phục, theo dõi System Proxy

**Files:**
- Create: `internal/sysproxy/sysproxy.go`, `internal/sysproxy/api_windows.go`, `internal/sysproxy/watch_windows.go`, `internal/sysproxy/sysproxy_test.go`, `internal/sysproxy/integration_windows_test.go` (`//go:build windows && integration`)

**Interfaces:**
- Consumes: `store.SysProxySnapshot`.
- Produces:
  ```go
  type API interface {
      Query() (store.SysProxySnapshot, error)
      Set(store.SysProxySnapshot) error // gồm INTERNET_OPTION_SETTINGS_CHANGED + REFRESH
  }
  const Bypass = "<local>;localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;[::1]"
  const FlagDirect, FlagProxy, FlagAutoProxyURL uint32 = 1, 2, 4
  type Manager struct { API API }
  func (m Manager) Snapshot() (store.SysProxySnapshot, error)
  func (m Manager) Existing(s store.SysProxySnapshot) (server, pac string, has bool) // proxy/PAC của app khác đang bật
  func (m Manager) Apply(addr string) error            // đặt rồi đọc lại; sai → ErrNotApplied
  func (m Manager) IsOurs(addr string) (bool, error)   // Server==addr và cờ Proxy bật
  func (m Manager) RestoreIfOurs(addr string, snap store.SysProxySnapshot) (restored bool, err error)
  func NewWindowsAPI() API
  func Watch(onChange func()) (stop func(), err error)  // RegNotifyChangeKeyValue trên Internet Settings (+Connections)
  var ErrNotApplied = errors.New("sysproxy: setting did not stick")
  ```

- [ ] **Step 1: Viết test thất bại** `sysproxy_test.go` với `API` giả:
  - `TestApply_ReadsBack`: API giả "nuốt" lệnh Set → `ErrNotApplied`.
  - `TestExisting`: `Flags` có `FlagProxy` và Server khác rỗng → `has`; có `FlagAutoProxyURL` + URL → `pac`; chỉ `FlagDirect` → không.
  - `TestRestoreIfOurs`: giá trị hiện tại là của Ghostline → khôi phục đúng bản chụp; hiện tại là `10.0.0.1:3128` (app khác) → `restored=false`, không gọi `Set`.
  - `TestIsOurs`.
  - `integration`: vòng `Snapshot → Apply("127.0.0.1:18080") → IsOurs → RestoreIfOurs` trên máy thật, kết thúc giá trị trùng bản chụp.
- [ ] **Step 2: Chạy** `go test ./internal/sysproxy/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** `api_windows.go` bằng `wininet.dll` `InternetQueryOptionW`/`InternetSetOptionW` với `INTERNET_OPTION_PER_CONNECTION_OPTION` (75), `INTERNET_PER_CONN_OPTION_LISTW`, các option `FLAGS`(1), `PROXY_SERVER`(2), `PROXY_BYPASS`(3), `AUTOCONFIG_URL`(4); sau Set gọi option 39 (`SETTINGS_CHANGED`) và 37 (`REFRESH`). `Watch` chạy goroutine với `RegNotifyChangeKeyValue(..., REG_NOTIFY_CHANGE_LAST_SET, event, true)`, gom sự kiện 500 ms.
- [ ] **Step 4: Chạy** `go test ./internal/sysproxy/` — Expected: PASS. Ghi trong báo cáo: test `integration` chờ người dùng chạy bằng terminal admin.
- [ ] **Step 5: Commit** `feat(sysproxy): snapshot, apply and restore the Windows system proxy`

---

### Task 12: `winutil` — firewall, profile mạng, IP LAN, DPAPI

**Files:**
- Create: `internal/winutil/firewall_windows.go`, `internal/winutil/firewall.go` (dựng tham số netsh, thuần), `internal/winutil/netprofile_windows.go`, `internal/winutil/lanip.go`, `internal/winutil/dpapi_windows.go`, test `internal/winutil/firewall_test.go`, `internal/winutil/lanip_test.go`, `internal/winutil/dpapi_windows_test.go`

**Interfaces:**
- Produces:
  ```go
  const FirewallRuleName = "Ghostline Proxy"
  func FirewallAddArgs(port int, exe string) []string  // ["advfirewall","firewall","add","rule","name=Ghostline Proxy","dir=in","action=allow","protocol=TCP","localport=<port>","program=<exe>","profile=private","remoteip=localsubnet"]
  func FirewallDeleteArgs() []string
  func AddFirewallRule(port int, exe string) error     // chạy netsh qua HiddenCmd
  func DeleteFirewallRule() error                      // "No rules match" → nil
  func CurrentNetworkIsPublic() (bool, error)          // INetworkListManager qua go-ole (đã có trong go.mod indirect qua Wails) — nếu không dùng được thì PowerShell `Get-NetConnectionProfile` qua HiddenCmd
  func LANAddrs(ifaces []net.Interface, addrs func(net.Interface) ([]net.Addr, error)) []netip.Addr // chỉ IP riêng, interface Up, không loopback
  func ProtectString(s string) (string, error)         // DPAPI CryptProtectData, base64
  func UnprotectString(b64 string) (string, error)
  ```

- [ ] **Step 1: Viết test thất bại:** `TestFirewallArgs` (đường dẫn exe có dấu cách và chữ có dấu là **một** phần tử, khớp danh sách ở trên); `TestDeleteFirewallRule_NoMatchIsNil` (bộ chạy lệnh giả trả "No rules match the specified criteria." với exit 1); `TestLANAddrs` (lọc bỏ public, loopback, interface Down); `TestDPAPI_RoundTrip` (Windows, không cần admin); `UnprotectString("rác")` → lỗi.
- [ ] **Step 2: Chạy** `go test ./internal/winutil/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt.** Bộ chạy lệnh tiêm được (`var runNetsh = func(args []string) ([]byte, error)`) để test.
- [ ] **Step 4: Chạy** `go test ./internal/winutil/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(winutil): firewall rule, network profile, LAN addresses and DPAPI`

---

### Task 13: `qr` — sinh mã QR

**Files:**
- Create: `internal/qr/qr.go`, `internal/qr/qr_test.go`
- Modify: `go.mod` (thêm `github.com/makiuchi-d/gozxing` — chỉ dùng trong test)

**Interfaces:**
- Produces: `func Encode(text string) ([][]bool, error)` — byte mode, mức sửa lỗi M, tự chọn version nhỏ nhất (1–10 là đủ cho `ip:port`; > version 10 trả lỗi), mask tốt nhất theo điểm phạt chuẩn. Ma trận chưa gồm quiet zone.

- [ ] **Step 1: Viết test thất bại:** `TestEncode_DecodesBack`: với `"192.168.1.5:8080"`, `"[fd00::1234]:8080"`, `"socks5://192.168.100.200:65535"` → vẽ ma trận ra `image.Gray` (mỗi module 4 px, quiet zone 4 module) → giải mã bằng `gozxing/qrcode` ra đúng chuỗi. `TestEncode_TooLong` → lỗi.
- [ ] **Step 2: Chạy** `go test ./internal/qr/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** (Reed–Solomon GF(256), bảng block cho version 1–10 mức M, đặt finder/timing/alignment/format/version, 8 mask).
- [ ] **Step 4: Chạy** `go test ./internal/qr/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(qr): minimal QR encoder for the LAN proxy address`

---

### Task 14: `watchdog` — khôi phục system proxy và firewall

**Files:**
- Modify: `internal/watchdog/recover.go`
- Test: `internal/watchdog/proxy_test.go`

**Interfaces:**
- Consumes: `store.State.SysProxy/Firewall` (Task 6), `sysproxy.Manager.RestoreIfOurs` (Task 11), `winutil.DeleteFirewallRule` (Task 12).
- Produces: `watchdog.Deps` thêm
  ```go
  RestoreSysProxy func(ours string, snap store.SysProxySnapshot) (bool, error) // nil = bỏ qua
  DeleteFirewall  func() error                                              // nil = bỏ qua
  ```
  Thứ tự trong `RestoreIfOrphaned` (nhánh owner đã chết): system proxy (chỉ khi `Set && !TakenOver && Snapshot != nil`) → firewall (khi `Firewall != nil`) → DNS → DPI → `CleanState()`. Nhánh state hỏng: luôn gọi `DeleteFirewall` (idempotent); **không** đụng system proxy (không có bản chụp). Lỗi khôi phục system proxy → giữ state (không ghi clean) để lớp sau thử lại, giống DNS.

- [ ] **Step 1: Viết test thất bại:** `TestRestore_ProxyBeforeDNS` (ghi lại thứ tự gọi: sysproxy, firewall, dns); `TestRestore_TakenOverSkipsSysProxy`; `TestRestore_SysProxyFailureKeepsState`; `TestRestore_CorruptDeletesFirewallOnly`; `TestRestore_OwnerAliveTouchesNothing`.
- [ ] **Step 2: Chạy** `go test ./internal/watchdog/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt**; sửa `shell.go` (`recoverDeps`) và `headless.go` (chế độ `--watchdog`/`--restore`) để truyền hai hàm thật.
- [ ] **Step 4: Chạy** `go test ./internal/watchdog/ ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(watchdog): restore system proxy and firewall before DNS`

---

### Task 15: `app` — pha proxy P1–P4, SUY_GIẢM nhiều lý do, Disconnect mới

**Files:**
- Create: `internal/app/proxyphase.go`, `internal/app/proxyphase_test.go`
- Modify: `internal/app/deps.go`, `internal/app/status.go`, `internal/app/errors.go`, `internal/app/orchestrator.go` (`disconnectLocked`, cuối `Connect`), `internal/app/health.go` (lý do `upstreams`), `internal/app/fakes_test.go`

**Interfaces:**
- Consumes: `proxy.Server` qua interface, `sysproxy.Manager` qua interface, `store` v2.
- Produces:
  ```go
  // deps.go
  type Proxy interface {
      Start(ctx context.Context, cfg ProxyRun) error
      Stop(ctx context.Context) error
      SelfTest(ctx context.Context) error
      Alive() bool
  }
  type ProxyRun struct { Listen []netip.AddrPort; ShareLAN bool }
  type SysProxy interface {
      Snapshot() (store.SysProxySnapshot, error)
      Existing(store.SysProxySnapshot) (server, pac string, has bool)
      Apply(addr string) error
      RestoreIfOurs(addr string, snap store.SysProxySnapshot) (bool, error)
  }
  type Firewall interface { Add(port int) error; Delete() error }
  // Deps thêm: Proxy Proxy; SysProxy SysProxy; Firewall Firewall
  //   ConfirmOverride func(server, pac string) bool // hỏi người dùng (SYSPROXY_EXISTING); nil = không ghi đè
  // status.go: Snapshot thêm
  //   Reasons []string `json:"reasons"` // "upstreams","proxy"
  //   Proxy   ProxyStatus `json:"proxy"`
  type ProxyStatus struct { Running bool `json:"running"`; Addr string `json:"addr"`; SystemProxy bool `json:"systemProxy"`; ShareLAN bool `json:"shareLan"`; Error *AppError `json:"error,omitempty"` }
  // errors.go: CodeProxyPortBusy="PROXY_PORT_BUSY", CodeProxySelfTest="PROXY_SELFTEST_FAILED", CodeProxyFirewall="PROXY_FIREWALL",
  //   CodeSysProxyExisting="SYSPROXY_EXISTING", CodeSysProxyFailed="SYSPROXY_FAILED", CodeSysProxyTakenOver="SYSPROXY_TAKEN_OVER",
  //   CodeSysProxyRestore="SYSPROXY_RESTORE_FAILED", CodeRulesParse="RULES_PARSE", CodeListFetch="LIST_FETCH_FAILED",
  //   CodeListUnsupported="LIST_UNSUPPORTED_FORMAT", CodeListTooLarge="LIST_TOO_LARGE", CodeUpstreamProxy="UPSTREAM_PROXY_FAILED"
  func (o *Orchestrator) addReason(r string)    // Status → Degraded
  func (o *Orchestrator) clearReason(r string)  // tập rỗng → Protected
  func (o *Orchestrator) startProxyPhase(ctx context.Context) error // gọi cuối Connect (sau CONNECTED) và từ ReapplyProxy; caller giữ opMu
  func (o *Orchestrator) stopProxyPhase(ctx context.Context) error  // bước 1–3 của Disconnect
  func (o *Orchestrator) ReapplyProxy(ctx context.Context) error    // giữ opMu: stop rồi start; không làm gì khi chưa kết nối
  func (o *Orchestrator) OnSysProxyChanged()                        // từ sysproxy.Watch: nếu không còn là của mình → TakenOver
  ```
  Port bận: `Proxy.Start` lỗi bind → tra `System.PortOwners(port)` → `PROXY_PORT_BUSY{port,pid,name}`.

- [ ] **Step 1: Viết test thất bại** `proxyphase_test.go` (fake cho Proxy/SysProxy/Firewall ghi lại thứ tự gọi vào một `[]string` chung với fake DNS/engine hiện có):
  - `TestProxyPhase_FailureAtEachStep` (bảng P1..P4): lỗi ở bước N → undo các bước < N theo thứ tự ngược; `Status==degraded`, `Reasons==["proxy"]`, `Proxy.Error.Code` đúng; DNS **không** bị restore; state không còn `sysproxy`/`firewall` sau undo.
  - `TestProxyPhase_StateWrittenBeforeChange`: bản chụp `sysproxy` có trong state **trước** lần gọi `Apply`; `firewall.rule` có trong state trước `Add`.
  - `TestProxyPhase_ExistingDeclined`: `Existing` trả has, `ConfirmOverride` false → không `Apply`, pha P thành công, `Proxy.SystemProxy==false`.
  - `TestDisconnect_Order`: thứ tự `sysproxy.restore, firewall.delete, proxy.stop, dns.restore, dpi.stop, engine.stop, state.clean`.
  - `TestDisconnect_TakenOverNotRestored`.
  - `TestReasons_UpstreamsAndProxy`: thêm `upstreams` rồi `proxy` → degraded; xoá `upstreams` → vẫn degraded; xoá `proxy` → protected. Sửa `heal`/`healthLoop` dùng `addReason("upstreams")`/`clearReason("upstreams")` và test giai đoạn 1 của health vẫn đạt.
  - `TestProxyPhase_ReapplyRacesDisconnect` (Trọng tâm review 5): gọi `ReapplyProxy` và `Disconnect` đồng thời 50 lần → không lần nào kết thúc với `Apply` được gọi sau `proxy.stop` cuối cùng; trạng thái cuối `disconnected`, `-race` sạch.
  - `TestHealth_ProxyListenerDiedRestartsOnce`: `Alive()` false → `ReapplyProxy` một lần; vẫn false → reason `proxy`.
  - `TestConnect_ProxyDisabledUnchanged`: `proxy.enabled=false` → không fake proxy nào bị gọi (hành vi giai đoạn 1 giữ nguyên).
- [ ] **Step 2: Chạy** `go test ./internal/app/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt.** Pha P dùng `runSteps` với 4 `step` theo spec 6.1; các `step` P2/P3 ghi `States.Update` trước khi thay đổi hệ thống; lỗi pha P không trả lỗi ra `Connect`.
- [ ] **Step 4: Chạy** `go test -race ./internal/app/` — Expected: PASS (kể cả toàn bộ test giai đoạn 1).
- [ ] **Step 5: Commit** `feat(app): proxy phase with safe system proxy, firewall and degraded reasons`

---

### Task 16: `app` — binding cho trang Proxy và Rules

**Files:**
- Create: `internal/app/proxyservice.go`, `internal/app/rulesservice.go`, `internal/app/rulesservice_test.go`, `internal/app/proxyservice_test.go`
- Modify: `internal/app/service.go` (`SaveSettings` gọi `store.ValidateProxy`, phát hiện thay đổi proxy để gọi `ReapplyProxy`), `internal/app/events.go` (hằng số sự kiện mới)

**Interfaces:**
- Consumes: Task 3, 5, 6, 12, 13, 15.
- Produces (method của `*Service`, Wails tự bind):
  ```go
  // proxyservice.go
  type LANInfo struct { Addrs []string `json:"addrs"`; Public bool `json:"public"` }
  func (s *Service) GetLANInfo() LANInfo
  func (s *Service) GetQR(text string) ([][]bool, error)
  func (s *Service) SetProxyEnabled(on bool) error            // dùng cho menu khay
  func (s *Service) SaveUpstreamProxy(u store.UpstreamProxy, password string) error // password "" = giữ mật khẩu cũ; mã hoá DPAPI
  func (s *Service) DeleteUpstreamProxy(id string) error      // từ chối nếu rule/danh sách còn tham chiếu
  func (s *Service) TestUpstreamProxy(id string) error        // CONNECT www.google.com:443 qua upstream, TLS xong trong 5 s
  func (s *Service) GetFragCache() []string
  func (s *Service) ClearFragCache(host string) error         // "" = tất cả của mạng hiện tại
  func (s *Service) GetProxyConns() []proxy.ConnEvent         // RAM, chỉ khi bật hiện truy vấn
  func (s *Service) RetryProxy() error
  // rulesservice.go
  type RulesView struct { Rules []rules.Rule `json:"rules"`; Text string `json:"text"`; Lists []lists.List `json:"lists"` }
  func (s *Service) GetRules() RulesView
  func (s *Service) SaveRulesTable(rs []rules.Rule) []rules.LineError
  func (s *Service) SaveRulesText(text string) []rules.LineError // lỗi → không lưu
  func (s *Service) AddList(l lists.List) (lists.List, error)    // gán ID, tải ngay ở nền
  func (s *Service) UpdateList(l lists.List) error
  func (s *Service) DeleteList(id string) error                 // xoá cả lists/<id>.txt
  func (s *Service) MoveList(id string, to int) error
  func (s *Service) RefreshList(id string) error                // "" = tất cả
  func (s *Service) Catalog() []lists.CatalogItem
  func (s *Service) Explain(host string) rules.Decision
  ```
  Mỗi lần lưu rules hoặc danh sách thay đổi: biên dịch lại ở nền → `Holder.Store` → phát `rules:compiled` `{count, ms}`. Tổng mục > 2.000.000 → tắt danh sách được thêm/cập nhật gần nhất, ghi `LastError="LIST_TOO_LARGE"`, phát log `LIST_TOO_LARGE{id}`.
  `ServiceDeps` thêm: `Rules *rules.Holder`, `RulesPath string`, `Fetcher *lists.Fetcher`, `FragCache *store.FragCache`, `NetKey func() string`, `Proxy ProxyQuery` (`Stats() proxy.Stats; Conns() []proxy.ConnEvent`), `LANInfo func() LANInfo`.

- [ ] **Step 1: Viết test thất bại:**
  - `TestSaveRulesText_ErrorsDoNotSave`: text có 1 dòng lỗi → trả `LineError`, `rules.json` không đổi, `Holder` không đổi.
  - `TestSaveRulesTable_RecompilesAndEmits`: sau khi lưu, `Explain("ads.com").Block==true`, có sự kiện `rules:compiled`.
  - `TestAddList_FetchesInBackground`: Fetcher với server test → danh sách có `Detected`, `Counts`; sự kiện `lists:progress`.
  - `TestTotalLimitDisablesNewest`: hằng số giới hạn ghi đè = 10, hai danh sách 6 mục → danh sách thứ hai bị tắt với `LIST_TOO_LARGE`.
  - `TestDeleteUpstream_ReferencedRefused`.
  - `TestSaveUpstream_PasswordEncryptedAndKept`: `PassEnc` khác mật khẩu gốc; lưu lại với `password==""` giữ `PassEnc` cũ (DPAPI tiêm được bằng fake).
  - `TestSaveSettings_ProxyChangeReapplies`: đổi `Port` khi đang kết nối → `ReapplyProxy` được gọi; đổi `Fragment.Chunks` → **không** gọi.
- [ ] **Step 2: Chạy** `go test ./internal/app/` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** theo Interfaces.
- [ ] **Step 4: Chạy** `go test -race ./internal/app/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(app): proxy and rules service bindings`

---

### Task 17: `shell` — nối dây, khay, lịch tải danh sách, NSIS

**Files:**
- Create: `internal/shell/proxywire.go`
- Modify: `internal/shell/shell.go`, `internal/shell/ui.go` (menu khay "Proxy: bật/tắt"), `internal/shell/traytext.go` (+ test), `headless.go`, `build/windows/nsis/project.nsi`

**Interfaces:**
- Consumes: mọi task trước.
- Produces: không có API mới; nối dây theo các bước dưới.

- [ ] **Step 1: Viết test thất bại** `traytext_test.go`: `TestTrayText_ProxyItem` — nhãn "Proxy: bật"/"Proxy: tắt" (vi) và "Proxy: on"/"Proxy: off" (en) theo `settings.Proxy.Enabled`.
- [ ] **Step 2: Chạy** `go test ./internal/shell/` — Expected: FAIL.
- [ ] **Step 3: Nối dây trong `proxywire.go`** (gọi từ `Run`):
  1. `store.LoadRules` + `Fetcher.LoadCached` mọi danh sách đang bật → `lists.ToListSet` → `rules.Compile` → `Holder.Store`; `rules.json` hỏng (`recovered=true`) → `orch.AddWarning(AppError{Code: CodeRulesParse, Params: {"line": 0}})`. `Fetcher.WriteFile = store.WriteFileAtomic`.
  2. `engine.Config.Rules = holder.Load`, `BlockMode` từ settings (sửa bước `engine` trong `connectSteps` để truyền hai trường này qua `Deps`: thêm `Deps.Rules func() *rules.Compiled` và đọc `Settings().DNSBlockMode`).
  3. `dialer.New` với `Resolver: eng`, `Matcher: holder.Load`, `Cache` = adapter của `store.FragCache` theo `networkKey()` và hạn `CacheDays`, `Upstreams` đọc từ settings và giải mã `PassEnc` bằng `winutil.UnprotectString`, `SelfAddrs` từ `winutil.LANAddrs` + loopback, `Forbidden` = cổng proxy + `ListenV4/ListenV6`.
  4. Adapter `app.Proxy` bọc `proxy.Server` (tạo `proxy.New` mới mỗi lần `Start`), `app.SysProxy` = `sysproxy.Manager{API: sysproxy.NewWindowsAPI()}`, `app.Firewall` gọi `winutil.AddFirewallRule(port, exe)` / `DeleteFirewallRule`.
  5. `ConfirmOverride`: phát cảnh báo `SYSPROXY_EXISTING{server,pac}` lên UI và chờ trả lời qua method `Service.AnswerSysProxyOverride(bool)` (timeout 60 s → coi là không). Thêm method này vào `Service` cùng test trong `proxyservice_test.go`.
  6. `sysproxy.Watch(orch.OnSysProxyChanged)`.
  7. `lists.Scheduler` chạy goroutine với `Run` = `Service.RefreshList`.
  8. `recoverDeps` (và `headless.go`) truyền `RestoreSysProxy`/`DeleteFirewall` thật (Task 14).
  9. Thống kê proxy: `RunStats` phát thêm `proxy:stats` mỗi giây khi proxy chạy. `proxy.Config.OnConn` đẩy vào bộ đệm RAM 500 dòng và phát `proxy:conn` **chỉ khi** bật hiện truy vấn (dùng cờ `Bus.queryLog` hiện có).
- [ ] **Step 4: NSIS:** trong phần gỡ cài đặt, sau `--restore`, thêm `nsExec::Exec 'netsh advfirewall firewall delete rule name="Ghostline Proxy"'`.
- [ ] **Step 5: Chạy** `go build ./... && go test ./... && golangci-lint run` — Expected: PASS. `wails3 build` (hoặc `task build`) tạo được exe.
- [ ] **Step 6: Commit** `feat(shell): wire proxy, rules, system proxy and list schedule`

---

### Task 18: Frontend — store, i18n, trang Proxy, mã QR

**Files:**
- Modify: `frontend/src/app/api.ts`, `frontend/src/app/store.ts`, `frontend/src/app/bridge.ts`, `frontend/src/i18n/vi.json`, `frontend/src/i18n/en.json`, `frontend/src/modes/advanced/AdvancedView.tsx` (thanh bên), `frontend/src/modes/advanced/pages/Overview.tsx` (thẻ proxy), `frontend/src/modes/advanced/pages/Dpi.tsx` (ghi chú), `frontend/src/modes/simple/SimpleView.tsx` (dòng "Proxy: …"), `frontend/src/components/Warnings.tsx` (mã mới, hộp xác nhận `SYSPROXY_EXISTING`)
- Create: `frontend/src/components/neon/QRCode.tsx`, `frontend/src/modes/advanced/pages/Proxy.tsx`, `frontend/src/modes/advanced/pages/proxy.test.tsx`

**Interfaces:**
- Consumes: binding Task 16/17 (sinh lại bằng `wails3 generate bindings`), sự kiện `proxy:stats`, `proxy:conn`.
- Produces: store Zustand thêm `proxyStats`, `proxyConns`; component `<QRCode matrix={boolean[][]} label={string} />` (SVG, quiet zone 4, `role="img"` + `aria-label`).

- [ ] **Step 1: Viết test thất bại** `proxy.test.tsx` (mock `api.ts` như các test hiện có):
  - Bật "Chia sẻ LAN" → hiện danh sách địa chỉ từ `GetLANInfo` và một `img` QR có `aria-label` chứa `192.168.1.5:8080`.
  - `GetLANInfo().public==true` → hiện cảnh báo Public (chuỗi i18n `proxy.lan.public`).
  - Đổi cổng thành `53` → hiện lỗi kiểm tra, không gọi `SaveSettings`.
  - Bảng upstream: "Kiểm tra" gọi `TestUpstreamProxy(id)` và hiện kết quả.
  - Danh sách `frag-cache` hiện từ `GetFragCache`, nút xoá gọi `ClearFragCache`.
  - `proxyStats.byOutcome.blockedEvenFragmented > 0` → hiện gợi ý bật GoodbyeDPI (chuỗi `proxy.hint.goodbyedpi`).
  - `Warnings`: `SYSPROXY_EXISTING` hiện hộp xác nhận, bấm "Ghi đè" gọi `AnswerSysProxyOverride(true)`.
  - Chế độ Đơn giản: `snapshot.proxy.running` → dòng "Proxy: 127.0.0.1:8080".
- [ ] **Step 2: Chạy** `cd frontend && npm test -- --run` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** trang và component theo spec mục 8; thêm mọi khoá i18n mới vào **cả hai** file (test parity hiện có bảo vệ).
- [ ] **Step 4: Chạy** `cd frontend && npm test -- --run && npm run build` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): proxy page with LAN QR code and upstream proxies`

---

### Task 19: Frontend — trang Rules (Bảng ↔ Text, danh sách, Thử tên miền)

**Files:**
- Create: `frontend/src/modes/advanced/pages/Rules.tsx`, `RulesTable.tsx`, `RulesText.tsx`, `Lists.tsx`, `rules.test.tsx`
- Modify: `frontend/src/i18n/{vi,en}.json`, `frontend/src/modes/advanced/pages/Logs.tsx` (nguồn lọc `proxy`, `rules`)

**Interfaces:**
- Consumes: `GetRules`, `SaveRulesTable`, `SaveRulesText`, `AddList`, `UpdateList`, `DeleteList`, `MoveList`, `RefreshList`, `Catalog`, `Explain`; sự kiện `rules:compiled`, `lists:progress`.

- [ ] **Step 1: Viết test thất bại** `rules.test.tsx`:
  - Sửa ở tab Text, `SaveRulesText` trả `[{Line:2,Msg:"…"}]` → dòng 2 được đánh dấu lỗi, bảng không đổi; trả `[]` → chuyển sang tab Bảng thấy rule mới.
  - Tab Bảng: tắt một rule rồi sang tab Text thấy `#! …`.
  - Kéo đổi thứ tự (dùng nút ▲▼ có thể truy cập bằng bàn phím, kéo chuột là phụ) → `SaveRulesTable` nhận thứ tự mới.
  - Thêm danh sách từ URL `github.com/u/r/blob/main/x.txt` → `AddList` được gọi với URL người dùng nhập (chuẩn hoá ở Go); bảng hiện `Detected`, `Counts`, `Skipped`; bấm số dòng bỏ qua thấy `SkippedSamples`.
  - "Thêm nhanh": danh mục từ `Catalog()` hiện license và repo; bấm "Thêm" gọi `AddList` với hành động gợi ý.
  - Ô Thử tên miền: `Explain("x.ads.com")` trả `{Block:true, Source:{Kind:"list", ListID:"hagezi-light", Line:120}}` → hiện "Chặn — danh sách HaGeZi Light, dòng 120".
  - Rule có `sni=` hiện nhãn "cần giai đoạn 2B".
- [ ] **Step 2: Chạy** `cd frontend && npm test -- --run` — Expected: FAIL.
- [ ] **Step 3: Cài đặt.** `RulesText`: `<textarea>` cạnh cột số dòng, không dùng thư viện editor.
- [ ] **Step 4: Chạy** `cd frontend && npm test -- --run && npm run build` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): rules page with table/text editing, lists and domain tester`

---

### Task 20: Tài liệu và checklist phát hành

**Files:**
- Modify: `README.md`, `README.vi.md`, `docs/user-guide.md`, `docs/huong-dan-su-dung.md`, `docs/release-checklist.md`

- [ ] **Step 1:** README (EN + VI): thêm tính năng Proxy, Chia sẻ LAN, Fragment web, Rules và danh sách cộng đồng; mục "Hạn chế đã biết" thêm system proxy với UAC bằng tài khoản admin khác và ứng dụng không tôn trọng system proxy.
- [ ] **Step 2:** Hướng dẫn sử dụng (EN + VI): cấu hình proxy trên Android/iOS, mạng Public/Private, cú pháp rule (bảng mẫu khớp và hành động từ spec 7.1), thêm danh sách từ GitHub, ô Thử tên miền.
- [ ] **Step 3:** `release-checklist.md`: thêm toàn bộ dòng "Kiểm tra thủ công trước phát hành" của spec mục 11, và chạy các test `integration` mới (`sysproxy`) bằng terminal admin.
- [ ] **Step 4:** Kiểm tra liên kết nội bộ trong các file vừa sửa trỏ đúng file tồn tại.
- [ ] **Step 5: Commit** `docs: proxy, rules and community lists (EN + VI)`
