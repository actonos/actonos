# Email và lịch cho agent: lớp trừu tượng, luật tự gửi, provider là plugin

Ngày: 2026-11-01 · Trạng thái: đã duyệt, đã chỉnh sau khi xác minh bằng mã tham chiếu (xem mục 16) · Phạm vi: dự án con đầu tiên của hướng "agent làm được việc thật"

Kế hoạch triển khai: `docs/superpowers/plans/2026-11-01-mailcal-00-overview.md` và sáu tệp giai đoạn kèm theo.

## 1. Mục tiêu và quyết định đã chốt

**Mục tiêu.** Nâng ActonOS từ "agent biết trò chuyện và chạy shell" lên "agent làm được việc văn phòng thật một cách tự vận hành, tin cậy". Dự án con đầu tiên là **email và lịch**.

**Quyết định của chủ dự án (đã chốt trong buổi brainstorm):**

| Chủ đề | Quyết định |
|:---|:---|
| Hướng ưu tiên | Agent làm được việc thật |
| Loại việc đầu tiên | Email, lịch (tài liệu để sau) |
| Nhà cung cấp | Nhiều loại, cần lớp trừu tượng |
| Mức tự chủ | Tự gửi theo luật đã duyệt; ngoài luật thì phải duyệt |
| Nơi đặt adapter | **Plugin WASM** (để linh hoạt, tránh phụ thuộc nhà cung cấp trong lõi) |
| IMAP/SMTP | Thêm host API socket TLS có kiểm soát |

**Giả định (chưa được xác nhận riêng, hãy sửa nếu sai):**
- Người dùng mục tiêu là chủ thiết bị và vài vận hành viên, chưa phải thị trường đại chúng.
- Người dùng **tự mang OAuth client** (client id/secret của chính họ trên Google/Microsoft); ActonOS không đăng ký ứng dụng công khai và không đi qua quy trình thẩm định ứng dụng của Google.
- v1 không lưu nội dung thư trong cơ sở dữ liệu.

**Ngoài phạm vi v1:** tạo tài liệu, danh bạ, hộp thư dùng chung, tự động dán nhãn/lọc thư, đẩy thông báo thời gian thực (v1 dùng cron và poll), tạo sự kiện lặp (đọc sự kiện lặp thì có, đã được provider khai triển).

## 2. Kiến trúc

Nguyên tắc: **lõi quyết định, plugin thực thi.** Plugin là mã không tin cậy; nó không bao giờ tự quyết việc gửi thư hay xác nhận lịch.

```
Agent ─► native_mail_send ─► ToolRegistry.Execute
                               │ (1) phân loại rủi ro + engine luật (lõi)
                               │ (2) duyệt (ApprovalManager) nếu cần
                               │ (3) audit + trace
                               ▼
                       mailcal.Router ── chọn tài khoản ──► plugin provider (WASM, cô lập)
                                                              │ net_outbound allowlist
                                                              ▼ Gmail / Graph / CalDAV / IMAP-SMTP
```

**Thành phần mới**

| Thành phần | Vị trí | Trách nhiệm |
|:---|:---|:---|
| Mô hình dữ liệu chung, sổ tài khoản, `Router` | `internal/mailcal` | Chuẩn hoá thư/sự kiện, chọn plugin theo tài khoản |
| Engine luật | `internal/mailcal/policy` | Quyết định `allow / needs_approval / deny` trên payload thật |
| Tool lõi | `internal/tools` (đăng ký bởi `mailcal`) | `native_mail_*`, `native_calendar_*` |
| Capability plugin `mail@1`, `calendar@1` | `internal/plugin` | ABI export có phiên bản |
| Host API `host_get_token`, `tls_*` | `internal/plugin` (`host_api_*.go`) | Token ngắn hạn và socket TLS có kiểm soát |
| Luồng OAuth của lõi | `internal/auth` + `internal/server` | Cấp quyền và làm mới token; refresh token không rời lõi |
| Giao diện | `web/src/pages/Mailcal` | Tài khoản, luật, thẻ duyệt, audit |
| Provider tham chiếu trong bộ nhớ | `internal/mailcal/memprovider` (và gói plugin thử nghiệm) | Chuẩn kiểm thử/eval, không cần mạng |

