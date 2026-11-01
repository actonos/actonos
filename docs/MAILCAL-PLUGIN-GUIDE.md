# Viết plugin nhà cung cấp thư và lịch

Hướng dẫn này dành cho người muốn thêm một nhà cung cấp mới (một máy chủ IMAP riêng, một API thư khác, một dịch vụ lịch khác) vào ActonOS. Mục tiêu của thiết kế là: **thêm nhà cung cấp chỉ cần thêm một thư mục plugin, không sửa tệp nào của lõi.** Plugin `echo-mail` trong `internal/mailcal/plugins/echomail/` được viết đúng theo hướng dẫn này để chứng minh điều đó: nó là thư mục duy nhất thay đổi, và các bài kiểm tra (`TestEveryShippedPluginAsksForOnlyWhatAProviderNeeds`, `TestEveryShippedPluginLoadsAndCertifies`) tự nhận ra nó mà không cần khai báo ở đâu cả.

Đặc tả đầy đủ: `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md`. Cách cài và cấu hình các plugin có sẵn: `docs/PLUGINS-MAILCAL.md`.

## 1. Ranh giới trách nhiệm

| Lõi ActonOS quyết định | Plugin thực hiện |
| --- | --- |
| Có được gửi hay không (công tắc tổng, quy tắc, hạn mức, duyệt của người) | Dịch một thao tác sang API của nhà cung cấp |
| Chữ ký, người nhận, tệp đính kèm của thư đi (bản nháp bất biến) | Gửi **đúng** thứ nhận được, không thêm không bớt |
| Làm mới token OAuth, giữ mật khẩu trong két | Dùng token hoặc mật khẩu được cấp cho từng lần gọi |
| Chặn địa chỉ nội bộ, giới hạn đích mạng | Chỉ nói chuyện với máy chủ được liệt kê trong manifest |

Hệ quả cần nhớ: **plugin luôn bị coi là mã không tin cậy.** Nó chạy trong hộp cát WASM, không có hệ tệp, không có mạng ngoài các đích manifest cho phép, và mọi lần gửi thư đều đi qua cổng của lõi trước khi tới plugin. Nếu bạn thấy mình cần plugin "tự kiểm tra quyền", hãy dừng lại: việc đó thuộc về lõi.

## 2. Plugin tối thiểu: `echo-mail`

Một nhà cung cấp chỉ đọc, lấy thư từ cấu hình của tài khoản. Ba tệp là đủ.

**`manifest.json`**

```json
{
  "id": "mailcal-echo-mail",
  "name": "Echo mail (example)",
  "version": "1.0.0",
  "description": "Example read-only provider.",
  "capabilities": ["mail@1"],
  "permissions": {}
}
```

`capabilities` là `mail@1`, `calendar@1` hoặc cả hai. `permissions` rỗng nghĩa là plugin không xin gì cả; đó là điểm xuất phát đúng.

**`echomail.go`** — cài đặt `provkit.Backend`. Nhúng `provkit.Base` để mọi thao tác bạn không làm trả lời `unsupported`, rồi ghi đè những thao tác bạn làm:

```go
type Backend struct{ provkit.Base }

func (Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI: []string{abi.CapMail},
		Ops: []string{abi.ExportCapabilities, abi.ExportMailList, abi.ExportMailSearch,
			abi.ExportMailGet, abi.ExportMailGetThread},
	}, nil
}

func (Backend) MailGet(_ context.Context, acct abi.AccountRef, id string) (abi.Message, error) {
	// ... tìm trong acct.Config["messages"]
	return abi.Message{}, abi.Errorf(abi.ErrNotFound, "no message %q", id)
}
```

Danh sách `Ops` phải khớp **chính xác** với những gì bạn cài đặt: bộ kiểm tra tuân thủ gọi thử từng thao tác không khai báo và đòi nó trả `unsupported`.

**`wasm/main.go`** — chỉ nối các export của ABI vào `Backend`; mỗi export một dòng:

```go
//go:build wasip1

package main

var backend = echomail.Backend{}

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailList, p, l) }
```

Cần thêm `acton_alloc`, `acton_free`, `acton_plugin_init` và `acton_mailcal_capabilities`; sao chép từ `internal/mailcal/plugins/echomail/wasm/main.go`.

