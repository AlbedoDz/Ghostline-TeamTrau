# Ghostline — Giai đoạn 2B: Thiết kế

- **Ngày:** 2026-10-05
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Giai đoạn 2B — chứng chỉ gốc giới hạn phạm vi, DoH server cục bộ kèm DNS cho LAN và Fake SNI (giải mã TLS, domain fronting).
- **Dựa trên:** [Giai đoạn 1](2026-10-04-ghostline-phase1-design.md), [Giai đoạn 2A](2026-10-04-ghostline-phase2a-design.md) và [engine zapret2](2026-10-05-ghostline-zapret2-design.md) (đã phát hành ở v0.3.0). Mọi thứ không nhắc lại ở đây giữ nguyên.

---

## 1. Mục tiêu

- **DNS cho thiết bị khác:** điện thoại, máy tính khác, router và TV trong mạng Private dùng DNS mã hoá của Ghostline. iOS dùng DoH qua một file `.mobileconfig`; các thiết bị còn lại dùng DNS thường cổng 53 trỏ vào máy. Từ máy ra Internet luôn được mã hoá.
- **DoH cục bộ cho máy này:** trình duyệt bật "DNS an toàn" tuỳ chỉnh trỏ vào `https://127.0.0.1/dns-query` mà vẫn đi qua engine của Ghostline.
- **Fake SNI:** với domain có rule `sni=`, proxy giải mã TLS của trình duyệt rồi mở kết nối TLS mới tới server với SNI giả (domain fronting), để vào được những trang mà fragment không qua được.

### Tiêu chí thành công

1. Điện thoại cùng mạng Private dùng được DNS của Ghostline: iOS qua DoH (cài một file profile), thiết bị khác qua DNS 53. Người dùng không phải nhập gì ngoài IP máy, hoặc chỉ cần quét mã QR.
2. Có rule `sni=` (lấy từ preset) thì trình duyệt trên máy mở được domain trong preset mà fragment không qua được, khi vượt DPI (cả hai engine) tắt.
3. **Không có tình huống nào để CA phiên sót lại trong Root.** Disconnect, crash hay bị kill thì CA được gỡ trong ≤ 3 giây; mất điện thì gỡ ở lần đăng nhập kế tiếp; gỡ cài đặt app thì gỡ mọi chứng chỉ Ghostline.
4. **Khoá CA phiên không bao giờ được ghi xuống đĩa.** Khoá CA LAN chỉ ký được cho IP riêng và `*.ghostline.lan`. Kiểm chứng bằng test.
5. **Không rò DNS plain:** mọi truy vấn tới DoH server hoặc DNS 53 cho LAN đều đi qua engine mã hoá. Fake SNI phân giải tên miền qua engine như proxy 2A.
6. Người dùng 2A nâng cấp lên **không thấy hành vi nào thay đổi** cho tới khi tự bật tính năng mới. Mọi chuỗi giao diện mới có đủ tiếng Việt và tiếng Anh.

## 2. Phạm vi

### Có trong giai đoạn 2B

| Nhóm | Nội dung |
|---|---|
| Chứng chỉ | Hai CA ECDSA P-256 có Name Constraints và EKU chỉ `serverAuth`: **CA LAN** (lâu dài) và **CA Fake SNI phiên** (chỉ trong RAM). Cài/gỡ trong `LocalMachine\Root`, quét dọn phòng thủ, gỡ khi gỡ app |
| DNS server | DoH (RFC 8484) trên loopback và IP LAN, DNS UDP/TCP 53 trên IP LAN. Lọc nguồn IP riêng, rate limit, firewall chỉ Private. Trang cài đặt tạm cho điện thoại (`.crt`, `.mobileconfig` có `OnDemandRules` theo SSID), mã QR |
| Fake SNI | Rule `sni=<tên>` / `sni=none` và `connect=<domain>`. Giải mã ở proxy, kiểm tra chứng chỉ server theo SNI giả hoặc host thật, khớp ALPN, quay về đường 2A khi server từ chối. Preset là danh sách cộng đồng **có chữ ký**. Trang cảnh báo bắt buộc, banner cố định |

### Không có trong giai đoạn 2B

- **Fake SNI cho thiết bị khác** (điện thoại trỏ proxy vào máy). CA phiên sinh mới mỗi lần Connect nên thiết bị khác không thể tin cậy nó một cách thực tế. Fragment (2A) và engine vượt DPI vẫn áp dụng cho lưu lượng của thiết bị đi qua proxy.
- **Công tắc "gói giả mang SNI" riêng.** Engine zapret2 (mặc định khi cài mới) đã có gói giả trong chiến lược (`z-fake`, `z-fake-ttl`), và GoodbyeDPI cho phép tự thêm `--fake-with-sni` trong tham số tuỳ chỉnh từ v0.3.0. Không thêm gì ở `internal/dpi`.
- **CA "không giới hạn"** (CA dài hạn ký được mọi domain). Đã cân nhắc và bỏ; xem lại khi có nhu cầu thật.
- `sni=` với mẫu keyword, regexp hoặc CIDR (không biểu diễn được bằng Name Constraints).
- DoT (cổng 853) cho Android Private DNS: Android không tin CA người dùng cài cho Private DNS.
- Chạy DNS server khi chưa Connect (không có engine mã hoá để chuyển truy vấn đi).
- Lưu, xem hay sửa nội dung HTTP đã giải mã.
- `certstore` cho macOS/Linux. Chỉ có interface sẵn sàng (mục 4.1). Không dùng `smallstep/truststore` cho Windows vì nó cài vào kho `CurrentUser` và hiện hộp thoại mỗi lần cài, không hợp với CA phiên cài mỗi lần Connect.

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Phạm vi | Cả ba phần (CA, DNS server, Fake SNI) trong một spec. Không đụng tới engine vượt DPI |
| Cách làm | Mở rộng `engine` (listener của `dnsproxy`) và `internal/proxy` tự viết, chỉ dùng thư viện chuẩn của Go. Không dùng `goproxy`/`martian`, không chạy process riêng |
| Chứng chỉ gốc | **Hai CA giới hạn phạm vi.** CA LAN: Name Constraints chỉ IP riêng và `ghostline.lan`, lưu lâu dài. CA Fake SNI: sinh mỗi lần Connect, **khoá chỉ trong RAM**, Name Constraints đúng các domain có rule `sni=`. Không có tuỳ chọn CA không giới hạn |
| Kho chứng chỉ | `LocalMachine\Root` (app đã chạy quyền admin, không hiện hộp thoại) |
| Thiết bị dùng DNS server | Máy này (DoH loopback) và LAN (DoH + DNS 53 trên IP LAN) |
| Kích hoạt Fake SNI | Chỉ khi có rule `sni=` viết tay hoặc từ danh sách. **Không bao giờ tự động giải mã.** Preset bật theo nhóm |
| Kiểm tra chứng chỉ server | Chuỗi hợp lệ với kho gốc hệ thống và tên khớp **SNI giả hoặc host thật** (`sni=none`: chỉ host thật). Không bao giờ bỏ qua |
| Server từ chối SNI giả | Quay về đường 2A (fragment) bằng ClientHello gốc; trình duyệt không thấy lỗi |
| Preset | Danh sách cộng đồng dạng rule Ghostline, **có chữ ký**, cùng cơ chế và khoá với `servers.json` và `strategies.json`, cập nhật theo lịch, có bản dự phòng nhúng trong app |
| Quan hệ với Connect và Proxy | Pha D (DNS server) và pha S (Fake SNI) là pha phụ sau pha P. Lỗi → SUY_GIẢM, DNS vẫn được bảo vệ. Fake SNI cần proxy bật |
| Cổng DoH | `443`, đổi được |