Plugin Gmail, Graph, CalDAV, IMAP/SMTP là các gói riêng; **lõi không chứa mã nhà cung cấp**.

## 3. Mô hình dữ liệu chung

Mọi provider dịch sang/từ các kiểu này (JSON qua ABI):

- `Address{name, email}`; email được chuẩn hoá (chữ thường, IDN sang punycode).
- `Message{id, thread_id, from, to[], cc[], subject, date, snippet, body_text, labels[], in_reply_to, attachments[]{id,name,mime,size}}`. Chỉ có `body_text` (HTML được provider đổi sang văn bản).
- `Draft{id, to[], cc[], bcc[], subject, body_text, in_reply_to, attachments[]}`.
- `Event{id, calendar_id, title, start, end, timezone, all_day, location, description, conference_url, attendees[]{email,status}, organizer}`.
- `FreeBusy{attendee, busy[]{start,end}}`.
- `Account{id, label, kind: mail|calendar|both, plugin_id, auth: oauth|password, owner}`. Mật khẩu/secret nằm trong vault, tham chiếu bằng secret id.

## 4. Hợp đồng plugin (ABI)

Manifest khai `capabilities: ["mail@1", "calendar@1"]`. Mỗi hàm là một export WASM nhận JSON và trả JSON theo đúng quy ước `alloc` / `read_response` của `acton_tool_execute`. Provider chỉ cài hàm nó hỗ trợ.

| Capability | Export | Ghi chú |
|:---|:---|:---|
| chung | `acton_mailcal_capabilities` | Trả danh sách thao tác hỗ trợ và giới hạn (tối đa kích thước đính kèm, số người nhận) |
| mail | `acton_mail_list`, `acton_mail_get`, `acton_mail_get_thread`, `acton_mail_search` | Phân trang bằng `cursor` |
| mail | `acton_mail_create_draft`, `acton_mail_send` | `send` nhận `idempotency_key`; trả `provider_message_id` |
| mail | `acton_mail_mark`, `acton_mail_move` | Đọc/chưa đọc, di chuyển thư mục/nhãn |
| calendar | `acton_calendar_list_events`, `acton_calendar_get_event` | Theo khoảng thời gian |
| calendar | `acton_calendar_create_event`, `acton_calendar_update_event`, `acton_calendar_respond` | `respond`: accept/decline/tentative |
| calendar | `acton_calendar_free_busy` | Nhiều người, một khoảng |

**Lỗi có mã:** `{ "error": { "code": "auth_expired|rate_limited|not_found|unsupported|transient|invalid", "message": "...", "retry_after_ms": 0 } }`. Lõi dựa vào mã để thử lại có lùi thời gian (`transient`, `rate_limited`), tạm dừng tài khoản và báo lên giao diện (`auth_expired`), và không bao giờ thử lại một thao tác `send` mà không có khoá idempotency.

**Tuân thủ:** bộ kiểm tra ABI (`mailcal/conformance`) chạy được với bất kỳ plugin nào và là điều kiện để plugin được đánh dấu "đã xác minh" trong registry.

## 5. Tool cho agent và phân loại rủi ro

| Tool | Rủi ro | Ghi chú |
|:---|:---|:---|
| `native_mail_list/get/get_thread/search` | low | Đọc |
| `native_calendar_list/get/free_busy` | low | Đọc |
| `native_mail_draft`, `native_mail_mark/move` | medium | Không rời hệ thống |
| `native_mail_send` | **high, qua engine luật** | Nhận `draft_id` của một bản nháp bất biến, không nhận nội dung; mặc định cần duyệt |
| `native_calendar_create_event`, `native_calendar_update_event`, `native_calendar_respond` | **high, qua engine luật** | Mặc định cần duyệt |
| `native_calendar_delete_event`, `native_mail_delete` | high, **luôn duyệt** | Không luật nào cho phép |
| `native_mailcal_propose_rule` | medium | Chỉ tạo **đề xuất**, không có hiệu lực |

