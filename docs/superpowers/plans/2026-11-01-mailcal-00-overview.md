# Email và lịch cho agent: tổng quan kế hoạch và hợp đồng khoá

> **Dành cho agent thực thi:** BẮT BUỘC dùng superpowers:subagent-driven-development (khuyến nghị) hoặc superpowers:executing-plans để làm từng task. Mỗi bước dùng checkbox `- [ ]`. **Đọc tệp này trước mọi tệp giai đoạn.**

**Mục tiêu:** Cho agent đọc, soạn, gửi thư và quản lý lịch qua nhiều nhà cung cấp, tự gửi chỉ khi có luật đã duyệt, với adapter là plugin WASM.

**Kiến trúc:** Lõi quyết định, plugin thực thi. Lõi giữ mô hình dữ liệu, sổ tài khoản, bản nháp bất biến, engine luật, chốt chặn `ToolGate` trong `ToolRegistry.Execute`, OAuth và token. Plugin chỉ dịch giữa ABI JSON (`mail@1`, `calendar@1`) và API của nhà cung cấp.

**Tech stack:** Go (modernc SQLite, chi, wazero), React 19 + i18next, Playwright, harness eval `evals/runner`.

**Spec:** `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md` (mục 16 ghi các chỉnh sau khi dựng thử).

## Các tệp kế hoạch

| # | Tệp | Phát hành được khi |
|:-:|:---|:---|
| 1 | `2026-11-01-mailcal-01-foundation.md` | Agent đọc, tóm tắt, soạn nháp trên provider tham chiếu; bộ tuân thủ ABI xanh (cả WASM thật) |
| 2 | `2026-11-01-mailcal-02-policy-and-send.md` | 0 thư/sự kiện ghi ra ngoài khi chưa có luật hoặc người duyệt; 0 gửi trùng |
| 3 | `2026-11-01-mailcal-03-oauth-gmail.md` | Kết nối Google thật, đọc/soạn/gửi theo luật |
| 4 | `2026-11-01-mailcal-04-graph-caldav.md` | Graph và CalDAV (kèm đường mật khẩu `acton_account`) đạt cùng bộ tuân thủ, không sửa lõi |
| 5 | `2026-11-01-mailcal-05-tls-imap-smtp.md` | Gửi/nhận qua IMAP/SMTP thật (`acton_tcp`, STARTTLS do host chạy); test SSRF/rebinding xanh; vá rebinding của HTTP plugin |
| 6 | `2026-11-01-mailcal-06-polish-evals-docs.md` | Eval văn phòng ≥ 90% trên hai model; hướng dẫn viết plugin |

Các giai đoạn làm **tuần tự**; giai đoạn N chỉ bắt đầu khi cổng hoàn thành của N-1 xanh và đã gộp vào `master`. Giai đoạn 4 và 5 độc lập với nhau sau giai đoạn 3 nhưng không nên làm song song trên cùng một nhánh.

## Mã tham chiếu đã kiểm chứng

Trước khi viết kế hoạch, toàn bộ phần Go của giai đoạn 1-5 và các phần lõi rủi ro nhất đã được dựng và chạy xanh trên các nhánh **cục bộ** (chưa bao giờ đẩy lên remote). Mỗi nhánh là trạng thái cuối của một giai đoạn:

| Nhánh | Nội dung |
|:---|:---|
| `wip/mailcal-ref-p1` | Giai đoạn 1 (kèm `ToolGate` và engine luật dựng sớm để kiểm chứng; kế hoạch GĐ1 **không** lấy chúng) |
| `wip/mailcal-ref-p2` | + chốt chặn, engine luật, kho dữ liệu, thao tác ghi, API (GĐ2) |
| `wip/mailcal-ref-p3` | + `acton_oauth`, broker token, luồng OAuth, `provkit`, plugin Gmail/Calendar (GĐ3) |
| `wip/mailcal-ref-p4` | + `acton_account`, kho mật khẩu, `icskit`, plugin Graph và CalDAV (GĐ4) |
| `wip/mailcal-ref-p5` = `wip/mailcal-reference` | + `security.DialPolicy`, `acton_tcp`, vá HTTP, plugin IMAP/SMTP (GĐ5) |