Xây:

```bash
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared \
  -o plugin.wasm ./internal/mailcal/plugins/echomail/wasm
```

## 3. Hợp đồng ABI

- Mỗi export có chữ ký `(reqPtr, reqLen uint32) -> uint64`; kết quả đóng gói `(resPtr<<32 | resLen)`; cả hai vùng đệm là JSON. `provkit` và `wasmguest.Serve` lo toàn bộ phần này, bạn chỉ làm việc với kiểu Go trong `internal/mailcal/abi`.
- ABI chỉ được **thêm**, không đổi tên, không bỏ, không đổi nghĩa (`TestGoldenV1` canh điều này). Plugin viết cho `mail@1` sẽ chạy được với mọi lõi sau này hỗ trợ `mail@1`.
- Lỗi luôn có mã: `auth_expired`, `rate_limited` (kèm `RetryAfterMS`), `not_found`, `unsupported`, `transient`, `invalid`. Một lỗi không có mã là lỗi của plugin. `auth_expired` làm lõi tạm dừng tài khoản và yêu cầu người dùng đăng nhập lại, nên chỉ dùng khi thông tin đăng nhập thật sự hỏng.
- `MailList` trả thư **không có thân**; thân chỉ có ở `MailGet`. Nội dung thư là dữ liệu của người khác: lõi sẽ đóng khung nó là nội dung không tin cậy trước khi đưa cho agent, nhưng plugin không được diễn giải nó như lệnh.
- `Limits` là trần của nhà cung cấp (số người nhận, kích thước thân). Lõi áp dụng trước khi gọi; `0` là "không nêu giới hạn".

### Thao tác gửi (nếu bạn cài `acton_mail_send`)

Đây là phần dễ sai nhất, và là phần duy nhất có hậu quả không đảo ngược được.

1. **Gửi đúng `MailSendParams`**, không đổi người nhận, tiêu đề hay nội dung. Lõi đã duyệt chính bản này.
2. **`IdempotencyKey`**: gửi lại cùng khóa không được gửi hai lần. Nếu nhà cung cấp có khóa chống trùng, hãy dùng; nếu không, tìm thư đã gửi theo khóa (Gmail dùng tiêu đề `X-Acton-Idempotency-Key`) và trả `Replayed: true`. Nếu không thể bảo đảm "đúng một lần", hãy bảo đảm "nhiều nhất một lần" và báo rõ khi không biết thư đã đi chưa, như plugin IMAP/SMTP làm. **Không bao giờ tự thử lại một lần gửi không rõ kết quả.**
3. **`Bcc`**: người nhận Bcc không được lộ trong tiêu đề người khác nhận được.
4. Mọi lỗi mạng trong lúc gửi phải trả `transient` kèm thông điệp trung thực, không giả vờ thành công.

## 4. Manifest và quyền

Kiểm tra tự động (`allplugins_test.go`) áp dụng cho mọi plugin trong `internal/mailcal/plugins/*/manifest.json`:

- Không có `*` trần trong `net_outbound` hoặc `net_tcp`; chỉ cho phép dạng `*.ten-mien` khi thật cần.
- Nhà cung cấp **không xin** `secrets`, `storage`, `workspace`, `bus_events`. Token và mật khẩu đến qua các module chủ riêng, không qua `secrets`.
- Đăng nhập OAuth: khối `oauth` (`provider`, `auth_url`, `token_url`, `scopes`) và `permissions.oauth: ["<provider>"]` phải khớp nhau. Lõi chạy toàn bộ luồng (PKCE, `state` dùng một lần) và giữ refresh token; plugin chỉ nhận access token ngắn hạn qua `wasmguest.Tokens{}`. Xin phạm vi (scope) hẹp nhất; không xin phạm vi cho phép xóa vĩnh viễn hộp thư.
- Đăng nhập bằng mật khẩu ứng dụng: `permissions.credentials: true`; plugin nhận tên đăng nhập và mật khẩu qua `wasmguest.Credentials{}` cho từng lần gọi. Không được dùng cả OAuth lẫn mật khẩu trong một plugin.
- Mạng: HTTPS qua `wasmguest.HTTP{}` tới máy chủ trong `net_outbound`; kết nối TLS thô (IMAP, SMTP) qua `net_tcp` với dạng `host:port`. Lõi phân giải tên một lần và từ chối địa chỉ nội bộ/loopback, nên không thể dùng plugin để chạm vào mạng LAN.