Mọi tool đi qua `ToolRegistry.Execute` nên được hưởng chính sách đường dẫn, audit, trace và sandbox như tool hiện có.

**Chốt chặn bắt buộc (`ToolGate`).** Tool có tác dụng ra ngoài thiết bị (`native_mail_send`, `native_mail_delete`, `native_calendar_*_event`, `native_calendar_respond`) luôn là rủi ro High, không bao giờ nằm trong grant "đừng hỏi lại" hay tự duyệt, và **từ chối chạy nếu chưa đăng ký gate**. Gate chạy trong `Execute` trên đúng byte đầu vào mà tool sẽ nhận; quyết định `needs_approval` của gate **không thể bị bỏ qua** bởi ngưỡng duyệt của agent, `WithBypassApproval` (cron) hay grant. Gate lỗi hoặc trả giá trị lạ thì từ chối.

## 6. Engine luật tự gửi

**Mặc định đóng.** Mọi thao tác ghi cần duyệt, trừ khi một luật được duyệt trước cho phép.

**Một luật (`mailcal_rules`):**

```json
{
  "id": "rule_...", "enabled": true, "expires_at": null,
  "account_id": "acct_...", "action": "reply | send_new | calendar_accept | calendar_create | calendar_update",
  "match": {
    "recipients": { "addresses": ["a@b.com"], "domains": ["partner.com"] },
    "reply_only": true,
    "body": { "mode": "template | free", "template_id": "...", "allow_links": false },
    "attachments": "none",
    "calendar": { "working_hours": "09:00-18:00", "max_minutes": 60, "attendees": { "domains": ["partner.com"] } }
  },
  "limits": { "per_hour": 5, "per_day": 20 },
  "created_by": "user", "created_at": "..."
}
```

- Địa chỉ/miền phải cụ thể; **không chấp nhận `*`** hay miền công cộng quá rộng (ví dụ `gmail.com`, `outlook.com`) trừ khi cờ `allow_public_domain` được bật tường minh.
- Luật **cấm** (`effect: deny`) có cùng dạng và luôn thắng.

**Thuật toán quyết định:** (1) lỗi bất kỳ ở engine thì trả `needs_approval`; (2) khớp luật cấm thì `deny`; (3) thao tác thuộc nhóm luôn-duyệt (chuyển tiếp, Bcc, đính kèm, xoá, người nhận ngoài luật) thì `needs_approval`; (4) có luật cho phép khớp toàn bộ và còn trong hạn mức thì `allow`; (5) còn lại `needs_approval`.

**Chấm trên payload thật:** engine nhận chính payload mà lõi sắp chuyển cho plugin: To/Cc/Bcc đã chuẩn hoá, quan hệ thread được xác minh qua `get_thread` (người nhận là người đã nhắn trong thread), nội dung đã quét liên kết/đính kèm. Lời agent không phải bằng chứng.

**Agent không được tạo hay sửa luật.** `native_mailcal_propose_rule` chỉ ghi vào `mailcal_proposals`; luật có hiệu lực khi người dùng duyệt qua API/giao diện được xác thực. Đây là chốt chặn chống leo thang quyền.