## 4. Kiến trúc

### 4.1 Package mới

| Package | Việc |
|---|---|
| `internal/certs` | Sinh CA (ECDSA P-256) với Name Constraints và EKU `serverAuth`, ký chứng chỉ lá (cache LRU theo tên, tối đa 1000, sống 7 ngày), tính vân tay, xuất DER/PEM, sinh `.mobileconfig`. Thuần Go, không gọi Win32 |
| `internal/certstore` | Cài, gỡ, liệt kê chứng chỉ trong `LocalMachine\Root` theo thumbprint SHA-1 và theo tiền tố subject. Win32 (`CertOpenStore`, `CertAddEncodedCertificateToStore`, `CertFindCertificateInStore`, `CertDeleteCertificateFromStore`) nằm sau interface `API` để test được. Interface `Store` (`Install`, `Remove`, `List`) không mang kiểu Win32, để bản macOS/Linux sau này dùng `smallstep/truststore` mà không đổi `app` |
| `internal/proxy/mitm` | Nhánh Fake SNI: bắt tay TLS tới server bằng SNI giả, kiểm tra chứng chỉ, rồi bắt tay TLS với client bằng chứng chỉ lá, khớp ALPN, trả về hai `net.Conn` cho `relay` |
| `internal/dnsserver` | Phần không thuộc `dnsproxy`: chọn IP LAN để nghe, lọc nguồn, trang cài đặt tạm cho điện thoại (HTTP server 10 phút) |

### 4.2 Thay đổi ở package cũ

| Package | Thay đổi |
|---|---|
| `engine` | Thêm listener HTTPS (DoH) và UDP/TCP trên IP LAN qua `dnsproxy`, `TLSConfig.GetCertificate` lấy chứng chỉ lá từ `certs`. Bỏ truy vấn từ nguồn không phải IP riêng. `Ratelimit` 100/giây mỗi IP, từ chối `ANY`. `Restart` listener mà không đổi upstream |
| `rules` | Bật `sni=`, thêm `connect=`. Kiểm tra `sni=`/`connect=` chỉ đi với mẫu domain. `Compiled.SNIDomains()` trả tập domain cho Name Constraints. Định dạng danh sách mới `ghostline` (cú pháp rule text, hành động theo từng dòng). Cờ `trustedForSNI` cho danh sách |
| `rules/lists` | Kiểm tra chữ ký (cùng cơ chế với `servers.json`/`strategies.json`) cho danh sách có `signed: true`. Mục catalog mới nhóm "Fake SNI" |
| `tlsfrag` | Đọc danh sách ALPN trong ClientHello (`ALPN(hello) []string`) |
| `proxy`, `proxy/dialer` | Trong `tunnel`: nếu quyết định có `SNI` và có `MITM` đang hoạt động thì gọi `mitm`, lỗi phía server thì quay về đường cũ. `Dialer` thêm `connect=` (phân giải domain khác lấy IP) |
| `app` | Pha D và pha S (mục 6), lý do SUY_GIẢM `dnsserver`/`fakesni`, xoay CA phiên khi rule `sni=` đổi, binding cho trang DNS server và Fake SNI, mục Chứng chỉ trong Cài đặt |
| `store` | `settings.json` v4 (`dnsServer`, `fakeSni`), `state.json` v3 (`certs`, `firewall.rules`), file `lan-ca.crt` / `lan-ca.key` |
| `watchdog` | Gỡ CA phiên trước mọi bước khôi phục khác, xoá mọi luật firewall trong `state.json` |
| `winutil` | Liệt kê IP LAN kèm tên card, đọc SSID Wi-Fi hiện tại (`WlanQueryInterface`), luật firewall nhiều cổng và giao thức |
| `cli` | `--remove-certs` (gỡ mọi chứng chỉ Ghostline, dùng cho trình gỡ cài đặt) |

### 4.3 Ranh giới

- `certs` không phụ thuộc package nào của Ghostline. `certstore` chỉ phụ thuộc `golang.org/x/sys/windows`.
- `proxy/mitm` không biết `certs` hay `rules`. Nó nhận:
  ```go
  type LeafSource interface { Leaf(host string) (*tls.Certificate, error) }
  ```
  và một `Dial` đã đi qua rules/SSRF/upstream của `dialer`.
- `app` giữ `atomic.Pointer[mitm.Issuer]`: `nil` nghĩa là pha S không chạy, proxy đi đường 2A cho mọi kết nối. Khi xoay CA, `app` thay con trỏ; kết nối mới dùng CA mới, kết nối đang mở không bị ảnh hưởng.
- `engine` nhận chứng chỉ qua `func() *tls.Certificate`, không biết CA.

### 4.4 Thư mục dữ liệu (thêm)

```
lan-ca.crt             CA LAN (DER), công khai
lan-ca.key             khoá CA LAN, mã hoá DPAPI (phạm vi máy)
lists/<id>.txt.sig      chữ ký ed25519 (base64) của danh sách có chữ ký
```

Không có file nào cho CA phiên.

## 5. Chứng chỉ gốc

### 5.1 Chung

