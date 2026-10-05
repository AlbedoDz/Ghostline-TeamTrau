# Danh sách kiểm tra trước khi phát hành

Chạy trên Windows 11 x64, terminal **admin**. Đánh dấu từng mục; mục nào hỏng thì không phát hành.

## Test tự động

- [ ] `go test ./...` và `cd frontend && npm test` xanh
- [ ] `go test -tags integration ./internal/sysdns/... ./internal/startup/... ./internal/sysproxy/...` xanh (admin)

## Kiểm tra thủ công (spec §11)

- [ ] **Máy Windows 11 vừa cài mới**: tải zip portable, chạy, bấm Connect → **Đã bảo vệ ≤ 25 giây**. Ngắt rồi Connect lại → **≤ 5 giây** (dùng kết quả quét đã lưu).
- [ ] **Connect / Disconnect**: sau Disconnect, `Get-DnsClientServerAddress` cho mọi card mạng trả về đúng DNS ban đầu (DHCP hay tĩnh).
- [ ] **App bị kill**: khi đang kết nối chạy `taskkill /F /IM ghostline.exe`; trong **≤ 3 giây** DNS trở về như cũ (watchdog). Đo bằng `Get-DnsClientServerAddress` lặp mỗi 0,5 giây.
- [ ] **Khởi động lại máy khi đang kết nối** (giữ nút nguồn): sau khi đăng nhập, DNS được khôi phục dù không mở Ghostline (tác vụ `Ghostline Recovery`).
- [ ] **Cổng 53 bị ICS chiếm**: bật Mobile Hotspot → Connect báo `PORT53_BUSY` kèm tên dịch vụ; nút "Tạm dừng dịch vụ" hỏi xác nhận trong trang; không có gì bị dừng khi chưa xác nhận.
- [ ] **Đổi Wi-Fi sang dây mạng** khi đang kết nối: card mới được chụp DNS và đặt về loopback; Disconnect khôi phục cả hai card.
- [ ] **Máy ngủ rồi thức** khi đang kết nối: DNS vẫn hoạt động; nếu engine hỏng, nhật ký có "đang tìm máy chủ khác".
- [ ] **Wireshark** với filter `udp.port==53 || tcp.port==53` trên card mạng thật: khi đã bảo vệ, không có truy vấn DNS plain ra ngoài, trừ bootstrap tới 1.1.1.1/8.8.8.8 để phân giải hostname máy chủ DoH.
- [ ] **Vượt DPI**: bật GoodbyeDPI preset Nhẹ → chạy; tắt → service `WinDivert1.4` biến mất (`sc query WinDivert1.4`).
- [ ] **Installer**: cài, chạy, kết nối; gỡ cài đặt khi đang kết nối → DNS khôi phục, tác vụ `Ghostline` và `Ghostline Recovery` bị xoá, service `WinDivert1.4` bị xoá.
- [ ] **Ngôn ngữ**: chuyển VI ↔ EN, không còn chuỗi nào chưa dịch.

## Kiểm tra thủ công giai đoạn 2A (spec 2A §11)

- [ ] **Trình duyệt vào trang bị chặn theo SNI** qua system proxy (GoodbyeDPI tắt): bật proxy + "dùng cho máy này", Connect, mở trang → vào được; lần thứ hai không chậm thêm.
- [ ] **Điện thoại trong LAN** dùng proxy qua mã QR (mạng Private) → duyệt web được; trang Proxy hiện 1 thiết bị LAN.
- [ ] **Mạng Public** → trang Proxy hiện cảnh báo, điện thoại không vào được, Ghostline không tự đổi profile mạng.
- [ ] **`taskkill /F /IM ghostline.exe`** khi đang dùng system proxy + chia sẻ LAN → trong ≤ 3 giây system proxy trở về như cũ và luật `Ghostline Proxy` bị xoá (`netsh advfirewall firewall show rule name="Ghostline Proxy"` báo không có).
- [ ] **Khởi động lại máy** khi đang dùng system proxy → sau đăng nhập system proxy được khôi phục.
- [ ] **VPN đổi system proxy giữa chừng** → Ghostline báo `SYSPROXY_TAKEN_OVER`, không giành lại; Disconnect không ghi đè cài đặt của VPN.
- [ ] **Máy đã có proxy/PAC khác** → Connect hỏi xác nhận trước khi ghi đè; chọn "không dùng system proxy" thì proxy vẫn chạy.
- [ ] **Wireshark** (`udp.port==53 || tcp.port==53`) khi duyệt web qua proxy: không có DNS plain do proxy gây ra.
- [ ] **Import danh sách từ link GitHub** cho cả 6 mục "Thêm nhanh" → nhận đúng định dạng, có số mục, ô "Thử tên miền" chỉ đúng danh sách.
- [ ] **Gỡ cài đặt** khi đang chia sẻ LAN → luật `Ghostline Proxy` bị xoá.