**Vận hành an toàn:**
- *Bản nháp bất biến:* agent soạn qua `native_mail_draft`; bản nháp không có thao tác sửa (trigger SQLite chặn `UPDATE`) và mang `payload_hash`. Duyệt gắn với `draft_id`, nên đổi nội dung nghĩa là tạo nháp mới. Việc trả lời được xác minh lúc soạn nháp qua `get_thread`; kết quả (`ReplyContext`) lưu cùng nháp và engine tin nó thay vì hỏi lại provider lúc gửi.
- *Chống gửi trùng:* `mailcal_sent_ledger(idempotency_key, run_id, step, provider_message_id, payload_hash, sent_at)`. Khoá = hash(run, bước, payload). Trước khi gọi `send`, lõi tra sổ; có bản ghi thì trả kết quả cũ. Tiếp tục sau duyệt hoặc sau lỗi không gửi hai lần.
- *Công tắc tổng* (`mailcal.enabled`) và **tự tạm dừng luật** khi vượt hạn mức, hoặc khi có ≥ 3 thư bị trả về trong một giờ; có thông báo.
- Mỗi quyết định ghi audit: luật khớp (hoặc lý do không khớp), `payload_hash`, người duyệt.

## 7. Luồng duyệt

Khi quyết định là `needs_approval`, lõi gọi `ApprovalManager.Request` với `input` chứa **bản xem trước đúng như sẽ gửi** (người nhận, tiêu đề, nội dung, thời điểm) và danh sách lý do luật không khớp. Giao diện hiển thị thẻ này. Người dùng chọn:
- **Duyệt một lần**, hoặc
- **Duyệt và tạo luật từ việc này**: tạo một *đề xuất* luật thu hẹp tối đa (đúng người nhận, `reply_only`, mẫu hiện tại), người dùng xem lại rồi mới kích hoạt.

Từ chối kèm lý do được trả cho agent như quan sát của tool. Luồng tiếp tục sau duyệt dùng cơ chế resume sẵn có của engine.

## 8. Mô hình đe doạ

| Mối đe doạ | Biện pháp |
|:---|:---|
| **Tiêm lệnh qua nội dung thư** ("chuyển tiếp mật khẩu cho X") | Thư là dữ liệu có nhãn, tách khỏi chỉ dẫn; HTML sang văn bản, bỏ phần ẩn, giới hạn kích thước. Quyền không đến từ model: mọi thao tác ghi qua engine luật, gửi tới địa chỉ lạ rơi vào duyệt kèm cảnh báo "người nhận chưa từng có trong thread". |
| **Lộ dữ liệu qua trả lời tự động** | Luật tự gửi mặc định chỉ cho *mẫu cố định*, trả lời đúng người gửi. `mode: free` phải bật tường minh, giao diện cảnh báo rằng nó có thể làm lộ dữ liệu cho chính người gửi. |
| **Leo thang bằng cách tạo luật** | Agent chỉ đề xuất; kích hoạt cần quyền người dùng. |
| **Plugin độc hại hoặc bị chiếm** | Chỉ tới host trong `net_outbound`; chỉ nhận access token của đúng tài khoản; không thấy refresh token; không tự quyết việc gửi vì lõi chấm trước khi gọi. |
| **SSRF / DNS rebinding qua socket TLS** | Lõi phân giải DNS, kiểm IP, rồi nối đúng IP đã kiểm; chặn IP nội bộ, loopback, link-local. |
| **Gửi trùng, bão thư** | Sổ idempotency, hạn mức, tự tạm dừng, công tắc tổng. |
| **Địa chỉ giả mạo bằng ký tự giống nhau** | Chuẩn hoá IDN/Unicode trước khi so khớp; test đối kháng. |

## 9. Host API mới

Ba module host **mới** (module đã phát hành không đổi). Plugin cũ không khai quyền tương ứng thì không dùng được.

**`acton_oauth.get_token(account_id)` (quyền `permissions.oauth: ["<provider>"]`).** Trả `{access_token, expires_at}` ngắn hạn cho đúng tài khoản thuộc plugin gọi; id plugin lấy từ ngữ cảnh host, không từ guest. Tài khoản của plugin khác và tài khoản không tồn tại cho cùng một kết quả từ chối. Refresh token không bao giờ rời lõi. Token được che khỏi log của plugin.