- ECDSA P-256, chữ ký `ECDSAWithSHA256`, `BasicConstraints{CA: true, MaxPathLen: 0}`.
- `ExtKeyUsage` của CA chỉ `serverAuth`: dù lộ khoá cũng không ký được code, email hay chứng chỉ client.
- Name Constraints đánh dấu **critical**.
- Subject có tiền tố cố định để quét dọn: `Ghostline LAN CA` hoặc `Ghostline Fake SNI`.
- Cài bằng `CertAddEncodedCertificateToStore(..., CERT_STORE_ADD_REPLACE_EXISTING)` vào `LocalMachine\Root`; **đọc lại** theo thumbprint để xác nhận. Gỡ theo thumbprint; không tìm thấy thì coi như thành công (idempotent).

### 5.2 CA LAN

- **Subject:** `Ghostline LAN CA — <tên máy> <4 ký tự ngẫu nhiên>`. Sống 5 năm.
- **Sinh** lần đầu khi pha D chạy và chưa có `lan-ca.key`. Khoá mã hoá DPAPI phạm vi máy (`CRYPTPROTECT_LOCAL_MACHINE`), vì app chạy bằng UAC có thể dưới tài khoản admin khác. File chỉ cấp quyền cho Administrators và SYSTEM.
- **Name Constraints cho phép:** IP `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `127.0.0.0/8`, `169.254.0.0/16`, `fc00::/7`, `fe80::/10`, `::1/128`; DNS `ghostline.lan`. Không đặt ràng buộc cho email/URI; việc dùng CA cho mục đích khác đã bị chặn bằng EKU `serverAuth`.
- **Chứng chỉ lá cho DoH:** SAN gồm các IP LAN đang nghe, `127.0.0.1`, `::1`, `dns.ghostline.lan`. Sống 90 ngày. Ký lại khi tập IP đổi hoặc khi còn dưới 14 ngày (health check).
- **Cài vào Root của máy này** khi pha D chạy, để trình duyệt trên máy tin DoH loopback. Không gỡ khi Disconnect hay crash.
- **"Tạo lại CA LAN":** gỡ CA cũ khỏi Root, xoá file, sinh CA mới, cài lại nếu pha D đang chạy. Thiết bị khác phải cài lại.
- **"Gỡ CA LAN":** chỉ được khi pha D không chạy (nút tự tắt DNS server trước, có xác nhận). Gỡ khỏi Root và xoá file.

### 5.3 CA Fake SNI phiên

- **Subject:** `Ghostline Fake SNI — phiên <thời điểm>`. `NotBefore` lùi 1 giờ, sống 30 ngày. Health check xoay CA khi còn dưới 3 ngày.
- **Khoá chỉ nằm trong RAM**, không bao giờ được serialize. Khi pha S dừng, tham chiếu bị bỏ và cache chứng chỉ lá được xoá.
- **Name Constraints cho phép:** DNS theo các rule `sni=` đang có hiệu lực:
  - `youtube.com` → `youtube.com` (domain và mọi subdomain).
  - `*.googlevideo.com` → `.googlevideo.com` (chỉ subdomain).
  - `=example.com` → `example.com`. Rộng hơn rule gốc (gồm cả subdomain), nhưng proxy vẫn chỉ giải mã đúng theo rule.
  - **Cấm mọi IP:** `ExcludedIPRanges: 0.0.0.0/0, ::/0`.
- **Tối đa 1000 domain.** Vượt thì pha S lỗi với `FAKESNI_TOO_MANY{count}`.
- **Xoay CA** (khi tập domain `sni=` đổi, hoặc gần hết hạn):
  1. Sinh CA mới trong RAM.
  2. Thêm thumbprint mới vào `state.json` `certs.session`.
  3. Cài CA mới, đọc lại xác nhận.
  4. Thay `Issuer` trong `app`.
  5. Gỡ CA cũ, xoá thumbprint cũ khỏi `state.json`.
  Lỗi ở bước 3 → giữ CA cũ, ghi `CERT_INSTALL_FAILED`. Kết nối TLS đang mở không bị cắt.
- Tập domain chỉ đổi khi biên dịch rules xong (`rules:compiled`); có debounce 2 giây để sửa nhiều dòng liên tiếp chỉ xoay một lần.

### 5.4 Quét dọn phòng thủ

- `--restore`, lần mở app (sau khôi phục `state.json`) và trình gỡ cài đặt liệt kê `LocalMachine\Root` và gỡ **mọi** chứng chỉ có subject bắt đầu bằng `Ghostline Fake SNI` mà không thuộc phiên đang chạy.
- `--remove-certs` (trình gỡ cài đặt gọi sau `--restore`) gỡ thêm mọi chứng chỉ `Ghostline LAN CA` và xoá `lan-ca.*`.

### 5.5 Trình duyệt

- Chrome và Edge dùng kho của Windows nên tin cả hai CA ngay.
- Firefox trên Windows có cơ chế tự bật `security.enterprise_roots.enabled` khi phát hiện lỗi chứng chỉ do phần mềm trên máy. App không sửa profile Firefox; trang cảnh báo và trang DNS server ghi chú cách bật tay nếu cần.

## 6. Tích hợp Connect/Disconnect và an toàn hệ thống

### 6.1 Pha D — DNS server

Chạy sau pha P khi `dnsServer.enabled`.

| # | Bước | Chi tiết | Undo |
|---|---|---|---|
| D1 | CA LAN | Nạp hoặc sinh CA LAN. Cài vào Root nếu chưa có (đọc lại xác nhận). Ký chứng chỉ lá cho tập IP hiện tại | Không có (CA LAN tồn tại lâu dài) |
| D2 | Firewall | Chỉ khi `shareLan`. Ghi tên luật vào `state.json` `firewall.rules` trước, rồi tạo `Ghostline DNS (TCP)` cho `53,<dohPort>` và `Ghostline DNS (UDP)` cho `53` (mục 6.4) | Xoá luật, bỏ tên khỏi `state.json` |
| D3 | Mở listener | DoH trên `127.0.0.1:<dohPort>` và `[::1]:<dohPort>`. Khi `shareLan`: DoH và UDP/TCP 53 trên **từng IP LAN cụ thể** (không dùng `0.0.0.0`, tránh đụng ICS/Hyper-V). IP nào không bind được thì bỏ qua và ghi lại. Tự kiểm tra: gửi một truy vấn `*.verify.ghostline.test` qua DoH loopback, đạt khi engine thấy nonce trong 2 giây | Đóng listener |

- Không mở được DoH loopback, hoặc `shareLan` mà không IP LAN nào bind được → pha D lỗi: hoàn tác, SUY_GIẢM lý do `dnsserver`, mã `DNSSERVER_PORT_IN_USE{port,addrs}`.
- Một phần IP LAN không bind được → pha D vẫn thành công; giao diện liệt kê IP bị bỏ qua.

### 6.2 Pha S — Fake SNI

Chạy sau pha D (hoặc sau pha P nếu pha D không chạy) khi `fakeSni.enabled`, `fakeSni.ackVersion` bằng phiên bản cảnh báo hiện tại, và rules có ít nhất một domain `sni=`.

| # | Bước | Chi tiết | Undo |
|---|---|---|---|
| S0 | Điều kiện | Proxy phải đang chạy. Không → bỏ qua pha S, hiện `FAKESNI_NEEDS_PROXY` (không SUY_GIẢM, vì người dùng chưa đáp ứng điều kiện) | — |
| S1 | Sinh CA | Sinh CA phiên trong RAM với tập domain hiện tại | Bỏ CA |
| S2 | Ghi trạng thái | Thêm thumbprint vào `state.json` `certs.session` **trước khi** cài | Xoá thumbprint |
| S3 | Cài CA | Cài vào Root, đọc lại xác nhận | Gỡ CA |
| S4 | Kích hoạt | Đặt `Issuer` cho proxy. Tự kiểm tra: proxy MITM một kết nối tới server TLS giả trên loopback, client kiểm tra bằng kho của Windows, đạt trong 2 giây | Đặt `Issuer = nil` |

- Pha S lỗi → hoàn tác, SUY_GIẢM lý do `fakesni`, proxy đi đường 2A cho mọi kết nối.
- **Đổi cài đặt khi đang kết nối:** bật/tắt hoặc đổi cổng DNS server → chạy lại pha D. Bật/tắt Fake SNI → chạy pha S hoặc hoàn tác pha S. Đổi rule `sni=` → xoay CA (mục 5.3), không chạy lại pha. Chạy lại pha P → chạy lại cả pha S, vì `Issuer` gắn với proxy.

### 6.3 Disconnect (thứ tự mới)

1. Hoàn tác pha S: `Issuer = nil`, **gỡ CA phiên**, xoá `certs.session`.
2. Khôi phục system proxy (như 2A).
3. Xoá mọi luật firewall trong `state.json` `firewall.rules` (proxy, DNS, trang cài đặt).
4. Đóng trang cài đặt tạm (nếu đang mở), đóng listener DoH/DNS LAN.
5. Tắt proxy.
6. Trả DNS về, xoá cache DNS.
7. Dừng engine vượt DPI.
8. Tắt engine.
9. Đặt `state.json` thành `clean`, kill watchdog, xoá tác vụ `Ghostline Recovery`.

Bất biến: không lúc nào CA phiên nằm trong Root mà không có thumbprint trong `state.json`; không lúc nào DoH/DNS LAN mở khi engine đã tắt.

### 6.4 Firewall

- Tạo (mỗi luật một lệnh):
  `netsh advfirewall firewall add rule name="Ghostline DNS (TCP)" dir=in action=allow protocol=TCP localport=53,<dohPort> program="<exe>" profile=private remoteip=localsubnet`
  `netsh advfirewall firewall add rule name="Ghostline DNS (UDP)" dir=in action=allow protocol=UDP localport=53 program="<exe>" profile=private remoteip=localsubnet`
  `netsh advfirewall firewall add rule name="Ghostline Setup" dir=in action=allow protocol=TCP localport=8053 program="<exe>" profile=private remoteip=localsubnet` (chỉ khi mở trang cài đặt, xoá khi đóng)
- Xoá theo tên, idempotent.
- Mạng Public → hiện cùng gợi ý như 2A. Không tự đổi profile mạng.

### 6.5 Khôi phục khi crash

- Watchdog, khôi phục lúc mở app và `--restore` chạy theo thứ tự: **gỡ CA phiên** (mọi thumbprint trong `certs.session`) → system proxy → xoá mọi luật firewall trong `firewall.rules` → DNS → engine vượt DPI (kể cả gỡ dịch vụ `WinDivert` như v0.3.0) → `clean` → **quét dọn** `Ghostline Fake SNI` (mục 5.4).
- CA phiên được gỡ đầu tiên vì đó là thứ nguy hiểm nhất nếu bị bỏ sót.
- Vẫn **idempotent** và chỉ chạy khi process chủ đã chết (pid + thời điểm khởi động).
- Gỡ CA phiên thất bại → giữ thumbprint trong `state.json`, thử lại ở lần khôi phục sau; app hiện `CERT_REMOVE_FAILED` kèm nút "Thử gỡ lại".
- **Gỡ cài đặt (NSIS):** `--restore` → `--remove-certs` → xoá dự phòng các luật `Ghostline Proxy`, `Ghostline DNS (TCP)`, `Ghostline DNS (UDP)`, `Ghostline Setup`.

### 6.6 `state.json` phiên bản 3

```json
{
  "version": 3,
  "phase": "clean | dns_set",
  "pid": 1234,
  "pidStartTime": "…",
  "startedAt": "…",
  "snapshot": [ … ],
  "dpi": { "running": false, "pid": 0 },
  "sysproxy": { … },
  "firewall": { "rules": ["Ghostline Proxy", "Ghostline DNS (TCP)", "Ghostline DNS (UDP)"] },
  "certs": { "session": ["<sha1 hex>"] }
}
```

- Đọc v1/v2: `firewall.rule` (chuỗi) chuyển thành `firewall.rules` một phần tử; `certs` rỗng. Ghi lại luôn dùng v3.
- `certs` và `firewall` chỉ được ghi khi `phase` đã là `dns_set`, như 2A.

## 7. DNS server

### 7.1 Server

- Listener `HTTPSListenAddr` / `UDPListenAddr` / `TCPListenAddr` của `dnsproxy` trong cùng instance với engine, nên rules, cache, thống kê và bảo đảm không rò DNS giữ nguyên.
- DoH: đường dẫn `/dns-query`, GET và POST (`application/dns-message`), HTTP/2 và HTTP/1.1, TLS ≥ 1.2.
- **Lọc nguồn:** chỉ nhận loopback, IP riêng (v4 RFC 1918, v6 `fc00::/7`) và link-local. Nguồn khác: trả `REFUSED`, không chuyển đi đâu (firewall đã chặn từ ngoài subnet).
- **Chống lạm dụng:** `Ratelimit` 100 truy vấn/giây mỗi IP, từ chối `ANY` (`REFUSED`).
- **IP LAN đổi:** health check 30 giây so tập IP; đổi thì chạy lại pha D (ký lại chứng chỉ lá).
- **Thống kê:** số truy vấn qua DoH và DNS LAN; số thiết bị LAN khác nhau trong 10 phút gần nhất. IP thiết bị chỉ hiện khi bật "hiện truy vấn" và chỉ trong RAM.

### 7.2 Đưa cho thiết bị khác

Thẻ "Dùng cho thiết bị khác" (trang DNS server) hiện:

- **IP DNS** (từng IP LAN): dùng cho router, TV, Android (đặt DNS tĩnh trong cài đặt Wi-Fi) và iOS (*Cài đặt › Wi-Fi › (i) › Định cấu hình DNS › Thủ công*).
- **URL DoH:** `https://<ip>/dns-query` (kèm cổng khi khác 443).
- **Vân tay SHA-256** của CA LAN.
- **Nút "Mở trang cài đặt cho điện thoại":** mở HTTP server tạm (`internal/dnsserver`) trên từng IP LAN, cổng `8053`, **tự đóng sau 10 phút** (có nút đóng sớm), cùng bộ lọc IP riêng, kèm luật firewall tạm `Ghostline Setup` (ghi vào `state.json`). App hiện mã QR (`internal/qr`) và đồng hồ đếm ngược. Trang có tiếng Việt và tiếng Anh (theo `Accept-Language`), gồm:
  - `ghostline-lan-ca.crt` (DER, `application/x-x509-ca-cert`) cho Windows, macOS, Android.
  - `ghostline.mobileconfig` cho iOS 14 trở lên (mục 7.3).
  - Vân tay CA để so với vân tay trên máy tính, phòng ai đó trong LAN tráo file.
  - Hướng dẫn: iOS bật "Tin cậy hoàn toàn" trong *Cài đặt › Cài đặt chung › Giới thiệu › Cài đặt tin cậy chứng chỉ*; Android tắt Private DNS và đặt DNS tĩnh; Steam Deck (SteamOS) đặt DNS thủ công **cho riêng mạng Wi-Fi nhà**. Mọi thiết bị đều dùng được DNS 53 mà không cài chứng chỉ; chứng chỉ chỉ cần cho DoH.
  - Trang chỉ phục vụ file công khai, không có form, không nhận dữ liệu.
