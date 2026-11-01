# Giai đoạn 5: host API socket TLS có kiểm soát và plugin IMAP/SMTP

> **Dành cho agent thực thi:** BẮT BUỘC dùng superpowers:subagent-driven-development (khuyến nghị) hoặc superpowers:executing-plans. Mỗi bước dùng checkbox `- [ ]`. Đọc `2026-11-01-mailcal-00-overview.md` trước. **Điều kiện bắt đầu:** giai đoạn 4 đã gộp vào `master` (giai đoạn này dùng `acton_account.get_credentials` và `provkit` của GĐ3-4).

**Mục tiêu:** Đọc và gửi thư qua bất kỳ máy chủ IMAP/SMTP nào bằng mật khẩu ứng dụng, với một host API socket TLS mà plugin WebAssembly dùng được **mà không mở được đường nào vào mạng nội bộ**, và đóng luôn cửa sổ DNS-rebinding của đường HTTP plugin hiện có.

**Kiến trúc:** Module host **mới** `acton_tcp` (`connect/send/recv/close`). Lõi tự phân giải tên **một lần**, kiểm mọi địa chỉ, rồi nối đúng IP đã kiểm (`security.DialPolicy`); chỉ có TLS. Với SMTP và IMAP nâng cấp STARTTLS, **chính host chạy phần văn bản thô** bằng các byte cố định (chào, `EHLO`, `STARTTLS`) rồi mới trao kết nối đã mã hoá cho plugin, nên plugin không bao giờ gửi được thông tin đăng nhập hay bất cứ gì khi kênh chưa mã hoá, và kẻ ở giữa không gỡ được STARTTLS. Plugin dùng nguyên thư viện `emersion/go-imap/v2` và `go-smtp` chạy trong WASM trên `net.Conn` của `wasmguest`.

**Tech stack:** Go (crypto/tls, wasip1), `github.com/emersion/go-imap/v2` (beta.8), `github.com/emersion/go-smtp`, `github.com/emersion/go-sasl`.

**Spec:** `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md` (mục 8, 9, 14, 16).

## Global Constraints

- Hợp đồng khoá C1-C8. Chỉ **thêm**: quyền `permissions.net_tcp`, module host `acton_tcp`, plugin mới, ba thư viện (gồm `go-message` gián tiếp).
- **Không có kết nối rõ (plaintext).** Chế độ hợp lệ duy nhất: `tls`, `smtp-starttls`, `imap-starttls`; chế độ khác bị từ chối (`TestOnlyTheListedHostAndPortAreReachable`).
- Từ chối nếu **bất kỳ** địa chỉ nào tên phân giải ra là địa chỉ nội bộ (loopback, riêng tư `10/8 172.16/12 192.168/16 fc00::/7`, link-local `169.254/16 fe80::/10`, CGNAT `100.64/10`, multicast, unspecified, broadcast) hoặc tên là `localhost`/`*.localhost`; chống rebinding bằng cách **phân giải đúng một lần** và nối IP đó.
- Giới hạn cố định: kết nối+bắt tay 10 giây; nhàn rỗi 60 giây mỗi thao tác; tối đa 4 kết nối đồng thời mỗi plugin; 50 MiB mỗi hướng mỗi kết nối; một lần `send` ≤ 1 MiB; một lần `recv` ≤ 64 KiB; phần STARTTLS của host ≤ 10 giây. Nhật ký chỉ ghi `host:port` và chế độ, **không bao giờ nội dung**.
- STARTTLS: SMTP bắt buộc máy chủ **quảng bá** `STARTTLS` và trả `220`; mọi byte còn dư trong bộ đệm sau câu trả lời cuối ⇒ huỷ (chặn tiêm lệnh trước bắt tay); IMAP bắt buộc lời chào `* OK` và `a1 OK`.
- Gửi thư **tối đa một lần** (không bao giờ trùng) và nói thẳng khi không chắc (xem Task 4). Cần thư mục Sent trên máy chủ.
- `net_tcp` mặc định **rỗng** trong manifest phát hành: người dùng thêm máy chủ của mình và cài plugin không ký.
- Thay đổi hành vi có chủ ý, ghi vào `CHANGELOG.md`: client HTTP mặc định của plugin **không còn dùng `HTTP_PROXY`** (proxy tự phân giải tên, nằm ngoài chính sách).
- Giao diện: khoá `en` và `vi`; không chuỗi cứng; không emoji. Không `Co-Authored-By`.

