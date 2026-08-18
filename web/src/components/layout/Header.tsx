import { useState, useEffect } from 'react';
import { Menu, Sparkles } from 'lucide-react';
import { api } from '@/lib/api';
import type { NavTab } from '@/components/layout/Sidebar';

export interface HeaderProps {
  activeTab: NavTab;
  onOpenMobileSidebar: () => void;
  collapsed?: boolean;
  onLogout?: () => void;
}

export function Header({ activeTab, onOpenMobileSidebar, onLogout }: HeaderProps) {
  const [metrics, setMetrics] = useState<any>(null);

  useEffect(() => {
    api.getMetrics().then((m) => setMetrics(m)).catch(() => null);
    const interval = setInterval(() => {
      api.getMetrics().then((m) => setMetrics(m)).catch(() => null);
    }, 10000);
    return () => clearInterval(interval);
  }, []);

  const tabTitles: Record<NavTab, { title: string; category: string }> = {
    dashboard: { title: 'Dashboard', category: 'Overview' },
    agents: { title: 'Agents', category: 'AI Management' },
    'agent-studio': { title: 'Agent Studio', category: 'Configuration' },
    chat: { title: 'Chat', category: 'Conversation' },
    missions: { title: 'Missions & Tasks', category: 'Operations' },
    operations: { title: 'Live Operations', category: 'Observability' },
    automations: { title: 'Automations', category: 'Scheduling' },
    tools: { title: 'Tools', category: 'System Tools' },
    skills: { title: 'Skills', category: 'Agent Skills' },
    workspace: { title: 'Workspace', category: 'Files' },
    channels: { title: 'Chat Channels', category: 'Connections' },
    connectors: { title: 'Connectors', category: 'Services' },
    settings: { title: 'Settings', category: 'System' },
  };

  const current = tabTitles[activeTab] || { title: 'ActonOS', category: 'Kernel' };

  return (
    <header className="h-16 px-4 sm:px-8 bg-canvas/80 backdrop-blur-md border-b border-onyx/10 sticky top-0 z-30 flex items-center justify-between">
      {/* Left: Mobile trigger & Page title */}
      <div className="flex items-center gap-3">
        <button
          onClick={onOpenMobileSidebar}
          className="lg:hidden p-2 rounded-full bg-soft-meadow border border-onyx/10 text-deep-ink hover:bg-white transition-colors cursor-pointer"
          aria-label="Open sidebar"
        >
          <Menu className="w-5 h-5" />
        </button>

        <img
          src="/actonos_logo.png"
          alt="ActonOS"
          className="h-6 w-auto object-contain lg:hidden"
        />

        <div className="hidden lg:flex items-center gap-2">
          <span className="text-caption font-mono uppercase text-slate font-medium">
            {current.category}
          </span>
          <span className="text-slate font-mono">/</span>
          <h2 className="font-serif font-bold text-deep-ink text-body sm:text-heading-sm tracking-tight">
            {current.title}
          </h2>
        </div>
      </div>

      {/* Right: Telemetry mini badges & Lock Action */}
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
          <span className="hidden sm:inline">Active</span>
        </div>

        {onLogout && (
          <button
            onClick={onLogout}
            className="flex items-center gap-1.5 px-3 py-1 bg-soft-meadow hover:bg-white text-slate hover:text-deep-ink rounded-full border border-onyx/10 text-caption font-sans font-medium transition-colors cursor-pointer"
            title="Lock ActonOS Session"
          >
            <span>Lock</span>
          </button>
        )}
      </div>
    </header>
  );
}