- **Nút "Lưu file…":** xuất `.crt` và `.mobileconfig` ra đĩa.

### 7.3 `.mobileconfig`

- Payload `com.apple.security.root` chứa CA LAN.
- Payload `com.apple.dnsSettings.managed`: `DNSProtocol=HTTPS`, `ServerURL=https://dns.ghostline.lan[:port]/dns-query`, `ServerAddresses=[các IP LAN]`. Dùng tên miền thay vì IP vì iOS kiểm tra chứng chỉ theo tên ổn định hơn.
- **`OnDemandRules`:** `Connect` khi `SSIDMatch` là SSID đã chọn, còn lại `Disconnect`. Nhờ vậy ra khỏi mạng nhà, iPhone không cố dùng DNS server không tới được. SSID tự điền nếu máy đang dùng Wi-Fi (`winutil`), nếu không thì người dùng nhập; lưu ở `dnsServer.iosSsid`. Không có SSID thì nút tải `.mobileconfig` bị khoá kèm lời giải thích.
- Profile không ký (iOS hiện "Chưa xác minh"); trang hướng dẫn ghi rõ điều này.
- **Hạn chế đã biết, ghi trên trang hướng dẫn:** khi máy tính tắt hoặc Ghostline Disconnect trong lúc iPhone đang ở mạng nhà, iPhone có thể không phân giải được tên miền cho tới khi tắt profile trong *Cài đặt › VPN và quản lý thiết bị*. Hành vi quay về DNS thường của iOS phải được kiểm tra trên máy thật trong lúc làm, rồi cập nhật lời hướng dẫn cho đúng.