## Review Focus

1. **SSRF qua tên đẹp**: tên công khai trỏ vào `10.x`, `169.254.169.254` (metadata đám mây), `127.0.0.1`; tên vừa có IP công khai vừa có IP nội bộ; literal `::1`, `fd00::1`, `100.64.0.1`, `0.0.0.0`, `255.255.255.255` (`TestSafeAddrRefusesLocalAndPrivateTargets`, `...ANameThatAlsoPointsInside`, `TestProductionPolicyBlocksLocalTargetsEvenIfListed`).
2. **DNS rebinding**: bộ phân giải trả địa chỉ khác ở lần gọi thứ hai; chứng minh chỉ phân giải **đúng một lần** (`TestTheNameIsResolvedOnceSoRebindingCannotSwapTheTarget`, `TestThePluginHTTPClientDialsOnlyTheVettedAddress`).
3. **Gỡ STARTTLS và tiêm văn bản thô**: máy chủ không quảng bá STARTTLS (bị gỡ khỏi danh sách `EHLO`) vẫn chấp nhận lệnh; máy chủ gửi thêm dữ liệu ngay sau `220` (`TestStartTLSIsNeverDowngradedOrInjected`: cả hai đột biến đã được kiểm là bị bắt).
4. **Chứng chỉ**: tên không khớp, CA không tin cậy, bắt tay hỏng (`TestTLSVerifiesTheNameNotTheDialedIP`); TLS tối thiểu 1.2.
5. **Cạn tài nguyên**: kết nối thứ năm, vượt ngân sách byte, `recv` không có dữ liệu ⇒ hết giờ trả `0` chứ không treo cả instance (`TestAtMostFourConnectionsAndSlotsAreReleased`, `TestEachDirectionHasAByteBudget`, `TestRecvTimesOutWithoutData`); đóng plugin đóng mọi socket (`TestClosingThePluginClosesItsSockets`).
6. **Gửi trùng với SMTP** (không có khoá idempotency): mất phản hồi sau `DATA`, thư mục Sent có bản sao đánh dấu `\Draft` chưa được gỡ ⇒ gửi lại **từ chối đoán** (`TestAnUncertainSubmissionIsNeverRetriedBlindly`); người nhận bị từ chối (5xx) ⇒ xoá bản sao tạm để lần sau sạch (`TestARefusedRecipientLeavesNothingBehindSoTheRetryStartsClean`); cùng khoá gửi hai lần ⇒ replay (`TestSendingTheSameKeyAgainIsAReplayNotASecondMessage`).
7. **Thư lạ**: HTML ẩn lệnh bị bỏ (`TestGetReturnsTextAndDropsHiddenInstructions`); id thư giả (`INBOX:0`, `x:1:`) bị từ chối; `thread_id` chứa xuống dòng bị từ chối; Bcc chỉ nằm trong phong bì, không trong header.

## Mã tham chiếu đã kiểm chứng

Nhánh `wip/mailcal-ref-p5` (tách từ `wip/mailcal-ref-p4`). Tệp mới: `git checkout wip/mailcal-ref-p5 -- <tệp>`; tệp đã có: `git diff wip/mailcal-ref-p4 wip/mailcal-ref-p5 -- <tệp> | git apply --index`. Đã chạy xanh, kể cả `-race`: dialer, các hàm host với máy chủ TLS/SMTP/IMAP giả, **plugin WASM biên dịch thật (~11 MB) đọc và gửi thư qua STARTTLS do host chạy**, tới máy chủ go-imap (`imapmemserver`) và go-smtp chạy trong tiến trình. Chưa có: nối dây `main.go`, web, kiểm trên máy chủ thật.

