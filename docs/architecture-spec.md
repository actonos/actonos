# **TÀI LIỆU ĐẶC TẢ THIẾT KẾ & KIẾN TRÚC KỸ THUẬT TOÀN DIỆN**

# **HỆ ĐIỀU HÀNH AI AGENT TỰ HÀNH ĐA NHIỆM: ACTONOS (EXTENSIBLE OS ENGINE)**

**ActonOS** là hệ điều hành thiết bị chuyên dụng (Single-Purpose Appliance OS) được thiết kế làm **Nền Tảng Kernel Tự Hành Tùy Biến (Extensible Agent OS Kernel)** vận hành 24/7. Hệ thống không mã hóa cứng các vai trò (roles) hay tác vụ cố định, mà cung cấp cơ hạ tầng linh hoạt tuyệt đối cho phép người dùng tự khởi tạo, cấu hình, gán quyền và mở rộng bất kỳ AI Agent nào với các bộ Skill, MCP Server, WASM Plugin và SaaS Connector tùy chọn.  
Toàn bộ hệ thống được đóng gói thành một file nhị phân tĩnh duy nhất (actond) viết bằng **Golang**, hỗ trợ triển khai cắm-là-chạy trên phần cứng **MiniPC (Bare-metal)** hoặc dưới dạng **Docker Container** trên hạ tầng Cloud/NAS.

## **1\. Triết lý Thiết kế & Nguyên tắc Nền tảng**

* **Single Static Binary (actond):** Toàn bộ Agent Core Orchestrator, Dynamic Agent Engine, Web Server, Onboarding Portal, Database, Hybrid RAG, Integration Hub và Web UI được biên dịch thành duy nhất 1 file nhị phân tĩnh (CGO\_ENABLED=0).  
* **Tiêu hao Tài nguyên Tối thiểu:** Mức chiếm dụng RAM khi chạy nền (idle) từ **20MB – 40MB**, thời gian khởi động lên Web UI dưới 2 giây trên CPU Intel N-Series (N100/N95) hoặc AMD Ryzen.  
* **Kiến trúc Khởi Tạo & Tùy Biến Đa Agent (Universal Agent Engine):** Không giới hạn số lượng Agent. Người dùng tự định nghĩa Persona, System Prompt, Bộ Công cụ (Tools), Scope Ủy quyền và Mô hình LLM cho từng Agent thông qua giao diện Dashboard hoặc REST API/JSON Schema.  
* **Multi-Agent Swarm & Delegation Engine:** Hỗ trợ mô hình Agent-to-Agent Delegation qua Goroutines. Một Agent chính có thể kích hoạt các Sub-Agent chuyên biệt do người dùng tự thiết lập để xử lý song song chuỗi công việc dài hạn.  
* **Mô hình Dual-Runtime:** Tự động nhận diện môi trường qua lớp Hardware Abstraction Layer (HAL) để bật các module quản lý phần cứng (Wi-Fi Hotspot, D-Bus) hoặc chạy chế độ Container ảo hóa.  
* **Hệ điều hành Bất biến (Stateless OS \+ Stateful Data):** Phân vùng hệ thống dạng Read-Only; dữ liệu người dùng và cấu hình Agent nằm hoàn toàn tại /data. Cơ chế cập nhật Atomic Swap 1 file duy nhất với tính năng Watchdog tự phục hồi (Rollback) chống brick thiết bị.  
* **Chuẩn hóa Giao thức Quốc tế:** Tích hợp sâu **Model Context Protocol (MCP)**1, **OAuth 2.1 với PKCE (S256)**4, **WebAssembly (WASM)**, và **Tailscale nhúng (tsnet)**.

## **2\. Cơ Sở Khoa Học & Mô Hình Tính Toán Nhận Thức**

ActonOS cung cấp hạ tầng nhận thức tự điều chỉnh cho mọi Agent do người dùng tạo ra:

| Phân tầng Bộ nhớ | Loại Dữ liệu Lưu trữ | Cơ chế Vận hành & Lưu trữ |
| :---- | :---- | :---- |
| **1\. Working Memory** | Scratchpad, trạng thái tác vụ hiện tại, biến tạm, kết quả Tool calls | Lưu trực tiếp trong RAM (Goroutine Context), giải phóng ngay khi hoàn thành Task. |
| **2\. User Profile Memory** | Hồ sơ người dùng, phong cách giao tiếp, quy ước đặt tên, sở thích | Trích xuất tự động qua Async Reflection, lưu dưới dạng Key-Value JSON & SQLite table. |
| **3\. Procedural Memory** | Lịch sử xử lý lỗi, chuỗi lệnh tối ưu cho từng bài toán (Best Practices) | Lưu dạng Workflow Pattern, được nhúng vào System Prompt khi phát hiện task tương tự. |
| **4\. Episodic Memory** | Nhật ký các cuộc hội thoại và tác vụ trong quá khứ kèm mốc thời gian | Lưu trên SQLite FTS5 kết hợp Chromem Vector Indexing. |

### **A. Mô hình Trí nhớ Suy giảm theo Đường cong Quên lãng Ebbinghaus**

Mỗi mẩu ký ức ![][image1] được tính toán điểm truy xuất ![][image2] đối với câu truy vấn ![][image3] tại thời điểm ![][image4]:  
![][image5]  
*Trong đó:*

* *![][image6]*: Hệ số suy giảm theo thời gian (![][image7] là khoảng thời gian từ lần truy xuất cuối; ![][image8] là tốc độ suy giảm).  
* ![][image9]: Trọng số tầm quan trọng (gán nhãn nội tại lúc trích xuất).  
* ![][image10]: Độ tương đồng cosin giữa vector embedding của truy vấn và ký ức.  
* ![][image11]: Các trọng số chuẩn hóa thỏa mãn ![][image12].

### **B. Cơ chế Ra Quyết định Phân nhánh theo Độ Bất định (Uncertainty-Gated Search)**

Để tối ưu chi phí token và độ trễ, Agent hoạt động như một hệ thống POMDP thích ứng dựa trên Entropy quyết định:  
![][image13]

* **Khi Entropy ![][image14] (Độ tự tin cao):** Thực thi theo mô hình **Greedy ReAct 1-bước** để đạt độ trễ cực thấp (![][image15]).  
* **Khi Entropy ![][image16] (Nhiệm vụ phức tạp / Dữ liệu mơ hồ):** Kích hoạt **Tree-of-Thoughts / Language Agent Tree Search (LATS)** để tìm kiếm đường đi tối ưu theo hàm thưởng (Reward Function).

### **C. Hợp nhất Truy xuất Lai Chuẩn hóa (Calibrated Hybrid Retrieval)**

Hợp nhất điểm số Lexical Search (SQLite FTS5) và Semantic Search (Dense Vector) bằng chuẩn hóa Sigmoid:  
![][image17]

### **D. Hệ thống Xác thực Đa tầng Xác định (Deterministic Verification)**

> 1. **Tầng 1: Deterministic Static Analysis (Pure Go \- Độ trễ \~0ms)**  
   * AST Parser: Phân tích cú pháp Shell/Python/JSON/SQL, chặn lệnh cấm trước khi chạy.  
   * Invariant Checker: Kiểm tra đường dẫn có thoát khỏi /workspace hay không.  
   * Schema Validator: Kiểm tra JSON output khớp 100% Type Definition.  
   * *Nếu vi phạm:* Chặn lập tức, trả lỗi cú pháp chính xác về LLM để sửa lại.  
> 2. **Tầng 2: Semantic Verification (Kích hoạt cho tác vụ logic ngôn ngữ)**  
   * Kiểm tra tính nhất quán nội dung phản hồi so với User Profile và yêu cầu ban đầu.

## **3\. Kiến trúc Hệ thống Toàn diện (Master Architecture)**

| Phân tầng Kiến trúc | Thành phần Chi tiết & Dịch vụ Vận hành |
| :---- | :---- |
| **1\. Tầng Giao tiếp & Kết nối** | \- **Native Tsnet:** Node Tailscale Mesh VPN nhúng trực tiếp, hỗ trợ truy cập từ xa E2E không cần mở port modem. \- **Web UI SPA:** React 19 / Tailwind v4 qua go:embed \+ Layered Asset Override. \- **Event-Driven Bus:** Telegram, Discord, Slack, WhatsApp, Webhooks, MQTT Adapters. \- **Zero-Config Portal:** Captive Portal DNS Hijack tại 192.168.4.1 / acton.local. |
| **2\. Dynamic Agent Framework & Swarm** | \- **Universal Agent Configurator:** Cho phép tạo Agent không giới hạn (Custom Persona, Instructions, Model Selection, Tool Binding). \- **Swarm Orchestrator:** Phân rã tác vụ và điều phối Sub-Agents chạy song song qua Goroutines. \- **Zero-Trust Delegation:** Phân quyền Scope-based cho từng Agent (Grant Once, Autonomous Execution). |
| **3\. Enterprise Auth & Integrations** | \- **OAuth 2.1 Provider Engine:** PKCE Flow (S256), Dynamic Client Registration (DCR)4. \- **Token Refresh Daemon:** Tự động duy trì và làm mới Bearer Tokens 5 phút trước khi hết hạn5. \- **SaaS Connectors via MCP/APIs:** Google Workspace (Gmail/Drive/Calendar)6, Notion4, GitHub3, Slack6, Salesforce6, Databases5. |
| **4\. Dynamic Tooling Hub** | \- **MCP Host Engine:** Tích hợp MCP Servers chuẩn (stdio/SSE)2. \- **WASM Runtime:** tetratelabs/wazero thực thi các Micro-Plugins cách ly. \- **Skill-as-a-Folder:** Nạp nóng các thư mục Skill script (JSON Schema \+ Executable) qua fsnotify. |
| **5\. Acton Daemon Core (actond)** | \- **Unified Event Bus:** Async Channel Router điều phối tin nhắn hai chiều. \- **ReAct Orchestrator:** Plan-and-Solve Loop, Token Pruner, Context Compactor. \- **Model Cascade Router:** Primary Model ![][image18] Fallback Chain (Lỗi 429\) ![][image18] Local Ollama. \- **Hybrid Memory Engine:** SQLite FTS5 (Lexical) \+ Chromem-go (Vector) RRF. \- **Hardware-bound Vault:** AES-256-GCM (Argon2id \+ DMI UUID \+ CPU Serial \+ Salt). |
| **6\. Lớp Trừu tượng Hardware (HAL)** | \- **Bare-metal MiniPC Mode:** NetworkManager D-Bus (Hotspot/Client), Sandbox Bubblewrap (bwrap)9 \+ Cgroups v2, Hardware Stats (/sys, D-Bus), OTA Engine Watchdog. \- **Docker Container Mode:** Host/Bridge Network Pass-through, Sandbox WASM (wazero) / Jailed Exec, Container Metrics API, Docker Image Pull. |
| **7\. Nền tảng Gốc** | \- **Bare-metal:** Debian 12 Minimal (Kernel 6.x LTS, Non-free Wi-Fi/NIC Drivers). \- **Container:** Alpine Linux Minimal Base (![][image19] image size). |