Mỗi kế hoạch đưa mã vào `master` theo từng phần nhỏ: tệp mới bằng `git checkout wip/mailcal-ref-pN -- <tệp>`, tệp đã có bằng `git diff wip/mailcal-ref-p(N-1) wip/mailcal-ref-pN -- <tệp> | git apply --index`. **Chuỗi lệnh của GĐ1 → GĐ5 đã được chạy thử từ đầu đến cuối trên một bản sao sạch của `master`** và mọi test đều xanh ở cuối mỗi giai đoạn (kể cả bước `go mod tidy`). GĐ6 (eval, vận hành giao diện, hướng dẫn, tài liệu) và các phần web, e2e, nối dây `cmd/actond` của từng giai đoạn **chưa có** mã tham chiếu: mã nằm trong chính kế hoạch.

Các nhánh này là công cụ của kế hoạch, không phải thứ để gộp nguyên khối. Nếu bị mất, tệp giai đoạn vẫn mô tả đủ chữ ký, hành vi và test để viết lại, nhưng tốn công hơn nhiều. Hỏi chủ dự án trước khi xoá chúng (xem GĐ6, Task 5 Step 6).

## Hợp đồng khoá (các giai đoạn sau KHÔNG được đổi)

Đổi bất kỳ mục nào dưới đây là thay đổi phá vỡ; phải sửa spec và mọi kế hoạch sau nó trước.

### C1. ABI plugin v1 (additive-only)

- Tên export (xem `internal/mailcal/abi/abi.go`): `acton_mailcal_capabilities`, `acton_mail_{list,get,get_thread,search,create_draft,send,mark,move,delete}`, `acton_calendar_{list_events,get_event,create_event,update_event,respond,delete_event,free_busy}`.
- Chữ ký mọi export: `(reqPtr, reqLen uint32) -> uint64` đóng gói `(resPtr<<32 | resLen)`; yêu cầu là `abi.Request{account:{id,config}, params}`; trả lời là `abi.Response{result|error}`.
- Mã lỗi: `auth_expired | rate_limited | not_found | unsupported | transient | invalid`.
- Được thêm trường tuỳ chọn và export mới. **Không** đổi tên, bỏ, hay đổi nghĩa. Bảo vệ bằng `internal/mailcal/abi/testdata/golden_v1.json` (`TestGoldenV1`): giai đoạn sau chỉ được thêm vào golden, không sửa dòng cũ.
- Địa chỉ luôn qua `abi.NormalizeEmail` (một hộp thư, chữ thường, IDN → punycode, từ chối tên hiển thị và danh sách).

### C2. Go interfaces

- `mailcal.Provider` (`internal/mailcal/provider.go`) và `plugin.Caller` (`CallJSON`).
- `plugin.CapabilityHook` (`PluginActivated` chạy **sau** khi nhả khoá Manager; `PluginDeactivated` chạy **trong** khoá, không được gọi lại Manager).
- Giai đoạn sau thêm phương thức vào `Service`, không đổi chữ ký hiện có.

### C3. Chốt chặn thực thi (`internal/tools/gate.go`)

- `ToolGate.Check(ctx, agentID, toolName, input) (GateDecision, error)`; `GateAllow | GateRequireApproval | GateDeny`.
- Tool trong `externalEffectTools` luôn rủi ro High, không grant, không tự duyệt, và bị từ chối nếu chưa có gate. Giai đoạn sau **chỉ thêm tên** vào danh sách đó, không bao giờ bớt.
- Gate lỗi hoặc trả giá trị lạ ⇒ từ chối (fail closed).

### C4. Quyết định luật (`internal/mailcal/policy`)

- `Evaluate(rules, Request, now, Usage) Decision` là hàm thuần; mọi lỗi/thiếu dữ kiện ⇒ `needs_approval`; chỉ trả `allow` khi một luật bật, chưa hết hạn, cùng tài khoản và hành động, khớp mọi ràng buộc và còn hạn mức.
- Luật `deny` thắng; nhóm luôn-duyệt (xoá, Bcc, đính kèm, chuyển tiếp) không luật nào mở.
- Luật allow không chấp nhận `*`, miền công cộng (trừ `allow_public_domain`), thiếu hạn mức.

