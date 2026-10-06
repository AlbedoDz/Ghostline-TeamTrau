# Ghostline — Giai đoạn 3: Thiết kế

- **Ngày:** 2026-10-06
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Giai đoạn 3 — DNS Lookup, Advanced DNS Scanner, quét IP sạch Cloudflare, công cụ STAMP, import/export cài đặt.
- **Dựa trên:** [Giai đoạn 1](2026-10-04-ghostline-phase1-design.md), [2A](2026-10-04-ghostline-phase2a-design.md), [engine zapret2](2026-10-05-ghostline-zapret2-design.md), [2B](2026-10-05-ghostline-phase2b-design.md) (đã phát hành ở v0.4.1). Mọi thứ không nhắc lại ở đây giữ nguyên.

---

## 1. Mục tiêu

- **Tự chẩn đoán:** người dùng biết được một tên miền có bị nhà mạng đầu độc DNS hay không, và server nào trả lời đúng, nhanh, có DNSSEC, có lọc quảng cáo.
- **Tìm đường vào Cloudflare:** tìm IP Cloudflare còn kết nối được và nhanh trên mạng hiện tại, rồi dùng IP đó cho các domain cần thiết bằng một rule `ip=`.
- **Công cụ cho người dùng kỹ thuật:** đọc và tạo stamp `sdns://`, xem bản ghi DNS chi tiết (TTL, cờ, dạng `dig`).
- **Mang cấu hình sang máy khác:** xuất và nhập toàn bộ cài đặt người dùng trong một file.

Giao diện chọn hướng **kết hợp**: mặc định hiện kết luận dễ hiểu (ví dụ "Bị đầu độc: nhà mạng trả về IP riêng"), có phần "Chi tiết" mở ra cho người cần số liệu thô.

### Tiêu chí thành công

1. Tra một tên miền bị chặn: trong một màn hình thấy kết quả qua Ghostline và qua các server khác, cùng kết luận "đầu độc", "khác nhau" hoặc "khớp".
2. Advanced Scanner chấm từng server theo 5 tiêu chí (mục 6), không làm chậm hay chặn được Connect, huỷ được giữa chừng trong ≤ 1 giây.
3. Quét IP sạch Cloudflare trả về ít nhất 10 IP dùng được trong ≤ 60 giây trên mạng gia đình bình thường. Từ một kết quả, chỉ cần một bước là tạo được rule `ip=`.
4. Stamp tạo ra rồi giải mã lại cho đúng các trường ban đầu (kiểm chứng bằng test vòng). Stamp nào có trong `servers.json` cũng giải mã được.
5. Xuất trên máy A rồi nhập trên máy B thì có cùng server tự thêm, ghim, rule, danh sách, cài đặt vượt DPI và proxy. **File nhập không bao giờ tự bật Fake SNI, DNS server cho LAN hay chia sẻ proxy trong LAN.**
6. Người dùng v0.4 nâng cấp lên **không thấy hành vi nào thay đổi** ngoài trang mới. Mọi chuỗi mới có đủ tiếng Việt và tiếng Anh.

## 2. Phạm vi

### Có trong giai đoạn 3

| Nhóm | Nội dung |
|---|---|
| DNS Lookup | Tra một tên miền với 10 loại bản ghi, qua Ghostline, qua server trong danh sách, qua địa chỉ tự nhập, hoặc qua DNS nhà mạng (có cảnh báo). Chế độ so sánh nhiều nguồn |
| Advanced Scanner | Chấm server theo độ trễ (nhiều lượt), tỉ lệ mất gói, DNSSEC, lọc quảng cáo, chống đầu độc. Quét danh sách có sẵn đã lọc hoặc danh sách dán vào. Ghim, thêm, xuất CSV |
| IP sạch Cloudflare | Quét IPv4 của Cloudflare bằng TCP và TLS kèm `/cdn-cgi/trace`, đo tốc độ tải cho các IP tốt nhất, lưu kết quả theo mạng, tạo rule `ip=` |
| STAMP | Giải mã mọi loại stamp, tạo stamp DoH/DoT/DoQ/DNSCrypt/plain từ các trường hoặc từ URL, thêm vào danh sách server |
| Import/export | Một file `.ghostline.json` chứa cài đặt, rule, danh sách đăng ký, server tự thêm và danh sách đen vượt DPI. Có xem trước, chọn từng phần, kiểm tra an toàn |

### Không có trong giai đoạn 3