## **4\. Khung Khởi Tạo & Quản Lý Agent Tùy Biến (Universal Agent Framework)**

ActonOS cung cấp môi trường quản lý và vận hành Agent hoàn toàn tĩnh và cấu hình động. Người dùng có quyền tự tạo không giới hạn số lượng Agent phục vụ bất kỳ mục đích nào (Từ trợ lý học tập cá nhân, lập trình viên, nhà thiết kế, cho đến chuyên viên phân tích tài chính).

### **A. Cấu Trúc Khai Báo Agent (Agent Schema Manifest)**

Mỗi Agent được định nghĩa thông qua file JSON/YAML hoặc giao diện Web Dashboard:

JSON  
{  
  "agent\_id": "agent\_dev\_assistant\_01",  
  "name": "Senior Software Architect",  
  "description": "Chuyên gia phân tích kiến trúc, viết mã nguồn và tự động kiểm thử",  
  "avatar\_icon": "code-bracket",  
  "model\_config": {  
    "primary\_model": "anthropic/claude-3-7-sonnet",  
    "fallback\_model": "google/gemini-2.5-flash",  
    "temperature": 0.2  
  },  
  "system\_instructions": "Bạn là một Kỹ sư Phần mềm cấp cao. Hãy luôn kiểm tra cú pháp mã nguồn, chạy unit tests trong Sandbox trước khi phản hồi...",  
  "authorized\_tools": \[  
    "mcp\_github\_\*",  
    "wasm\_code\_formatter",  
    "skill\_run\_bash",  
    "native\_file\_ops"  
  \],  
  "delegation\_scope": {  
    "max\_monthly\_budget\_usd": 100.0,  
    "allowed\_workspace\_paths": \["/data/workspace/project\_alpha/"\],  
    "require\_human\_approval\_level": "High"  
  },  
  "trigger\_rules": \[  
    { "type": "channel\_mention", "channel": "telegram", "filter": "@dev\_bot" },  
    { "type": "cron\_schedule", "expression": "0 8 \* \* 1-5" }  
  \]  
}

### **B. Điều Phối Luồng Đa Agent (Multi-Agent Swarm Engine)**

Hệ thống cho phép các Agent tự kết nối và làm việc nhóm:  
\[NGƯỜI DÙNG GỬI YÊU CẦU PHỨC TẠP\]  
│  
▼  
┌───────────────────────────────────────────────────────────────┐  
│ AGENT CHÍNH (Orchestration Agent) │  
│ \- Phân rã yêu cầu thành các tác vụ nhỏ (Sub-tasks) │  
│ \- Xác định Agent chuyên trách phù hợp trong Hệ thống │  
└───────────────────────┬───────────────────────────────────────┘  
│  
┌───────────────┼───────────────┐  
▼ ▼ ▼  
┌──────────────┐┌──────────────┐┌──────────────┐  
│ SUB-AGENT A ││ SUB-AGENT B ││ SUB-AGENT C │  
│ (Xử lý Code) ││ (Xử lý Data) ││ (Viết Report)│  
└───────┬──────┘└───────┬──────┘└───────┬──────┘  
│ │ │  
└───────────────┼───────────────┘  
▼  
┌───────────────────────────────────────────────────────────────┐  
│ AGENT CHÍNH │  
│ \- Hợp nhất kết quả từ các Sub-Agents │  
│ \- Kiểm định chất lượng (Verification) \-\> Phản hồi Người dùng │  
└───────────────────────────────────────────────────────────────┘

## **5\. Mở Rộng Kỹ Năng & Công Cụ Động (Dynamic Tooling Hub)**

ActonOS cho phép nạp thêm bất kỳ công cụ nào mà người dùng cần chỉ bằng thao tác bấm chọn hoặc kéo thả file.  
\+-----------------------------------------------------------------------------------+  
| ACTONOS TOOL REGISTRY |  
\+-----------------------------------------------------------------------------------+  
| \[TẦNG 1: MCP HOST ENGINE\] |  
| \- Kết nối các MCP Server chuẩn qua stdio (local binary) hoặc SSE (mạng Internet) |  
| \- Tự động chuyển đổi MCP Tool Schema sang định dạng Tool Call của LLM |  
\+-----------------------------------------------------------------------------------+  
| \[TẦNG 2: WEBASSEMBLY RUNTIME (wazero)\] |  
| \- Nạp các file .wasm trong thư mục /data/plugins/ |  
| \- Chạy an toàn tuyệt đối trong RAM, không cần quyền truy cập OS |  
\+-----------------------------------------------------------------------------------+  
| \[TẦNG 3: SKILL-AS-A-FOLDER (fsnotify)\] |  
| \- Quét thư mục /data/skills/\<tên\_skill\>/ |  
| \- Đọc skill.json (mô tả tool) \+ run.sh hoặc run.py (script thực thi) |  
| \- Tự động nạp nóng (Hot-reload) ngay khi người dùng thêm file mới |  
\+-----------------------------------------------------------------------------------+

### **Phân Hệ Xác Thực OAuth 2.1 & Token Refresh Daemon**

Mọi kết nối tới các dịch vụ SaaS (Gmail, Notion, Figma, GitHub) đều thông qua chuẩn OAuth 2.1 với **PKCE (S256)**. Tiến trình token\_refresher.go tự động duy trì tính liên tục của kết nối bằng cách tự động đổi Access Token mới trước khi hết hạn 5 phút.

## **6\. Phân Cấp An Toàn, Sandboxing & Audit Logging**

### **A. Sandbox Thực thi Lệnh (Bubblewrap \+ Cgroups v2)**

Khi Agent cần chạy lệnh shell trên Bare-metal MiniPC:

> 1. **Giới hạn Tài nguyên qua Cgroups v2:**  
   * memory.max: **512MB**  
   * cpu.max: **50000 100000** (Tối đa 50% của 1 core)  
   * pids.max: **30** tiến trình con  
> 2. Cô lập Không gian Tên qua Bubblewrap1:  
>    Bash  
>    bwrap \\  
>      \--ro-bind /usr /usr \\  
>      \--ro-bind /bin /bin \\  
>      \--ro-bind /lib /lib \\  
>      \--ro-bind /lib64 /lib64 \\  
>      \--proc /proc \\  
>      \--dev /dev \\  
>      \--unshare-all \\  
>      \--die-with-parent \\  
>      \--cap-drop ALL \\  
>      \--disable-userns \\  
>      \--bind /data/workspace /workspace \\  
>      \--setenv PATH "/usr/bin:/bin:/data/bin" \\  
>      \--chdir /workspace \\  
>      \--uid 1000 \--gid 1000 \\  
>      bash \-c "\<agent\_command\>"

### **B. Ma Trận Phê Duyệt An Toàn & Audit Logging Standard**

Mọi Tool do Agent gọi đều trải qua bộ lọc rủi ro:

| Cấp độ Rủi ro | Ví dụ Hành động / Tool | Cơ chế Xử lý & Phê Duyệt |
| :---- | :---- | :---- |
| **Low (Read-Only)** | Search Notion, Đọc Gmail, Web Fetch, Xem file workspace6 | **Tự động thực thi 100%**. |
| **Medium (Scoped Write)** | Tạo file code, Viết Notion page, Sửa bản thảo Gmail5 | **Tự động thực thi** trong ranh giới Workspace đã cho phép. |
| **High (Destructive / Finance)** | Push mã nguồn lên main, Sửa DB sản xuất, Chuyển tiền, Gửi mail | **Chờ phê duyệt khẩn (Human-in-the-Loop)**: Gửi thẻ Confirm lên Telegram/Web UI. |

Mọi lịch sử thực thi được ghi cấu trúc JSON-lines tuân thủ chuẩn OpenTelemetry Traces tại /data/logs/audit.jsonl:

JSON  
{  
  "timestamp": "2026-08-16T23:55:00Z",  
  "trace\_id": "9a8b7c6d5e4f3210123456789abcdef0",  
  "agent\_id": "agent\_dev\_assistant\_01",  
  "tool\_name": "skill\_run\_bash",  
  "risk\_level": "Medium",  
  "execution\_time\_ms": 142,  
  "status": "Success"  
}

## **7\. Bảng Công nghệ Tiêu chuẩn (Tech Stack Specification)**

| Phân hệ | Công nghệ lựa chọn | Mục đích & Tiêu chí Kỹ thuật |
| :---- | :---- | :---- |
| **Core Daemon** | **Golang (CGO\_ENABLED=0)** | File nhị phân tĩnh nguyên khối, hiệu năng cao, khởi động tức thì, quản lý Goroutines tối ưu. |
| **Frontend UI** | **React 19 / Tailwind v4 (Vite)** | Đóng gói nén gzip/brotli sẵn, nhúng vào binary qua go:embed, bundle size ![][image20]. |
| **Remote Access** | **tailscale.com/tsnet** | Nhúng thẳng node Tailscale vào binary Go; truy cập từ xa mã hóa E2E không cần cài thêm service ngoài. |
| **Tooling Protocol** | **Model Context Protocol (MCP)** | Chuẩn kết nối mở với các công cụ (PostgreSQL, Git, Filesystem, Browser) qua stdio và SSE1. |
| **Plugin Runtime** | **tetratelabs/wazero (WASM)** | Chạy plugin WebAssembly an toàn, thuần Go 100%, không cần CGO, an toàn trên cả Bare-metal và Docker. |
| **Auth Subsystem** | **OAuth 2.1 (PKCE S256)** | Chuẩn xác thực quốc tế cho SaaS Integrations, loại bỏ bớt implicit grant. |
| **Native Sandbox** | **Bubblewrap (bwrap) \+ Cgroups v2** | Cô lập lệnh shell: mount / dạng Read-Only, cô lập filesystem, giới hạn RAM/CPU/PIDs9. |
| **Storage & RAG** | **modernc.org/sqlite \+ chromem-go** | Cơ sở dữ liệu quan hệ (FTS5) kết hợp Vector Search nhúng; không cần cài server database riêng. |
| **Bảo mật Vault** | **AES-256-GCM \+ Argon2id** | Mã hóa cứng API Key dựa trên định danh phần cứng (product\_uuid \+ salt). |
| **Base OS (MiniPC)** | **Debian 12 Minimal Slim** | Tương thích tối đa driver Wi-Fi/NIC (Intel, Realtek), kích thước sau nén ![][image21]. |

