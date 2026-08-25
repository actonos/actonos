export type AgentStatus = 'active' | 'stopped' | 'error';
export type ApprovalLevel = 'Low' | 'Medium' | 'High';

export interface ModelConfig {
  primary_model: string;
  fallback_model?: string;
  reasoning_effort?: string;
  max_tokens?: number;
}

export interface DelegationScope {
  max_monthly_budget_usd: number;
  max_concurrent_runs?: number;
  max_tokens_per_hour?: number;
  allowed_workspace_paths: string[];
  require_human_approval_level: ApprovalLevel;
}

export interface HealthReport {
  status: string;
  version?: string;
  git_commit?: string;
  build_time?: string;
  uptime_seconds?: number;
  runtime_mode?: string;
  agents_active?: number;
  tailscale_connected?: boolean;
  tailscale_ip?: string;
  components?: Record<string, string>;
}

export interface TriggerRule {
  type: string;
  channel?: string;
  filter?: string;
  expression?: string;
}

export interface AgentHeartbeatConfig {
  enabled: boolean;
  interval_minutes?: number;
  directives?: string;
  target_channel?: string;
  target_account_id?: string;
  active_hours_start?: string;
  active_hours_end?: string;
  active_hours_timezone?: string;
}

export interface AgentManifest {
  agent_id: string;
  name: string;
  description: string;
  avatar_icon?: string;
  status: AgentStatus;
  is_system?: boolean;
  model_config: ModelConfig;
  system_instructions: string;
  authorized_tools: string[];
  /** Channels this agent listens to. ['*'] = all (default). */
  listen_channels: string[];
  heartbeat_config?: AgentHeartbeatConfig;
  delegation_scope: DelegationScope;
  trigger_rules?: TriggerRule[];
  created_at?: string;
  updated_at?: string;
}

/** A single connected account for a channel type (multi-account support). */
export interface ChannelAccount {
  id: string;
  name?: string;
  label?: string;
  channel?: string; // plugin channel id, e.g. 'telegram' | 'zalo' | 'webhook'
  token?: string;
  phone_id?: string;
  webhook_secret?: string;
  enabled: boolean;
  bound_agent_ids?: string[]; // e.g. ['*'] or ['agent_support', 'agent_devops']
  default_chat_id?: string;
  routing_mode?: 'exclusive' | 'mention' | 'fallback';
  requires_pairing?: boolean;
}

export interface ConversationItem {
  id: string;
  agent_id: string;
  title: string;
  channel?: string; // 'web' | 'mission' | 'system' | 'webhook' | installed plugin channel id
  is_pinned?: boolean;
  message_count?: number;
  last_message?: string;
  created_at: string;
  updated_at: string;
}

export interface ChatMessageRecord {
  id: string;
  conversation_id: string;
  agent_id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  tool_calls_json?: string;
  created_at: string;
}

export interface SkillRequirements {
  config?: string[];
  env?: string[];
  bins?: string[];
  tools?: string[];
  os?: string[];
}

export interface ToolInfo {
  name: string;
  description: string;
  category: 'native' | 'mcp' | 'wasm' | 'skill';
  schema: Record<string, unknown>;
  enabled?: boolean;
  requirements_met?: boolean;
  requirements?: SkillRequirements;
  missing_requirements?: string[];
}

export interface ContainerStatus {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  cpu_percent: number;
  memory_usage_mb: number;
  memory_limit_mb: number;
}

export interface SystemMetrics {
  cpu: {
    model: string;
    cores: number;
    usage_percent: number;
    temperature_celsius: number;
  };
  memory: {
    total_mb: number;
    used_mb: number;
    actond_mb: number;
  };
  disk: {
    total_gb: number;
    used_gb: number;
    data_dir_gb: number;
  };
  containers: ContainerStatus[];
  runtime_mode: string;
  canvas_url?: string;
  uptime_seconds: number;
  timestamp: string;
}

export interface MCPServerStatus {
  id: string;
  transport: string;
  command?: string;
  args?: string[];
  url?: string;
  enabled: boolean;
  connected: boolean;
  updated_at: string;
}

export interface RealtimeSnapshot {
  type: 'snapshot';
  timestamp: string;
  metrics?: SystemMetrics;
  runs?: AgentRun[];
  approvals?: ApprovalRequest[];
  tokens?: TokenUsageSummary;
  notifications_unread?: number;
  latest_notification?: NotificationItem;
}

export interface TailscaleStatus {
  connected: boolean;
  ip?: string;
  fqdn?: string;
  hostname: string;
  peers_count: number;
  auth_key_set: boolean;
}


export interface LLMProviderInfo {
  id: string;
  name: string;
  configured: boolean;
  masked_key: string;
  base_url: string;
  default_model: string;
  enabled: boolean;
  last_latency: number;
  last_tested: string;
  status: 'connected' | 'error' | 'not_configured' | 'configured';
}

