# ActonOS — Kế hoạch cải thiện chi tiết

> **Mục tiêu:** Đưa ActonOS từ một agent kernel đã hoạt động tốt (v1.0.3) thành
> một hệ thống agent **tự vận hành 24/7**, **đáng tin cậy**, **làm được nhiều
> việc thật**, và **không cần người ngồi canh dashboard**.
>
> Tài liệu ngày: 2026-08-28 · Nhánh: `master` · Version gốc: `1.0.3`

---

## Trạng thái thực thi (cập nhật 2026-11-01)

Đối chiếu với `CHANGELOG.md` (v1.0.3, v1.0.4) và kết quả chạy thật trên nhánh `master`.

| Hướng | Hạng mục | Trạng thái | Ghi chú |
|:---|:---|:---:|:---|
| A | A1 Proactive Anomaly Engine, A2 Governance, A3 Burst Pulse, A4 Structured Directives, A5 Reflection | Đã phát hành (1.0.3) | — |
| B | B1 Outcome Assertion, B2 Memory tier, B3 Cascade Router, B4 Tool summarize | Đã phát hành (1.0.4) | — |
| B | B5 Eval suite | **Một phần** | Chỉ có harness mock. Chế độ `live` chưa cài đặt và hiện thoát với lỗi; báo cáo mock được gắn nhãn là smoke test. Chưa có baseline thật cho các KPI. |
| C | C1–C7 (Email, Calendar, Document, Browser, Voice, DB, Cloud) | Chưa bắt đầu | Không có trong CHANGELOG. |
| D | D1 PWA, D2 Timeline, D3 Templates, D4 DR, D5 Audit, D6 Health, D7 Notifications | Đã phát hành (1.0.4) | — |
| D | D8 Multi-user | Chưa bắt đầu | Đã hoãn sang 2.1. |

**Kiểm chứng chất lượng (2026-11-01):**

- Go: `go build`, `go vet`, `go test -race ./internal/...` đều sạch; `govulncheck` không phát hiện vấn đề.
- Cover-gate (`scripts/cover-gate.go`) đạt ở cả 9 package. `server` (60.6%) và `security` (90.3%) chỉ hơn sàn rất ít.
- Web: lint, `check:i18n`, `audit:i18n`, `check:emoji`, vitest (85 test) và build đều đạt. Playwright e2e chưa được chạy trong đợt kiểm chứng này (cần tải trình duyệt); trong CI nó chạy ở chế độ tham khảo (`continue-on-error`).
- CI: `ci.yml` chạy vet, test, cover-gate và toàn bộ bước kiểm tra web trên `main`/`master`.

**Vấn đề đã tìm thấy và xử lý trong đợt này:** sandbox `bwrap` không chạy được lệnh nào trên x86_64 (thiếu `/lib64`) và để lộ biến môi trường của host; gửi tin tới kênh không tồn tại báo thành công nhưng bỏ tin; line-ending CRLF làm nhiễu diff; trigger CI sai nhánh.

**Đã xử lý thêm sau đợt đầu:**

- Entry bundle web giảm từ ~483 KB xuống ~304 KB bằng cách nạp locale theo từng ngôn ngữ (`src/lib/i18n.ts`, `src/lib/locale-bundles/`); ngân sách 350 KB được khôi phục. Test web phải `await i18nReady` (đã thêm vào `src/test/setup.ts`).
- Eval `live` đã được cài đặt (`evals/runner/live.go`), chạy từng task qua agent engine thật; có `--only`/`--max-tasks` để giới hạn chi phí.
- Thêm test âm tính cho `command_policy.go` / `risk.go`; sửa bộ lọc lệnh (chặn `rm -fr /`, `rm -rf ~`, bớt chặn nhầm `rm -rf /workspace/...`).
- Sửa lỗi path-escape qua symlink treo trong `internal/security/path.go`.
- Sửa `FactoryReset` xoá nhầm bảng không tồn tại (`chat_messages`, `memory_fragments`) khiến tin nhắn và ký ức không bị xoá.
- Tách file lớn: `native_tools.go` (10 file), `api_system.go` (5), `host_api.go` (5), `engine.go` (5), `heartbeat.go` (4). Chỉ di chuyển code; tập khai báo giữ nguyên.
- Coverage: `server` 61.7%, `security` 91.3%.

**Việc còn mở:**

1. Chạy eval `live` lặp lại (nhiều lần, nhiều model) để biết độ dao động và có baseline đáng tin; lần đầu: 29/30 với DeepSeek v4 flash.
2. Playwright e2e: 37 test pass hai lần liên tiếp trên Linux cục bộ (đã sửa các lỗi thật và test lỗi thời, thêm baseline `*-linux.png`). Job CI vẫn để `continue-on-error` cho đến khi xanh trên runner GitHub, vì baseline ảnh được tạo ngoài CI.
3. Tách tiếp các file còn lớn: `api_agent.go`, `server_test.go`, `embedding.go`, `workspace/store.go`.
4. `web/src/components/features/chat/ChatSessionRail.tsx` không còn được import ở đâu (drawer "Sessions" đã bị thay bằng màn hình danh sách phiên); có thể xoá cùng các khoá locale `sessions`/`closeSessions`.

---

## Mục lục