## 8. Fake SNI

### 8.1 Rule

- `sni=<tên>`: gửi SNI giả là `<tên>` (tên miền hợp lệ, chuẩn hoá IDN). `sni=none`: không gửi SNI.
- `connect=<domain>`: phân giải `<domain>` qua engine để lấy IP kết nối, thay cho IP của host thật. Chỉ có tác dụng ở proxy. Không đi kèm `ip=` (lỗi `RULES_PARSE`).
- `sni=` chỉ đi với mẫu `domain`, `=domain`, `*.domain`. Dùng với keyword, regexp hoặc CIDR → `RULES_PARSE` ("sni= chỉ dùng với mẫu domain"). Danh sách có dòng như vậy → dòng đó tính vào `skipped`.
- Đi kèm được `upstream=` và `fragment=`. Không đi kèm `block` (như 2A).
- `sni=` và `connect=` **không tác động DNS** (mục 7.5 của 2A).

Ví dụ:

```
youtube.com          sni=www.google.com connect=www.google.com
*.googlevideo.com    sni=www.google.com
vercel.com           sni=nextjs.org connect=nextjs.org
example.org          sni=none
```

### 8.2 Danh sách và preset

- **Định dạng `ghostline`:** cú pháp rule text (mục 7.1 của 2A), mỗi dòng có hành động riêng. Nhận diện bằng dòng đầu `# ghostline-rules v1`. Danh sách định dạng này không dùng trường `action` chung.
- **`trustedForSNI`:** `sni=` và `connect=` từ một danh sách **chỉ có hiệu lực** khi danh sách có `trustedForSNI: true`. Danh sách chưa được tin thì hai trường này bị bỏ qua (các hành động khác vẫn áp dụng), bảng danh sách ghi "có N rule Fake SNI bị bỏ qua".
  - Danh sách preset chính thức: `trustedForSNI` bật sẵn, nhưng **bắt buộc có chữ ký hợp lệ** bằng cùng khoá công khai với `servers.json`. Chữ ký sai hoặc thiếu → giữ bản cũ đã kiểm tra, `LIST_SIGNATURE_INVALID{id}`.
  - Danh sách khác: người dùng tự bật `trustedForSNI`, có hộp xác nhận liệt kê số domain và cảnh báo rằng danh sách này quyết định SNI giả và IP kết nối cho những domain đó.
  - Lý do: một danh sách độc hại có thể đặt `sni=` và `connect=` trỏ tới server của kẻ tấn công có chứng chỉ hợp lệ cho tên SNI giả. Khi đó việc kiểm tra chứng chỉ vẫn qua, và nội dung đã giải mã sẽ đi tới kẻ tấn công.