- Quét IPv6 của Cloudflare (dải quá lớn để lấy mẫu có ý nghĩa).
- Tự động dùng IP sạch: không tự đổi rule theo lịch, không tự quét lại.
- Quét IP sạch cho CDN khác (Fastly, Akamai, Gcore).
- Lookup qua DNS nhà mạng ở chế độ Đơn giản; Đơn giản không có trang Công cụ.
- Đồng bộ cài đặt qua mạng; mã hoá file xuất.
- Từ giai đoạn 1 vẫn chưa làm: tự tải và cài bản cập nhật, ký số file exe, Windows ARM64. **Không bao giờ có telemetry.**

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Phạm vi | Cả 5 phần trong một spec và một bản phát hành, **v0.5.0** |
| Vị trí giao diện | Trang mới **"Công cụ"** ở chế độ Nâng cao, 4 tab: Lookup, Scanner, IP Cloudflare, STAMP. Import/export nằm trong trang Cài đặt |
| Không chặn Connect | Các công cụ chạy độc lập với `Orchestrator`, không giữ `opMu`. Connect và Disconnect không đợi và không huỷ công cụ. Mỗi loại công cụ chỉ chạy **một tác vụ một lúc** |
| DNS không mã hoá | Mặc định không bao giờ. **Ngoại lệ duy nhất:** Lookup có nguồn "DNS nhà mạng" (plain UDP tới DNS gốc trong bản chụp, hoặc IP tự nhập) để so sánh khi chẩn đoán. Nguồn này phải bấm chọn, có nhãn "không mã hoá" và không bao giờ được chọn sẵn |
| Gốc so sánh của Lookup | Là **Ghostline** khi đã kết nối (truy vấn `127.0.0.1:53` của engine). Chưa kết nối thì là server có độ trễ tốt nhất trong lần quét gần nhất |
| Dải IP Cloudflare | Danh sách IPv4 **nhúng vào app** (lấy từ `cloudflare.com/ips-v4`), cập nhật ở mỗi bản phát hành bằng `tools/gencfranges`. Không tải qua mạng lúc chạy |
| Host kiểm tra IP Cloudflare | Mặc định `speed.cloudflare.com` (SNI và `Host`), đổi được. Chứng chỉ phải hợp lệ với kho gốc hệ thống và đúng tên host |
| Dùng IP sạch | Sao chép, hoặc "Tạo rule": người dùng nhập domain, app thêm rule `ip=<IP>` vào bảng rule người dùng (cần proxy hoặc DNS của Ghostline mới có tác dụng) |
| File xuất | JSON có phiên bản, **không chứa bí mật**: không khoá CA, không `state.json`, không cache, không IP/MAC gateway |
| An toàn khi nhập | Chỉ nhập được khi **đã ngắt kết nối**. Luôn tắt `fakeSni.enabled`, `dnsServer.enabled`, `dnsServer.shareLan`, `proxy.shareLan`, `startWithWindows`, đặt `trustedForSNI` của mọi danh sách về `false` (trừ preset chính thức có chữ ký hợp lệ). Rule `sni=`/`connect=` trong file phải được người dùng tích xác nhận riêng |
| Thư viện | Không thêm dependency Go. `dnsstamps`, `miekg/dns`, `dnsproxy/upstream` đã có |

## 4. Kiến trúc

### 4.1 Package mới

| Package | Việc |
|---|---|
| `internal/lookup` | Một truy vấn (tên, loại, nguồn) → `Answer` (rcode, bản ghi kèm TTL, cờ AD/TC/RA, độ trễ, dạng `dig`). So sánh nhiều `Answer` → kết luận. Nhận `func(model.Server) (upstream.Upstream, error)` để dựng upstream, không biết `app` |
| `internal/scanner/advanced` | Các phép kiểm tra (mục 6.2) và việc gộp điểm. Dùng lại `scanner.Scan` (hàng đợi, số worker, huỷ) với một `Checker` mới |
| `internal/cfscan` | Dải IP nhúng, lấy mẫu IP, kiểm tra một IP (TCP → TLS → HTTP trace), đo tốc độ tải, cache theo mạng |
| `internal/stamps` | Giải mã stamp sang `StampFields` và tạo stamp từ `StampFields` hoặc URL. Bọc `dnsstamps` |
| `internal/backup` | Định dạng file xuất, `Build` (từ dữ liệu hiện có) và `Plan` (đọc file → xem trước + danh sách cảnh báo), `Apply` (ghi nguyên tử từng phần) |
| `tools/gencfranges` | Tải `https://www.cloudflare.com/ips-v4`, kiểm tra định dạng CIDR, ghi `internal/cfscan/ranges_v4.txt` |

### 4.2 Thay đổi ở package cũ

| Package | Thay đổi |
|---|---|
| `app` | Các hàm mới gắn vào `*Service` hiện có (frontend chỉ có một module binding): Lookup, Scanner, IP Cloudflare, STAMP, `ExportSettings`, `PreviewImport`, `ApplyImport`. Sự kiện mới `tools:scan`, `tools:cfscan` |
| `scanner` | Tách phần chung `queryOnce(ctx, u, name, qtype)` để `advanced` và `lookup` dùng lại. Hành vi của `DNSChecker` giữ nguyên |
| `servers` | `FromStamp` gọi `stamps.Decode`, không đổi kết quả |
| `store` | `settings.json` v5 (khối `tools`), đường dẫn `cfscan-cache.json`. Hàm `MigrateSettings(raw []byte) (Settings, error)` dùng chung cho đọc file và nhập |
| `rules` | `AppendUserRules(rs []Rule)` thêm vào cuối, bỏ trùng theo mẫu và hành động |
| `cli` / `headless.go` | `--export <file>` (dùng cho hỗ trợ người dùng). **Không** có `--import` dòng lệnh |