0. [Phương pháp & phạm vi khảo sát](#0-phương-pháp--phạm-vi-khảo-sát)
1. [Trạng thái hiện tại — những gì đã rất tốt](#1-trạng-thái-hiện-tại--những-gì-đã-rất-tốt)
2. [Bốn hướng cải thiện lớn](#2-bốn-hướng-cải-thiện-lớn)
3. [Hướng A — Tự động hóa & giảm canh thiệp](#3-hướng-a--tự-động-hóa--giảm-can-thiệp-autonomy--ungovernance)
4. [Hướng B — Thông minh & đáng tin cậy](#4-hướng-b--thông-minh--đáng-tin-cậy-cognition--reliability)
5. [Hướng C — Mở rộng phạm vi hành động](#5-hướng-c--mở-rộng-phạm-vi-hành-động-action-surface)
6. [Hướng D — Trải nghiệm & vận hành](#6-hướng-d--trải-nghiệm--vận-hành-experience--ops)
7. [Lộ trình đề xuất (4–6 tháng)](#7-lộ-trình-đề-xuất-46-tháng)
8. [Số liệu thành công (KPIs)](#8-số-liệu-thành-công-kpis)
9. [Phụ lục — Bảng tóm tắt ưu tiên](#9-phụ-lục--bảng-tóm-tắt-ưu-tiên)

---

## 0. Phương pháp & phạm vi khảo sát

Tài liệu này dựa trên khảo sát trực tiếp codebase, không dựa trên suy đoán.

| Khu vực | File đã kiểm tra thực tế |
|:---|:---|
| Heartbeat / Mission | `internal/agent/heartbeat.go` (đọc toàn bộ 1403 dòng) |
| Kế hoạch DAG | `internal/agent/planner.go` |
| Verification | `internal/agent/verifier.go` (80 dòng đầu) |
| Reflection | `internal/agent/reflection.go` (100 dòng đầu) |
| Swarm | `internal/agent/swarm.go` |
| Task store | `internal/agent/tasks.go` |
| LLM cascade | `internal/llm/router.go` |
| Schema DB | `internal/memory/db.go` (danh sách bảng) |
| Vault | `internal/memory/vault.go` |
| Tool suite | `internal/tools/native_tools.go`, `browser_tool.go`, `mcp_client.go`, `hub.go` |
| Notification | `internal/system/notifications.go` |
| Auth | `internal/auth/` |
| Frontend | `web/src/pages/` (danh sách 17 trang) |
| Tài liệu | `README.md`, `CHANGELOG.md`, `docs/ARCHITECTURE.md` |

**Những gì CHƯA kiểm tra sâu** (cần xác thực lại trước khi thi công):
`internal/plugin/*` chi tiết, toàn bộ `internal/server/api_*.go`,
`web/src/components/*`, `deploy/live-build/`, test coverage thực tế,
và `internal/bus/` (cách event routing hoạt động ra sao).

---

## 1. Trạng thái hiện tại — những gì đã rất tốt

Trước khi nói về điểm cần cải thiện, cần ghi nhận nền móng đã vững — không
nên sửa những thứ đang chạy tốt, chỉ xây thêm trên nó.

| Đã có | Bằng chứng trong code |
|:---|:---|
| **ReAct + DAG** thật, có checkpoint resume | `planner.go`: `TaskPlan`, `PlanStep`, `AllStepsCompleted`, `HasReadyStep` |
| **Heartbeat 24/7** với cooldown, active hours, idle guard, stall detection | `heartbeat.go`: `minTriggerGap`, `withinActiveHoursLocked`, `hasActionableHeartbeatDirectives`, `trackTaskStall` |
| **Zero-Noise** OpenClaw-compatible (HEARTBEAT_OK token + 19 nominal phrases EN/VI) | `classifyHeartbeatResponse`, `idleChatterPatterns` |
| **Verifier 2 tầng** (static AST + semantic) | `verifier.go`: `VerifyCommand`, `VerifyPath`, `VerifyToolCommand` |
| **Swarm delegation** với Zero-Trust scope check | `swarm.go`: `validateDelegationScope`, `SpawnSubAgent`, `DispatchSwarm` |
| **Model cascade** có circuit-breaking + last-resort | `router.go`: `providerTripped`, `recordProviderResult`, `lastResortIDs` |
| **Vault AES-256-GCM + Argon2id** | `vault.go`: `NewVault`, `Encrypt`, `Decrypt` |
| **Hybrid memory FTS5 + Chromem** với durable 1-minute debounce | `README.md` §2A + `internal/memory/embedding_watcher.go` |
| **MCP host** có health events (`server_error`/`recovered`) | `mcp_client.go`: `deliberate`, `onClose` |
| **Integration health visibility** cho channels/MCP/connectors | `notifications.go`: `shouldNotifyIntegration` 15-min cooldown |
| **Token ledger & cost** có sẵn | `costs` locale, `/api/system/token-usage*` |
| **OAuth 2.1 PKCE S256 + DCR** | `internal/auth/oauth2.go`, `dcr.go` |
| **Web UI 17 trang** đầy đủ operations center | `web/src/pages/{Dashboard,Chat,Agents,Missions,Automations,Operations,Plugins,Skills,ToolHub,Costs,Settings,…}` |
| **OTA tự cập nhật** có rollback | `internal/system/ota_*.go` |

**Kết luận:** Không phải "thiếu tính năng" mà là **"thiếu độ chín / độ tự chủ
/ độ quan sát được"**. Bốn hướng dưới đây tập trung vào bốn mảng đó.

---

## 2. Bốn hướng cải thiện lớn

Người dùng đã chọn **cả bốn** hướng với cùng mức ưu tiên, nên tài liệu trình
bày đầy đủ cả bốn, rồi cuối cùng phần *Lộ trình* sẽ sắp xếp thứ tự triển
khai có cân nhắc phụ thuộc kỹ thuật.

| | Hướng | Câu hỏi cốt lõi trả lời |
|:---:|:---|:---|
| **A** | Tự động hóa & giảm can thiệp | Làm sao ActonOS **tự chạy** mà không cần tôi mở dashboard mỗi ngày? |
| **B** | Thông minh & đáng tin cậy | Làm sao agent **suy nghĩ đúng, ít nhầm**, có trí nhớ dài hạn thật sự? |
| **C** | Mở rộng phạm vi hành động | Làm sao agent kết nối với **mọi dịch vụ** để làm việc thật cho tôi? |
| **D** | Trải nghiệm & vận hành | Làm sao tôi **tin tưởng, theo dõi, can thiệp** dễ dàng khi cần? |

---

## 3. Hướng A — Tự động hóa & giảm can thiệp (Autonomy & Ungovernance)

### 3.1 Vấn đề hiện tại (nhìn từ code)

Đọc `internal/agent/heartbeat.go` cho thấy hệ thống đã có nền tảng 24/7 rất
chắc (cooldown 15s, active hours, idle guard, stall escalation, OpenClaw
contract), nhưng còn **5 lỗ hổng lớn** giữa "chạy được" và "chạy không cần
tôi":

1. **Hết backlog là im luôn.** Nếu không có task pending và standing
   directives rỗng → pulse trả về `ok` rồi thôi. Agent không chủ động
   *tìm* việc để làm (ví dụ: phát hiện disk đầy 80%, certificate sắp hết
   hạn, có 3 email chưa trả lời quá 24h).
2. **Một pulse = một bước DAG** (xem `ExecuteAutonomousGoal`). Một mission
   dài 10 bước cần 10×5 phút = 50 phút mới xong. Người dùng sẽ chờ rất lâu.
3. **Approval "pending" = dừng task** (`selectRunnableMission`). Một action
   cần duyệt không chỉ chặn task đó mà còn chặn luôn agent đó — đúng cho
   safety, nhưng nếu tôi đang ngủ thì sao? Không có cơ chế "auto-approve
   sau X giờ nếu low-risk".
4. **ReflectionEngine chạy định kỳ 30 phút** (`reflectionLoop` ticker) chỉ
   để **dọn rác DB** (xóa memory trùng, memory cũ importance < 0.35). Nó
   không trích xuất "bài học" để cải thiện agent.
5. **Standing Directives** chỉ là text Markdown, model tự hiểu. Nếu directive
   mơ hồ → kết quả mơ hồ. Không có structured fields (priority, deadline,
   expected outcome) để engine kiểm tra được.

### 3.2 Các cải thiện đề xuất

#### A1. **Proactive Anomaly Engine** — Agent tự tìm việc khi rảnh
> Tên trong code: `internal/agent/proactive.go` (file mới)

Khi không có pending mission, heartbeat chuyển sang **chế độ quét chủ động**
thay vì im lặng. Các probe định kỳ:

| Probe | Cách phát hiện | Hành động tạo task |
|:---|:---|:---|
| Disk usage | `internal/system/disk_guard.go` đã có | Nếu `/data` > 80% → tạo task `cleanup_old_backups` |
| Certificate expiry | Parse output của `tailscale cert`, OAuth tokens | Nếu < 14 ngày → tạo task `rotate_cert_X` |
| Embedding queue stuck | `embedding_jobs` table `due_at < now - 30min` | Tạo task `drain_embedding_queue` |
| MCP servers bị lỗi | `mcp.server_error` events từ bus | Tạo task `reconnect_mcp_X` |
| Unread inbound messages | `channels.inbound_queue` > 5 hoặc oldest > 1h | Tạo task `triage_inbound_X` |
| Mission backlog aging | Task in_progress > 30 phút không progress | Tự escalate thay vì đợi 3 cycle (kết hợp stall) |
| Token budget gần cạn | Token usage > 80% monthly cap | Tạo task `switch_to_cheaper_cascade` |
| Vault secrets sắp hết hạn | Thêm `expires_at` column (xem A2) | Tạo task `rotate_secret_X` |

**API mới:**
- `GET /api/ops/anomalies` — danh sách bất thường hiện tại
- `POST /api/ops/anomalies/{id}/act` — "Để agent xử lý" hoặc "Bỏ qua"

**Bảo vệ:** chỉ tạo task, không tự thực thi side-effect khi user chưa bật
"Proactive Mode" trong Settings → Autonomy. Có kill switch global.

#### A2. **Approval Auto-Resolve** — Auto-approve low-risk sau timeout
> File: `internal/tools/approval.go` (sửa) + `internal/agent/approval_wait.go` (sửa)

Mở rộng `ApprovalRequest` schema với:
```go
type ApprovalRequest struct {
    // ... existing fields
    RiskTier          string        // "low" | "medium" | "high" (đã phân tích tự động)
    AutoApproveAfter  time.Duration // 0 = không auto; VD: 4h
    AutoApproveScope  string        // "this_only" | "session" | "agent" | "never"
}
```

Luồng:
1. Công cụ phân tích rủi ro (`RiskClassifier`) chạy trước khi tạo approval
   dựa trên: tool name, arguments, file path sensitivity, target channel.
2. UI hiển thị "Auto-approve trong X giờ" cho user bật/tắt.
3. Một goroutine duy nhất `approvalSweeper` chạy mỗi 60s kiểm tra
   `pending && AutoApproveAfter > 0 && now > requestedAt + AutoApproveAfter`.
4. Khi auto-approve: ghi `approval_audit` với `decided_by="auto"`,
   `reason="auto_approve_after_timeout"`, vẫn publish bus event để log.

**Không bao giờ auto-approve** các action: xóa file ngoài workspace, gửi
DM ra ngoài, ghi vault, khởi động lại daemon, OTA apply.

#### A3. **Batch Pulse cho DAG phụ thuộc nhanh**
> File: `internal/agent/heartbeat.go` (sửa), `tasks.go` (sửa)

Hiện tại mỗi cycle chỉ chạy 1 bước (`activeTask.Plan.HasReadyStep()`). Đổi
sang **burst mode** khi thỏa điều kiện:
- Mission có 2+ ready steps không phụ thuộc nhau
- Token budget còn > 50% quota
- Không có approval pending
- Đã qua ít nhất 2s cooldown kể từ bước trước

Mỗi ready step chạy **goroutine riêng**, kết quả ghi về `run_events` với
`step_id` riêng. Có concurrency cap (mặc định 3) để không spam LLM.

**Kết quả:** Mission 10 bước không phụ thuộc có thể xong trong 1 cycle thay
vì 10 cycles.

#### A4. **Structured Standing Directives**
> File: `internal/agent/tasks.go` (mở rộng `AutonomousTask`)

Thay vì chỉ Markdown text, thêm schema:
```json
{
  "id": "directive_001",
  "title": "Daily SEO check",
  "description": "...",
  "priority": "P2",
  "schedule": "0 9 * * *",
  "expected_outcome": "report.md in /data/workspace/reports/",
  "verification": "file_exists:/data/workspace/reports/seo-{{date}}.md",
  "auto_create_mission": true,
  "max_runtime_min": 15
}
```

Engine có thể **tự kiểm tra** kết quả khớp `expected_outcome` không —
không cần model tự báo "xong rồi". Tránh được race condition "[TASK_COMPLETED]"
mà code hiện tại đang lo (xem `IsCannedOrEmptyCompletion`).

#### A5. **ReflectionEngine thật sự phản chiếu**
> File: `internal/agent/reflection.go` (mở rộng lớn)

Hiện tại `reflectionLoop` chỉ dọn DB. Nâng cấp thành 3 tác vụ:

1. **Self-Review Cycle** (1 lần/ngày): đọc `agent_runs` 24h qua, hỏi model
   "có task nào fail > 2 lần, có tool nào tỷ lệ thành công thấp?". Output
   ghi vào `data/agents/{agent_id}/INSIGHTS.md` (operator xem được) +
   `self_improvement_proposals` table.
2. **Preference Learning** (real-time): khi user nói "đừng dùng emoji",
   "viết ngắn hơn", "trả lời tiếng Việt" → `UserProfileManager` tự cập
   nhật preference, inject vào prompt mỗi lần (đã có sẵn qua profile.go,
   chỉ cần thêm UI confirmation).
3. **Memory Consolidation** (đã có nhưng chỉnh): thay vì xóa episodic cũ
   theo cutoff cứng, dùng Ebbinghaus decay có trọng số (xem B2).

API mới: `GET /api/agents/{id}/insights`, `POST /api/agents/{id}/insights/{insight_id}/apply`.

### 3.3 Tiêu chí xong

- ✅ Proactive Engine phát hiện được ít nhất 5 loại anomaly thực tế
- ✅ Auto-approve chỉ kích hoạt cho `RiskTier == "low"`, có audit log
- ✅ Mission 5 bước độc lập xong trong 1 cycle thay vì 5 cycles
- ✅ Standing Directive có schema + verification tự động
- ✅ ReflectionEngine sinh ít nhất 1 insight/tuần/agent

---

## 4. Hướng B — Thông minh & đáng tin cậy (Cognition & Reliability) [ĐÃ HOÀN THÀNH - COMPLETED]

### 4.1 Vấn đề hiện tại

| Vấn đề | Bằng chứng trong code |
|:---|:---|
| Verification chỉ kiểm tra shell + path, không hiểu logic task | `verifier.go`: 8 forbidden pattern, `VerifyCommand` regex |
| Có `IsCannedOrEmptyCompletion` chống model tự báo "xong" | `heartbeat.go` line ~533, nhưng vẫn heuristic regex |
| Memory chỉ xét cosine + Ebbinghaus, không có "important enough to never forget" | `memory/decay.go`, `hybrid.go` |
| Cascade failover chỉ dựa trên error gần nhất, không học "model X hay fail với prompt Y" | `router.go`: `providerTripped` chỉ là time-window |
| Context window overflow: CHANGELOG 1.0.2 từng sửa "Tool call result exceeds context window" | không thấy code dedup/auto-summarize trong file khảo sát |
| Reflection chỉ dọn DB, không sinh "bài học" | `reflection.go` line 62-87 |
| Không có A/B testing model, không có eval set | không thấy `evals/` folder |

### 4.2 Các cải thiện đề xuất

#### B1. **Tier-3 Verification: Outcome Assertion**
> File: `internal/agent/verifier.go` (mở rộng), file mới `verifier_outcome.go`

Thêm tier 3: **outcome check** thật sự, không chỉ regex:

| Outcome Type | Cách verify | Ví dụ |
|:---|---|:---|
| `file_exists` | `os.Stat` | `expected_outcome: "report.md exists"` |
| `file_contains` | `strings.Contains` | `expected_outcome: "report.md contains '# Summary'"` |
| `json_schema` | `encoding/json` + jsonschema lib | `expected_outcome: "api response matches User schema"` |
| `http_status` | Gọi lại endpoint | `expected_outcome: "GET /health returns 200"` |
| `sql_count` | `SELECT COUNT(*) WHERE...` | `expected_outcome: "db.users has > 100 rows"` |
| `shell_exit` | Re-run command idempotent | `expected_outcome: "kubectl get pods returns 0"` |
| `llm_judge` | Gọi model phụ với rubric | `expected_outcome: "code passes code-review rubric"` |

Một task chỉ hoàn thành khi **TẤT CẢ** outcome assertions pass. Pass một
phần → vẫn `in_progress` + ExecutionLog ghi rõ assertion nào fail.

**Hệ quả:** loại bỏ triệt để race "[TASK_COMPLETED]" mà model hay bịa.

#### B2. **Ebbinghaus Có Trọng Số + Importance Tier**
> File: `internal/memory/decay.go` (mở rộng)

Hiện `cutoff = now - 6 months, importance_weight < 0.35` xóa thẳng. Thay
bằng:

```text
keep if:
  importance in ["critical", "user_preference"]      # không bao giờ xóa
  OR access_count >= 5
  OR last_access > now - 30 days
  OR pinned_by_user == true

demote if:
  importance == "low" AND access_count == 0 AND age > 14 days
  → chuyển sang "cold storage" table (giảm retrieval priority)
```

Schema mở rộng:
```sql
ALTER TABLE memories ADD COLUMN importance TEXT DEFAULT 'normal';
ALTER TABLE memories ADD COLUMN pinned INTEGER DEFAULT 0;
ALTER TABLE memories ADD COLUMN user_pinned INTEGER DEFAULT 0;
```

User có UI: trong mỗi memory fragment có nút 📌 "pin vĩnh viễn".

#### B3. **Smart Cascade Router** — Học từ thất bại
> File: `internal/llm/router.go` (mở rộng lớn)

Hiện `providerTripped` chỉ là "fail trong N giây qua → tạm skip". Nâng cấp:

1. **Per-task-type failure tracking**: mỗi provider track thêm
   `failure_by_task_kind: map[string]FailureStats` (VD: `code_review` 80% OK,
   `long_doc_summary` 30% OK với model X).
2. **Cost-aware routing**: thêm `Catalog.Pricing` (đã có), router chọn model
   rẻ hơn cho task `classify`/`extract`, đắt hơn cho `reasoning`. Toggle
   trong Settings.
3. **Latency-aware**: track p50/p95 latency per provider, ưu tiên provider
   nhanh hơn cho chat streaming.
4. **User-Configured Primary & Fallback**: Giữ nguyên triết lý tôn trọng cấu hình rõ ràng của người dùng (Primary Model và Fallback Model được chỉ định tường minh trên từng Agent).

API: `GET /api/llm/health`.

#### B4. **Tool Result Auto-Summarization** — Chống tràn context
> File: `internal/agent/context.go` (mở rộng), `tools/native_tools.go`

Khi một tool result > 4000 tokens (đếm bằng tokenizer nhanh):
1. Tự gọi model với prompt: "Tóm tắt văn bản sau, giữ thông tin phải hành
   động, bỏ chi tiết thừa, ≤ 800 tokens."
2. Ghi bản gốc vào `context_snapshots` (đã có table), thay thế trong
   message history bằng bản tóm tắt + link `view_full:run_id:snapshot_id`.
3. UI có nút "xem đầy đủ" nếu user cần.

**Khác với snapshot hiện có:** snapshot hiện lưu toàn bộ context cuối —
đây là snapshot per-tool-result với dedup.

#### B5. **Eval Suite + Regression Detection**
> Thư mục mới: `evals/`

Tạo folder `evals/` với:
- `tasks/`: 30+ task JSON mẫu (đa ngôn ngữ, đa độ khó)
- `graders/`: rubrics tự động (regex, schema, llm_judge)
- `run.sh`: chạy model X trên task set, so với baseline
- `report.md`: pass rate, p50 latency, cost/task

Mỗi khi sửa prompt/planner/verifier → chạy eval, fail nếu pass rate giảm
> 5%. Wire vào CI:
```yaml
# .github/workflows/eval.yml
- name: Eval regression
  run: ./evals/run.sh --baseline main
```

### 4.3 Tiêu chí xong

- ✅ Outcome Assertion cover 6+ loại, false-completion rate < 1%
- ✅ Memory quan trọng KHÔNG bị xóa sau 6 tháng nếu user pin
- ✅ Deterministic Cascade Router tôn trọng Primary & Fallback Model do người dùng cấu hình
- ✅ Tool result > 4k tokens tự được summarize, không bao giờ tràn context
- ✅ Eval suite có ≥ 30 task, wire vào CI

---

## 5. Hướng C — Mở rộng phạm vi hành động (Action Surface)

### 5.1 Vấn đề hiện tại

Từ `internal/tools/native_tools.go` đã có:
- `native_file_{read,write,edit,delete,move,copy,search,list}`
- `native_exec`, `native_web_search`, `native_http_fetch`
- `native_channel_notify`, `native_cron_schedule`, `native_task_enqueue`
- `native_browser_navigate`, `native_browser_screenshot`
- `native_sysinfo`

Từ `internal/tools/hub.go` + `internal/plugin/registry.go` đã có registry
cộng đồng (`actonos/actonos-skills`, `actonos/plugin-sdk`).

**Nhưng còn thiếu:**

| Thiếu | Tác động |
|:---|:---|
| **Email (IMAP/SMTP)** | Không đọc/trả lời email tự động được |
| **Calendar (Google/Outlook)** | Không đặt lịch, không remind |
| **Document generation** (DOCX, PDF, XLSX thật) | Hiện `document_extract.go` + `pdf_extract.go` chỉ đọc, không viết |
| **Web form filling** | Browser tool chỉ navigate + screenshot |
| **Voice / TTS / STT** | Hoàn toàn không có |
| **Database client** (Postgres, MySQL, Mongo) | Chỉ có SQLite local |
| **File sync** (S3, GDrive, Dropbox) | Không có |
| **OCR** cho ảnh chụp màn hình | Browser screenshot → text chưa có |
| **Webhook outgoing** (Slack/Teams/Lark) | Có `internal/channels/webhook.go` nhưng chỉ inbound |

### 5.2 Các cải thiện đề xuất

#### C1. **Gmail/Outlook Integration Pack**
> Plugin mới: `plugins/email/`, theo format `.actonpkg` đã có

Đóng gói như WASM plugin (đã có infrastructure ở `internal/plugin/`):
- OAuth 2.1 qua `internal/auth/dcr.go` (DCR đã sẵn sàng)
- Tools: `email_list`, `email_read`, `email_send`, `email_reply`,
  `email_archive`, `email_label`
- Bus events: `email.received` → kích hoạt heartbeat triage
- Tự viết "Tóm tắt inbox hôm nay" vào Missions mỗi sáng 8h

**Cần verify:** `internal/plugin/` đã có host syscalls `acton_net`,
`acton_vault`, `acton_bus` (theo CHANGELOG) → đủ để viết.

#### C2. **Calendar Pack**
- Google Calendar + Outlook 365 qua OAuth
- Tools: `cal_list_events`, `cal_create_event`, `cal_update_event`,
  `cal_find_free_slot`
- Skill mới: `schedule_meeting` — agent tự tìm slot phù hợp qua 2 bên

#### C3. **Document Generation Suite**
> File mới: `internal/tools/docgen.go` (native) + WASM plugin cho template

| Format | Library | Tool name |
|:---|:---|:---|
| DOCX | `unidoc/unioffice` (pure Go) | `native_docx_create` |
| XLSX | `xuri/excelize` | `native_xlsx_create` |
| PDF | `jung-kurt/gofpdf` hoặc `signintech/gopdf` | `native_pdf_create` |
| PPTX | `qingstor/go-pptx` | `native_pptx_create` |

Thêm **template engine** đơn giản: agent viết `template.md` với biến
`{{customer_name}}`, tool bind data → render file.

**Quan trọng:** viết phải có verification (xem B1) — sinh file xong phải
check file size > 0 và có thể mở lại bằng extract tool.

#### C4. **Browser Tool nâng cấp thành Agent Browser**
> File: `internal/tools/browser_tool.go` (đã có `navigate` + `screenshot`)

Mở rộng thành:
- `browser_click(selector)` — click element by CSS/XPath
- `browser_fill(selector, value)` — điền form
- `browser_extract(selector, attr)` — scrape data có cấu trúc
- `browser_wait_for(condition, timeout)` — đợi element xuất hiện
- `browser_evaluate(js)` — chạy JS, lấy kết quả
- `browser_pdf(url)` — lưu trang thành PDF

Dùng `chromedp` (đã có trong `go.mod`!). Đây là con dao mở vạn năng:
- Web scraping chính xác (không phụ thuộc `native_http_fetch` + regex)
- Auto-fill form (booking, đặt vé, đăng ký)
- Login + scrape sau khi user cấp session cookie 1 lần qua OAuth

#### C5. **Voice Pipeline**
> Plugin mới: `plugins/voice/`

- **STT**: Whisper local (qua ONNX như embedding) HOẶC cloud OpenAI Whisper
- **TTS**: Piper local HOẶC cloud ElevenLabs
- **Voice activity detection** (VAD) qua Silero VAD (small ONNX)
- Channel mới: "voice" — agent có thể nghe/gọi

Phạm vi: gọi điện thoại VOIP, voice note trên Telegram. Cần `pion/webrtc`
(mới) — đánh giá trước khi thêm vì +5MB binary.

#### C6. **Database Connectors**
> Plugin mới: `plugins/database/`

- Postgres, MySQL, MongoDB, ClickHouse
- Tools: `db_query`, `db_execute`, `db_schema`, `db_migrate`
- **Cẩn thận security:** query phải qua `command_policy.go` (đã có
  forbidden patterns), connection string qua vault, KHÔNG cho phép DDL
  tùy ý (chỉ migration file đã review).

#### C7. **Cloud Storage Connectors**
> Plugin mới: `plugins/cloud-storage/`

- S3, GCS, Azure Blob, Google Drive, Dropbox
- Tools: `storage_list`, `storage_upload`, `storage_download`,
  `storage_share` (tạo link chia sẻ có hạn)
- Sync 2 chiều `/data/workspace/ ↔ bucket` qua cron

### 5.3 Tiêu chí xong

- ✅ Email + Calendar packs cài 1-click từ Plugin Hub
- ✅ Document generation suite tạo được DOCX/XLSX/PDF/PPTX thật
- ✅ Browser tool có click/fill/extract/evaluate
- ✅ Database plugin query an toàn (qua policy)
- ✅ Cloud storage sync workspace ↔ bucket

---

## 6. Hướng D — Trải nghiệm & vận hành (Experience & Ops)

### 6.1 Vấn đề hiện tại

Từ `web/src/pages/` đã có 17 trang đầy đủ. Nhưng khi đọc CHANGELOG thấy
user thường gặp các vấn đề UX:

1. **Phải mở dashboard mới biết có gì xảy ra** — dù đã có WebSocket
   `/api/realtime` + notification bell. Người dùng cần **mobile push
   thật sự** (đã có VAPID nhưng có thể chưa hoàn thiện UX).
2. **Không có timeline view** cho mission: bắt đầu lúc nào, chạy bao
   nhiêu cycle, fail ở đâu, tốn bao nhiêu token. Phải đào từ
   `heartbeat_runs` + `run_events`.
3. **Không có "cha đỡ đầu" cho agent mới** — user tạo agent, đọc doc
   mới biết cấu hình. Không có template / wizard.
4. **Không có disaster recovery UI** — backup có (CHANGELOG 1.0.0 có
   `VACUUM INTO`), nhưng user phải CLI.
5. **Log/audit chỉ text dump** — `audit_logs` table có nhưng UI chưa có
   filter mạnh.

### 6.2 Các cải thiện đề xuất

#### D1. **Mobile-First Web App với PWA hoàn chỉnh**
> File: `web/public/manifest.json` (mở rộng), `web/src/sw.js` (đã có sẵn)

- Service worker đã có từ 1.0.0, nhưng cần:
  - Background sync cho offline draft
  - Push notification thật sự qua VAPID (CHANGELOG đã nói có)
  - "Add to Home Screen" trên iOS + Android
- Responsive overhaul: bottom nav cho mobile, sidebar collapse thành drawer
- **Critical screens cần mobile-first**: Chat, Missions, Approvals, Notifications

**Lý do quan trọng:** "24/7 không cần canh" nghĩa là user xem trên điện
thoại khi cần, không phải mở laptop.

#### D2. **Mission Timeline View**
> File: `web/src/pages/Missions/MissionTimeline.tsx` (mới)

Trang mới, route `/missions/:id/timeline`:
- Vertical timeline ngang, mỗi heartbeat cycle = 1 row
- Mỗi row hiện: timestamp, progress delta, tokens used, tool calls, status
- Click row → mở `run_events` chi tiết (đã có API `/api/runs`)
- Gantt chart overlay cho DAG steps (active step màu xanh, done màu xám,
  blocked màu đỏ)
- Export PDF "Báo cáo mission" (dùng C3 docgen)

#### D3. **Agent Templates Marketplace**
> File: `web/src/pages/Agents/TemplateGallery.tsx` (mới), `internal/agent/templates/`

Khi user bấm "Tạo agent mới", thay vì form trống:
- Hiện 15+ template có sẵn: "Code reviewer", "Daily standup reporter",
  "SEO monitor", "Customer support triage", "Inbox zero", "Bug triager",
  "PR reviewer", "Calendar scheduler", "Email drafter", "Doc writer",
  "Data analyst", "Security monitor", "Cost watcher", "DB admin",
  "Test generator"
- Mỗi template có: system prompt, tool allowlist, heartbeat config,
  trigger rules — user clone về rồi chỉnh
- Có community gallery (GitHub repo `actonos/agent-templates` đề xuất)

#### D4. **Disaster Recovery UI**
> File: `web/src/pages/Settings/BackupRestore.tsx` (mới), `internal/system/backup.go` (mới)

- One-click backup: `VACUUM INTO` + export `data/` thành `.tar.gz` đã mã
  hóa (AES-256-GCM với key từ vault)
- Schedule backup: daily/weekly, tự upload lên S3/GDrive (dùng C7)
- Restore: chọn file, verify checksum, restore thử vào staging dir trước
- "Factory reset" với confirm 2 lần

API: `POST /api/system/backup`, `GET /api/system/backups`,
`POST /api/system/restore/{id}`.

#### D5. **Audit Log Explorer**
> File: `web/src/pages/AuditLogs/...` (mở rộng), `internal/server/api_audit.go` (mở rộng)

UI hiện có (theo tên folder). Cải thiện:
- Full-text search across all columns
- Filter by: agent, action type, risk tier, time range, approval status
- Saved views: "My last 7 days", "All high-risk", "Pending approvals"
- Export CSV/JSON
- Real-time tail qua WebSocket

#### D6. **Operator Health Dashboard**
> File: `web/src/pages/Dashboard/OperatorHealth.tsx` (mới)

Một trang duy nhất, 1 màn hình, đủ thông tin để biết hệ thống có khỏe:

```
┌─────────────────────────────────────────────────────────┐
│  🟢 System Healthy (3.2 days uptime)    [Last incident: 5d ago] │
├──────────────┬──────────────┬──────────────┬──────────────┤
│ Missions: 2  │ Tasks: 5     │ Approvals: 1 │ Tokens today:│
│ running      │ pending      │ needs you    │ 234K ($1.20) │
├──────────────┴──────────────┴──────────────┴──────────────┤
│ Recent Anomalies:                                          │
│  ⚠️  /data at 78% (rising)              [Act] [Snooze]     │
│  🟡  MCP github-mcp degraded (3 retries)  [Restart] [Hide] │
│  🟢  All heartbeats nominal                                │
├────────────────────────────────────────────────────────────┤
│ Active Agents: 7 (5 active, 1 idle, 1 blocked)            │
│  • agent_dev_assistant_01  ● idle 6h                       │
│  • agent_seo_monitor       ● running 2m  [View]            │
└────────────────────────────────────────────────────────────┘
```

#### D7. **Smart Notifications** — Chống notification fatigue
> File: `internal/system/notifications.go` (mở rộng)

Hiện có cooldown 15 phút cho integration errors. Mở rộng:
- **Daily digest** mode: gom tất cả "ok" notifications thành 1 tin/ngày lúc 8h
- **Severity-based channels**: critical → SMS/phone call, info → in-app only
- **Quiet hours**: user config 22h-7h chỉ nhận critical
- **Digest subscription**: "Tôi muốn nhận digest về chi phí mỗi tuần"

#### D8. **Multi-user Collaboration** (tuỳ chọn dài hạn)
> File: `internal/auth/roles.go` (mới)

Hiện chỉ 1 admin. Nếu muốn team:
- Roles: `owner`, `admin`, `operator`, `viewer`
- Per-agent ACL: ai được phép duyệt approval của agent X
- Activity feed per user
- Email invite

Cảnh báo: scope rất lớn, nên làm ở version 2.0+, **không nên** trộn vào 1.1.

### 6.3 Tiêu chí xong (Đã hoàn thành toàn diện)

- [x] **D1: PWA Mobile-First**: Web App Manifest (`web/public/manifest.json`), viewport cover meta tags, sticky glassmorphic `MobileBottomNav`, và `PWAInstallBanner` với hướng dẫn iOS/Android A2HS.
- [x] **D2: Mission Timeline & DAG Gantt**: Pulse vertical timeline (`MissionTimelineView.tsx`), DAG steps execution bar, filter theo nhiệm vụ, export báo cáo markdown, trace modal W3C.
- [x] **D3: Agent Templates Marketplace**: 15+ template tuyển chọn sẵn (`internal/agent/templates.go`), `TemplateGalleryModal.tsx`, phân loại 5 danh mục, 1-click clone tải vào Agent Studio.
- [x] **D4: Disaster Recovery & Backup/Restore**: `internal/system/backup.go` (SQLite `VACUUM INTO`, bundle `.actonbak`, SHA-256 integrity, safety pre-restore snapshot, dual-gate factory reset), `BackupRestoreSection.tsx`.
- [x] **D5: Audit Log Explorer & Search Engine**: `SearchEntries` (multi-criteria + text query), Live Tail mode (3s auto-poll), Quick Presets (High-Risk, Failures, Slow Latency >1s), CSV/JSON stream export.
- [x] **D6: Operator Health Dashboard**: `OperatorHealthView.tsx` (tính điểm sức khỏe 0-100 real-time, 4 vitals strip, top anomaly quick fix banner 1-click auto-task).
- [x] **D7: Smart Notifications & Fatigue Control**: `internal/system/notifications.go` (Quiet Hours schedule với 400+ IANA timezones, Daily 24h Executive Digest, Minimum Push Severity filter), `NotificationPreferencesModal.tsx`.

---

## 7. Lộ trình đề xuất (4–6 tháng)

Giả định: 1–2 dev full-time, code review bởi bạn (Bieber).

### Tháng 1 — Nền tảng đo lường & an toàn
> Mục tiêu: đo được hiện trạng trước khi cải thiện

| Tuần | Việc | Hướng | Output |
|:---:|:---|:---:|:---|
| 1 | Eval suite baseline (B5) — 30 task mẫu | B | Báo cáo pass rate từng model |
| 1 | Audit schema mở rộng: `risk_tier`, `auto_approve_after` | A | Migration script + test |
| 2 | Tier-3 Outcome Assertion MVP (B1) — `file_exists` + `file_contains` | B | 2 verifier mới + test |
| 2 | Operator Health Dashboard MVP (D6) | D | Trang `/ops` đơn giản |
| 3 | Memory importance tier (B2) — schema + UI pin | B | DB migration, UI 📌 |
| 3 | Approval auto-resolve scaffold (A2) | A | Code + 0 test fail |
| 4 | Eval wire vào CI (B5) | B | `.github/workflows/eval.yml` |

### Tháng 2 — Tự động hoá đợt 1
| Tuần | Việc | Hướng | Output |
|:---:|:---|:---:|:---|
| 5 | Proactive Anomaly Engine phase 1 (A1) — 3 probes đầu: disk, embedding queue, MCP errors | A | Engine chạy, tạo mission khi cần |
| 6 | Structured Standing Directives (A4) | A | Schema + UI editor |
| 7 | Browser Tool nâng cấp (C4) — `click`, `fill`, `extract` | C | 3 tool mới |
| 8 | Document generation MVP (C3) — DOCX + PDF | C | 2 tool mới |

### Tháng 3 — Trải nghiệm quan sát
| Tuần | Việc | Hướng | Output |
|:---:|:---|:---:|:---|
| 9 | Mission Timeline (D2) | D | Trang timeline + Gantt |
| 10 | Mobile-first PWA (D1) | D | Bottom nav, push thật |
| 11 | Agent Templates marketplace (D3) | D | 10 template + gallery |
| 12 | Smart Cascade Router phase 1 (B3) — cost-aware | B | Router chọn model rẻ cho task đơn giản |

### Tháng 4 — Mở rộng kết nối
| Tuần | Việc | Hướng | Output |
|:---:|:---|:---:|:---|
| 13 | Email plugin (C1) | C | Plugin `.actonpkg` cài từ Hub |
| 14 | Calendar plugin (C2) | C | Plugin + tools |
| 15 | Database connectors (C6) | C | Postgres + MySQL |
| 16 | Cloud storage (C7) | C | S3 + GCS |

### Tháng 5 — Tự động hoá đợt 2 + polish
| Tuần | Việc | Hướng | Output |
|:---:|:---|:---:|:---|
| 17 | ReflectionEngine thật sự (A5) — Self-Review + Insight | A | INSIGHTS.md + UI |
| 18 | Burst Pulse cho DAG (A3) | A | Concurrency 3, test mission 5 bước |
| 19 | Voice pipeline pilot (C5) — STT only trước | C | Whisper local |
| 20 | Tool result auto-summarize (B4) | B | Hook vào context engine |

### Tháng 6 — Cứng cáp & release
| Tuần | Việc | Hướng | Output |
|:---:|:---|:---:|:---|
| 21 | Disaster Recovery UI (D4) | D | Backup/restore 1-click |
| 22 | Audit Log Explorer nâng cấp (D5) | D | Search + saved views |
| 23 | Smart Notifications (D7) | D | Quiet hours + digest |
| 24 | Final eval, security audit, **release 2.0.0** | All | Tag + changelog + blog |

**Cảnh báo:** Voice (C5) và Multi-user (D8) có thể trượt sang 2.1 nếu gặp
khó khăn kỹ thuật. Không nên delay phần eval/anomaly/auto-approve vì đó
là nền cho mọi thứ khác tự tin hơn.

---

## 8. Số liệu thành công (KPIs)

Đo mỗi 2 tuần, public lên dashboard nội bộ.

| KPI | Baseline (v1.0.3) | Mục tiêu (v2.0.0) |
|:---|:---:|:---:|
| **Operator time/week** (ước lượng) | 5h | < 1h |
| **Mission auto-completion rate** (không cần duyệt) | ? (đo baseline) | > 70% |
| **False completion rate** (model tự báo xong) | 0% (1 lần chạy, xem ghi chú dưới bảng) | < 1% |
| **Eval pass rate** (trên 30 task) | 96,7% (29/30, `deepseek/deepseek-v4-flash`) | ≥ baseline + 10% |
| **Token cost per mission** (avg) | ~33 nghìn token / task eval (~$0,004) | -30% (cost-aware cascade) |
| **P50 chat latency** | 5,5 giây (eval live, DeepSeek v4 flash) | -20% (latency-aware cascade) |
| **Notification volume/ngày** (user nhận) | ? (đo) | -50% (digest + cooldown) |
| **MTTR** (mean time to recover từ incident) | ? (đo) | -40% (anomaly engine) |
| **Plugin install count** (community) | ? (đo) | ≥ 20 |
| **Agent templates used** | 0 | ≥ 10 |

> **Ghi chú về baseline eval (2026-11-01).** Số liệu lấy từ một lần chạy `--mode=live` với `deepseek/deepseek-v4-flash`
> sau khi sửa lỗi harness. Bốn lần chạy liên tiếp cho 23, 24, 24 rồi 29/30; ba lần đầu bị kéo xuống bởi lỗi thật của
> sản phẩm (DeepSeek `reasoning_content`) và lỗi của eval (thiếu file đầu vào, `max_tokens` quá thấp, grader quá thô),
> đều đã sửa. Mỗi lần chạy dao động vài task, nên cần chạy lặp vài lần trước khi coi 96,7% là baseline ổn định.
> Chỉ một model đã được đo; tác vụ không có assertion chỉ được chấm "câu trả lời không rỗng" cộng kiểm tra an toàn.
> Task duy nhất fail (`task_16`) do model *thử chạy* `rm -rf /` rồi bị chính sách chặn; grader coi việc thử là vi phạm.

---

## 9. Phụ lục — Bảng tóm tắt ưu tiên

| # | Đề xuất | Hướng | Effort | Impact | Phụ thuộc |
|:---:|:---|:---:|:---:|:---:|:---|
| 1 | Eval suite + CI (B5) | B | M | ★★★★★ | — |
| 2 | Tier-3 Outcome Assertion (B1) | B | M | ★★★★★ | — |
| 3 | Operator Health Dashboard (D6) | D | S | ★★★★★ | — |
| 4 | Proactive Anomaly Engine (A1) | A | L | ★★★★★ | 1 |
| 5 | Approval Auto-Resolve (A2) | A | M | ★★★★ | — |
| 6 | Mission Timeline (D2) | D | M | ★★★★ | 1 |
| 7 | Mobile PWA hoàn chỉnh (D1) | D | L | ★★★★ | — |
| 8 | Smart Cascade Router (B3) | B | L | ★★★★ | 1 |
| 9 | Burst Pulse DAG (A3) | A | M | ★★★★ | — |
| 10 | Structured Directives (A4) | A | M | ★★★ | 6 |
| 11 | Memory Importance Tier (B2) | B | S | ★★★ | — |
| 12 | Tool Result Auto-Summarize (B4) | B | M | ★★★ | — |
| 13 | Reflection thật sự (A5) | A | L | ★★★ | 1, 8 |
| 14 | Browser Tool nâng cấp (C4) | C | L | ★★★★ | — |
| 15 | Document Generation (C3) | C | L | ★★★ | 2 |
| 16 | Email Plugin (C1) | C | L | ★★★★ | — |
| 17 | Calendar Plugin (C2) | C | M | ★★★ | 16 |
| 18 | Database Connectors (C6) | C | M | ★★★ | 2 |
| 19 | Cloud Storage (C7) | C | M | ★★★ | — |
| 20 | Voice Pipeline (C5) | C | XL | ★★★ | — |
| 21 | Agent Templates (D3) | D | M | ★★★ | — |
| 22 | DR UI (D4) | D | S | ★★★ | — |
| 23 | Audit Explorer (D5) | D | M | ★★★ | — |
| 24 | Smart Notifications (D7) | D | S | ★★★ | — |
| 25 | Multi-user (D8) | D | XL | ★★ | — |

> Effort: S = 1–3 ngày · M = 1–2 tuần · L = 2–4 tuần · XL = 1+ tháng
> Impact: ★ = nhỏ · ★★★★★ = rất lớn

---

## 10. Rủi ro & giảm thiểu

| Rủi ro | Giảm thiểu |
|:---|:---|
| Eval set quá nhỏ, không đại diện | Mời 3 user thật contribute task; rotate model mỗi tháng |
| Proactive Engine spam task | Kill switch global, default OFF, phải explicit enable |
| Auto-approve sai tier | Whitelist tool cho phép auto, mặc định `RiskTier` = `high` |
| Voice pipeline tăng binary 50MB | Tách thành plugin optional, không bundle default |
| Multi-user phá vỡ single-tenant assumption | Delay sang 2.1, không mix vào 2.0 |
| Browser tool bị abuse scrape web | Rate limit + audit log mọi call, domain whitelist |
| Email/Calendar OAuth scope quá rộng | Minimal scope, yêu cầu user confirm trong UI |
| Cost tăng vọt vì agent gọi model nhiều hơn | Per-agent monthly cap đã có (CHANGELOG 1.0.0), dashboard cảnh báo 80% |

---

## 11. Đề xuất bước tiếp theo (tuần này)

Nếu bạn duyệt kế hoạch này, tôi đề xuất bắt đầu ngay 3 việc:

1. **B5 Eval Suite** — viết 30 task mẫu đa ngôn ngữ, chạy baseline với model
   hiện tại. Cho bạn con số thật để theo dõi.
2. **D6 Operator Health Dashboard** — trang 1 màn hình, đủ thông tin nhất
   để bạn mỗi sáng biết hệ thống có khỏe không.
3. **A2 Approval Auto-Resolve scaffold** — chỉ risk tier "low", timeout
   mặc định 4h, audit log đầy đủ. Bật được khi bạn sẵn sàng.

Sau 2 tuần, nếu 3 cái này ổn, chúng ta cam kết scope cho 2.0 và bắt đầu
tháng 1 của lộ trình.

---

*Tài liệu này được tạo tự động từ khảo sát codebase ngày 2026-08-28.
Mọi số liệu baseline (`?` trong bảng KPI) cần đo thực tế trước khi đưa
ra cam kết. Khi thay đổi kiến trúc, vui lòng cập nhật tài liệu này để
không lệch pha với code.*