## Không được đụng tới

Mọi module host đã phát hành (`acton_oauth`, `acton_account`, `acton_host`, `acton_net`, `acton_sys`, ...), `internal/tools/`, `policy/`, `writeops.go`, ABI. Đường HTTP cũ chỉ đổi **bộ vận chuyển mặc định** (Task 3), không đổi cổng kiểm allowlist.

## Cấu trúc tệp

| Việc | Đường dẫn |
|:---|:---|
| Dialer an toàn | `internal/security/{safedial.go,safedial_test.go}` |
| Host API socket | `internal/plugin/{host_api_tcp.go,host_api_tcp_test.go}`; sửa `types.go` (`Permissions.NetTCP`), `security_gate.go` (`CheckTCPAccess`), `host_api.go` (module `acton_tcp`, `HostContext.Dial/TCPByteLimit/tcpConns`), `loader.go` (`CloseAllTCP`) |
| HTTP chống rebinding | sửa `internal/plugin/host_api_net.go`; `rebinding_test.go` |
| Guest | `internal/mailcal/provkit/wasmguest/tcp.go`; `refplugin/imapguest/main.go`; `imapwasm_test.go` |
| Plugin IMAP/SMTP | `internal/mailcal/plugins/imapsmtp/{imapsmtp.go,manifest.json,imapsmtp_test.go,wasm_test.go,wasm/main.go}` |
| Phụ thuộc | `go.mod`, `go.sum` |
| Web và tài liệu | `web/src/pages/Mailcal/AccountModal.tsx` (+test), locales, `Makefile`, `docs/PLUGINS-MAILCAL.md`, `docs/SECURITY.md` |

## Task 1: Dialer an toàn (`security.DialPolicy`)

**Files:** Create `internal/security/safedial.go`, `internal/security/safedial_test.go`.

**Interfaces:**
- Produces: `security.DialPolicy{Resolve, AllowLoopback, TLS *tls.Config, ConnectTimeout}` (giá trị không là chính sách sản xuất); `(DialPolicy).SafeAddr(ctx, host) (net.IP, error)`; `DialTCP(ctx, host, port) (net.Conn, error)`; `UpgradeTLS(ctx, conn, host) (*tls.Conn, error)` (xác minh theo **tên**, không theo IP; TLS ≥ 1.2); `DialTLS(ctx, host, port)`; `ErrUnsafeAddress`.
- Quy tắc: `AllowLoopback` và `Resolve` chỉ dành cho test; không bao giờ đặt trong mã sản xuất (kiểm bằng `grep -rn "AllowLoopback" --include=*.go . | grep -v _test`, kết quả chỉ được là định nghĩa trong `safedial.go`).

- [ ] **Step 1: Test trước, đỏ**

```bash
git checkout wip/mailcal-ref-p5 -- internal/security/safedial_test.go
go vet ./internal/security 2>&1 | head -3
```
- [ ] **Step 2: Cài đặt và chạy**

```bash
git checkout wip/mailcal-ref-p5 -- internal/security/safedial.go
go test ./internal/security -count=1 -race -v 2>&1 | grep -E '^(---|ok|FAIL)'
grep -rn "AllowLoopback" --include=*.go . | grep -v _test
```
Expected: PASS (6 test mới); `grep` chỉ ra `safedial.go`.

- [ ] **Step 3: Đột biến:** trong `SafeAddr` đổi `for _, ip := range ips { if p.unsafe(ip) ...` thành chỉ kiểm `ips[0]`: `TestSafeAddrRefusesANameThatAlsoPointsInside` đỏ. Đổi `UpgradeTLS` để đặt `ServerName` bằng IP đã nối: `TestTLSVerifiesTheNameNotTheDialedIP` đỏ. Hoàn nguyên.
- [ ] **Step 4: Commit** `feat(security): add a dialer that resolves once and refuses local and private addresses`.

## Task 2: Host API `acton_tcp` và quyền `net_tcp`

**Files:** như bảng "Host API socket".