## **8\. Thiết kế Phân vùng Ổ cứng & Mô hình Dual-Runtime**

### **A. Phân vùng Ổ cứng MiniPC (Bare-metal Mode)**

Hệ thống tự động định dạng và phân chia 3 phân vùng khi cài đặt từ USB:  
\[Ổ cứng MiniPC: /dev/nvme0n1 hoặc /dev/sda\]  
├── Partition 1: ESP (512 MB, FAT32) ──► Mount tại /boot/efi (UEFI Bootloader)  
├── Partition 2: System Root (4 GB, Ext4) ──► Mount tại / (READ-ONLY, Chứa Kernel, Base OS, bwrap)  
└── Partition 3: User Data (Còn lại) ──► Mount tại /data (READ-WRITE, Tự động mở rộng full disk)  
├── bin/ ──► Chứa symlink actond trỏ tới bản build đang chạy  
├── releases/ ──► /v1.0.0/actond, /v1.0.1/actond (Lưu các bản build)  
├── config/ ──► vault.db (API Keys mã hóa, user settings)  
├── agents/ ──► agent\_manifests.json (Cấu hình các Agents do user tạo)  
├── tokens/ ──► oauth\_tokens.vault (Mã hóa OAuth2 Refresh/Access Tokens)  
├── storage/ ──► app.db (SQLite chat logs, FTS5, vector index)  
├── logs/ ──► audit.jsonl (OpenTelemetry structured audit log)  
├── overrides/ ──► Thư mục nạp đè Web UI / Prompts tùy biến  
├── plugins/ ──► Chứa các file tool .wasm mở rộng  
├── skills/ ──► Chứa các thư mục Skill script (JSON \+ Shell/Python)  
├── mcp-servers/ ──► File cấu hình và thực thi các MCP server  
└── workspace/ ──► Môi trường cô lập cho Agent đọc/ghi dữ liệu

### **B. Mô hình Triển khai Docker (Container Mode)**

Khi chạy trong môi trường Docker, toàn bộ dữ liệu trạng thái được ánh xạ qua 1 Volume:

Bash  
docker run \-d \\  
  \--name actonos-agent \\  
  \-p 8080:8080 \\  
  \-v /local/acton-data:/data \\  
  \-e RUNTIME\_MODE=docker \\  
  \--restart unless-stopped \\  
  actonos/agent:latest

## **9\. Trải nghiệm Onboarding & Vòng đời Vận hành**

> 1. **Khởi động Tiến trình:** Cắm nguồn MiniPC hoặc khởi chạy Container ![][image18] Tiến trình actond bắt đầu chạy.  
> 2. **Kiểm tra Môi trường Execution:**  
   * **Trong Docker:** Mở Web UI tại cổng 8080, nhận cấu hình qua UI hoặc file .env.  
   * **Trên Bare-metal MiniPC:** Kiểm tra file cấu hình tại /data/config/vault.db.  
> 3. **Cấu hình Lần đầu (Nếu chưa kích hoạt hoặc Mất mạng \> 60s):**  
   * Bật Wi-Fi Hotspot: ActonOS-XXXX (IP Gateway: 192.168.4.1).  
   * Bật Captive Portal DNS Hijack tự kích hoạt trình duyệt trên thiết bị người dùng.  
   * Người dùng chọn mạng Wi-Fi nhà, nhập API Key LLM, thực hiện OAuth 1-click kết nối các dịch vụ cần thiết5, nhập Tailscale Auth Key (tùy chọn) và đặt PIN quản trị.  
   * Lưu cấu hình vào Vault mã hóa ![][image18] Tắt Hotspot ![][image18] Kết nối Wi-Fi nhà.  
> 4. **Trạng thái Hoạt động Thành công:**  
   * Kết nối Wi-Fi / Ethernet nội bộ.  
   * Phát tán tên miền nội bộ http://acton.local qua mDNS.  
   * Kết nối mạng Tailscale Mesh (tsnet) để điều khiển từ xa qua Internet.  
   * Khởi động Agent Engine, nạp MCP Servers và mở cổng Web Dashboard để người dùng tạo và quản lý các Agent.

## **10\. Cơ chế Cập nhật Tự phục hồi (Atomic OTA & Watchdog)**

> 1. **Kiểm tra Cập nhật:** Tải bản build mới actond.vX.Y.Z về /data/releases/vX.Y.Z/.  
> 2. **Xác thực An toàn:** Kiểm tra tính toàn vẹn Checksum SHA256 và chữ ký số GPG.  
> 3. **Atomic Symlink Swap:** Thay đổi con trỏ Symlink /data/bin/actond trỏ sang bản build mới.  
> 4. **Khởi động lại Service:** Gọi lệnh systemctl restart actond.  
> 5. **Health Watchdog Monitoring:** Thăm dò endpoint http://127.0.0.1:8080/api/health trong vòng 30 giây.  
   * *Nếu trả về 200 OK:* Đánh dấu phiên bản mới hoạt động thành công.  
   * *Nếu bị Treo / Crash Loop \> 3 lần:* Tự động đảo Symlink trỏ ngược lại bản build cũ và restart service lập tức.

## **11\. Cấu trúc Dự án Monorepo Chuẩn (Monorepo Blueprint)**

actonos/  
├── Makefile \# Build pipeline: Web UI \-\> Go Binary \-\> Docker Image \-\> ISO  
├── go.mod  
├── go.sum  
│  
├── cmd/  
│ └── actond/  
│ └── main.go \# Entrypoint chính (Tự detect Bare-metal vs Docker)  
│  
├── internal/  
│ ├── agent/ \# Lõi thực thi AI & Universal Agent Engine  
│ │ ├── engine.go \# POMDP & ReAct State Machine  
│ │ ├── manager.go \# Trình quản lý khởi tạo/chỉnh sửa Agent Manifests  
│ │ ├── swarm.go \# Sub-Agent Spawning & Swarm Delegation Manager  
│ │ ├── planner.go \# Module phân rã task và lập kế hoạch  
│ │ ├── verifier.go \# Deterministic Static AST & Invariant Checker  
│ │ ├── reflection.go \# Goroutine chạy ngầm trích xuất fact & learning  
│ │ ├── profile.go \# Quản lý User Persona & Dynamic Preferences  
│ │ ├── heartbeat.go \# Daemon chạy nền cho các tác vụ chủ động (Proactive)  
│ │ ├── context.go \# Context Window Manager, Token Pruning  
│ │ └── types.go  
│ │  
│ ├── auth/ \# Phân hệ Xác thực & Dynamic OAuth 2.1  
│ │ ├── oauth2.go \# Luồng PKCE Authorization Flow (S256)  
│ │ ├── dcr.go \# Dynamic Client Registration handler  
│ │ ├── delegation.go \# Scope Manifest & Zero-Trust Delegation Manager  
│ │ └── token\_refresher.go \# Background Token Refresh Daemon  
│ │  
│ ├── bus/ \# Hệ thống Event-Driven Bus  
│ │ ├── eventbus.go \# Go Channel Pub/Sub trung tâm  
│ │ └── messages.go \# Định dạng thông điệp thống nhất (Incoming/Outgoing)  
│ │  
│ ├── channels/ \# Các Adapter kênh giao tiếp  
│ │ ├── adapter.go \# Interface ChannelAdapter  
│ │ ├── telegram.go \# Telegram Bot Adapter  
│ │ ├── discord.go \# Discord Bot Adapter  
│ │ └── webhook.go \# Generic Webhook Adapter  
│ │  
│ ├── llm/ \# Interface & Router LLM  
│ │ ├── provider.go \# Interface LLMProvider chuẩn  
│ │ ├── router.go \# Fallback Cascade Router (Claude \-\> Gemini \-\> Local)  
│ │ ├── openai.go  
│ │ ├── anthropic.go  
│ │ ├── gemini.go  
│ │ ├── deepseek.go  
│ │ └── ollama.go  
│ │  
│ ├── tools/ \# Dynamic Tooling Hub  
│ │ ├── registry.go \# Quản lý tập trung toàn bộ Tool Schema  
│ │ ├── mcp\_client.go \# MCP Host kết nối qua stdio / SSE  
│ │ ├── wasm\_runner.go \# Wazero WASM runtime cho micro-plugins  
│ │ ├── skill\_watcher.go \# fsnotify quét thư mục /data/skills/  
│ │ └── native\_tools.go \# Native HTTP fetch, File system, System Info  
│ │  
│ ├── sandbox/ \# Môi trường cách ly thực thi lệnh  
│ │ ├── executor.go \# Interface Sandbox chung  
│ │ ├── bwrap\_linux.go \# Bubblewrap \+ Cgroups v2 (Bare-metal)  
│ │ └── subshell.go \# Fallback Runner cho Docker  
│ │  
│ ├── memory/ \# Lưu trữ dữ liệu & Hybrid RAG  
│ │ ├── db.go \# SQLite engine (modernc.org/sqlite)  
│ │ ├── fts.go \# SQLite FTS5 Lexical Search  
│ │ ├── vector.go \# Vector store nhúng (chromem-go)  
│ │ ├── decay.go \# Thuật toán suy giảm Ebbinghaus Decay  
│ │ ├── hybrid.go \# Thuật toán Calibrated Sigmoid-Normalized Fusion  
│ │ └── vault.go \# AES-256-GCM Hardware-bound Vault  
│ │  
│ ├── system/ \# Hardware Abstraction Layer (HAL)  
│ │ ├── hal.go \# Interface HAL (Network, Metrics, Power, OTA)  
│ │ ├── baremetal\_linux.go \# NetworkManager D-Bus, Udev, Systemd control  
│ │ ├── docker\_hal.go \# Stub HAL cho môi trường Container  
│ │ ├── metrics.go \# Đọc CPU, RAM, nhiệt độ chip  
│ │ ├── tsnet.go \# Tailscale nhúng trực tiếp trong Go  
│ │ └── ota.go \# Atomic update & Watchdog engine  
│ │  
│ └── server/ \# Web Server & Asset Delivery  
│ ├── router.go \# Chi router, WebSocket hub cho Streaming Chat  
│ ├── layered\_fs.go \# Ưu tiên /data/overrides/ trước khi load go:embed  
│ ├── api\_setup.go \# API Onboarding & cấu hình Wi-Fi  
│ ├── api\_agent.go \# API tạo, sửa, xóa, điều khiển Agents  
│ ├── api\_integrations.go \# API quản lý OAuth & Tích hợp SaaS  
│ ├── api\_tools.go \# API quản lý kết nối MCP & Skills  
│ ├── api\_system.go \# API trạng thái phần cứng, Tailscale, OTA  
│ └── static.go \# //go:embed nhúng thư mục web/dist  
│  
├── web/ \# Giao diện Frontend (React 19 \+ Tailwind v4 \+ Vite)  
│ ├── src/  
│ │ ├── pages/  
│ │ │ ├── SetupWizard/ \# Màn hình Onboarding khi phát Hotspot  
│ │ │ ├── Chat/ \# Giao diện Chat & streaming suy luận  
│ │ │ ├── Agents/ \# Quản lý và Khởi tạo Agents tùy biến  
│ │ │ ├── Workspace/ \# Trình xem & quản lý file trong sandbox  
│ │ │ ├── Integrations/ \# Quản lý OAuth 1-click dịch vụ bên thứ ba  
│ │ │ ├── ToolHub/ \# Giao diện quản lý MCP, Skills và WASM Plugins  
│ │ │ └── Settings/ \# Quản lý API Key, Tailscale, Metrics, Update  
│ │ └── App.tsx  
│ ├── package.json  
│ └── vite.config.ts  
│  
├── deploy/ \# Đóng gói và phát hành  
│ ├── docker/  
│ │ ├── Dockerfile \# Multi-stage build ra Alpine image (\< 35MB)  
│ │ └── docker-compose.yml \# Template chạy thử nghiệm nhanh  
│ └── live-build/ \# Script đóng gói file ISO cho MiniPC  
│ ├── auto/  
│ ├── config/  
│ └── preseed/  
│ └── auto-install.cfg \# Script tự động phân vùng và cài đặt từ USB  
│  
└── scripts/  
├── dev.sh \# Chạy mock server để phát triển trên máy cá nhân  
└── build-iso.sh \# Script 1-click xuất ra file ActonOS-v1.0.iso