## Kiểm tra thủ công engine zapret2 (spec zapret2 §10.2)

- [ ] **Cài mới, Connect:** các trang mẫu mở được bằng zapret2; YouTube trên Chrome chạy QUIC (kiểm tra `chrome://net-internals`) vẫn xem được.
- [ ] **Tự dò với zapret2** trên ít nhất hai nhà mạng (ví dụ Viettel, VNPT): tìm được chiến lược; ghi lại chiến lược thắng để chỉnh `lists/strategies.json`.
- [ ] **Nâng cấp từ v0.2.5:** engine là GoodbyeDPI 0.2.3rc3, preset cũ chạy được, dịch vụ `WinDivert1.4` cũ đã bị gỡ.
- [ ] **Đổi GoodbyeDPI → zapret2 → GoodbyeDPI khi đang kết nối:** `sc qc WinDivert` trỏ đúng file `.sys` của engine đang chạy.
- [ ] **Kill `ghostline.exe` bằng Task Manager:** `winws2` chết theo; dịch vụ được gỡ trong ≤ 3 giây.
- [ ] **Để Defender chặn `winws2.exe`** (bỏ loại trừ, giải nén lại): Connect quay về GoodbyeDPI, banner hướng dẫn hiện ra.
- [ ] **Phạm vi danh sách đen + tự phát hiện:** mở một trang bị chặn không có trong danh sách ba lần → domain hiện trong danh sách tự thêm, lần sau mở được.
- [ ] **Tự chạy một `winws2` ngoài Ghostline rồi Connect:** lỗi `DPI_START_FAILED` với gợi ý đúng.
- [ ] **Defender với bản build:** `ghostline.exe` (đã nhúng `winws2.exe`) không bị Defender xoá khi tải về và khi chạy.
- [ ] **Nâng zapret2:** `grep -n "_G\[\|load(" assets/zapret2/lua/*.lua` — mọi khoá tham số mới mà thư viện tra như tên hàm phải được thêm vào `funcKeys` trong `internal/dpi/zapret2/validate.go`.
- [ ] **Ký danh sách chiến lược** (tăng `version` mỗi lần đổi): `go run ./tools/genservers -sign-file lists/strategies.json -sign-env SERVERLIST_SIGNING_KEY`, commit cả `lists/strategies.json.sig`.
- [ ] **Gửi `winws2.exe` và `ghostline.exe`** lên https://www.microsoft.com/wdsi/filesubmission (báo nhầm); ghi lại mã gửi.

## Cần xác minh trên máy thật (reviewer không kiểm chứng được)

- [ ] **Luật firewall với đường dẫn có dấu cách và chữ có dấu** (`C:\Program Files\…`, `C:\Users\Đức Hạnh\…` cho bản portable): `netsh` tạo đúng luật `Ghostline Proxy` cho exe đó.

- [ ] **Tắt máy khi app đang ẩn ở khay**: Windows gửi `WM_QUERYENDSESSION` tới cửa sổ ẩn, và DNS được trả về trước khi tắt.
- [ ] **Khởi động cùng Windows** (`--autostart` qua Task Scheduler) rồi Connect: watchdog vẫn sống sau khi tác vụ kết thúc (thử `taskkill /F` → DNS khôi phục).
- [ ] **Windows 11 có cấu hình DoH riêng cho từng card mạng**: sau Disconnect, cấu hình DoH ban đầu vẫn còn.
- [ ] **Danh sách đen GoodbyeDPI và zapret2 khi tên người dùng có dấu** (ví dụ `C:\Users\Đức Hạnh`): phạm vi "danh sách đen" hoạt động.
- [ ] **Nhật ký → lưu file** trong WebView2 tải được file `ghostline-log.txt`.