### C5. Tên tool

Đọc: `native_mail_{list,search,get,get_thread}`, `native_calendar_{list,get,free_busy}`. Soạn/sửa hộp thư (Medium): `native_mail_draft`, `native_mail_mark`, `native_mail_move`. Ghi ra ngoài (High, qua gate): `native_mail_send`, `native_mail_delete`, `native_calendar_create_event`, `native_calendar_update_event`, `native_calendar_respond`, `native_calendar_delete_event`. Đề xuất: `native_mailcal_propose_rule` (Medium, chỉ tạo đề xuất).

### C6. Dữ liệu

Bảng: `mailcal_accounts` (config không chứa bí mật; `rejectSecrets`), `mailcal_drafts` (bất biến, có trigger chặn UPDATE và `payload_hash`), giai đoạn 2 thêm `mailcal_rules`, `mailcal_proposals`, `mailcal_decisions`, `mailcal_sent_ledger`, `mailcal_settings`. Chỉ dùng `CREATE TABLE IF NOT EXISTS` và `ALTER TABLE ... ADD COLUMN`; không đổi cột đã có. Không có bảng cache; không lưu nội dung thư.

### C7. Nội dung thư là dữ liệu không tin cậy

Mọi byte từ provider đi qua `SanitizeText` + `WrapUntrusted` trước khi vào ngữ cảnh model. Mọi tool mới trả nội dung ngoài phải dùng lại chúng.

### C8. Bí mật

Token, mật khẩu, refresh token **không bao giờ** nằm trong `config_json`, log, audit, trả lời API hay tin nhắn lỗi. Chỉ `auth_json` giữ tham chiếu (`mailcal:<account_id>` cho token OAuth; id bí mật trong vault cho mật khẩu).

### C9. Module host chỉ được thêm, không được sửa

Module host đã phát hành (`acton_sys`, `acton_net`, `acton_vault`, `acton_storage`, `acton_bus`, `acton_ws`, `acton_workspace`, `acton_host`) không đổi chữ ký hay nghĩa. Dự án thêm ba module mới: `acton_oauth` (GĐ3), `acton_account` (GĐ4), `acton_tcp` (GĐ5). Trường manifest mới (`oauth`, `permissions.oauth`, `permissions.credentials`, `permissions.net_tcp`) đều tuỳ chọn và có `omitempty`, nên manifest cũ phân tích như trước.

## Nguy cơ liên giai đoạn

| Nguy cơ | Xảy ra ở | Cách chặn |
|:---|:---|:---|
| Ngưỡng duyệt, `WithBypassApproval` (cron) hay grant "đừng hỏi lại" bỏ qua duyệt thư | 2 | `ToolGate` trong `Execute`; `TestGateApprovalCannotBeBypassed` |
| Nội dung duyệt khác nội dung gửi (TOCTOU) | 2 | Duyệt gắn `draft_id`, nháp bất biến, hash kiểm lại lúc gửi |
| Gửi trùng khi retry hoặc tiếp tục sau duyệt | 2, 3-5 | Sổ idempotency; provider phải tôn trọng `idempotency_key` (conformance kiểm) |
| `SameSite=Strict` làm callback OAuth mất cookie phiên | 3 | Callback công khai, xác thực bằng `state` dùng một lần |
| Plugin dùng token của tài khoản khác | 3 | `host_get_token` ràng buộc `HostContext.PluginID` với chủ tài khoản |
| Refresh token lọt vào WASM | 3 | Chỉ trả access token ngắn hạn |
| WASM không mở được socket thô | 5 | Host API `tls_*` có allowlist và nối đúng IP đã kiểm |
| DNS rebinding | 5 | Phân giải một lần, nối IP đã kiểm; test bằng resolver giả |
| Guest lặp vô hạn treo daemon | 1 trở đi | `WithCloseOnContextDone(true)` + hạn `pluginCallTimeout` (đã có) |
| Plugin cũ vỡ do host API mới | 3, 5 | Module host mới là module **riêng**; không sửa `acton_host` hiện có |
| Khoá `Manager` bị giữ khi gọi ngược | 1 | Kích hoạt hook sau khi nhả khoá |
| Bão thư / luật chạy quá mức | 2 | Hạn mức, tự tạm dừng, công tắc tổng |