## **12\. Kế Hoạch Triển Khai Kỹ Thuật (Roadmap)**

GIAI ĐOẠN 1: CORE ENGINE, AGENT MANAGER & STORAGE (Tuần 1 \- 2\)  
├── 1\. Khởi tạo repository Monorepo, cấu hình Makefile và interface LLMProvider.  
├── 2\. Triển khai Universal Agent Engine (agent/manager.go) và Agent Swarm Manager (agent/swarm.go).  
├── 3\. Triển khai Phân hệ OAuth 2.1 PKCE Engine và Token Refresh Daemon.  
└── 4\. Triển khai Hybrid Memory: SQLite FTS5 \+ Chromem-go \+ Ebbinghaus Decay.  
GIAI ĐOẠN 2: DYNAMIC TOOLING, FRONTEND & REMOTE ACCESS (Tuần 3 \- 4\) ├── 1\. Xây dựng Dynamic Tooling Hub (MCP Host2, Wazero WASM, Skill-as-a-Folder). ├── 2\. Nhúng Tailscale (tsnet) trực tiếp vào daemon Go để hỗ trợ Remote Access. ├── 3\. Xây dựng giao diện React 19 / Tailwind v4 (gồm trang Quản lý Agents & Tool Hub). └── 4\. Viết module HAL: D-Bus NetworkManager (Bare-metal) và Fallback HAL (Docker).  
GIAI ĐOẠN 3: DOCKER, LIVE-BUILD ISO & SANDBOXING (Tuần 5\) ├── 1\. Viết Dockerfile multi-stage build xuất bản image chính thức (\< 35MB). ├── 2\. Viết cấu hình Debian live-build và preseed.cfg tự động phân vùng ổ đĩa. ├── 3\. Hoàn thiện Sandbox Bubblewrap \+ Cgroups v21 và OpenTelemetry Audit Logging. └── 4\. Xuất file ISO cài đặt ActonOS-v1.0.iso.  
GIAI ĐOẠN 4: KIỂM THỬ PHẦN CỨNG & XUẤT XƯỞNG (Tuần 6\)  
├── 1\. Flash ISO vào USB và cài đặt thực tế trên các dòng MiniPC (Intel N100 / AMD Ryzen).  
├── 2\. Thử nghiệm độ bền: Ngắt nguồn điện đột ngột 50 lần liên tiếp để xác nhận an toàn dữ liệu.  
├── 3\. Kiểm tra tự động hoàn tác khi cố tình cài đặt bản cập nhật OTA bị lỗi.  
└── 4\. Đo đạc hiệu năng: Đảm bảo RAM chạy nền \< 40MB và phản hồi dưới 2 giây.

#### **Works cited**

