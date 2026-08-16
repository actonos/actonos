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
  label: string;
  token?: string;
  phone_id?: string;
  enabled: boolean;
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
