import type { Page } from '@playwright/test';

const metrics = {
  cpu: { usage_percent: 18.4, temperature_celsius: 46.2, cores: 8, model: 'Acton MiniPC' },
  memory: { used_mb: 4096, total_mb: 16384, actond_mb: 148 },
  disk: { used_gb: 82.5, total_gb: 512, data_dir_gb: 12.4 },
  uptime_seconds: 86400,
  containers: [
    {
      id: 'container-1',
      name: 'actonos',
      image: 'actonos/latest',
      state: 'running',
      status: 'healthy',
      cpu_percent: 2.4,
      memory_usage_mb: 256,
    },
  ],
};

function response(pathname: string) {
  if (pathname === '/api/auth/status') {
    return { initialized: true, authenticated: true, user_name: 'Operator' };
  }
  if (pathname === '/api/dashboard/summary') {
    return {
      agents_count: 2,
      agents_active: 1,
      tools_count: 8,
      tools_native: 4,
      tools_mcp: 2,
      tools_skills: 2,
      tools_wasm: 0,
      cron_count: 1,
      storage: { storage_bytes: 1000, vectors_bytes: 2000, workspace_bytes: 3000, logs_bytes: 4000, total_bytes: 10000 },
      recent_audit: [],
      metrics,
      timestamp: new Date().toISOString(),
    };
  }
  if (pathname.startsWith('/api/system/tokens')) {
    return { today_tokens: 1200, today_cost_usd: 0.12, month_tokens: 12500, month_cost_usd: 1.42, by_model: [], by_agent: [] };
  }
  if (pathname.startsWith('/api/heartbeat')) {
    return pathname.endsWith('/config')
      ? { enabled: true, interval_minutes: 5, directives: '', target_channel: 'all', target_account_id: 'all', auto_delegate: true }
      : { runs: [] };
  }
  if (pathname === '/api/agents') {
    return {
      agents: [
        {
          agent_id: 'agent_system_core',
          name: 'Nova',
          description: 'System operator',
          status: 'active',
          is_system: true,
          authorized_tools: ['*'],
          listen_channels: ['*'],
          model_config: { primary_model: 'openai/gpt-5', reasoning_effort: 'medium', max_tokens: 4096 },
          delegation_scope: { require_human_approval_level: 'Medium', max_monthly_budget_usd: 50 },
        },
      ],
      count: 1,
    };
  }
  if (pathname === '/api/tasks') return { tasks: [], count: 0 };
  if (pathname === '/api/tools') return { tools: [], count: 0 };
  if (pathname === '/api/workspace') return { files: [], current_dir: '' };
  if (pathname === '/api/runs') return { runs: [] };
  if (pathname === '/api/approvals') return { approvals: [], count: 0 };
  if (pathname === '/api/plugins') return { plugins: [], count: 0 };
  if (pathname === '/api/integrations/channels/accounts') return { accounts: [], count: 0 };
  if (pathname === '/api/integrations/authorizations') return { users: [], count: 0 };
  if (pathname === '/api/integrations/pairing/codes') return { codes: [], count: 0 };
  if (pathname === '/api/integrations/pairing/pending') return { pending: [], count: 0 };
  if (pathname === '/api/integrations/pairing/policy') return { policies: {} };
  return {};
}

export async function mockAuthenticatedApi(page: Page) {
  await page.route(/\/api\/.*/, async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: response(url.pathname) }),
    });
  });
}