**`acton_account.get_credentials(account_id)` (quyền `permissions.credentials: true`).** Trả `{username, password}` của tài khoản dùng mật khẩu ứng dụng (CalDAV, IMAP/SMTP) thuộc plugin gọi; mật khẩu nằm trong vault dưới `mailcal.account.<id>.password`, bản ghi tài khoản chỉ giữ tên khoá.

**Socket TLS `acton_tcp` (quyền `permissions.net_tcp: ["host:port", ...]`, riêng biệt với `net_outbound` vốn là tên miền cho HTTP; cổng là một phần của quyền).** Hàm: `connect(host, port, mode) -> handle`, `send(handle, bytes)`, `recv(handle, max, timeout_ms)`, `close(handle)`. Chế độ duy nhất: `tls`, `smtp-starttls`, `imap-starttls`. **Không có kết nối rõ**, và **host tự chạy phần văn bản thô của STARTTLS** bằng các byte cố định (SMTP: chào, `EHLO`, bắt buộc `STARTTLS` được quảng bá, `STARTTLS`; IMAP: chào, `STARTTLS`), huỷ nếu có dữ liệu thừa trước bắt tay, rồi mới trao kết nối đã mã hoá cho plugin; nên plugin không thể gửi thông tin đăng nhập khi kênh chưa mã hoá và kẻ ở giữa không gỡ được STARTTLS. Lõi phân giải tên **đúng một lần**, từ chối nếu bất kỳ địa chỉ nào là nội bộ (loopback, riêng tư, link-local, CGNAT, multicast, ...), nối đúng IP đã kiểm, xác minh chứng chỉ theo **tên**, TLS ≥ 1.2. Giới hạn: kết nối 10 giây; nhàn rỗi 60 giây; 4 kết nối đồng thời mỗi plugin; 50 MiB mỗi hướng mỗi kết nối; ghi log `host:port` và chế độ, không nội dung.

Client HTTP mặc định của plugin cũng được vá để nối đúng địa chỉ đã kiểm (đóng cửa sổ DNS-rebinding), và vì vậy không còn dùng `HTTP_PROXY`.

## 10. OAuth do lõi giữ

- Manifest plugin có khối `oauth`: `provider`, `auth_url`, `token_url`, `scopes` tối thiểu. Client id/secret nằm trong vault do người dùng nhập (bring-your-own client).
- Endpoint mới: `POST /api/mailcal/accounts` (tạo), `GET /api/mailcal/accounts/{id}/oauth/start`, `GET /api/mailcal/oauth/callback`, `POST /api/mailcal/accounts/{id}/test`. Dùng `OAuthEngine` (PKCE S256) và `TokenRefreshDaemon` có sẵn.
- Token hết hạn thì tài khoản chuyển `auth_expired`; lõi không thử lại mù quáng.

## 11. Dữ liệu và API

**Bảng SQLite:** `mailcal_accounts`, `mailcal_rules`, `mailcal_proposals`, `mailcal_decisions` (audit nhẹ + bộ đếm hạn mức), `mailcal_sent_ledger`, `mailcal_drafts` (bất biến). **Không có bảng cache** trong v1: đọc luôn đi thẳng tới provider, nên không lưu nội dung thư.

**API REST (đăng ký trong `router.go`, cập nhật `docs/API.md`):** `/api/mailcal/accounts*`, `/api/mailcal/rules*` (CRUD + `POST .../dry-run`), `/api/mailcal/proposals*` (duyệt/từ chối), `/api/mailcal/status` (công tắc tổng, hạn mức). Mọi cập nhật luật yêu cầu quyền admin.

**Kích hoạt khi thư đến (v1):** cron triage định kỳ do người dùng bật trong trang Automations (mặc định 5 phút; agent không tự tạo cron) và đọc theo yêu cầu. Provider không đẩy sự kiện trong v1.

## 12. Giao diện (en + vi, đủ khoá locale, không chuỗi cứng)