> 1. Model Context Protocol \- GitHub, [https://github.com/modelcontextprotocol](https://github.com/modelcontextprotocol)  
> 2. mcp package \- github.com/modelcontextprotocol/go-sdk/mcp \- Go Packages, [https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)  
> 3. Awesome Model Context Protocol (MCP) Servers \- GitHub, [https://github.com/subratadasGit/awesome-mcp-servers](https://github.com/subratadasGit/awesome-mcp-servers)  
> 4. MCP | Google Antigravity Docs, [https://antigravity.google/docs/mcp](https://antigravity.google/docs/mcp)  
> 5. How to Build and Ship a Self‑Hosted MCP Server (Notion \+ GitHub) with Auth, Rate Limits, [https://towardsai.com/p/machine-learning/how-to-build-and-ship-a-self%E2%80%91hosted-mcp-server-notion-github-with-auth-rate-limits](https://towardsai.com/p/machine-learning/how-to-build-and-ship-a-self%E2%80%91hosted-mcp-server-notion-github-with-auth-rate-limits)  
> 6. GitHub \- nokia/modelcontextprotocol-servers: Model Context Protocol Servers, [https://github.com/nokia/modelcontextprotocol-servers](https://github.com/nokia/modelcontextprotocol-servers)  
> 7. How to Use MCP Servers to Connect Claude Code to Notion, Gmail, and More | MindStudio, [https://www.mindstudio.ai/blog/mcp-servers-connect-claude-code-notion-gmail](https://www.mindstudio.ai/blog/mcp-servers-connect-claude-code-notion-gmail)  
> 8. GitHub \- paulsmith/mcp-go: Implementation of Model Context Protocol (MCP) for Go, [https://github.com/paulsmith/mcp-go](https://github.com/paulsmith/mcp-go)  
> 9. Sandbox untrusted Linux apps and CLI tools with Bubblewrap \- Botmonster Tech, [https://botmonster.com/self-hosting/sandbox-linux-apps-cli-tools-bubblewrap/](https://botmonster.com/self-hosting/sandbox-linux-apps-cli-tools-bubblewrap/)  
> 10. Sandboxing untrusted code with Bubblewrap \- Siddhant Tiwary \- Medium, [https://siddhanttiwary.medium.com/sandboxing-untrusted-code-with-bubblewrap-52ab55d113b1](https://siddhanttiwary.medium.com/sandboxing-untrusted-code-with-bubblewrap-52ab55d113b1)  
> 11. bwrap(1) — bubblewrap — Debian unstable, [https://manpages.debian.org/unstable/bubblewrap/bwrap.1.en.html](https://manpages.debian.org/unstable/bubblewrap/bwrap.1.en.html)

[image1]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABIAAAAaCAYAAAC6nQw6AAABF0lEQVR4Xu3SMUtCURjG8TcyKChDdAlpDYIghwgEF7eECBHXoCkRnIpwMKLP0Fa4uLcVEdEg+Akc2tWxiKAhiJb+rx4Pr/c63vE+8MN7n8M913vOEYkTJ7okUcKmu1/CHsqmW8AWqu5X72eyiltcYYRjPOAULXzgEHe4xAkGOJdADlDDLr7xinU3toEh3rHvOk0HXZn8CZ+6TCap4A9FM7aNTzRNpw93cY+E6X1u8IaM6Y7wi4LpppM3TOezhp6E36KTD5A1nU7whR3T+Uzfcma6eZ+wgmdHry9kdu38+sz7BDu5brvuom6OHou2BBb8Gn2kTafn6gd50+lDT3jBI3JmbJxlCcxMFpGS8MHTXjdED22cOJHmH2DYLhijlc5IAAAAAElFTkSuQmCC>

[image2]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAFQAAAAaCAYAAAApOXvdAAAEhElEQVR4Xu2ZfaiUVRDGJ7IotLIPFamIG1ZEQYaaBEUGEd6+CI0MKgiiIomINCWVWIgIQSP6QBJJC4KChKISk8hL/iMapIIIQmASRUVEQUFE5fPz7LDnzp5994srrO4DD3vfObtnz5mZ88ycvWZDDHGq4DzxjGgcYEwRz4rGfsCEC8TbLTkLXChO8zdkuFNcbyeXQy8TP6y/9gWcskb8VnxaXCruFdeJX4lXN956HNeLO8XpwX4y4GbxI2skVNeYJG4Q3xcnZ3Yyc484ZilzHWeLH4sPZLZBxGniS+IWG3/Msb8hrsxsXeFG8Tvx2jggrBJfC7aF4n5xRrAPGi4SD1rz/gA+OSSOxIFOQJS+Fy+JA8Jz4j3ZM9HbbOVFDBrmiX+JD8YB4XxLklcaa4t3xP/F58XTw9gs8YLs2aO6KLM5zhVHxUvrz+jyXPHezEZArhTvq7/y3Cs4If59zMMrctQOU8WZlpLlD0tFmFoQ975JfM96WCNaiEPhv+KX4qOWHBQxRzxaf82Bxr4lvmBp/GHxE/FxcbX4i3iXuNFS8XtEPCIus+6Bzr9paX4yaKv4ubjbUmZV4UzxWUunjDVB/l5rqWbkQEN3iecEe1uQSS9bcqY7FqKTUQbuFn8ULw92dPUJ8TpLUf/CGlWSbECjfxZvqNsAJ2PMxhe8dmCtBIXOw+e/wtLcOKbTbKrSTwd7Zd2svyeQ8rRHROt3S07FSTlafcmTlpyJFPwj3pqNMeevNr5q4sQxSz0fXUanYH4Cn3cYnJY/rXmtVajST0er5GkJHIhGlqJKxv1nza1DK4c6iDiRJwMcFLW/xZsymzv5qczWDrQ2n1nzJnEKDo0yVAWcz/fH/joHe+W0kSgdgUW9buUM8ajHCFY5FK1Bc2LW4eQj4sWZDUf+ZuVWrRVcOnCq943edcQgVoHPUGyo4lWaW7XXIsgchL10d31IPGyN6uwgy36wctQ86xB9R+loU4m318nfVFvXVjSSilu6zrpDqb4Ob2+iflLJYQmunz4PxegVa9ZykombY8f9Nv0nWTg/2JEBCtLiYAc4DYfeFgesoZ+lo507mXaJ6sqxI2BszDez3pJ21+rPOTwQ7jy4wpq1/irxJ0u99Uhmd/iacBhzPGPlNpBLTX4aKsEGtorLLUWLv2mVXrV0PJdYWVs940oFoGYpEHn7MWpJ/Ll5OJhjm7hD/FScnY2xObSbLiFmDOC9B8R3LTn3G2vWTzoT1sE8HNsIAsPnv7Z0ciioca+cJsZiDWkJJiUTgTfgNNsLrH1Eatask4DPRSdQ+DiWccHYOXqlo40dbc9/V8jBZziGfB/Z3Uo/H7MU0BJYD4GP63WQ2Vw980SYMBCIfdZdQekGbIKj3A6un6XbDMFeZ+Uj3wmoIR9YOeATAm44CHncSL8gY94Wr4kDGTy77rDUjr1Yf87XQtu3Ntg6BXMhSfklZMJB5DiWpcLVDygYt0RjAFdinMiNyclvuC5BvN5vvf2eSQBqlhKml2D0BTSOjXV8kxgA8J8Kfn844c4cYoghhjhVcQxhKc6U6MRokwAAAABJRU5ErkJggg==>

[image3]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAaCAYAAACO5M0mAAAA10lEQVR4Xu3RMQsBYRzH8b8wUTaKQQxKySuQkQwMXoGJxS4vQMpokngHXoKBzaSsShlksyoZ+P49d3pcjBb51ae7+91zPf+7E/nnG/EjixJCCCL9soJksEIfTSyxRs9elMQWXficroEbas61BDDBASm3JB2cxIzyiJ5oMRPzkEaPer1A2OmkKmaLlluQBPYYWt1zYcXqCriINZ8mJ2Zrt4xg7nTP+TT6lm1sMBXzWY7imc+OllHEsRPPfO+i811R995wo78rhgHOKMuHrfMYYWwpvqz4ldwBPwAkC14X0V8AAAAASUVORK5CYII=>

[image4]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAcAAAAcCAYAAACtQ6WLAAAAkElEQVR4XmNgGOQgCYh3A7EwugQHEG+FYhAbBeCVlAHiJ0DciizIA8SSQBwKxL+BOAKIxYGYFSQZD8SzgPg+EP8E4qVAPAmIlUGSIEC6fTDgAsS/oDQGqALi50CshC4Bs28PEHMzQFzZxQCxikEEiK8yIOwLAuICIGYEcUBEIxDfAeKVUDbYj8hAAIpHATIAAP3zGM9f3v8PAAAAAElFTkSuQmCC>

[image5]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAmwAAAAxCAYAAABnGvUlAAAJEElEQVR4Xu3decit2xzA8Z8METLP0+VeJELhRghFSMbrDyJEEi7hGup26SVKHDKWZIgyRVwZQ3lxcw2llKmQQyKEUmTIsL6t53f22r+733cPZ9vnfc/9fmr1Pns9++z9DOvZ6/f81tr7REiSJEmSJEmSJEmSJEmSJEmSJEmSJEmSJEmSJEmSJEmSJEmSJEnSrly1lZvWytPwpFohSZKk9XyulWsOj98zLH+xlXsNjxe5XyvXqZXFc2qFJEk6Pq7eyuNaOWdaPndYd89Wrjc8PhOu0sqLp79nq0+0cuG0fLVW7hM9gHtZK79q5a3TukVY94tWXtLKNcq60UdbuUutPEZom6+rlSvg2F5ZPKqVN9XKHeIcPaFWSpK2g87+Ka18vJWPxSwTc+tWvppPOsM+2MoFtfIswvHfn5avG/3Yp2WZsRvHaueJrN1Da+UR9MpW/juVP8Vsmy9q5c35pDU8MM78Tceu0A62OZS+ic+3cn6tlCRtjk7sW6Xu8ugBAH7Syt2GdbtEdolhwkTW6SPD47PJedGzm++fHnP8bzEtkxHL83EQArq31coFPtDKo2vlAU7Uih37bcwCWJBdfcf0dxM/buX2tXKBe9SKY+QRrdysVp4BfK58s1ZKkjb3gFb+VeoYXstOsc6r2iUyTHX4iyzUDUrd2eDb0186XPaPuWgZOLDPDDM9b3q8CEN+DEM9vZU7Rz9uH2rlua3cd3je+6IPta7i7bVix8iujef/VnF62cHvRj+WyyybK3iUZcB/FHDuuMmSJG0BGYdfx6xzJNOTyPDUbMwbW3lvK6+K/q3DD0ef+0ZH+M6YH8Zb5OXRM3oMzf20lTvMrz7l3dGHaQk6xvl0bNO25scQBL2lla8PdX+OHij9MA7etlVcO3rG61mtXNbKXedXz6kZo/ziwN5Qx7YehtdgGDWX2fZLWrlW9Awdf/GGaf0qlgVsZFFe2sqPpsevaOXn0d/vj3F6gTXHgAzbeA54/fFLFbQ19ucZrXwm+vE+GT14fUErf8gnTvj33yh1iywL2GgzF8dsnthjorcZsqB1m9fBeXlaK8+OPqRIRhA3b+X1rTy4le9Pz6O8Nvqx/vT0PDDXccS1yRDyjaK3yb/EehlEpkjwmtwEcM0+cn71oTiOq94cSJJWxDDKD6IHbplR4wN37LxuE72T/lTMAoiT0TMfoOMiY3cQXmtvWubfHDaER2dMIFnRYdes26boGOnw/jY9JsAgQKQz5P2XDUNyLOgsF30z8z/RA1qO5Sdjs/lTt4vlgdo6avBdsa0ExFkIzMfHY6BH5oQAiH3M40ewQiCD/MIE+02Wr6L+0lh87EBbqueZ4dzE+zPPjfe741RH9u3f0zLt6+S0nNh/trGq+/3w8rgGuLQZvpGbP5NC5i6f84VYrd0s2neyYwS6YJtYzz7RlhLbQ0BKEMf1Sjsd28gvh+V7t/Lk6DdImelifuqqQ6Zc77w2/55t5jXWCfbY1lUympKkFXDXnfgg564+O5IasIHOKifA8zw6j+wMCMAO6wxYnwEdf+mUD0JH9c9aGf09udPfpgwMx31juXbUFZ0XGQuyDyOOKYHvb6IHPZnd2sQ2A7bMwB3kidEzm1mYvzg+vv7sqacwpzDnQHK+OG/Ivxwj5p1V1P8urnjsEsFazeaMAVsah+wJIDMgW9R+CNg4X1Xdb25Ilu33Xszmw2WQhROxWrtZtO/7MR9wgW/90pYSQRBZTK6zv07rfjasr/8eGVDjXbHeFAf2cW9apv0sC0ZHbCvbL0k6TWQhCEzSeTGflWBop3aaY1A2ZtSYe0UHyQc8P/hKGTsGAq39mAWIBHoZGBKU1G+10RF/ZVo3DrOSBVvUCfC6dLwHlXE/R3QqF0/LdPg8Bh0v6OTvH31IatXAi45tDC7Yh8N+auOoWjYkCoKGHKLej35OycbQTghcGNrjZ2HWReBVM1AEceMNBjKjhpMxy8px40GgyJDpbac6sj0EO8vUm5SKNjIGRrnMjUsGcWwn+75Ohmkv5ueTkmElk/ePoY5rkv26ZcyOD9+AzW3mJqHan/6O28c1xfklA5fqNYjxmmA6A24Sfcj57tEz0s+MnlmsNxeLPj8kSRvg970InL7Xygujd7Jjh0iHW4ctMxsFAqrMrhHYMOfs1cNjOsf8sAedBO/HcNDY0b4o+rDP2EHzOmQO8vUSAWIOwW4LGZIvR++ELo8ebBF03DB6Z09gWjujZcg8fS36vt6prDsuVgnYLop+/PgpCeYk0nFzzjIIJ2g5bJi8ek0rv4+eOaKtPWRYR1AyBt7cFJANSzw/AxKGKT8b88E+bY8AZJllARsIUpnfRbu5LHqbIRuXuOHgdwMJrNZBW6Hdc21xEwRuFJhTyf7QPsF1xT6SweX6TfvDcuK8MBSac+64Zi+Mfizz9fD3mL9ewfEbrwkwBP2l6NdrfsmB4KxelwSrNeiWJG2AIRmCsnPi4PlN34n5yePjcv0wro/p8OsQKZ04zxuHkcBQS82eHDQUlUHithBc5LYzOZuS6IQ2nUTO9q8z/HTUrBKwgX2kLPqvtAhoNpm/twivUzNW4/Ed34dzWofvyLqRsVpmlYANtG3aDdcQbYa/iS/KEHhmkLQO2k29QeDxeD1wrMHxzmXsxRWvD9aP/5agjDmGBJ0EsYljWa9X1GsCmcnk/IL5fOP7ssxNiyRpR8gukUlYF53Ew2pl9I6DzoLXzI6GTo+fm1jmqdHn4OzSouFXrY6gj8zltpBlWtSuliHgOL9W/p8QuD0/esaNLwfsGt8KHYPHimDqkujz6Mbs54lh+TAEdZnJJMPG65HpY2g035csaw6hSpJ2gA9ghk4P6wAW4S5+0b8hAMrJ3JnVYmjmQaeecTCG25jXs0vHce7ZUcJ8vkXtYFOc//FnLFa1F9vdjmVqtnmXCJIvqJUF2WyGOseM2mOH5cOMxzGzamOWD2TutpVZlSRJulLi99kYXn58XSFJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkiRJkqTj63803j+YeS63xgAAAABJRU5ErkJggg==>

[image6]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEkAAAAaCAYAAAD7aXGFAAAC0klEQVR4Xu2Xy6tNURzHf/LIM3lPKITIAGGgMKJMGBhIGShCJAMlz3IHJK+8H0lJkjwjERMMkQH+ASVlYGZgYCDfj9/ezjrLuXc/zrWPsj/16Z671t7rrL327/db65h1hpFylewbd1TAYLlWDok7qmSQPC3vy6FRH9B/XI4P2vpY+Umz0Dx4EabILtk/aq+UcfKdXBJ3iNVyS9S2QD6y1ouaBeNdiRtzsEcujRurhmg6ax4lKUQRDzQ5aIPt5teX4bLcFDfmYKE8b83zqxwm8cY8qlJYnFtyWPL/RHlKfpAvzBdrQNKXxWzzBf8sH5rXmSIwrwdydNxRJSPka7kiaJsvL1jz22OSz+WMoC0v3MO9ZR6U1L5t5b63JQy4X17qxm2y3++rfRG2ylfyRtC3XF5NL0og4p5aI7q4lvHi70hlHmntWiOvW7mUYQw2l7lxRxWw2+ySO+Q8+V5OSvoo5NSQEOpJ2XrEfWXqEbS1SOQqb3yxFd8iub5LXkw+U6ifmL9xYEJ3rbHdEzV35Eo5wYrVFdL5mXkKzzFP6w3mkcrCHZYH5WZ5wJojHbj/njUfRTKZJl+apwfb6l7znOVB87LefJLhmYe2x+bjUDso3GkNIU1Omp+b9snhSXseGO+m+Uuh4BMZ081rHp/XyWXm33VMDvx1VwM2kWtW4Hw2S36Ru62R32PMzy7h7pQFO1McfaTfqOQvYx81r0MptFGPytQVxkxrGRBVPANRc8Z8IVgo6uPY4DpodV7rFt4IW+gna9QOFuaceV0pM/memCkP2Z/h3xsQPdS9MHpIvY1yanAdz8zRI3eqEUVf5Xf50TzljpjXj95eoBRqHnYCnonatSju6AlW/ofcGXf8RdpJsXaJUzQX5PE3a/1m+eHYiV/q/xzsKKQY22UIi8dOR9GtMf8d9NZ8y+dUy6KdsALb4/8CacU2iXWK1dTU1NTU1NTUZPITfg1i0bmEZ+IAAAAASUVORK5CYII=>

[image7]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAADYAAAAaCAYAAAD8K6+QAAABy0lEQVR4Xu2WPyhFURzHfxIR5W8koiiFUQZlMFgMDEopgxdKNpGUiaQwGIxSpAxkk3+RxchqI4syyGJQWHy//c7pXSfPe3n3vnp1PvXJeX6/d+875/x+514Rj8fjCYFReAEr3EDE5MBluAMLfobShxc8NoZ+8SRUwju44QbCoA4+ia5cpumA73DYDaRDMayBg/ALDsFqmBdMiohS0XvPwjfYDatgbiDn34zATfgIP+CeaEk0BZMiIB9Ow234YuR4VULscd9foicYV5QllIrJFiqS/rL0wE/zNxnsixXR8k3FXv1aQibgK2xxA2EwD59hoxuIGO4+e/oGljmxtLH9dQmLRE/DNdHyjBrbX1vmM0t8XfSk7oMHsA2Ow1MYg80mf9+ME2IvbvtrAE6JrmbUsPxYhuwv3o/35f1r4ZhoJZ3DctF+fYAzJjcm8QX5FSYtwnvRVeA4E88wUgh34S08hJOiv4cTqYdHsN/ktou+7tmS5UbMmfGf8FCgmcaesiy/INy1a4n3PXeVzznml4hOstPEsooueCI6YU6Gk+IJamNXsFW0ZLMKvpUsmDF36Ey0HAl3iofJEmww/8sa+MoV7HX3QW9PcI/H4wmPbxi5TFEQOwV9AAAAAElFTkSuQmCC>

[image8]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAwAAAAaCAYAAACD+r1hAAAA1UlEQVR4Xu2SPQ5BQRSFr6CQSNBodAqVWqFWSHQ0dmAHtG8R9qBRiyUQm1AIUYlCJcE5rklmxhuPQqLwJV9z7/yeGZGfp/jwLTJwCE9wBvNuO8wIXmDLb4SowwMcw5TXiyUH53ANK24rzABeYd9vhKjCHZyIhpEIB01FJ3FyIkxoK3osHu8lHbiETdGLMwAGEUsXbmBDNFJGy4gZ9RMcfIQ9q9YWfUQ+pgMHnUW/hv1YJbiCC1gwxRrcwwhmTdEiEt2Fu93hilwpbQoerJflg8/45/vcALruH5wG+666AAAAAElFTkSuQmCC>

[image9]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAG8AAAAaCAYAAAC5KgISAAAEoElEQVR4Xu2ZW8hmUxjH/3LIRDnMRBMX45BTMw2ZUaMhFy5IJFwwDilTc2imzEwzU66+SYopGiQlJRdSIlyQEF8pjdygBjXUkCiFGy6Qw/Oz9upd37PXfvfe77v3O77av/r3staetfda/7We9az1SQMDA4uPU0zH+8IJoR3aO5ocazrDtNx0qqvrm/TdJ7u6zrnB9Ki6Ne+A6RZfMUMYuI9Mz5nuc3V9w2R52PSqaYera8Qdpp9N/yT61bQ5fci4zPS+wkzpkqWmN01X+IoZgXnPqzzzl5g2mJ4x7TdduLC6NStMt/vCgstVHu/GHKMw8/42XevqgI68ruqXT8t1prdVHsBZkDOPUP6O6cGi/FLT56Zbk2eacLFpq+k9018K78kxlXmnmT42HTGdtbDqPxjcT01n+oqOYLA+1OSTgwHGhFRNJ0LOvL0K48G4RO40faF2Y4B5N5uuNH2nnszjH/9metl0nKuLq/IJV941Dyn//nFcohDK31UIb4iZfpvCwDXBmxcnsh/otQrbyU2uvAm84xuV24xMZR6zir1up68wlpkOKZ9UkC2tV3g5/43RKxRmWywD9rUbTVerOtm5XqGDZ/uKDLznXtNLavb8OLx5mP5TUZYSJziTrC29mRdX1p8KRnho+Nvi17PdtM/0mWnO9JTCPnGP6auijpXwrEJIZN+s2tton9DCDK+DZ19RN8cMb140yQ90VXkTejMvhomvlY/nrJgfTOe6cp59XGFlErZ+0cKMkQ8lAdqtMEGAthiA3ESIHeSZcdAW2d86XzEh3jzeTxTyA/2/NC9+VNV+Q2d4MR+Qssa0USFssWIe0cikE01vmOZVTgRyEwFiBwnh46A9VvhFKicpqfiGJnjzCN+Lxrxx+x1UmRfhaPFH8RshYz2ihftDNPQthaOHJ3aw6jsiDDJ7HaE+Jik5YUITvHlVJlWVN6EX81gp4/Y7qDPvAZVXE0Zyrkkzs5UKoXVbUpYSO1gXNpkErLzckWYSvHn0g/74gY7m0d+29GJe3fkOMPV702pfodFqYs87KSnPhUdMwzxMXKfydRDP8p4mK2ZToS7w5vE7r9CvNPTmIgzPLtVou6iiF/Pq9jsgdWZQczcvcb/LhcfU0BNMrxXldJj7UZ+0rFVImpqcz8gyCZ25b2qLNw/uUsiwzyn+H3PIog9qlOFiGhcXv6s+eYrmvaC80a3Mu1vBkPQ+80flL2bjTMzNdD6a1XRNUkYWiglzSRncbzqsMFG2qNwJ9t555Y8ROU43vagQQs/T6EzZlpx5nEWfVrgA4MyKcYcUrskiPM+dbMyoczC5mNxsIXGcOehztFqVPNfKvLbMKb86GTBCrzei6s9G3KLnzKFdjJhz5XXwXjrOkeVLhdUSxQRtQs48oO0LFG5rxl0uYBBbwjT0at75pk8U9qs+oH1CEr+zpsq8puxRfdiso1fzYJfpMZVX2bTQ3j6FQei67SZMYx5JHLdHuaNPG3o3j7DxpNr/WaSOqxRCchdXXZMwjXkca6a9W4XezQOyRzbv3A3JJHA8IVM9WsYBf1z+QGGfJLLMkvTdvZs3MDAwMDCwmPgXz2z6K14F2SAAAAAASUVORK5CYII=>

[image10]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAF4AAAAZCAYAAAC4j5m6AAAExUlEQVR4Xu2Yaah1UxjH/zJkzDxFpqRMIUPI8CaKDyRDKdNbvBkiRSj5cE0fhJIpRIaSDCmZI94i4ydFr8IHUvKFUpRk+P969uOss+7e555zz+217+386989e6291n6eZz3TutIMM8ywfrCxuXU9uEyBHujTeyDoI+ZR9cQyBXo8o5470kbmg+bl9cQyxwXmw+qx559irlXPvWMR2Mx8xTyrnugDEO5N86p6YoXgPPND9dCpDje/NvevJ1YI9jG/MY+rJ0ZhQ4VhbjGvNA8xNxh6Q9rePNe8SfEua0psYZ5mXmeeYB5p7ljMX2a+b25VjCXIjawp9+XvseYO+dKUSB35TuZidETXPfOlKYD+7yjsMxZ2USx4TSHEmeafCgMChLvI/MxcpRDyLvNtxVqAF39gHm3u2rz/vULRxFMNaxCajF+jiIhLmnEO8R/zseZ5GmDoe8zrzY+b3wB9f1Xoj+GmBXrQ4dROOw8I9IS5zty5GVtl/qWBcBTEX8zjm2fAuucVBYXcfa3mf/BWDQy/paKo3vHf7ABEwvnm3uYP5o3N+HaKCGk7rEmBDjeb2yoMn3si+5MK2ZBxWiD7Wo2xF4b5zbyvGMN4CEhobqqIhC81P+T5CJFBTsN4HNb9ir6WD+NBGdJp+DRqgvaSsZ0Uxel385hingPhUKfFGkVUsjff4FsJ5EfupQC6kBmw30icrgjn2iAJouBbtZ8ia1iLYUgXLzTPyTu1sOETHMCzCm8su4IbzJOL52kxp4gqoitBC4jjLAXQD3tl9ujEQobHy/H2tlNMw+OVgEjZy7zYfM/8W4OL0kKGpyP4UcOpiDRwr+ZH2mKB/OjxouKgATLPaek6rbFTTebVUhiApx6qSDV4Iu/sXswDjETuP0hRtM4o5rIGZC5lb77RVSgz5eEICdICHp8g9Z1tvmReaF6tqAMJlKXz6ipsFP3vNHz46H+3BrrzfeQ+0LzUfMNcbe6nkP255ncXsMlYhRohUY68d1IxTiG9vZk/zPxZcS1OoOCnCo/kHZRB4EwtjD3QjCeoI9QLDrNGdhd58yPdPKTBYWMY/tXA5Yu9UbB0FuT53PxDwzWiRKbNbPeQlXSIfmA3RUfF/FuKQ+WwWEOHx3dXq9t5mKfBKOvlSCAAHoviLys8/FEN59oDzI/MV83HFemHNWlofn+iyNGsZR+EKPcgIr5Qe+pIIxBZ7E+rekQxjzE56MydKFcWXbz9dUV6Q5Y2YBjuKD+ZTytu0bTOCQy9h6JTy+glEpEl0ywH3pUueQcZy8gfCyiPYtvUEwWY4500eGJzRSrIPdpyHGH9lUbf7FjXtj/KcpAYjwsYHtm2D4WYqBgFIg5Pbos8vJ4WlpoDqF+023wXJ+IQuiKK8XUaLty9AMLfpkhBXXm4Cxg+PY2aQmSl95cgbXYZZhxwmEQODoCMGD07HubeVUR/XvISqRucVLf1AnI2HQ/FaxIcrMjpdEmkvLoZANQJ8i/d0GJB+pprfuPhpKO8BHKgFFuMW/+LYV9FUa3HewUuWBTiMv+PA1IZaabO7wk6krrzmhSbaDjN1emovBQmeCYysjHoNeig+N/MpOCGi8efWE/8j1hjnloPriRgdNq6c8wrFAVyhhlm6D3+BdQO0EGVqRgMAAAAAElFTkSuQmCC>

[image11]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAFQAAAAaCAYAAAApOXvdAAADqElEQVR4Xu2XS6hNURjHP3lE3o9IXtGVpDzyiLyuupKB5wQxE5IIAyLpIEVRHjHwjBKhDJABRSjKQIqJUpTIQDIgDPD9+tZqr7Puvtexz5bJ+tW/01r7nP2t9b3WOiKJRCKRSCQSicT/o61qvGqGqn0w30bV1X1Cg2pMMK6X3qq5qkHRfEcn6CS2rm7Z48JgB3vYDeks2b6xgz3sFoIXHVJtV91XHQyezVd9EnNiH9UL1XvVsOA7RZmguiZm95VqhJvvorqnOuXGa1S/VHvduCiLVedU+1XPJHPqANVr1WY3PiJmb7kb/zWNqh2q7qoHqguSZSAvx4k4k7ktqndSv0PJvmNiGc9Gv6omumcjVR/FHAlDVC+lPofiPALEJwF8qxronjWpfrhPmCSWRIUdukpsE1PENrbUzfdUPZFqB7OIW2IOrgdKb5NYdVxUPRYLKLCR0MGAE7yDYbTYOmttPVTDMjEb2MJmO/eMQIUO5p1nJXNwYSpiLx7qxnGm+LkDUvtG/gS2sFkJ5sKq8OwW6/GetarVwbhW4qShUm46+Z7tq4dWUBjft65KFrm4FIGFEOmyoEd/V01zYw7AuO2QVcfFKqZetkr1GUBWElAqwEOQD0vmh0L0V72R6hdjnDmeAQbITp/B81SXVVNV+1R7pPpk5ObQV7LI54GNsNz8Opj3kFX0buA03qU6KVmL8NA+sBfeUmI4lAgYgQOy/ovYXjxLnDwEdqHqhtg+Y7u5+BPcN34WhbPCaE5XbRMzQDmsFAsA35sjdlJzgHh4zmkZ9qsY2klsg4wN10G2NLjxItVYsT6Oo0O4nWCvEs2H0E44F3y2r5DqE51Di2oIr1WzVTvFEuOS2CFeE5Q4PfOK6rZqveqh6pFYZE9IFp1eqsGq62Jly8bjyBH1b2KO9lkew29wznMxGwSHoH1QnRfrbdwbPcNVk8XWF7eAjaqfqjtiLSwPAs6ViSxln/RKAkGVnBbLQgIWQtA/iwV2nFjl1QyO6SfZgshGooXig4gsZWGtXaGI6lGxd7ZGD7Fy9YvldwQhr11UnPKg0rBHa2iJvD2xX9aY1y541wbVU6f4T0FpNIllV2uLp9/Sd1sq+b8Fh1E1ZOk6af5vJuy3ZUDbuivmcPo8FRRXYmnwz6ISTwaQbTiTHlQWZAclzQFIvw1h02dUo6L5emgU+2fF7YaDcFbV05LpIPkl4qFkF0jzVlEv2MyrCu7IM+PJEsAeZ0bZ+0gkEolE4t/yG/k+nH+OCWc1AAAAAElFTkSuQmCC>

[image12]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAJoAAAAaCAYAAABPT0XPAAAD90lEQVR4Xu2ZW4hNURjHP7lE7pdccpcXT8ilZtyOGkm5hWIaSUm8kDyQSZqSoiiXeHAJD3ItCSkeCEVR8qC8KCSeJIUHJf7/vr3mrLNn7zNnrzXN2cesX/07s9fe5/jWXv/1rW8tIoFAIBAIBAKBQKCW6QNthAbF2gMBb4ZAa6EL0DfoAzTKfqBWOAxNjTfmlB3Qknjjfw6NtgKaCV2RGjbaCWhGvDGn7IaWxRu7EBfF02jdRQd7PtTTau8G9Y8+yWTR7GOuOwJXo1UjZh+jjRXNhkNj7X2lGP8A0f6wFsojXkZjJ49CzdBj6Ih1b7nousyBGga9gb5Ak6xnfHExWrVidjXaKtFBOgS9lqLZRkPvoZ3R9XHoL9QUXecNL6MVoL3QQOgJdEmKs58d50BxwNi2C/osHTNoBhejFaQ6MbsYjaY6G31yYnyCxkT3GqDf0SeZLTpJXI3WCH3MoJeiGb9SvIy2GZoC1UG/oHVR+2DohZQOIl/QPdFBzEpv0QDjOgctTmjnwKQtdz4xMxvy+2awk+AzI6RtTPuhDQntw0WX8iRYRDeKTorn0GWoR3TvgJQajzGfl6Lx8oaX0Qwtop2eGF1zIL9CW8wDURt3iWkGKMca6HSC3kI3E9oPSvvnNS2SPWYOKjNMuZdVL23joZgB7ie0H4Mm8ItliE8KTry7kfi3aTspuqTmEW+j9YMeQTekONtYV/DFzIquCV8SZ2dH4rJ0kmrE7LJ0Gvhdu1ak4TlJmluf0AlD05r+ZCVt1UgTs7a9kWoPb6Pxi/wBu9N8MfaPsvPMDCZ78IVfg+aIZh8uKy67JVejucQ8F7ol7mdhPkbjILGe5I6YsM8/pfT3eDBKGZiFV0J3RN8xl99yjBddOSoVz8d4TlYp3kZj/cICmjUDoctpInsGzoP2iHaeqX2T6CDzOdZY70Q7mhVXo2WNmXXUemg1dFXSa6py+BiNmxTWj6wjCWOxd5isSU9Fn4ZF0D7RTMXD0oJ1rxrQaHZN6QSXHdY316EH0DboKfRM9B9gLWJmFGfBOOi26HECB7m92ZaGq9FI1phHihbbrrs6H6NxEvJog1mN8bIW47EMB44bImataa1PK6w1v4sup9PFbXL4wgnKmH+ITgzqj2jcW63nMmF2W6x/CDMBZ1jSDpBZjQH4Hhv4GI1kiZlLKI1oltKs+BiNJMXGuNNqJR7mbodeRbKzXZehQfTYgC/Dh6XiseZnhBmCGa0OWhC7Vwn1orvYzoClyUNRI3KpYlngumrUNDzNbok35hwOHmsd1m0uG5fOpCD6PwncNZ+BFpbc7UL0kuR0n3e466tGreMC3y9ry3gJEAgEAoFAIBAIBAK1wT+3ZdxySNbLNQAAAABJRU5ErkJggg==>

[image13]: <data:image/png;base64,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>

[image14]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAFgAAAAaCAYAAAAzBZtTAAAD9ElEQVR4Xu2YS6hOURTHl1Dej8gjEhKJGAgTBnSJAYmEZMKAwZUojwFFMjJBdCMSJY9MPUJcKYSEDJTkkUeIgTDw/v/v+ra7z7r7fGef+13f1e386t+93177O98+a6+19jpHpKCgIExHqKcdzElLXKNNQqfshyZZQxlGQ2uhTt4YHbwLmu+N/Q+0g0ZCM6GuxlaWadB76Lenj9AyqC90Ffrp2T5DJyT5Ix2gfdAqbywLOvIUdA3qbmx9oLOSb7P+JZ2hOuggtBq6AQ1OzIiAX/4F1ViD6Bhth0V30jIDqpd8qc0I5TWfQwONjcyCLkDdrKGZcN2ToSvQJmMrBwPhgOi9839m2xlooz8pi97QbegZNChpaoAXY/QutQbR3T0P1VpDGfpBl6G30BtoeNLcADfrOrTYGnLSHpoumik7RbMjD4tE1zm29JkbdaykULAFYS38AJ0WTXcffuY47ZxnmQA9lrAtjfWi5eQI9EX0GiF2SHhNMTDauDkPoM1Qj6Q5Cm7GLei4NK6BGVVfUnR2MTIZoaGwZ0Q/E41wRrplpYTraBpjoJOii6ODv0NTEjMamS1aQvLUO2bUcugetEJyHkgGbhDLmJ9FLGdcU64I3gP9gOaJXsDXQtEf4ZwQdBIVA6NgNzS19NmVnjl/ZyRhZL+EJlpDAEboBuiuqEMYwZXAtTJybQnjWr5Kuj+a4OovU/WoaEH39VTS669LF6ZyDDwM2YK5dCtX24mLlrQNcDB7LokePqEsaw79oSfQN+iFJ3ZR5dbchErqr3NwqLRYeGgxIkZ4Y+VKE3EOjrmZSg8zC7OHQedHapY/gribXGcNkl1/8ziYh5rrpa3S0s05OLS2NPx2bK+EW8AYmDU2UoeJliz/0MuE/W/aQcMx2tIcEOvgoaKLslHloiSthseWiBB09DjonOj1RyXNmfA3WWv9+s/azjF3hmRSSf9LXMpwk9LgHNZd1l+LczDrZ+i05+HyWrSbqARG3iFRZ9PpMac/g4v9r2shXb9fJzmi191gqP66J5ZP0Hhj82F0c57/PsHBurhE0p/yhkCvJL2nZPTwoImudxkwI1g2tlhDAB5yd0QdzQ2phS5K+D6aUCMaGX4dfCfqDKYxL8TT09n4P/u+LvyyYS70UPS9hQ/fZfjXYF/q5jBa2Qvbdxz2xpk59RJ2fjVYAN0XDUAG0YCkuTow/R5JuIZXgutDt5rxasPN7WUHqwnTZ7to6sXUtljYzt0s/Y2BTrAPSWlqrYxoNnyUZVvEx+CWgBu1TfTJLGbTWP/5fsM+JKWp0hdIrQLf3fL9btQhkAHbINa9lrhWm4JPUmvsYE7YLvLRu3BuQUFBQUFu/gC91d6HNHGY9wAAAABJRU5ErkJggg==>

[image15]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAE4AAAAZCAYAAACfIRhSAAADk0lEQVR4Xu2XW8hOWRjHHyHknFMyk0OMJCKnCBlJpEFjRChJDrlBQuRObrhQiBLJhUO4cSHKlE8uyJQrh1JySCaE1NCkHP4/z368632/7z18h4sv7X/9et+91157r/Vfz3rWWma5cuXKlaspWiymiC6ijegrVogx6UNSJ7FMHBV7xfDi4u+i/iRxQBwW80Tboid+ErUT58TXEs6L7slz/L8qdpsbjKn3xaLkGUzbJq6LwaKXOGVudPvkuVavzlZbg4+Ju+KZuGANR8l28Y/omdxbLh6Iftn1OPFSTP3xhNkQ8VTMSe61WjGFiJjTok9JWUM6aN7pcsIsTDtZcn+C+E/Mz673mJvU/8cTZl3FDXHCPCJbnSK3XBOHxK/FxRVVzbgR4o3VN446H8wN6yguWX3jmNZ1VojWHmKg+EMMMK83I7tmaiOinXcvFIOsvuH0baVYJ0aZRzgpp1HiIzPNR3WfFT7eGGH0fnFHPBc3xdikPAwqZxz3w6ByxsX9zeKdeR7FcCJxqfli8tY8Z54Va8VW8/dvsoLWiDPm5gMLEMHCd2oShi0Qt8Qu0a24uFGi8TuskNdYUenExOyaaKCjlYzDFMypZhxiin8UV8xXavSL+aC9EEOze0QRObfO/D3k7L/Fn1k56i2OZOUVRbJnhIiOjeYva67IQ+liEJ1gZGn8XKtuHAvEI6vNuKjHVAuF8azC6dTk3XXm74l08K/YIIaJDubtT+s0qN/FE7HeCqPV0opOYASGtORURVGPSA7FN0u/kRqH2Aax+se26b35wNakNOrYNzVnmq4SX8wHIlQ67dhSMMqlnQoDdlphWpUzjhxMZKDmGIfo/3jz71LntRiZlFdV5DlWrKYuDOzPGLnUuJiqdeYNjs4zTZguoVniU/aLeBerL6twiBx0zzz5h5pqHJCPWY1D5EPyYvqumtXGirci6YhX02TzOulGeYn434pPBSwYTJHB2TXf5BTB4hQnDDqB4cyG0DTxyvw7oVgc0iRfyTiMZwAwjj7SlhCphF0Ag9Fk0ZnR4rI4boVOVhJ1tpg3aLV5ZBA1JF/KQhjL6sVz7K8wjQ6Vnmcx47H5dmKl+bEsfRfbns9WfLTbar6RjnsPzU8l/MY9yomqi+J29stRjoErbWuzhGkcxGNpryY2lRgy24rPqKlo3G/iLzHdyh/nSBl0Mt3UtoT4fuwiiD6irVwbcuXKlStXrly5Wou+AbRD0cCyIhLtAAAAAElFTkSuQmCC>

[image16]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAFgAAAAaCAYAAAAzBZtTAAAD6ElEQVR4Xu2YXahOWRjHHxnF+BoRI5PQNCVTLsQVF6NDXPhoJKS5mZtxcYQRczOjmeTeTDSREiVMbkloHCmENCNTUxIz+WhoXAgXvv+/s97F2uus/e69va+T2L/6d953Pfusvd5nP8+znrXNampq0vSThsaDFWnHHO8kOGWbNC02NGGitEbqH4zh4M3Sl8HY20Af6TNptjQwsjXlC+mO9DzQXekraYR0Qnoa2O5L+yx7kw+krdKKYKwIHPmbdFIaHNmGS4es2sN6kwyQfpV2SCul09InmStKwD8/kzpig7kxbDvNPcmYWVKXVUttIpQ5/5FGRzaYIx2RBsWGXoZA2G7ut/OZbDsofRdeVMQw6Zx0TRqTNXXDZETv8thg7ukeljpjQxNGSr9L/0m3pAlZczc8rFPS0tiQgIdOFqQefqssMbfOzxvfuceehkrfj1r4v3TAXLqH8J1x7FwXM0W6bGlbHuvMlZNd0gNzc6TYZOk1xWBfLR2XZkp9s+bXhlJ1Vtprr9ZARnU1VDq7iEwiNBX2RPQ1cxFOpMd8Y+k6msckab+5xeHgx9L0zBWvmGuuhJStd+wLq6QL5iKflG4F5qCMhVlEOWNNlSL4F+mJtNDcBKEWm7sJ16TASagMRMHP0ozGd1965r28IguRfV2aGhsKoGx9Lf3R+Mv3qrBWIjcuYazloeX7owe+/pKqu80V9FBXLb/++nQhlcvAZkgL5tOtWW0HHy15D6AIIpjouyh9Lw3JmpsySroiPZL+DUQX1WzNPWil/noHp0pLDJsWEfFpMNasNIF3cOkfkwM1eYG5QNpg5RxN9hB0YaQW+SOJ/5HfxgYrrr9VHMym5nvpWHnp5h2cWltVcDIZ9Je0NrKlIGviSB1vrmSFm14h9L95Gw1j2PIcUNbB48wtil05xEdJXg1vtUSAj94zVq1McE9qbVj/KTeM+T2kkFb6X/Apw0PKg2uou0RPjHfwMUsfP9lcbprrJqri6y+OpbNIzd8Mgov+17eQvt/nRFc6ev0PTNVff2K5J02ObCFEN9eF7xM8RM8yyz/ljZVuWH5PSfSw0ZSud9a+Vo1N7rw5R9OOdUpHLf07etBhLjLCOnjbnDNIYyZi9/Q2PtP3fcg/R8yXLpl7bxHCu4xwDlomfw1OoBeO33H80LB7yJwuSzs/htSnBBCxlIR2HDYWSX+aC0CC6OOsuXeg8P9t6RreCr4P/TEaT8Ehh0xq5ynOw8P9KB7sTUifjdKWxud2QTtHNIZt3XsLR9nj5o7B7YAH9ZO0vvG5xty7W97vltoECqANou61Y653CmogO3gr0C5y9K6dW1NTU1NTmRdowNZRvH5hFQAAAABJRU5ErkJggg==>

[image17]: <data:image/png;base64,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>

[image18]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABUAAAAYCAYAAAAVibZIAAAAdklEQVR4XmNgGAWjYMABBxCnATEPugQlgBGIW4HYGF2CUgAysBeIWdAlKAEg1xYAcRyUjRUIALEkiVgOiOcD8WQg5mOgEjAB4tVALIMuQS4QBuLFQCyPLkEJyALiCHRBSgAonU4FYml0CUoAKLZ5ofQoGAX0AAA5bAi7Yfn2hgAAAABJRU5ErkJggg==>

[image19]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEMAAAAWCAYAAACbiSE3AAAAqUlEQVR4Xu3VIQoCURSF4SsyQQSLTDAYBMFmnawbsLoEm83kCrRarRYXYDRYBME1uBT/hwjeuwHDuT98ZU57zLwxy7Isy7J/1EUVH6o1wRkn1GGTqIUGVxww9LNGbcxxwx59P2tUDmGBO7bo+VmjciEu8cTaPpekbDO8sELHT5r9vh0bE/1EYt9742HCl2cs/lYHftasHMoUFxwx8rNu5SB2GMchy7JMrTesTRFIjbruQQAAAABJRU5ErkJggg==>

[image20]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEkAAAAWCAYAAACMq7H+AAAAq0lEQVR4Xu3VIQoCURSF4StiEMEiBoNBEGxWs27A6hJsNpMr0Gq1WlyAcYJFEFyDS/F/DMK7N2izvPPBX+a0x8wbMxERETHrUCs+lNqELnSmftiK1qAZVXSkoZ/L1qQF3ehAPT+XLR3Oku60o66fy5Yu4hU9aWP15SzBnF60prafJJe/TVvTp/bV5156mC7tn+Lvf+BnyaXDmtKVTjTys0TpgPY0joOIiPzHG0DMEUjO2YjJAAAAAElFTkSuQmCC>

[image21]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEwAAAAWCAYAAABqgnq6AAAAr0lEQVR4Xu3WIQoCQRiG4V/EIIJFDAaDINisZvcCVo9gs5k8gVar1eIBjBssguAZPMq+gw74j+yiYDB8D7xlvzbsDmsmIiIiv9OiRvpQ3o3oSAfqJps81WhCOe2o72eJ6pTRmbbU8bNE4aBmdKE1tf0sUbjE53SjpT0udqkwpTstqOknKfP6lq1Mn+PH4j12NV34X0l/KXp+ljLh4MZ0oj0N/CxVwmFtaJgOIiLy/wqLBBFIP5Ml+AAAAABJRU5ErkJggg==>