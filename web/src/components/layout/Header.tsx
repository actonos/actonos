import { useState, useEffect } from 'react';
import { Menu, Sparkles } from 'lucide-react';
import { api } from '@/lib/api';
import type { NavTab } from '@/components/layout/Sidebar';

export interface HeaderProps {
  activeTab: NavTab;
  onOpenMobileSidebar: () => void;
  collapsed?: boolean;
}

export function Header({ activeTab, onOpenMobileSidebar }: HeaderProps) {
  const [metrics, setMetrics] = useState<any>(null);

  useEffect(() => {
    api.getMetrics().then((m) => setMetrics(m)).catch(() => null);
    const interval = setInterval(() => {
      api.getMetrics().then((m) => setMetrics(m)).catch(() => null);
    }, 10000);
    return () => clearInterval(interval);
  }, []);

  const tabTitles: Record<NavTab, { title: string; category: string }> = {
    agents: { title: 'Autonomous Agents', category: 'Universal Engine' },
    chat: { title: 'ReAct Chat Canvas', category: 'Cognition & Reasoning' },
    tools: { title: 'Tool Hub & MCP', category: 'Dynamic Tooling' },
    workspace: { title: 'Workspace Explorer', category: 'Sandboxed Filesystem' },
    integrations: { title: 'SaaS & Channels', category: 'Multi-Channel Connectors' },
    settings: { title: 'Settings & Hardware', category: 'System Administration' },
  };

  const current = tabTitles[activeTab] || { title: 'ActonOS', category: 'Kernel' };

  return (
    <header className="h-16 px-4 sm:px-8 bg-canvas/80 backdrop-blur-md border-b border-onyx/10 sticky top-0 z-30 flex items-center justify-between">
      {/* Left: Mobile trigger & Page title */}
      <div className="flex items-center gap-3">
        <button
          onClick={onOpenMobileSidebar}
          className="lg:hidden p-2 rounded-full bg-soft-meadow border border-onyx/10 text-deep-ink hover:bg-white transition-colors"
          aria-label="Open sidebar"
        >
          <Menu className="w-5 h-5" />
        </button>

        <div className="flex items-center gap-2">
          <span className="hidden sm:inline text-caption font-mono uppercase text-slate font-medium">
            {current.category}
          </span>
          <span className="hidden sm:inline text-slate font-mono">/</span>
          <h2 className="font-serif font-bold text-deep-ink text-body sm:text-heading-sm tracking-tight">
            {current.title}
          </h2>
        </div>
      </div>

      {/* Right: Telemetry mini badges */}
      <div className="flex items-center gap-2.5">
        {metrics && (
          <div className="hidden md:flex items-center gap-3 px-3 py-1 bg-soft-meadow rounded-full border border-onyx/10 text-caption font-mono text-slate">
            <span>CPU: {metrics.cpu?.usage_percent?.toFixed(0) || 0}%</span>
            <span>•</span>
            <span>RAM: {metrics.memory?.used_mb || 0} MB</span>
          </div>
        )}

        <div className="flex items-center gap-1.5 px-3 py-1 bg-soft-meadow rounded-full border border-onyx/10 text-caption font-mono text-deep-ink font-medium">
          <Sparkles className="w-3.5 h-3.5 text-hi-yellow" />
          <span className="hidden sm:inline">Cascade Active</span>
        </div>
      </div>
    </header>
  );
}