### 4.3 Ranh giới

- `lookup`, `scanner/advanced`, `cfscan`, `stamps`, `backup` không phụ thuộc `app` và không gọi Win32.
- `cfscan` nhận `Dial func(ctx, network, addr string) (net.Conn, error)` để test bằng server giả. App truyền `net.Dialer` thường: quét IP sạch **không đi qua proxy** của Ghostline, vì mục đích là đo đường trực tiếp.
- `backup` không biết Wails; nó đọc và ghi qua các interface nhỏ (`SettingsStore`, `RulesStore`, `CustomServers`, `TextFile`).

### 4.4 Thư mục dữ liệu (thêm)

```
cfscan-cache.json      kết quả quét IP Cloudflare theo mạng (khoá: NetworkKey như scan-cache)
```

Advanced Scanner và Lookup không lưu gì xuống đĩa.

## 5. DNS Lookup

### 5.1 Truy vấn

- **Loại bản ghi:** A, AAAA, CNAME, MX, TXT, NS, SOA, HTTPS, CAA, PTR. Với PTR, người dùng nhập IP và app tự đổi sang `in-addr.arpa`/`ip6.arpa`.
- **Nguồn:** `ghostline` (engine cục bộ, chỉ khi đã kết nối) · `server:<id>` · `address:<url hoặc stamp>` · `isp:<ip>` (plain UDP 53).
- Bật bit DO (DNSSEC OK) để đọc được cờ AD. Mỗi nguồn có thời hạn 5 giây.
- **Chế độ so sánh:** một tên miền, một loại bản ghi, 2–6 nguồn chạy song song. Mặc định gồm Ghostline (hoặc server tốt nhất) và 2 server nhanh nhất từ lần quét gần nhất.

### 5.2 Kết luận (chế độ so sánh)

So tập địa chỉ của A/AAAA giữa các nguồn, bỏ qua thứ tự và TTL:

| Kết luận | Điều kiện |
|---|---|
| `poisoned` | Một nguồn trả IP không công khai (`scanner.IsPublicIP`), hoặc NXDOMAIN trong khi các nguồn khác có địa chỉ |
| `differs` | Tập địa chỉ khác nhau và **không** có ASN chung. ASN đọc từ một bảng tiền tố nhỏ nhúng sẵn cho CDN phổ biến (Cloudflare, Google, Akamai, Fastly, Amazon CloudFront); ngoài bảng thì chỉ so tập IP. Chú thích: "CDN có thể trả IP khác nhau theo vị trí" |
| `match` | Cùng tập địa chỉ, hoặc cùng nhà CDN trong bảng |
| `failed` | Nguồn đó lỗi hoặc hết thời hạn; không tính vào kết luận chung |

Kết luận chung lấy mức nặng nhất: `poisoned` > `differs` > `match`. Các loại bản ghi khác chỉ hiện bảng, không kết luận.

### 5.3 Hiển thị

- Mặc định: thẻ kết luận, bảng nguồn × (trạng thái, độ trễ, địa chỉ chính, AD).
- "Chi tiết": mỗi nguồn một khối văn bản kiểu `dig` (header, flags, question, answer, authority, additional, TTL), nút sao chép.
- Không có nút tạo rule từ kết quả Lookup: `upstream=` trong rule chỉ upstream proxy (socks/http), Ghostline chưa có rule chọn DNS server theo tên miền.

## 6. Advanced DNS Scanner

### 6.1 Đầu vào

- **Danh sách có sẵn**, lọc theo giao thức (DoH/DoT/DoQ/DNSCrypt), thẻ (`no-log`, `no-filter`, `dnssec`), nguồn (`builtin`, `remote`, `dnscrypt`, `custom`) và chỉ server đã ghim.
- **Dán vào:** URL, stamp, mỗi dòng một server, phân tích bằng `servers.ParseImport` (giống "Thêm server"). Dòng lỗi được liệt kê, không chặn quét.
- **Tuỳ chọn:** số lượt đo độ trễ (3–20, mặc định 5), số worker (4–32, mặc định 8), thời hạn mỗi truy vấn (1–10 giây, mặc định 3), các tên miền kiểm tra đầu độc (mặc định lấy từ `probeSites`).

### 6.2 Phép kiểm tra cho mỗi server

Một upstream dùng cho mọi phép, dựng một lần. Thứ tự:

1. **Kết nối:** dùng `scanner.DNSChecker`. Lỗi thì dừng, ghi lý do như hiện nay.
2. **Độ trễ:** N truy vấn A tới tên miền ngẫu nhiên dạng `<8 ký tự>.<testDomain>` (tránh cache) cách nhau 100 ms. Ghi min/trung vị/p90/jitter (độ lệch chuẩn) và **tỉ lệ mất** (lỗi hoặc quá thời hạn / N). NXDOMAIN tính là thành công.
3. **DNSSEC:** truy vấn `dnssec-failed.org` A với DO. `SERVFAIL` → server **có kiểm tra** DNSSEC. Có địa chỉ → **không** kiểm tra. Kèm theo, `cloudflare.com` A với DO có cờ AD → xác nhận.
4. **Lọc quảng cáo:** truy vấn `doubleclick.net` và `googleadservices.com`. Nếu cả hai trả `0.0.0.0`/`::`, NXDOMAIN hoặc IP không công khai thì "có lọc". Một trong hai thì "lọc một phần".
5. **Chống đầu độc:** với mỗi tên miền kiểm tra, có địa chỉ công khai thì đạt. Có IP không công khai hoặc NXDOMAIN thì "bị đầu độc" (kèm tên miền).

Mọi phép sau bước 1 có thể lỗi riêng; lỗi được ghi `unknown` cho tiêu chí đó, không đánh trượt cả server.

### 6.3 Điểm và sắp xếp

- Server không đạt bước 1 hoặc bị đầu độc: nằm cuối, không có điểm.
- Còn lại sắp theo: tỉ lệ mất tăng dần (làm tròn 10%), rồi trung vị độ trễ tăng dần. DNSSEC và lọc quảng cáo chỉ là cột lọc, không cộng điểm, vì tuỳ nhu cầu người dùng.
- Không ghi đè `scan-cache.json`; kết quả nâng cao chỉ ở trong RAM cho tới khi đóng app hoặc quét lại.

### 6.4 Hành động trên kết quả

Ghim / bỏ ghim (dùng `SetPinnedMany`), thêm server dán vào danh sách tự thêm, "Chỉ dùng server này" (như trang Server), xuất CSV (UTF-8 có BOM để Excel mở đúng tiếng Việt).

### 6.5 Giới hạn

- Tối đa `tools.scanner.maxServers` server một lần (mặc định 500, chỉnh được 50–2000). Bộ lọc khớp nhiều hơn thì quét các server hữu ích nhất trước: đã ghim, chạy tốt ở lần quét trước (nhanh trước), rồi theo nguồn (có sẵn, danh sách tải về, tự thêm, DNSCrypt), và báo số server bị bỏ qua. Danh sách dán vào dài hơn giới hạn thì báo `SCAN_TOO_MANY`. Toàn bộ lượt quét ≤ 10 phút, hết thì dừng và hiện phần đã có.
- Tiến độ phát qua sự kiện `tools:scan` (đã xong/tổng, kết quả từng server), tối đa 10 sự kiện/giây.

## 7. Quét IP sạch Cloudflare

### 7.1 Lấy mẫu

- Từ dải IPv4 nhúng sẵn, chia thành các khối /24. Mặc định lấy **ngẫu nhiên 1 IP trong mỗi /24**, xáo trộn thứ tự, rồi lấy tối đa `maxIPs` (mặc định 2000, giới hạn 200–10000).
- Hạt giống ngẫu nhiên theo thời gian, nên mỗi lần quét lấy IP khác nhau. Không bao giờ quét IP ngoài các dải nhúng sẵn, và kiểm tra lại điều này trước khi quay số.

### 7.2 Kiểm tra một IP

Thời hạn mặc định cho cả chuỗi là 2 giây, chỉnh được 1–5 giây:

1. TCP tới `IP:443` → ghi thời gian kết nối.
2. TLS 1.2+ với SNI = host kiểm tra, ALPN `http/1.1`, chứng chỉ phải hợp lệ với kho gốc hệ thống và đúng host. Sai tên hoặc lỗi chuỗi thì trượt (`tls_verify`).
3. `GET /cdn-cgi/trace` với `Host` = host kiểm tra. Phải có `200` và các dòng `colo=` và `ip=`. Ghi `colo` (mã sân bay của PoP) và thời gian TLS + HTTP.
4. **Độ trễ** = thời gian từ lúc TCP xong tới lúc nhận đủ trace.

Lý do trượt: `tcp_timeout`, `tcp_refused`, `tls_timeout`, `tls_reset`, `tls_verify`, `http_status`, `bad_trace`.

### 7.3 Đo tốc độ (tuỳ chọn, bật sẵn)

Sau khi kiểm tra xong, lấy 10 IP có độ trễ tốt nhất và tải `https://<host>/__down?bytes=<n>` qua chính IP đó (mặc định 1 MB, tối đa 10 giây mỗi IP, tuần tự). Ghi Mbit/s. Nếu đổi host kiểm tra sang host không có `/__down` thì tự tắt bước này, có ghi chú.