**Interfaces:**
- Produces: `PluginPermissions.NetTCP []string` (mục `host:port`; host có thể là `*.example.com`; **cổng là một phần của quyền**); `(*SecurityGate).CheckTCPAccess(host, port) error` (`ErrTCPUnauthorized`); module WASM `acton_tcp`:
  - `connect(reqPtr, reqLen) -> handle>0 | mã`, yêu cầu JSON `{"host","port","mode"}`;
  - `send(handle, ptr, len) -> bytes | mã`; `recv(handle, max, timeoutMs) -> bytes>0 | 0 (hết giờ) | mã` (dữ liệu qua `acton_sys.read_response`); `close(handle)`;
  - mã: `TCPErrNoHost=-1`, `Denied=-2`, `Blocked=-3`, `Connect=-4`, `TooMany=-5`, `OverLimit=-6`, `Closed=-7`.
  - `HostContext.Dial security.DialPolicy`, `TCPByteLimit int64`, `(*HostContext).CloseAllTCP()` (gọi trong `PluginInstance.Close`).
- STARTTLS do host chạy: `smtpStartTLS` (đọc `220`, gửi `EHLO acton.invalid`, bắt buộc `250` có dòng `STARTTLS`, gửi `STARTTLS`, bắt buộc `220`, `br.Buffered()==0`) và `imapStartTLS` (`* OK`, `a1 STARTTLS`, `a1 OK`, `br.Buffered()==0`).

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p5 -- internal/plugin/host_api_tcp_test.go
go vet ./internal/plugin 2>&1 | head -3
```
- [ ] **Step 2: Cài đặt**

```bash
git checkout wip/mailcal-ref-p5 -- internal/plugin/host_api_tcp.go
git diff wip/mailcal-ref-p4 wip/mailcal-ref-p5 -- internal/plugin/types.go internal/plugin/security_gate.go internal/plugin/host_api.go internal/plugin/loader.go | git apply --index
go test ./internal/plugin -count=1 -race
```
Expected: PASS, gồm 10 test: `TestImplicitTLSRoundTrip`, `TestSMTPStartTLSIsRunByTheHostAndEndsEncrypted`, `TestIMAPStartTLS`, `TestStartTLSIsNeverDowngradedOrInjected`, `TestOnlyTheListedHostAndPortAreReachable`, `TestProductionPolicyBlocksLocalTargetsEvenIfListed`, `TestAtMostFourConnectionsAndSlotsAreReleased`, `TestEachDirectionHasAByteBudget`, `TestRecvTimesOutWithoutData`, `TestClosingThePluginClosesItsSockets`.

> **Bài học đã ghi nhận khi dựng thử:** máy chủ giả trong test phải `defer c.Close()` (hoặc `runtime.KeepAlive`). Nếu không, bộ gom rác đóng kết nối nhàn rỗi giữa chừng và test chỉ đỏ khi chạy `-race`. Luôn chạy `-race` ít nhất 3 lần cho test mạng.

- [ ] **Step 3: Đột biến (đã làm, đều bị bắt):** bỏ kiểm `br.Buffered() != 0` ⇒ `TestStartTLSIsNeverDowngradedOrInjected` đỏ; bỏ yêu cầu `offered` ⇒ cũng đỏ (máy chủ giả gỡ `STARTTLS` khỏi `EHLO` nhưng vẫn chấp nhận lệnh).
- [ ] **Step 4: Commit** `feat(plugin): add controlled TLS sockets for plugins with host-run STARTTLS`.

## Task 3: Client HTTP mặc định của plugin chống rebinding

**Files:** Modify `internal/plugin/host_api_net.go`; Create `internal/plugin/rebinding_test.go`.

**Interfaces:** `rebindingSafeTransport(security.DialPolicy) *http.Transport` (`Proxy: nil`, `DialContext` đi qua `DialPolicy.DialTCP`); `sandboxedHTTPClient` dùng nó khi `HostContext.HTTPClient == nil`; client do người gọi cung cấp (test) giữ nguyên bộ vận chuyển.

- [ ] **Step 1: Test trước, đỏ:** `git checkout wip/mailcal-ref-p5 -- internal/plugin/rebinding_test.go && go vet ./internal/plugin` (đỏ: `Transport` là nil).
- [ ] **Step 2: Cài đặt:** `git diff wip/mailcal-ref-p4 wip/mailcal-ref-p5 -- internal/plugin/host_api_net.go | git apply --index && go test ./internal/plugin -count=1 -race`.
  Expected: `TestThePluginHTTPClientDialsOnlyTheVettedAddress` (đúng **1** lần phân giải), `TestThePluginHTTPClientRefusesLoopbackAndPrivateTargetsAtDialTime`, `TestACallerSuppliedClientIsStillHonoured` PASS, cùng toàn bộ test plugin cũ.
- [ ] **Step 3: Ghi `CHANGELOG.md`** về việc bỏ `HTTP_PROXY` cho HTTP của plugin và lý do; thêm câu vào `docs/SECURITY.md`.
- [ ] **Step 4: Commit** `fix(plugin): make plugin HTTP dial only the address it vetted`.

> Lưu ý phạm vi: đường WebSocket của plugin (`ws_connect`) vẫn dùng `security.ValidateOutboundURL` rồi để thư viện tự nối; cùng kiểu cửa sổ rebinding. Ghi một việc riêng (đề xuất: `spawn_task`) thay vì mở rộng giai đoạn này.

## Task 4: Bộ nối `net.Conn` phía guest và thư viện IMAP/SMTP trong WASM

**Files:** Create `internal/mailcal/provkit/wasmguest/tcp.go`, `internal/mailcal/refplugin/imapguest/main.go`, `internal/mailcal/imapwasm_test.go`; Modify `go.mod`, `go.sum`.

**Interfaces:** `wasmguest.DialTCP(host, port, mode) (*Conn, error)` (`ModeTLS`, `ModeSMTPStartTLS`, `ModeIMAPStartTLS`), `*Conn` implements `net.Conn`: `Read` thăm dò host theo lát 20 ms rồi `runtime.Gosched()` + `time.Sleep(1ms)` (guest đơn luồng: một `recv` dài sẽ chặn goroutine ghi lệnh kế tiếp của go-imap ⇒ treo); tôn trọng `SetReadDeadline` (lỗi hết giờ là `net.Error.Timeout()`); `Write` chia khúc ≤ 256 KiB; **sau STARTTLS của SMTP, `Conn` đưa sẵn một dòng chào `220 ...` vào bộ đệm đọc** vì host đã nuốt lời chào thật mà `smtp.NewClient` luôn chờ một lời chào trước khi gửi `EHLO`.

- [ ] **Step 1: Phụ thuộc**

```bash
go get github.com/emersion/go-imap/v2@v2.0.0-beta.8 github.com/emersion/go-smtp@latest github.com/emersion/go-sasl@latest
go get github.com/emersion/go-imap/v2/imapclient@v2.0.0-beta.8
```
(Cần mạng. Nếu `go.sum` thiếu `go-message`, lệnh thứ hai thêm nó.) Sau khi các gói được import trực tiếp ở Task 5: `go mod tidy`. Kiểm `GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build ./internal/mailcal/refplugin/imapguest` ra tệp ~10 MB (thư viện IMAP chạy được trong WASM; đây là điểm bất định lớn nhất của giai đoạn và **đã được kiểm chứng**).
- [ ] **Step 2: Test trước, đỏ:** `git checkout wip/mailcal-ref-p5 -- internal/mailcal/imapwasm_test.go` ⇒ thiếu `imapguest`.
- [ ] **Step 3: Cài đặt và chạy**

```bash
git checkout wip/mailcal-ref-p5 -- internal/mailcal/provkit/wasmguest/tcp.go internal/mailcal/refplugin/imapguest
go test ./internal/mailcal -count=1 -run 'AStockIMAP|TheGuestCannot' -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: `TestAStockIMAPClientRunsInsideWebAssemblyOverTheHostsTLSSockets` (đăng nhập, `SELECT`, đọc số thư, dưới 5 giây), `TestTheGuestCannotConnectWhereTheManifestDoesNotAllow` PASS.
- [ ] **Step 4: Commit** `feat(mailcal): let guests use the host's TLS sockets as a net.Conn`.