export interface ModelSpec {
  id: string;
  name: string;
  provider_id: string;
  provider_name: string;
  badge?: string;
  context_window?: string;
  category: string;
  prompt_per_1m: number;
  completion_per_1m: number;
  is_default?: boolean;
  supports_tools?: boolean;
  supports_vision?: boolean;
}

export interface ProviderSpec {
  id: string;
  name: string;
  category: string;
  description: string;
  default_base_url: string;
  accent_color: string;
  model_presets: ModelSpec[];
}

export interface CatalogResponse {
  models: ModelSpec[];
  providers: ProviderSpec[];
}

export interface ChannelDefinition {
  id: string;
  nameKey: string;
  descKey: string;
  category: 'messaging' | 'enterprise' | 'custom' | 'community';
  capabilities: string[];
  hasPhoneId?: boolean;
  docsUrl?: string;
  isComingSoon?: boolean;
}

export type ConnectorCategory = 'all' | 'productivity' | 'development' | 'knowledge' | 'messaging' | 'database';

export interface ModelUsageStat {
  model: string;
  total_tokens: number;
  cost_usd: number;
  percentage: number;
}

export interface AgentUsageStat {
  agent_id: string;
  total_tokens: number;
  cost_usd: number;
  percentage: number;
}

export interface DailyUsagePoint {
  date: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost_usd: number;
}

export interface TokenUsageRecord {
  id: string;
  timestamp: string;
  agent_id: string;
  model: string;
  provider: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  estimated_cost_usd: number;
  source: string; // 'chat' | 'stream' | 'cron' | 'heartbeat' | 'swarm' | 'channel'
  conversation_id?: string;
}

export interface TokenUsageSummary {
  total_prompt_tokens: number;
  total_completion_tokens: number;
  total_tokens: number;
  total_cost_usd: number;
  today_tokens: number;
  today_cost_usd: number;
  month_tokens: number;
  month_cost_usd: number;
  by_model: ModelUsageStat[];
  by_agent: AgentUsageStat[];
  daily_trend: DailyUsagePoint[];
}

export interface CronExecutionRecord {
  id: string;
  job_id: string;
  agent_id: string;
  status: 'success' | 'failed';
  prompt: string;
  output?: string;
  error?: string;
  duration_ms: number;
  tokens_used: number;
  executed_at: string;
}

export type TaskStatus = 'pending' | 'in_progress' | 'completed' | 'blocked' | 'cancelled';
export type TaskPriority = 'p0_critical' | 'p1_high' | 'p2_normal' | 'p3_low';

export interface AutonomousTask {
  id: string;
  title: string;
  description: string;
  status: TaskStatus;
  priority: TaskPriority;
  assigned_agent_id: string;
  target_channel?: string;
  target_account_id?: string;
  progress: number;
  execution_log?: string;
  session_id?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  stalled_cycles?: number;
  fail_count?: number;
}

export interface HeartbeatConfigData {
  enabled: boolean;
  interval_minutes: number;
  directives: string;
  target_channel: string;
  target_account_id: string;
  auto_delegate?: boolean;
  zero_noise?: boolean;
  /** Max characters allowed to accompany HEARTBEAT_OK before a reply is treated as a real alert (default 300). */
  ack_max_chars?: number;
  /** Daily HH:MM window (e.g. "09:00") restricting routine (non-manual) pulses. Leave both empty to run 24/7. */
  active_hours_start?: string;
  active_hours_end?: string;
  /** IANA timezone name for active_hours_start/end; defaults to the server's local timezone. */
  active_hours_timezone?: string;
}

export interface HeartbeatRun {
  id: string;
  agent_id: string;
  executed_at: string;
  status: 'ok' | 'action_taken' | 'error' | 'skipped';
  summary: string;
  tokens_used: number;
}

export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'expired';

export type DontAskAgain = 'task' | 'today';

export interface ApprovalRequest {
  id: string;
  trace_id: string;
  agent_id: string;
  tool_name: string;
  risk_level: 'Low' | 'Medium' | 'High';
  action_hash: string;
  input: Record<string, unknown>;
  status: ApprovalStatus;
  reason?: string;
  requested_at: string;
  expires_at: string;
  decided_at?: string;
  decided_by?: string;
  task_id?: string;
}

export interface ApprovalRequiredResult {
  status: 'approval_required';
  approval: ApprovalRequest;
}

export type MutationResult<T> = T | ApprovalRequiredResult;

export function isApprovalRequired<T>(
  result: MutationResult<T>
): result is ApprovalRequiredResult {
  return (
    typeof result === 'object' &&
    result !== null &&
    'status' in result &&
    result.status === 'approval_required' &&
    'approval' in result
  );
}

export type AgentRunStatus =
  | 'running'
  | 'completed'
  | 'failed'
  | 'approval_pending'
  | 'blocked'
  | 'cancelled';