### 7.4 Song song và giới hạn

- Mặc định 64 kết nối đồng thời (chỉnh 8–128). Mở tối đa 200 kết nối TCP mới mỗi giây, để không bị thiết bị mạng hay nhà mạng coi là quét cổng.
- Dừng sớm khi đã có `want` IP đạt (mặc định 50, 0 = quét hết mẫu).
- Huỷ: đóng mọi kết nối đang mở trong ≤ 1 giây.

### 7.5 Kết quả và dùng kết quả

- Bảng: IP, độ trễ, `colo`, tốc độ, lần kiểm tra. Sắp theo tốc độ (nếu có) rồi theo độ trễ.
- Lưu 100 IP tốt nhất vào `cfscan-cache.json` theo mạng hiện tại; mở lại tab là thấy kết quả cũ kèm thời điểm quét.
- **Sao chép** một hoặc nhiều IP (mỗi dòng một IP).
- **Tạo rule:** người dùng nhập một hoặc nhiều mẫu domain (gợi ý sẵn các domain có rule hoặc danh sách thuộc nhóm Cloudflare), chọn 1–4 IP. App thêm `<mẫu> ip=<IP1>,<IP2>` vào rule người dùng qua `rules.AppendUserRules`. Trang hiện chú thích: rule chỉ có tác dụng với ứng dụng dùng DNS hoặc proxy của Ghostline, và chỉ đúng với domain thật sự nằm sau Cloudflare.
- **Kiểm tra lại:** chạy lại 7.2 cho các IP đang chọn.

## 8. STAMP

### 8.1 Giải mã

- Dán một hoặc nhiều stamp (mỗi dòng một stamp). Mỗi stamp hiện thành một thẻ gồm: giao thức, địa chỉ server, tên host, đường dẫn, các hash chứng chỉ (hex), khoá công khai DNSCrypt (hex), tên provider, các cờ (DNSSEC, no-log, no-filter), cùng chuỗi stamp chuẩn hoá.
- Hỗ trợ đọc mọi loại `dnsstamps` biết, kể cả relay và ODoH. Loại Ghostline không dùng được thì có nhãn "Ghostline không dùng loại này" và không có nút thêm.

### 8.2 Tạo

- **Từ URL:** `https://host/path`, `tls://host`, `quic://host`, có thể kèm IP. App điền các trường rồi tạo stamp.
- **Từ các trường:** biểu mẫu theo giao thức (DoH, DoT, DoQ, DNSCrypt, plain). Kiểm tra từng trường: IP hoặc `IP:cổng` hợp lệ, hash 32 byte hex, khoá DNSCrypt 32 byte, tên provider DNSCrypt bắt đầu bằng `2.dnscrypt-cert.`.
- Stamp tạo xong được tự giải mã lại để so khớp; không khớp thì báo lỗi nội bộ, không hiện stamp.
- Nút **"Thêm vào danh sách server"** (chỉ với loại mã hoá) gọi `AddServers`.

## 9. Import/export cài đặt

### 9.1 Định dạng file

Tên mặc định `ghostline-<yyyy-mm-dd>.ghostline.json`:

```json
{
  "format": "ghostline-backup",
  "formatVersion": 1,
  "appVersion": "0.5.0",
  "createdAt": "2026-10-06T10:00:00Z",
  "sections": {
    "settings": { "...": "settings.json v5, bỏ các trường ở 9.2" },
    "rules": { "...": "rules.json: rule người dùng và danh sách đăng ký (không có nội dung cache)" },
    "customServers": [ "..." ],
    "dpiBlacklist": "nội dung dpi-blacklist.txt",
    "dpiAutoHostlist": [ "..." ]
  }
}
```

- Mỗi phần là tuỳ chọn. Người dùng chọn phần nào khi xuất (mặc định chọn hết).
- Kích thước tối đa khi nhập: 8 MB. Lớn hơn thì từ chối.

### 9.2 Không bao giờ xuất

`adapterGuids`, `advancedWindow`, `dnsServer.iosSsid` (tên Wi-Fi nhà), mọi file trong `state.json`, `scan-cache.json`, `frag-cache.json`, `cfscan-cache.json`, `lan-ca.*`, nội dung cache của danh sách (`lists/*.txt`), log. **Mật khẩu proxy upstream** (`proxy.upstreams[].passEnc`, đã mã hoá theo máy nên sang máy khác cũng không giải được) bị xoá khi xuất; người dùng phải nhập lại sau khi nhập file.

### 9.3 Nhập

1. **Chọn file** → `PreviewImport(path)`:
   - Kiểm tra `format`, `formatVersion` ≤ 1, kích thước, JSON hợp lệ.
   - Chạy `settings` qua `store.MigrateSettings` (file từ v0.4 trở về trước có thể mang settings v4).
   - Kiểm tra như khi lưu: settings qua `validate`, rule qua parser, danh sách qua `validateList`, server qua `ParseImport`.
   - Trả về **bản xem trước**: mỗi phần có số mục mới, số mục bị thay thế, lỗi, và các cảnh báo an toàn (9.4).