## Task 5: Plugin IMAP/SMTP

**Files:** Create `internal/mailcal/plugins/imapsmtp/*` (xem bảng); Modify `Makefile` (`plugin-imap-smtp`), `go.mod`.

**Interfaces:** `imapsmtp.Backend{Dial DialFunc, Creds netio.Credentials}` implements `provkit.Backend`; `DialFunc func(ctx, host string, port int, mode string) (net.Conn, error)` (test dùng `crypto/tls`; WASM dùng `wasmguest.DialTCP`); hằng `IdempotencyHeader = "X-Acton-Idempotency-Key"`, `ModeTLS|ModeIMAPStartTLS|ModeSMTPStartTLS`.

Cấu hình tài khoản (không bí mật): `imap_host`, `smtp_host`, `email` (bắt buộc), `imap_mode`/`smtp_mode` (`tls`|`starttls`; mặc định IMAP `tls`:993, SMTP `starttls`:587), `imap_port`, `smtp_port`, `sent_mailbox` (mặc định `Sent`), `trash_mailbox` (mặc định `Trash`), `username` (mặc định `email`). Không có chế độ plaintext.

Hành vi (đã cài và có test):

| ABI | Cách làm |
|:---|:---|
| mọi thao tác | mở kết nối riêng, `Login` (từ chối ⇒ `auth_expired`), làm việc, `Logout` |
| `mail_list` | `UID SEARCH` rồi lấy `limit` UID mới nhất sau `offset`; con trỏ = số; `UnreadOnly` = `NOT \Seen`; `Envelope+Flags+UID+InternalDate` và header `References` |
| id thư | `<hộp thư url-escape>:<uid>`; id sai hình ⇒ `invalid`; thiếu ⇒ `not_found` |
| `thread_id` | Message-ID gốc (phần tử đầu của `References`, hoặc `In-Reply-To`, hoặc chính nó) |
| `mail_get` | `BODY.PEEK[]` tối đa 512 KiB ⇒ `mimekit.Parse` (HTML ẩn bị bỏ) |
| `mail_get_thread` | `SEARCH OR HEADER Message-ID r (OR HEADER References r HEADER In-Reply-To r)` trong INBOX và Sent, tối đa 50 thư mỗi hộp |
| `mail_mark` | `UID STORE ±\Seen` |
| `mail_move` | `UID MOVE` |
| `mail_delete` | `MOVE` vào Trash, **không** expunge |
| `mail_send` | (1) `SEARCH HEADER X-Acton-Idempotency-Key k` trong Sent: có và không còn `\Draft` ⇒ replay; còn `\Draft` ⇒ `transient` "có thể đã gửi hoặc chưa", **không gửi lại**; (2) `APPEND` bản sao vào Sent với `\Draft`; (3) SMTP `EHLO`, `AUTH PLAIN`, `MAIL`, `RCPT` (To+Cc+Bcc), `DATA`; (4) thành công ⇒ gỡ `\Draft`, thêm `\Seen`; lỗi **trước `DATA`** hoặc 5xx ⇒ xoá bản sao tạm; lỗi **trong/sau `DATA`** ⇒ giữ bản sao `\Draft` và trả `transient` |