- **Preset:** nhóm "Fake SNI" trong catalog thêm nhanh, mỗi nhóm một danh sách, tải từ thư mục `lists/fakesni/` của repo Ghostline (raw GitHub, dự phòng jsDelivr), cập nhật 24 giờ một lần. Bản dự phòng được nhúng trong binary và dùng khi chưa tải được lần nào.
- **Nội dung preset:** bản đầu dự kiến các nhóm Google/YouTube, Fastly, Vercel, Netlify (theo kết quả khảo sát tháng 5/2026). **Một nhóm chỉ được đưa vào bản phát hành nếu đã kiểm tra là chạy được** lúc làm. Plan có một bước thử riêng để chọn nội dung; `release-checklist.md` có mục kiểm tra lại trước mỗi bản phát hành.

### 8.3 Luồng một kết nối

Nhánh mới trong `tunnel`, sau khi đã đọc ClientHello (2A đã đọc cho fragment) và trước bước fragment:

1. **Chọn nhánh.** Lấy SNI trong ClientHello. Không có SNI, SNI là IP, hoặc ClientHello có ECH (SNI ngoài là tên công khai của CDN) → đi đường 2A. Ngược lại khớp rules theo SNI; quyết định có `SNI` và `Issuer != nil` → nhánh MITM.
2. **Kết nối tới server trước.** Phân giải: `connect=` nếu có, nếu không thì như 2A (`ip=`, engine). Chống SSRF, đi thẳng hoặc qua upstream như 2A. Rule có `fragment=on` thì ClientHello giả cũng được cắt; mặc định không cắt.
3. **Bắt tay TLS với server:** `tls.Client` với `ServerName` = SNI giả (rỗng khi `sni=none`), TLS ≥ 1.2, `NextProtos` = danh sách ALPN client đã chào (đọc bằng `tlsfrag.ALPN`), `InsecureSkipVerify` + `VerifyConnection` tự viết:
   - chuỗi chứng chỉ phải hợp lệ với kho gốc hệ thống (`x509.SystemCertPool`, EKU `serverAuth`, thời gian hiện tại);
   - tên phải khớp **SNI giả hoặc host thật**; `sni=none` chỉ chấp nhận host thật;
   - **không bao giờ** chấp nhận chứng chỉ không qua bước này.
   Thời gian tối đa 10 giây.
4. **Server lỗi** (không kết nối được, alert, kiểm tra chứng chỉ thất bại, hết giờ) → ghi sự kiện `fakesni_server_rejected` hoặc `fakesni_verify_failed`, đóng kết nối server, **quay về đường 2A** với ClientHello gốc đã đệm. An toàn vì ở đường 2A trình duyệt tự kiểm tra chứng chỉ đầu-cuối, và lúc này chưa có byte nào được gửi cho client.
5. **Bắt tay TLS với client:** `tls.Server` trên conn phát lại ClientHello đã đệm, `GetCertificate` lấy chứng chỉ lá cho SNI từ `Issuer`, `NextProtos` chỉ gồm giao thức server đã chọn (rỗng nếu server không chọn). Thời gian tối đa 10 giây. Client từ chối → đóng cả hai, đếm `fakesniClientRejected` (không quay về được vì đã trả lời client).
6. **Relay** hai chiều bằng `relay` sẵn có trên hai `tls.Conn`: giữ nguyên timeout nghỉ 5 phút, thống kê byte và đóng nửa chiều (`CloseWrite`).

- **Không lưu nội dung đã giải mã**, không ghi log URL, header hay body. Sự kiện chỉ có domain (khi bật "hiện truy vấn"), kết quả và số byte, như thống kê 2A.
- Kết quả mới trong `dialer.Outcome`: `fakesni`, `fakesni_fallback`, `fakesni_client_rejected`.

## 9. Giao diện

### 9.1 Chế độ Nâng cao

Thanh bên: Tổng quan · Máy chủ · Vượt DPI · Proxy · Rules · **DNS server** · **Fake SNI** · Nhật ký · Cài đặt.

- **DNS server**
  - Công tắc: Bật DoH cục bộ · Chia sẻ cho LAN · ô Cổng DoH.
  - Danh sách địa chỉ đang nghe; IP bị bỏ qua có ghi chú lý do.
  - Thẻ "Dùng cho thiết bị khác" (mục 7.2), ô SSID cho iOS.
  - Thẻ CA LAN: vân tay, hạn dùng, "đã cài trên máy này", nút "Tạo lại" và "Gỡ" (có xác nhận).
  - Thống kê (mục 7.1). Gợi ý khi mạng là Public.
- **Fake SNI**
  - **Lần đầu mở là trang cảnh báo bắt buộc:** Fake SNI làm gì; Ghostline thấy nội dung HTTPS đã giải mã của các domain trong rule (chỉ trong RAM, không lưu); app có pinning sẽ lỗi; **không dùng cho ngân hàng hay tài khoản quan trọng**; CA chỉ ký được cho đúng các domain đó và bị gỡ khi ngắt; chỉ áp dụng cho trình duyệt trên máy này. Người dùng phải cuộn hết trang và tích "Tôi đã hiểu" thì nút "Tiếp tục" mới bật. Lần xác nhận lưu ở `fakeSni.ackVersion`; khi nội dung cảnh báo đổi, số phiên bản tăng và phải đọc lại.
  - Sau khi xác nhận: công tắc chính `fakeSni.enabled` (mặc định tắt); "Cần bật Proxy" kèm nút bật nhanh khi proxy tắt; các nhóm preset bật/tắt từng nhóm; bảng rule `sni=` đang có hiệu lực (bấm để sang Rules đã lọc); thẻ CA phiên (thumbprint, số domain, hạn dùng); bộ đếm `fakesni`, `fallback`, `clientRejected`, `verifyFailed`; ghi chú Firefox.
- **Rules:** bỏ nhãn "cần giai đoạn 2B"; bảng thêm cột SNI và Connect; ô "Thử tên miền" hiện "sẽ giải mã bằng Fake SNI (sni=…, connect=…)" hoặc lý do không áp dụng (Fake SNI tắt, danh sách chưa được tin…). Bảng danh sách thêm cột/cờ "Tin cho Fake SNI" và trạng thái chữ ký.
- **Cài đặt:** mục "Chứng chỉ" liệt kê mọi chứng chỉ Ghostline trong `LocalMachine\Root` (subject, thumbprint, hạn dùng, loại), nút "Gỡ tất cả chứng chỉ Ghostline" (tắt pha D và pha S trước nếu đang chạy, có xác nhận).
- **Tổng quan:** thẻ DNS server (địa chỉ, số thiết bị) và thẻ Fake SNI (bật/tắt, số domain).