2. Người dùng chọn phần cần nhập, chọn **Thay thế** (mặc định) hoặc **Gộp** (chỉ với rule, server tự thêm và danh sách đen vượt DPI: thêm mục chưa có, giữ mục cũ).
3. `ApplyImport(token, choices)`: token gắn với bản xem trước, sống 10 phút. Ghi từng file nguyên tử (`WriteJSONAtomic`); trước khi ghi, sao lưu file cũ thành `<tên>.bak-import`. Một phần ghi lỗi thì khôi phục **mọi** phần đã ghi từ `.bak-import` và báo `IMPORT_WRITE_FAILED`.
4. Sau khi nhập: nạp lại settings, rule, danh sách; xếp lịch tải các danh sách đăng ký; hiện "Đã nhập. Bấm Kết nối để dùng cấu hình mới".

### 9.4 Kiểm tra an toàn khi nhập

| Trường hợp | Xử lý |
|---|---|
| Đang kết nối hoặc đang kết nối dở | Nút Nhập bị tắt, có ghi lý do. `ApplyImport` trả `IMPORT_WHILE_CONNECTED` |
| `fakeSni.enabled`, `dnsServer.enabled`, `dnsServer.shareLan`, `proxy.shareLan`, `startWithWindows` là `true` | Đặt về `false`, liệt kê trong bản xem trước: "Bật lại trong trang tương ứng nếu cần" |
| `fakeSni.ackVersion` | Giữ giá trị của máy này, không lấy từ file (cảnh báo Fake SNI phải được đọc trên máy này) |
| Danh sách có `trustedForSNI: true` | Đặt về `false`, trừ preset chính thức có `signed: true`. Preset vẫn phải qua kiểm tra chữ ký khi tải |
| Rule người dùng có `sni=` hoặc `connect=` | Hiện danh sách các rule đó. Phần rule chỉ nhập được khi tích "Tôi hiểu các rule này chuyển hướng lưu lượng đã giải mã", hoặc chọn "Nhập nhưng bỏ các rule này" |
| Danh sách đăng ký trỏ tới URL `http://` | Cảnh báo, vẫn cho nhập (giống khi thêm tay) |
| `formatVersion` lớn hơn bản app hiểu | Từ chối: "File được tạo bởi bản Ghostline mới hơn" |

## 10. Giao diện

### 10.1 Trang "Công cụ" (chế độ Nâng cao)

Thêm `tools` vào thanh bên, giữa `fakesni` và `logs`. Bốn tab, nhớ tab đang mở trong store của frontend:

| Tab | Thành phần |
|---|---|
| Lookup | Ô tên miền, chọn loại bản ghi, chọn nguồn (nhiều lựa chọn, "DNS nhà mạng" có biểu tượng cảnh báo và chú thích), nút Tra. Thẻ kết luận, bảng nguồn, "Chi tiết" |
| Scanner | Chọn đầu vào (danh sách đã lọc / dán vào), tuỳ chọn gập lại, nút Quét/Huỷ, thanh tiến độ, bảng kết quả có lọc theo cột (DNSSEC, lọc quảng cáo, giao thức), thao tác hàng loạt |
| IP Cloudflare | Host kiểm tra, các tuỳ chọn gập lại, Quét/Huỷ, tiến độ (đã thử/đạt), bảng kết quả, Sao chép / Tạo rule / Kiểm tra lại |
| STAMP | Hai cột: Giải mã (ô dán, thẻ kết quả) và Tạo (từ URL hoặc biểu mẫu theo giao thức) |

Bảng dài hiển thị như bảng ở trang Server. Lỗi của từng tác vụ hiện trong tab đó, không dùng hộp thoại.

### 10.2 Trang Cài đặt

Mục mới **"Sao lưu và chuyển máy"**: nút "Xuất cài đặt…" (hộp thoại chọn phần, rồi hộp thoại lưu file của Wails) và "Nhập cài đặt…" (chọn file → màn xem trước có ô chọn từng phần, cảnh báo, Thay thế/Gộp → Nhập).

### 10.3 Chế độ Đơn giản và khay

Không đổi.

## 11. Cài đặt (`settings.json`, phiên bản 5)

Thêm vào phiên bản 4:

```json
{
  "version": 5,
  "tools": {
    "scanner": { "rounds": 5, "workers": 8, "timeoutMs": 3000, "maxServers": 500 },
    "cfscan": {
      "host": "speed.cloudflare.com",
      "maxIps": 2000,
      "want": 50,
      "concurrency": 64,
      "timeoutMs": 2000,
      "speedTest": true,
      "speedBytes": 1048576
    }
  }
}
```