Tính chất cần nói thẳng (đưa vào `docs/PLUGINS-MAILCAL.md` và mô tả PR): **SMTP không có khoá idempotency, nên giao thức trên cho "tối đa một lần"**, không phải "đúng một lần". Khi không chắc, kết quả là một lỗi cần người kiểm tra thư mục Sent, **không bao giờ** một thư trùng. Điều này đúng với tiêu chí thành công số 2 (0 gửi trùng) và đánh đổi bằng khả năng phải can thiệp tay hiếm hoi. `Service.writeCall` thử lại `transient` tối đa hai lần với cùng khoá; nhờ cơ chế trên các lần thử lại đó an toàn.

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p5 -- internal/mailcal/plugins/imapsmtp/imapsmtp_test.go internal/mailcal/plugins/imapsmtp/wasm_test.go internal/mailcal/plugins/imapsmtp/manifest.json
go vet ./internal/mailcal/plugins/imapsmtp 2>&1 | head -3
```
- [ ] **Step 2: Cài đặt và chạy**

```bash
git checkout wip/mailcal-ref-p5 -- internal/mailcal/plugins/imapsmtp/imapsmtp.go internal/mailcal/plugins/imapsmtp/wasm
go mod tidy
go test ./internal/mailcal/plugins/imapsmtp -count=1 -race -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: PASS, gồm 11 test native (máy chủ `imapmemserver` và `go-smtp` chạy trong tiến trình qua TLS) và 2 test WASM: `TestTheWasmPluginReadsAndSendsOverHostRunSTARTTLS` (plugin biên dịch thật đọc, gửi, gửi lại cùng khoá ⇒ `Replayed`, đúng **một** thư được giao), `TestTheWasmPluginCannotReachServersTheManifestDoesNotList`.
- [ ] **Step 3: Đột biến (đã làm, đều bị bắt):** bỏ nhánh `hasFlag(..., \Draft)` ⇒ `TestAnUncertainSubmissionIsNeverRetriedBlindly` đỏ; bỏ `UIDExpunge` bản sao tạm ⇒ `TestARefusedRecipientLeavesNothingBehindSoTheRetryStartsClean` đỏ; đổi khoá tìm trong Sent ⇒ `TestSendingTheSameKeyAgainIsAReplayNotASecondMessage` đỏ. Hoàn nguyên.
- [ ] **Step 4: Makefile** `plugin-imap-smtp`. **Commit** `feat(mailcal): add the IMAP and SMTP provider plugin`.