### 9.2 Banner Fake SNI

Khi pha S đang chạy, **mọi trang ở cả hai chế độ** có banner màu tím ở trên cùng, không đóng được: "Fake SNI đang bật — Ghostline đang giải mã HTTPS của N tên miền", link "Xem" (sang trang Fake SNI) và nút "Tắt". Tooltip khay thêm dòng "Fake SNI: bật".

### 9.3 Chế độ Đơn giản và khay

- Không thêm công tắc nào. Bảng thông số thêm "DNS cho LAN: 192.168.1.5" khi pha D chạy với `shareLan`.
- Pha D hoặc pha S lỗi → banner cam SUY_GIẢM như 2A.
- Menu khay không đổi.

### 9.4 Frontend

- Trang mới `modes/advanced/pages/{dnsserver,fakesni}/`, component `FakeSniBanner`, `FakeSniWarning`, `CertList`.
- Sự kiện Wails mới: `dnsserver:stats`, `fakesni:stats`, `certs:changed`, `setup:countdown`.
- Toàn bộ chuỗi mới có trong `vi.json` và `en.json` (test parity hiện có). Nội dung trang cài đặt cho điện thoại nằm phía Go (`internal/dnsserver`), có bản tiếng Việt và tiếng Anh.

## 10. Cài đặt (`settings.json`, phiên bản 4)

Thêm vào phiên bản 3 (v3 là bản của engine zapret2):

```json
{
  "version": 4,
  "dnsServer": {
    "enabled": false,
    "shareLan": false,
    "dohPort": 443,
    "iosSsid": ""
  },
  "fakeSni": {
    "enabled": false,
    "ackVersion": 0
  }
}
```

- Nâng cấp từ v3 (và từ v1/v2 qua các bước cũ): thêm các khối mặc định, giữ nguyên mọi trường cũ, ghi lại thành v4.
- Kiểm tra khi lưu: `dohPort` 1–65535, khác 53, khác `8053` và khác `proxy.port`; `iosSsid` ≤ 32 byte UTF-8.
- `rules.json` v1 thêm trường tuỳ chọn cho danh sách: `trustedForSNI`, `signed`, `signatureOk`. Thiếu thì coi là `false`; không đổi số phiên bản.

## 11. Xử lý lỗi

| Mã | Khi nào | Hành động gợi ý |
|---|---|---|
| `DNSSERVER_PORT_IN_USE{port,addrs}` | Không bind được cổng DoH/53 | Hiện process đang giữ cổng · Đổi cổng DoH · Thử lại |
| `DNSSERVER_FIREWALL{detail}` | Không tạo được luật firewall DNS | Thử lại · Tắt chia sẻ LAN |
| `CERT_INSTALL_FAILED{kind}` | Cài CA vào Root lỗi hoặc đọc lại không thấy | Thử lại · Mở nhật ký |
| `CERT_REMOVE_FAILED{thumbprint}` | Gỡ CA lỗi | Cảnh báo cố định · "Thử gỡ lại" · hướng dẫn gỡ tay bằng `certlm.msc` |
| `CERT_KEY_UNREADABLE` | `lan-ca.key` không giải mã được DPAPI (đổi máy, hỏng file) | "Tạo lại CA LAN" |
| `FAKESNI_NEEDS_PROXY` | Bật Fake SNI khi proxy tắt | Bật proxy |
| `FAKESNI_TOO_MANY{count}` | Hơn 1000 domain `sni=` | Tắt bớt nhóm preset hoặc danh sách |
| `FAKESNI_SELFTEST_FAILED` | S4 không đạt | Thử lại · Mở nhật ký |
| `LIST_SIGNATURE_INVALID{id}` | Chữ ký danh sách sai hoặc thiếu | Giữ bản cũ, ghi chú trong bảng |
| `SETUP_PAGE_FAILED{detail}` | Không mở được trang cài đặt cổng 8053 | Dùng "Lưu file…" |

- Panic trong goroutine MITM của một kết nối: bắt lại, ghi log kèm stack trace, chỉ đóng kết nối đó.
- Panic trong listener DoH/DNS LAN: xử lý như pha D chết (chạy lại một lần, rồi SUY_GIẢM).

## 12. Kiểm thử

Phát triển theo TDD. Test cần Windows thật gắn build tag `integration`.