- Nâng cấp từ v4: thêm khối `tools` mặc định, giữ nguyên mọi trường cũ, ghi lại thành v5.
- Kiểm tra khi lưu theo các giới hạn ở mục 6.1 và 7. `host` phải là tên miền hợp lệ (không phải IP), ≤ 253 ký tự. `speedBytes` 100 KB–25 MB.
- File xuất v0.5 luôn mang settings v5.

## 12. Xử lý lỗi

| Mã | Khi nào | Hành động gợi ý |
|---|---|---|
| `TOOL_BUSY{tool}` | Bắt đầu một tác vụ khi tác vụ cùng loại đang chạy | Huỷ tác vụ đang chạy |
| `LOOKUP_NOT_CONNECTED` | Chọn nguồn Ghostline khi chưa kết nối | Kết nối · Chọn nguồn khác |
| `LOOKUP_BAD_NAME` | Tên miền hoặc IP (PTR) không hợp lệ | Sửa ô nhập |
| `SCAN_TOO_MANY{count,max}` | Danh sách dán vào dài hơn `maxServers` | Tăng giới hạn hoặc dán ít dòng hơn |
| `CFSCAN_NO_NETWORK` | Không có mạng, hoặc mọi IP đều trượt ở TCP trong 200 lần thử đầu | Kiểm tra mạng · Tắt VPN/firewall khác |
| `CFSCAN_HOST_INVALID` | Host kiểm tra không phải tên miền hợp lệ | Khôi phục mặc định |
| `STAMP_INVALID{detail}` | Không giải mã được, hoặc trường không hợp lệ khi tạo | Sửa trường được chỉ ra |
| `IMPORT_INVALID{detail}` | Sai định dạng, quá lớn, phiên bản mới hơn | Chọn file khác |
| `IMPORT_WHILE_CONNECTED` | Nhập khi đang kết nối | Ngắt kết nối |
| `IMPORT_EXPIRED` | Token xem trước quá 10 phút | Chọn lại file |
| `IMPORT_WRITE_FAILED{file}` | Ghi lỗi; mọi phần đã được khôi phục | Thử lại · Mở nhật ký |
| `EXPORT_WRITE_FAILED{detail}` | Không ghi được file xuất | Chọn thư mục khác |

- Panic trong goroutine của công cụ: bắt lại, ghi log kèm stack trace, kết thúc tác vụ với lỗi, app vẫn chạy.

## 13. Kiểm thử

Phát triển theo TDD. Test cần mạng thật hoặc Windows thật gắn build tag `integration`.

| Phần | Test |
|---|---|
| `lookup` | Server DNS giả (`miekg/dns` trên cổng ngẫu nhiên): mỗi loại bản ghi, PTR tự đổi tên, cờ AD/TC, NXDOMAIN, hết thời hạn. Kết luận: IP riêng → `poisoned`; NXDOMAIN với nguồn khác có IP → `poisoned`; cùng CDN khác IP → `match`; khác hẳn → `differs`; nguồn lỗi không ảnh hưởng kết luận. Định dạng `dig` ổn định (golden file) |
| `scanner/advanced` | Server giả: độ trễ và tỉ lệ mất với đồng hồ giả và lỗi chèn vào; DNSSEC (SERVFAIL/có địa chỉ/AD); lọc quảng cáo đủ/một phần/không; đầu độc; lỗi một phép → `unknown`, không trượt cả server; sắp xếp; huỷ ≤ 1 giây; giới hạn `MaxServers` |
| `cfscan` | Dải nhúng parse được, mọi IP lấy mẫu nằm trong dải (property test), 1 IP mỗi /24. Server TLS giả qua `Dial` giả: trace hợp lệ, sai trạng thái, thiếu `colo`, chứng chỉ sai tên → `tls_verify`, reset, timeout. Giới hạn 200 kết nối/giây (đồng hồ giả). Dừng sớm khi đủ `want`. Huỷ đóng mọi conn. Cache theo mạng. `integration`: quét thật 200 IP |
| `stamps` | Vòng tạo → giải mã cho từng giao thức; giải mã mọi stamp trong `servers.json` và danh sách DNSCrypt; trường sai (hash sai độ dài, IP sai, provider DNSCrypt sai tiền tố) bị từ chối; fuzz `Decode` |
| `backup` | Vòng xuất → nhập ra cùng dữ liệu. Các trường ở 9.2 không bao giờ có trong file (kể cả mật khẩu proxy). Mọi trường hợp ở 9.4. Nhập settings v1–v4 qua `MigrateSettings`. Lỗi ghi ở phần thứ 2 → phần 1 được khôi phục từ `.bak-import`. Gộp không tạo trùng. Token hết hạn (đồng hồ giả) |
| `store` | Nâng `settings.json` v4 → v5; kiểm tra giới hạn của `tools` |
| `rules` | `AppendUserRules` bỏ trùng, giữ thứ tự, tôn trọng `MaxUserRules` |
| `app` | Công cụ chạy song song với Connect/Disconnect không bị khoá (test có thời hạn); `TOOL_BUSY`; nhập khi đang kết nối bị từ chối; sau khi nhập settings được nạp lại |
| Frontend | Trang Công cụ: 4 tab, nguồn "DNS nhà mạng" không bao giờ được chọn sẵn; thẻ kết luận theo từng loại; bảng scanner lọc theo cột; Tạo rule từ IP; STAMP tạo/giải mã; màn xem trước nhập: cảnh báo, ô tích rule `sni=`, nút Nhập tắt khi đang kết nối. Parity `vi.json`/`en.json` |
| Kiểm tra thủ công trước phát hành | Trên mạng Viettel, VNPT, FPT: Lookup một tên miền bị chặn qua DNS nhà mạng và qua Ghostline cho ra `poisoned`. Quét IP Cloudflare có ≥ 10 IP trong ≤ 60 giây; một rule `ip=` từ kết quả mở được trang thật sau Cloudflare. Xuất trên máy A, nhập trên máy B (cài mới): rule, server, ghim, vượt DPI giống nhau; Fake SNI và DNS server cho LAN vẫn tắt. Windows Defender Firewall và phần mềm diệt virus không cảnh báo khi quét IP Cloudflare ở mặc định |

