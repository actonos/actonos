export type AgentStatus = 'active' | 'stopped' | 'error';
export type ApprovalLevel = 'Low' | 'Medium' | 'High';

export interface ModelConfig {
  primary_model: string;
  fallback_model?: string;
  temperature?: number;
  max_tokens?: number;
}

export interface DelegationScope {
  max_monthly_budget_usd: number;
  allowed_workspace_paths: string[];
  require_human_approval_level: ApprovalLevel;
}

export interface TriggerRule {
  type: string;
  channel?: string;
  filter?: string;
  expression?: string;
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
  channel?: string; // 'telegram' | 'whatsapp' | 'discord' | 'webhook'
  token?: string;
  phone_id?: string;
  webhook_secret?: string;
  enabled: boolean;
  bound_agent_ids?: string[]; // e.g. ['*'] or ['agent_support', 'agent_devops']
  default_chat_id?: string;
}

export interface ConversationItem {
  id: string;
  agent_id: string;
  title: string;
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

export interface ToolInfo {
  name: string;
  description: string;
  category: 'native' | 'mcp' | 'wasm' | 'skill';
  schema: Record<string, any>;
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
  uptime_seconds: number;
  timestamp: string;
}

export interface TailscaleStatus {
  connected: boolean;
  ip?: string;
  fqdn?: string;
  hostname: string;
  peers_count: number;
  auth_key_set: boolean;
}

export interface ConnectorInfo {
  id: string;
  name: string;
  category: string;
  icon: string;
  risk_level: 'Low' | 'Medium' | 'High';
  description: string;
  connected: boolean;
  auth_type?: 'oauth' | 'token';
  account_name?: string;
  account_email?: string;
  avatar_url?: string;
  connected_at?: string;
  scopes?: string[];
  expires_at?: string;
  client_id?: string;
  client_secret?: string;
  direct_token?: string;
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
}

export interface HeartbeatConfigData {
  enabled: boolean;
  interval_minutes: number;
  directives: string;
  target_channel: string;
  target_account_id: string;
  auto_delegate: boolean;
  zero_noise: boolean;
}

export interface HeartbeatRun {
  id: string;
  agent_id: string;
  executed_at: string;
  status: 'ok' | 'action_taken' | 'error';
  summary: string;
  tokens_used: number;
}