| Phần | Test |
|---|---|
| `certs` | Chứng chỉ lá cho domain trong danh sách qua `x509.Verify`; domain ngoài danh sách, IP công khai, IP bất kỳ (CA phiên) **bị từ chối**. CA LAN: IP riêng và `dns.ghostline.lan` qua, IP công khai và domain khác bị từ chối. EKU khác `serverAuth` bị từ chối. Hạn dùng, `MaxPathLen`. Cache LRU. `.mobileconfig` là plist hợp lệ, có `OnDemandRules` đúng SSID |
| `certstore` | `API` giả: cài → đọc lại → gỡ; gỡ khi không có là thành công; quét dọn theo tiền tố chỉ gỡ đúng loại và chừa phiên đang chạy. `integration`: vòng thật trên `CurrentUser\Root` của máy test |
| `proxy/mitm` | Server TLS giả với CA test: SNI giả được gửi đi, `sni=none` không gửi SNI; ALPN h2 và http/1.1 khớp hai phía; chứng chỉ server sai tên, hết hạn hoặc không chain tới gốc → `verify_failed` và quay về đường 2A; server alert → quay về; client không tin CA → `clientRejected`; ECH và SNI là IP → không MITM. Fuzz `tlsfrag.ALPN` |
| `proxy` (đầu cuối) | `http.Client` thật qua proxy tới server `httptest` TLS với Fake SNI, cả HTTP/1.1 và HTTP/2; `Resolver` giả có **bẫy** (không được gọi DNS hệ thống), cả khi dùng `connect=` |
| `rules` | `sni=`/`connect=` parse, lỗi với keyword/regexp/CIDR, `connect=` với `ip=` lỗi; `SNIDomains()` chuyển mẫu đúng; định dạng `ghostline`; `trustedForSNI` tắt thì bỏ qua `sni=`/`connect=` nhưng giữ hành động khác; chữ ký đúng/sai/thiếu |
| `engine` | DoH GET/POST trả lời đúng; nguồn IP công khai bị bỏ; rate limit; `ANY` bị từ chối; DNS 53 trên IP LAN vẫn đi qua resolver giả có bẫy (**không rò**); chứng chỉ được thay nóng |
| `dnsserver` | Chọn IP LAN, bỏ qua IP bind lỗi; trang cài đặt tự đóng sau 10 phút (đồng hồ giả), chỉ nhận IP riêng, đúng `Content-Type` |
| `app` | Pha D và pha S lỗi ở từng bước → đúng undo, DNS vẫn được bảo vệ, SUY_GIẢM đúng lý do. S2 trước S3 (thumbprint luôn có trong `state.json` trước khi cài). Xoay CA: thứ tự 5 bước, lỗi cài giữ CA cũ. Thứ tự Disconnect mới. Chạy lại pha P kéo theo pha S |
| `watchdog` / `store` | Thứ tự khôi phục mới (CA phiên đầu tiên); `CERT_REMOVE_FAILED` giữ thumbprint; nâng `state.json` v2→v3 (`firewall.rule` → `rules`) và `settings.json` v3→v4; kill process giữa S2 và S3 → không còn gì sót |
| Frontend | Trang cảnh báo bắt buộc (nút chỉ bật khi đã cuộn hết và tích), `ackVersion`; banner ở cả hai chế độ; trang DNS server (QR, đếm ngược, Public); parity `vi.json`/`en.json` |
| Kiểm tra thủ công trước phát hành | iPhone cài `.mobileconfig`, DoH hoạt động ở mạng nhà, ra 4G vẫn có mạng; hành vi khi Ghostline Disconnect. Android đặt DNS tĩnh. Steam Deck đặt DNS thủ công (xác định đặt được ở Game Mode hay phải sang Desktop mode, cập nhật hướng dẫn cho đúng). Router/TV đặt DNS. Từng nhóm preset Fake SNI mở được trang khi vượt DPI (cả hai engine) tắt. `certlm.msc` không còn `Ghostline Fake SNI` sau Disconnect, `taskkill /F`, khởi động lại máy và gỡ app; không còn `Ghostline LAN CA` sau gỡ app. Chrome, Edge, Firefox với Fake SNI. Wireshark: không có DNS plain do DoH server, DNS LAN hay Fake SNI gây ra. |

## 13. Đóng gói và tài liệu

- Không thêm dependency Go nào cho lõi, không thêm file nhị phân nào.
- Thư mục `lists/fakesni/` trong repo chứa preset và file `.sig`; ký bằng `go run ./tools/genservers -sign-file <file> -sign-env SERVERLIST_SIGNING_KEY` như `servers.json`.
- Trình gỡ cài đặt NSIS: thêm `--remove-certs` và xoá dự phòng các luật firewall mới.
- README (EN + VI) và hướng dẫn sử dụng: mục DNS server (máy này, LAN, iOS, Android, router), Fake SNI (cảnh báo, giới hạn chỉ máy này, Firefox), gỡ chứng chỉ bằng tay.
- `release-checklist.md`: thêm các mục kiểm tra thủ công ở mục 12 và bước xác nhận lại nội dung preset.
- Phát hành dưới dạng **v0.4.0**.

## 14. Cấu trúc repo (thêm)

```
internal/
  certs/            ca.go, leaf.go, constraints.go, mobileconfig.go
  certstore/        store.go, api_windows.go
  dnsserver/        listen.go, filter.go, setuppage.go, setuppage_{vi,en}.html
  proxy/mitm/       mitm.go, verify.go
lists/fakesni/      google.txt, fastly.txt, … kèm .sig (ký bằng `tools/genservers -sign-file`)
frontend/src/modes/advanced/pages/{dnsserver,fakesni}/
```

## 15. Rủi ro và cách giảm thiểu

| Rủi ro | Giảm thiểu |
|---|---|
| Lộ khoá CA cho phép giả mạo trang web với máy này | CA phiên: khoá chỉ trong RAM, Name Constraints chỉ các domain `sni=`, cấm IP, EKU `serverAuth`. CA LAN: chỉ IP riêng và `ghostline.lan`, khoá DPAPI phạm vi máy, file chỉ Administrators/SYSTEM |
| CA phiên sót lại trong Root sau crash | Ghi thumbprint trước khi cài; gỡ đầu tiên ở cả 4 lớp khôi phục; quét dọn theo tiền tố subject; gỡ khi gỡ app; cảnh báo cố định khi gỡ lỗi |
| Danh sách độc hại chuyển hướng lưu lượng đã giải mã tới server kẻ tấn công | `sni=`/`connect=` từ danh sách chỉ có hiệu lực khi `trustedForSNI`; preset chính thức bắt buộc có chữ ký; danh sách khác cần người dùng xác nhận |
| Fake SNI làm hỏng ứng dụng có pinning hoặc trang nhạy cảm | Chỉ giải mã domain có rule; cảnh báo bắt buộc; banner cố định; Fake SNI chỉ cho proxy (ứng dụng không qua proxy không bị ảnh hưởng) |
| Server/CDN đổi chính sách fronting, preset hết tác dụng | Quay về đường 2A tự động; preset cập nhật qua mạng; kiểm tra preset ở mỗi bản phát hành; bộ đếm `fallback` cho người dùng thấy |
| Kiểm tra chứng chỉ server quá lỏng | Luôn chain tới kho gốc hệ thống; tên chỉ được là SNI giả hoặc host thật; `sni=none` chỉ host thật; không bao giờ `InsecureSkipVerify` không kèm kiểm tra |
| DNS 53 cho LAN bị dùng để khuếch đại hoặc truy vấn từ ngoài | Chỉ nghe IP LAN; lọc nguồn IP riêng; firewall chỉ Private và `localsubnet`; rate limit; từ chối `ANY` |
| iPhone mất DNS khi rời mạng nhà hoặc khi máy tính tắt | `OnDemandRules` theo SSID; hướng dẫn tắt profile; kiểm tra trên máy thật trước phát hành |
| Ai đó trong LAN tráo file CA ở trang cài đặt | Trang tự đóng sau 10 phút; vân tay hiện trên cả máy tính và trang để so; tuỳ chọn "Lưu file…" |
| Xung đột cổng 443/53 với phần mềm khác (IIS, ICS, Hyper-V) | Nghe từng IP cụ thể; bỏ qua IP lỗi; đổi được cổng DoH; báo process đang giữ cổng |