## 14. Đóng gói và tài liệu

- Không thêm dependency Go, không thêm file nhị phân.
- `internal/cfscan/ranges_v4.txt` nhúng bằng `go:embed`; `release-checklist.md` thêm bước chạy `go run ./tools/gencfranges` và xem lại diff.
- README (EN + VI) và hướng dẫn sử dụng: mục Công cụ (Lookup và cách đọc kết luận, Scanner, IP Cloudflare kèm cách tạo rule và giới hạn, STAMP), mục Sao lưu và chuyển máy (những gì không được xuất, những gì bị tắt khi nhập).
- `release-checklist.md`: thêm các mục kiểm tra thủ công ở mục 13.
- Phát hành dưới dạng **v0.5.0**.

## 15. Cấu trúc repo (thêm)

```
internal/
  lookup/              query.go, compare.go, dig.go, cdn.go
  scanner/advanced/    checks.go, score.go
  cfscan/              ranges.go, ranges_v4.txt, sample.go, probe.go, speed.go, cache.go
  stamps/              decode.go, encode.go
  backup/              format.go, build.go, plan.go, apply.go
  app/                 toolsservice.go, backupservice.go
tools/gencfranges/     main.go
frontend/src/modes/advanced/pages/tools/   Tools.tsx, Lookup.tsx, Scanner.tsx, CfScan.tsx, Stamp.tsx
frontend/src/modes/advanced/pages/settings/Backup.tsx
```

## 16. Rủi ro và cách giảm thiểu

| Rủi ro | Giảm thiểu |
|---|---|
| Lookup qua DNS nhà mạng làm lộ tên miền đang tra | Chỉ có ở Lookup, chỉ khi người dùng tự chọn nguồn đó, không bao giờ chọn sẵn, có nhãn "không mã hoá". Scanner chỉ nhận server mã hoá (`ParseImport` từ chối plain) |
| Quét IP Cloudflare bị coi là quét cổng, bị chặn hoặc bị báo | Chỉ quét dải của Cloudflare, chỉ cổng 443, giới hạn 200 kết nối mới/giây và 64 đồng thời, mặc định 2000 IP; không tự quét theo lịch |
| IP "sạch" hôm nay bị chặn ngày mai, hoặc Cloudflare đổi PoP | Hiện thời điểm quét; nút Kiểm tra lại; rule `ip=` có nhiều IP; hướng dẫn quét lại khi trang không vào được |
| Rule `ip=` cho domain không nằm sau Cloudflare làm hỏng trang | Chú thích ngay ở bước Tạo rule; gợi ý domain từ nhóm Cloudflare; rule tắt hoặc xoá được như mọi rule khác |
| File nhập độc hại bật MITM hoặc mở cổng LAN | Mọi cờ nguy hiểm bị tắt khi nhập; `trustedForSNI` bị đặt lại; rule `sni=`/`connect=` cần xác nhận riêng; chỉ nhập khi đã ngắt kết nối |
| File xuất làm lộ thông tin cá nhân | Không xuất tên Wi-Fi, GUID card mạng, gateway, mật khẩu proxy, log hay cache; danh sách trường bị loại được test |
| Nhập lỗi giữa chừng làm hỏng cấu hình | Sao lưu `.bak-import` trước khi ghi; lỗi thì khôi phục mọi phần; ghi nguyên tử từng file |
| Advanced Scanner làm chậm Connect hoặc tốn băng thông | Chạy ngoài `opMu`; giới hạn worker và số server; truy vấn DNS nhỏ; huỷ được |
| Dải IP Cloudflare nhúng sẵn bị cũ | Cập nhật bằng `tools/gencfranges` ở mỗi bản phát hành; dải của Cloudflare hiếm khi thay đổi |