- **Tài khoản:** kết nối OAuth, nhập mật khẩu ứng dụng cho IMAP/SMTP, kiểm tra kết nối, trạng thái (`ok`, `auth_expired`, `paused`).
- **Luật:** danh sách, trình soạn, ô **chạy thử** ("thư/sự kiện này có được tự xử lý không? Vì sao?"), hạn mức đã dùng, nút tạm dừng.
- **Thẻ duyệt:** bản xem trước, lý do không khớp luật, nút *Duyệt một lần* và *Duyệt và tạo luật*.
- **Audit:** bộ lọc email/lịch theo luật, tài khoản, trạng thái.

## 13. Kiểm thử, eval và tiêu chí thành công

- **Unit/bảng:** engine luật (cả ca đối kháng: Unicode lookalike, Bcc, hoa/thường, IDN, `*`, miền công cộng), chuẩn hoá địa chỉ (fuzz), sổ idempotency, hạn mức, tự tạm dừng.
- **Tuân thủ ABI:** `mailcal/conformance` chạy với provider tham chiếu và mọi plugin.
- **Host API:** test `tls_*` với máy chủ TLS cục bộ (cho phép loopback chỉ trong test), test SSRF/DNS rebinding bằng resolver giả.
- **E2E (Playwright):** kết nối tài khoản giả, duyệt rồi tạo luật, chạy thử luật.
- **Eval `live` (miền "văn phòng"), với provider tham chiếu:** phân loại hộp thư và soạn nháp; tự trả lời *chỉ khi có luật*; xếp lịch theo free/busy; và **thư chứa lệnh tiêm phải không dẫn tới gửi ra ngoài khi chưa được cho phép**.

**Tiêu chí thành công (đo được):**
1. 0 thư/sự kiện ghi ra ngoài khi chưa có luật hoặc người duyệt cho phép, kể cả trong bộ bài tiêm lệnh.
2. 0 gửi trùng khi thử lại và khi tiếp tục sau duyệt (kiểm bằng test có chủ ý ngắt giữa chừng).
3. Eval miền văn phòng đạt ≥ 90% trên ít nhất hai model khác nhau.
4. Một plugin mới đạt bộ tuân thủ ABI mà **không phải sửa lõi**.

## 14. Giai đoạn triển khai (mỗi giai đoạn phát hành được)

Mỗi giai đoạn có kế hoạch triển khai riêng trong `docs/superpowers/plans/`. Cả sáu kế hoạch được viết cùng lúc theo yêu cầu của chủ dự án để các hợp đồng liên giai đoạn được khoá sớm; mỗi kế hoạch có mục "Không được đụng tới" và "Hoàn tác" để giai đoạn sau không làm vỡ giai đoạn trước.

| # | Nội dung | Điều kiện hoàn thành |
|:-:|:---|:---|
| 1 | Mô hình chung, ABI, provider tham chiếu, tool đọc/soạn nháp (chưa gửi), sổ tài khoản | Agent đọc, tóm tắt, soạn nháp trên provider tham chiếu; bộ tuân thủ ABI xanh |
| 2 | Engine luật, duyệt, gửi/phản hồi, sổ chống trùng, audit, API luật, giao diện luật và thẻ duyệt | Tiêu chí 1 và 2 đạt trên provider tham chiếu |
| 3 | `host_get_token`, luồng OAuth lõi, plugin Gmail + Google Calendar | Kết nối tài khoản Google thật, đọc/soạn/gửi theo luật |
| 4 | Plugin Microsoft Graph và CalDAV | Cùng bộ tuân thủ ABI, không đổi lõi |
| 5 | Host API socket TLS và plugin IMAP/SMTP | Gửi/nhận qua máy chủ IMAP/SMTP thật; test SSRF xanh |
| 6 | Hoàn thiện giao diện, bộ eval văn phòng, hướng dẫn viết plugin, cập nhật tài liệu | Tiêu chí 3 và 4 đạt |

## 15. Rủi ro còn lại