## Task 6: Nối dây, giao diện, tài liệu

**Files:** Modify `web/src/pages/Mailcal/AccountModal.tsx` (+test), `web/src/locales/{en,vi}/mailcal.json`, `docs/PLUGINS-MAILCAL.md`, `docs/SECURITY.md`, `CHANGELOG.md`. `cmd/actond` **không cần đổi** (broker mật khẩu đã nối ở GĐ4).

- [ ] **Step 1: Test web trước.** Khi plugin có `auth_kind === 'password'` và manifest khai `capabilities` chứa `mail@1`, `AccountModal` hiển thị biểu mẫu IMAP/SMTP: máy chủ IMAP/SMTP, cổng, chế độ (`tls`/`starttls`), `email`, `username`, tên hộp thư Sent/Trash, mật khẩu. Test: (a) `config` được gửi có đủ khoá trên và **không** có `password`; (b) hiển thị khối **"Thêm các dòng này vào `permissions.net_tcp` của plugin"** với đúng `imap.host:port` và `smtp.host:port` đang nhập (người dùng cần chúng để cài); (c) cổng không phải số hoặc chế độ lạ hiện lỗi trước khi gửi.
- [ ] **Step 2: Cài đặt** theo mô tả; khoá locale `account.imap*`, `account.smtp*`, `account.netTcpHint` ở `en` và `vi`.
- [ ] **Step 3: Kiểm**

```bash
cd web && npx tsc --noEmit && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run src/pages/Mailcal && npm run build && node scripts/check-bundle.mjs
```
- [ ] **Step 4: Tài liệu** `docs/PLUGINS-MAILCAL.md`: cách cài plugin IMAP/SMTP (sao chép thư mục thành `mailcal-imap-smtp-<tên>`, đặt `id` khác, điền `net_tcp`, cài không ký), bảng nhà cung cấp phổ biến và cổng/chế độ, mật khẩu ứng dụng, "gửi tối đa một lần" và cách xử lý thư báo "có thể đã gửi hoặc chưa" (kiểm Sent, gỡ cờ `\Draft` bằng tay hoặc xoá bản sao rồi bảo agent gửi lại). `docs/SECURITY.md`: mô hình `acton_tcp`. **Commit** `feat(web): add the IMAP and SMTP account form and document the plugin`.