export interface AgentRun {
  id: string;
  trace_id: string;
  agent_id: string;
  goal: string;
  source: string;
  status: AgentRunStatus;
  termination_reason?: string;
  iterations: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  started_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface RunEvent {
  id: string;
  run_id: string;
  trace_id: string;
  step: number;
  type: string;
  status: string;
  tool_name?: string;
  data?: Record<string, unknown>;
  duration_ms?: number;
  created_at: string;
}

export type NotificationType = 'approval' | 'error' | 'warning' | 'info' | 'success';

export interface NotificationItem {
  id: string;
  title: string;
  message: string;
  type: NotificationType;
  category: string;
  link?: string;
  is_read: boolean;
  created_at: string;
}

export interface NotificationListResponse {
  notifications: NotificationItem[];
  total: number;
  page: number;
  limit: number;
  unread_count: number;
}

export interface PushSubscriptionKeys {
  p256dh: string;
  auth: string;
}

export interface PushSubscriptionPayload {
  endpoint: string;
  keys: PushSubscriptionKeys;
  user_agent?: string;
}

export interface VAPIDKeyResponse {
  public_key: string;
}

export interface TerminalShellOption {
  id: string;
  name: string;
  available: boolean;
}

export interface TerminalInfoResponse {
  os: string;
  default_shell: string;
  available_shells: TerminalShellOption[];
}

export interface WorkspaceFile {
  id: string;
  parent_id: string;
  name: string;
  path: string;
  virtual_path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  kind?: string;
  mime_type?: string;
  version: number;
  content_hash?: string;
  ai_indexed?: boolean;
  ai_state?: string;
}

export interface WorkspaceBreadcrumb {
  id: string;
  name: string;
  virtual_path: string;
}

export interface WorkspaceFileListResponse {
  files: WorkspaceFile[];
  parent_id: string;
  dir: string;
  count: number;
  virtual_root: string;
  breadcrumbs: WorkspaceBreadcrumb[];
}

export interface WorkspaceFileDetailResponse {
  id: string;
  file_id: string;
  parent_id: string;
  name: string;
  path: string;
  virtual_path: string;
  size: number;
  kind: string;
  mime: string;
  mime_type: string;
  version: number;
  content_hash: string;
  content?: string;
  raw_url: string;
}

export interface WorkspaceMutationResponse {
  status: string;
  id: string;
  file_id?: string;
  parent_id?: string;
  name: string;
  virtual_path?: string;
  version?: number;
}

export interface WorkspaceStatsResponse {
  total_size: number;
  total_files: number;
  total_directories: number;
  indexed_files: number;
  breakdown: {
    documents: number;
    code: number;
    data: number;
    media: number;
    other: number;
  };
}

export interface SemanticChunkItem {
  id: string;
  ordinal: number;
  content: string;
  token_count: number;
  active: boolean;
  created_at: string;
}

export interface WorkspaceChunksResponse {
  state: string;
  model_id?: string;
  model_revision?: string;
  chunker_version?: string;
  active_generation?: number;
  indexed_at?: string;
  chunk_count: number;
  chunks: SemanticChunkItem[];
}

export type PluginCapability = 'tool' | 'channel' | 'connector';
export type PluginStatus = 'running' | 'stopped' | 'disabled' | 'error';

export interface PluginPermissions {
  net_outbound?: string[];
  secrets?: string[];
  storage?: boolean;
  bus_events?: string[];
}

export interface PluginToolDef {
  name: string;
  description: string;
  parameters?: Record<string, unknown>;
  schema?: Record<string, unknown>;
  category?: string;
}

export interface PluginChannelDef {
  name: string;
  display_name: string;
  requires_pairing?: boolean;
}

export interface PluginConnectorDef {
  name: string;
  display_name: string;
  auth_type?: string;
  actions?: string[];
}

export interface PluginManifest {
  id: string;
  name: string;
  version: string;
  author?: string;
  description?: string;
  license?: string;
  capabilities: PluginCapability[];
  permissions: PluginPermissions;
  tools?: PluginToolDef[];
  channels?: PluginChannelDef[];
  connectors?: PluginConnectorDef[];
  config_schema?: Record<string, unknown>;
  config?: Record<string, unknown>;
}

export interface PluginInfo {
  manifest: PluginManifest;
  enabled: boolean;
  status: PluginStatus;
  error?: string;
  loaded_at?: string;
  path?: string;
  memory_bytes?: number;
}

export interface RegistryPlugin {
  id: string;
  name: string;
  version: string;
  author?: string;
  description?: string;
  license?: string;
  category?: string;
  filename?: string;
  icon?: string;
  tags?: string[];
  stars?: number;
  download_url?: string;
  url?: string;
  sha256?: string;
  size_bytes?: number;
  size?: number;
  capabilities?: PluginCapability[];
  permissions?: PluginPermissions;
  tools?: PluginToolDef[];
  channels?: PluginChannelDef[];
  connectors?: PluginConnectorDef[];
  config_schema?: Record<string, unknown>;
  installed?: boolean;
  installed_status?: PluginStatus;
  installed_version?: string;
}

export interface VaultSecretMeta {
  name: string;
  updated_at: string;
  is_provider?: boolean;
}


