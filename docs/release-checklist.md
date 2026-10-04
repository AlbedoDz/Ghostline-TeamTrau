# Danh sách kiểm tra trước khi phát hành

Chạy trên Windows 11 x64, terminal **admin**. Đánh dấu từng mục; mục nào hỏng thì không phát hành.

## Test tự động

- [ ] `go test ./...` và `cd frontend && npm test` xanh
- [ ] `go test -tags integration ./internal/sysdns/... ./internal/startup/...` xanh (admin)

## Kiểm tra thủ công (spec §11)

- [ ] **Máy Windows 11 vừa cài mới**: tải zip portable, chạy, bấm Connect → **Đã bảo vệ ≤ 25 giây**. Ngắt rồi Connect lại → **≤ 5 giây** (dùng kết quả quét đã lưu).
- [ ] **Connect / Disconnect**: sau Disconnect, `Get-DnsClientServerAddress` cho mọi card mạng trả về đúng DNS ban đầu (DHCP hay tĩnh).
- [ ] **App bị kill**: khi đang kết nối chạy `taskkill /F /IM ghostline.exe`; trong **≤ 3 giây** DNS trở về như cũ (watchdog). Đo bằng `Get-DnsClientServerAddress` lặp mỗi 0,5 giây.
- [ ] **Khởi động lại máy khi đang kết nối** (giữ nút nguồn): sau khi đăng nhập, DNS được khôi phục dù không mở Ghostline (tác vụ `Ghostline Recovery`).
- [ ] **Cổng 53 bị ICS chiếm**: bật Mobile Hotspot → Connect báo `PORT53_BUSY` kèm tên dịch vụ; nút "Tạm dừng dịch vụ" hỏi xác nhận trong trang; không có gì bị dừng khi chưa xác nhận.
- [ ] **Đổi Wi-Fi sang dây mạng** khi đang kết nối: card mới được chụp DNS và đặt về loopback; Disconnect khôi phục cả hai card.
- [ ] **Máy ngủ rồi thức** khi đang kết nối: DNS vẫn hoạt động; nếu engine hỏng, nhật ký có "đang tìm máy chủ khác".
- [ ] **Wireshark** với filter `udp.port==53 || tcp.port==53` trên card mạng thật: khi đã bảo vệ, không có truy vấn DNS plain ra ngoài, trừ bootstrap tới 1.1.1.1/8.8.8.8 để phân giải hostname máy chủ DoH.
- [ ] **Vượt DPI**: bật GoodbyeDPI preset Nhẹ → chạy; tắt → service `WinDivert` biến mất (`sc query WinDivert`).
- [ ] **Installer**: cài, chạy, kết nối; gỡ cài đặt khi đang kết nối → DNS khôi phục, tác vụ `Ghostline` và `Ghostline Recovery` bị xoá, service `WinDivert` bị xoá.
- [ ] **Ngôn ngữ**: chuyển VI ↔ EN, không còn chuỗi nào chưa dịch.

## Cần xác minh trên máy thật (reviewer không kiểm chứng được)

- [ ] **Tắt máy khi app đang ẩn ở khay**: Windows gửi `WM_QUERYENDSESSION` tới cửa sổ ẩn, và DNS được trả về trước khi tắt.
- [ ] **Khởi động cùng Windows** (`--autostart` qua Task Scheduler) rồi Connect: watchdog vẫn sống sau khi tác vụ kết thúc (thử `taskkill /F` → DNS khôi phục).
- [ ] **Windows 11 có cấu hình DoH riêng cho từng card mạng**: sau Disconnect, cấu hình DoH ban đầu vẫn còn.
- [ ] **Danh sách đen GoodbyeDPI khi tên người dùng có dấu** (ví dụ `C:\Users\Đức Hạnh`): phạm vi "danh sách đen" hoạt động.
- [ ] **Nhật ký → lưu file** trong WebView2 tải được file `ghostline-log.txt`.