## Task 7: Cổng giai đoạn 5

- [ ] **Step 1: Toàn bộ test, nhiều lần cho test mạng**

```bash
go build ./... && go vet ./... && go test ./internal/... ./evals/... ./cmd/... -count=1
go test -race -count=3 ./internal/security ./internal/plugin ./internal/mailcal/plugins/imapsmtp
git diff --stat wip/mailcal-ref-p4 HEAD -- internal/tools internal/mailcal/policy internal/mailcal/writeops.go internal/mailcal/abi
cd web && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run && cd ..
```
Expected: xanh; `git diff --stat` cuối **không in gì**.
- [ ] **Step 2: Ma trận SSRF/rebinding chạy như cổng độc lập:** `go test ./internal/security ./internal/plugin -run 'SafeAddr|NameIsResolvedOnce|Loopback|ProductionPolicy|OnlyTheListed|Rebinding|PluginHTTPClient' -count=1 -v` phải liệt kê và PASS toàn bộ test trong mục Review Focus 1-2 (tiêu chí spec: "test SSRF xanh").
- [ ] **Step 3: Nghiệm thu với máy chủ thật (người dùng làm; không tự động hoá; không đưa khoá vào tệp):**
  1. **Máy chủ thật có mật khẩu ứng dụng** (Fastmail, hoặc Gmail IMAP với mật khẩu ứng dụng, hoặc Dovecot+Postfix tự dựng): kết nối, `RunShape`, đọc, tóm tắt, soạn nháp.
  2. **Gửi qua luồng duyệt**: duyệt một thư; kiểm thư đến, thư mục Sent có **đúng một** bản (không còn cờ `\Draft`), bấm duyệt lần hai không gửi thêm.
  3. **Kiểm giả định chưa thể kiểm bằng máy chủ giả**: (a) máy chủ giữ header `X-Acton-Idempotency-Key` trong Sent (nếu không, bước 1 của `mail_send` vô hiệu, đổi sang tìm theo `Message-ID` đã đặt `acton-<hash>@miền`, đã được gửi đi trong header); (b) máy chủ hỗ trợ cờ `\Draft` trên thư thường trong Sent và `UID EXPUNGE` (UIDPLUS); nếu không, `moveTo`/xoá bản sao tạm cần phương án `STORE \Deleted` + `EXPUNGE` toàn hộp (ghi quyết định vào PR); (c) STARTTLS cổng 587 và TLS ngầm cổng 465 đều chạy.
  4. **Cô lập**: từ máy chủ thử, cố đặt `smtp_host` thành `127.0.0.1` hoặc một IP riêng trong `net_tcp` và xác nhận lỗi "the address is local or private".
- [ ] **Step 4: Tài liệu và `CHANGELOG.md`**; **commit** `docs: record the TLS socket API and the IMAP/SMTP provider`.

## Hoàn tác

`git revert` Task 7 → 1. Gỡ plugin IMAP/SMTP khỏi `<data>/plugins/` làm tài khoản báo "provider plugin is not loaded". Hoàn tác Task 3 trả lại bộ vận chuyển HTTP mặc định (và `HTTP_PROXY`); các task khác không phụ thuộc vào nó. Nếu một bản sao `\Draft` bị bỏ lại trong Sent, nó vô hại: chỉ cần người dùng xoá bằng ứng dụng thư.

## Điều giai đoạn sau phải biết

- Mọi plugin mạng mới dùng `acton_tcp` hoặc HTTP đều đi qua `security.DialPolicy`; đừng thêm đường dial riêng.
- `ReplyContext.Participants` (GĐ2) cho IMAP dựa vào `thread_id` = Message-ID gốc; nếu máy chủ không lưu `References`, chuỗi bị tách và bản nháp trả lời sẽ bị từ chối ("không xác minh được") thay vì bị gắn nhãn đã xác minh sai: đó là hành vi mong muốn.
- GĐ6 chạy bộ eval và `allplugins_test.go` cho cả bốn nhóm plugin (Gmail, Graph, CalDAV, IMAP/SMTP).