Nếu nhà cung cấp cần máy chủ do người dùng chọn (như IMAP), hãy giao manifest với danh sách rỗng và nói rõ trong tài liệu rằng người dùng sao chép thư mục, thêm máy chủ của mình và cài bản không ký, như plugin IMAP/SMTP.

## 5. Kiểm thử và chứng nhận

Viết bài kiểm tra Go thuần cho `Backend` (xem `echomail_test.go`); `provkit/netio` có cài đặt `Stdlib` để chạy mã mạng nguyên bản với máy chủ giả (`httptest`, `imapmemserver`, `go-smtp`) mà không cần biên dịch sang WASM.

Rồi chạy bộ kiểm tra tuân thủ trên chính tệp `.wasm` đã biên dịch:

```bash
go run ./cmd/mailcal-conformance \
  --wasm build/plugins/mailcal-echo-mail/plugin.wasm \
  --manifest build/plugins/mailcal-echo-mail/manifest.json
```

Mặc định lệnh chạy **ngoại tuyến**: không token, không mạng; mọi thao tác cần tài khoản phải trả lỗi có mã, không treo, không sập. Kết quả là `conformance.json` đặt cạnh `plugin.wasm`, chứa mã băm SHA-256 của đúng bản dựng đó. Đổi một byte là chứng nhận hết khớp. Nếu plugin của bạn dựa trên `refstore`, thêm `--seed reference` để chạy cả bộ kiểm tra hành vi.

`make conformance` làm việc này cho bốn plugin đi kèm.

Với tài khoản thử thật, người dùng (không phải CI) có thể chạy thêm `--live`; thông tin đăng nhập chỉ đi qua biến môi trường `MAILCAL_PLUGIN_TOKEN` hoặc `MAILCAL_PLUGIN_USERNAME` và `MAILCAL_PLUGIN_PASSWORD`, và **không bao giờ** được ghi vào chứng nhận hay nhật ký (`TestACertificateNeverContainsSecrets`).

**Chứng nhận chỉ là thông tin, không phải ranh giới bảo mật.** Trang tài khoản hiện nhãn "Đã xác minh ABI 1" khi `conformance.json` khớp `plugin.wasm` trong thư mục cài đặt; plugin chưa có nhãn vẫn chạy trong cùng hộp cát và sau cùng các cổng duyệt. Vì tệp `.wasm` biên dịch có thể khác giữa các phiên bản Go, chứng nhận được tạo khi xây (`build/`), không được commit vào kho.

## 6. Đóng gói

- Thư mục cài đặt là `<dữ liệu>/plugins/<id>/` gồm `plugin.wasm`, `manifest.json` và (tùy chọn) `conformance.json`. Thư mục trùng `id` trong manifest để nhãn xác minh tìm được chứng nhận.
- Gói ký theo `internal/plugin/signature.go`; bản không ký cần `ACTONOS_ALLOW_UNSIGNED_PLUGINS=1`.
- Thêm một mục `plugin-<tên>` vào `Makefile` theo mẫu của `plugin-caldav` nếu bạn muốn nó nằm trong `make plugins`.

## 7. Danh sách kiểm tra trước khi gửi

- [ ] `go test ./internal/mailcal/...` xanh (gồm hai bài kiểm tra "mọi plugin").
- [ ] `Ops` khớp đúng thao tác đã cài; thao tác còn lại trả `unsupported`.
- [ ] Manifest xin ít quyền nhất; không có `*` trần.
- [ ] Mọi lỗi có mã; không có `panic` trên đầu vào lạ (thử thư không có tiêu đề, ngày sai, thân rất lớn).
- [ ] Nếu có gửi: thử gửi hai lần cùng `IdempotencyKey` và thử mạng đứt giữa chừng.
- [ ] `mailcal-conformance` in `OK` trên bản `.wasm` bạn sắp phát hành.
- [ ] `git status` chỉ cho thấy thư mục plugin của bạn (và, nếu muốn, một mục Makefile).