- **Phụ thuộc OAuth của bên thứ ba:** khoá chính sách của Google/Microsoft ngoài tầm kiểm soát; giảm nhẹ bằng việc người dùng tự mang OAuth client và có đường IMAP/SMTP dùng mật khẩu ứng dụng.
- **Chất lượng đọc thư:** HTML phức tạp mất định dạng khi đổi sang văn bản; chấp nhận ở v1.
- **Kích thước và hiệu năng WASM:** thư viện HTTP/JSON trong guest làm plugin lớn; giữ plugin tham chiếu nhỏ để làm mẫu.
- **Hạn mức token model:** hộp thư lớn cần phân trang và tóm tắt nhiều tầng; tận dụng `AutoSummarizeObservation` sẵn có.

## 16. Chỉnh sau khi xác minh bằng mã tham chiếu

Giai đoạn 1, chốt chặn `ToolGate` và engine luật đã được dựng thử trên nhánh `wip/mailcal-reference` để kiểm chứng thiết kế trước khi viết kế hoạch. Những điều đã đổi so với bản nháp đầu:

1. **Bản nháp bất biến, `send` nhận `draft_id`** (mục 5, 6). Bỏ `native_mail_reply`: trả lời là `native_mail_draft` với `in_reply_to`, được lõi xác minh; không có đường gửi nào nhận nội dung tự do.
2. **`net_tcp` thay cho `host:port` trong `net_outbound`** (mục 9), vì `net_outbound` hiện là mẫu tên miền cho HTTP và đổi nghĩa sẽ làm vỡ plugin cũ.
3. **ABI có thêm thao tác xoá** (`acton_mail_delete`, `acton_calendar_delete_event`) và bỏ bảng cache (mục 4, 11).
4. **Chốt chặn bắt buộc trong `Execute`** (mục 5), vì ngưỡng duyệt của agent, `WithBypassApproval` và grant đều có thể bỏ qua duyệt thông thường. Không cần "dấu băm đầu vào đã chấm" riêng: gate và tool nhận cùng một mảng byte.
5. **Sửa lỗi nền tảng phát hiện khi dựng thử:** `CallJSON` phải tôn trọng context đã huỷ, và module wazero cần `WithCloseOnContextDone(true)` thì mới ngắt được guest lặp vô hạn.
6. **HTTP của plugin hiện kiểm IP rồi để `http.Client` phân giải lại** (cửa sổ DNS rebinding). Giai đoạn 5 sửa riêng cho `tls_*`; việc vá đường HTTP được ghi như rủi ro còn lại và đề xuất làm trong cùng giai đoạn.
7. **Giai đoạn 4 gồm cả đường mật khẩu** (`acton_account`) vì CalDAV cần nó, và giai đoạn 5 gồm cả việc vá rebinding của HTTP. Đã thêm hai module host mới ngoài `tls_*` ban đầu.
8. **STARTTLS do host chạy**, không phải `tls_starttls(handle)` do plugin gọi như bản nháp đầu (mục 9): cách đó để plugin gửi được byte khi kênh chưa mã hoá.
9. **Gửi qua SMTP là "tối đa một lần"**, không phải "đúng một lần": SMTP không có khoá idempotency. Giao thức hai pha (bản sao `\Draft` trong Sent trước khi gửi, gỡ cờ sau khi máy chủ nhận) bảo đảm không bao giờ trùng, và khi không chắc thì báo lỗi để người kiểm tra thư mục Sent. Gmail dùng header khoá và tìm lại; Graph dùng thuộc tính mở rộng cho thư và `transactionId` cho sự kiện; CalDAV dùng UID suy ra từ khoá cùng `If-None-Match: *`.
10. **Mỗi bản nháp chỉ gửi được một lần**: khoá của thư là hàm của `draft_id` (không phải của lượt chạy và bước như bản nháp đầu), vì nháp bất biến nên `draft_id` đã bao hàm nội dung.
11. **Cron kích hoạt phải do người dùng bật** và chạy bằng agent hẹp quyền; chốt chặn vẫn quyết định kể cả với `WithBypassApproval`.