## Quy ước chung cho mọi task

- Commit theo Conventional Commits có phạm vi package. **Không** thêm `Co-Authored-By` hay dòng "Generated with" (quy tắc của dự án).
- Mỗi task kết thúc bằng: `go build ./... && go vet ./... && gofmt -l <đường dẫn đã sửa>` (rỗng) và các test của task. Task có web chạy thêm `cd web && npx tsc --noEmit && npm run lint && npm run check:i18n && npm run audit:i18n`.
- Văn bản giao diện: khoá ở **cả** `web/src/locales/en` và `vi`; không chuỗi cứng; không emoji.
- Thêm endpoint ⇒ cập nhật `internal/server/router.go`, `docs/API.md`, `.agents/rules/source-registry.md`. Thay đổi người dùng thấy ⇒ `CHANGELOG.md` mục `[Unreleased]`.
- Test Go gọi `go test ... -count=1`. Test WASM chậm: dùng module đã biên dịch dùng chung (`refplugintest.Instance`).
- Không đẩy lên remote và không force-push khi chưa có lệnh rõ ràng của chủ dự án.
- Cuối mỗi giai đoạn: chạy cổng của giai đoạn (có trong tệp giai đoạn), cập nhật `CHANGELOG.md`, rồi mới bắt đầu giai đoạn sau.

## Cờ tính năng và hoàn tác

`mailcal` mặc định **không có tài khoản nào**, nên không có tool nào xuất hiện cho agent (`SyncTools` chỉ đăng ký khi có tài khoản phù hợp). Từ giai đoạn 2 có thêm công tắc tổng `mailcal.enabled` (mặc định **tắt** cho tới khi người dùng bật). Hoàn tác một giai đoạn = `git revert` các commit của nó; các bảng mới vô hại nếu để lại vì chỉ được tạo bằng `IF NOT EXISTS`.

## Tự rà soát tổng thể (đã làm)

- **Độ phủ spec:** ABI/mô hình (GĐ1), tool và rủi ro (GĐ1-2), engine luật, duyệt, ledger, công tắc tổng, tự tạm dừng (GĐ2), OAuth và `acton_oauth` (GĐ3), Graph, CalDAV, đường mật khẩu (GĐ4), `acton_tcp`, IMAP/SMTP, vá rebinding (GĐ5), UI, audit, eval, tài liệu (GĐ2, 6). Các điểm lệch so với bản nháp spec ghi ở spec mục 16.
- **Nhất quán tên:** `Service.Decide/Send`, `RuleStore`, `ProposalStore`, `DecisionStore`, `Ledger` (GĐ2); `TokenBroker`, `OAuthFlow`, `VaultClients` (GĐ3); `CredentialBroker` (GĐ4); `security.DialPolicy`, `hostTCPConn` (GĐ5) được định nghĩa ở giai đoạn đầu tiên dùng đến và giữ nguyên tên sau đó (đã kiểm khi chạy thử chuỗi lệnh).
- **Không có chỗ giữ chỗ:** mọi bước có lệnh cụ thể và kết quả mong đợi; các phần chưa có mã tham chiếu (web, e2e, nối dây `main.go`, GĐ6) có mã hoặc đặc tả đủ trong chính tệp.
- **Điểm chưa chắc, để người duyệt quyết:** (1) thư viện IMAP là `emersion/go-imap/v2` ở bản **beta.8** (API có thể đổi trước bản ổn định; đã kiểm chạy trong WASM và qua socket của host, ghim phiên bản trong `go.mod`); (2) bốn giả định chỉ kiểm được trên dịch vụ thật và đã được liệt kê thành bước nghiệm thu thủ công: header idempotency của Gmail có được giữ không, thuộc tính mở rộng của Graph có theo thư sang Sent Items không, máy chủ CalDAV có tự gửi lời mời không, và máy chủ IMAP có giữ header/cờ `\Draft` như giả định không; (3) client HTTP của plugin không còn dùng `HTTP_PROXY` (GĐ5); (4) đường WebSocket của plugin còn cửa sổ rebinding (ngoài phạm vi, ghi vào backlog).
