import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Bot,
  Calendar,
  Folder,
  Gauge,
  LayoutDashboard,
  MessageSquare,
  Search,
  Settings,
  ShieldCheck,
  Target,
  Wrench,
  Bell,
  Radio,
  type LucideIcon,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { NavTab } from '@/components/layout/Sidebar';
import type { AgentManifest, AutonomousTask, ToolInfo } from '@/lib/types';

interface SearchFile {
  name: string;
  path: string;
  is_dir: boolean;
}

interface CommandItem {
  id: string;
  label: string;
  description?: string;
  keywords: string;
  icon: LucideIcon;
  execute: () => void;
}

export interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  onNavigate: (tab: NavTab) => void;
  onOpenChat: (agentID?: string) => void;
  onEditAgent: (agentID: string) => void;
}

export function CommandPalette({
  isOpen,
  onClose,
  onNavigate,
  onOpenChat,
  onEditAgent,
}: CommandPaletteProps) {
  const { t } = useTranslation(['nav', 'common']);
  const [query, setQuery] = useState('');
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [tasks, setTasks] = useState<AutonomousTask[]>([]);
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [files, setFiles] = useState<SearchFile[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const [activeIndex, setActiveIndex] = useState(0);

  useEffect(() => {
    if (!isOpen) return;
    setQuery('');
    setActiveIndex(0);
    window.setTimeout(() => inputRef.current?.focus());
    Promise.all([
      api.listAgents().catch(() => ({ agents: [], count: 0 })),
      api.listTasks().catch(() => ({ tasks: [], count: 0 })),
      api.listTools().catch(() => ({ tools: [], count: 0 })),
      api.listWorkspaceFiles().catch(() => ({ files: [], current_dir: '' })),
    ]).then(([agentData, taskData, toolData, fileData]) => {
      setAgents(agentData.agents || []);
      setTasks(taskData.tasks || []);
      setTools(toolData.tools || []);
      setFiles(fileData.files || []);
    });
  }, [isOpen]);

  const run = (action: () => void) => {
    action();
    onClose();
  };

  const commands = useMemo<CommandItem[]>(() => {
    const navigation: CommandItem[] = [
      ['dashboard', LayoutDashboard],
      ['operations', Gauge],
      ['missions', Target],
      ['agents', Bot],
      ['chat', MessageSquare],
      ['automations', Calendar],
      ['channels', Radio],
      ['workspace', Folder],
      ['tools', Wrench],
      ['notifications', Bell],
      ['audit-logs', ShieldCheck],
      ['settings', Settings],
    ].map(([tab, icon]) => ({
      id: `nav:${tab}`,
      label: t(`nav:links.${tab}`),
      description: t('nav:search.navigation'),
      keywords: `${tab} ${t(`nav:links.${tab}`)}`,
      icon: icon as LucideIcon,
      execute: () => onNavigate(tab as NavTab),
    }));

    return [
      ...navigation,
      {
        id: 'action:new-agent',
        label: t('nav:search.createAgent'),
        description: t('nav:search.action'),
        keywords: 'new create agent',
        icon: Bot,
        execute: () => onEditAgent('new'),
      },
      ...agents.flatMap((agent) => [
        {
          id: `agent:edit:${agent.agent_id}`,
          label: agent.name,
          description: t('nav:search.editAgent', { id: agent.agent_id }),
          keywords: `${agent.name} ${agent.agent_id} ${agent.description || ''}`,
          icon: Bot,
          execute: () => onEditAgent(agent.agent_id),
        },
        {
          id: `agent:chat:${agent.agent_id}`,
          label: t('nav:search.chatWith', { name: agent.name }),
          description: agent.agent_id,
          keywords: `chat ${agent.name} ${agent.agent_id}`,
          icon: MessageSquare,
          execute: () => onOpenChat(agent.agent_id),
        },
      ]),
      ...tasks.map((task) => ({
        id: `task:${task.id}`,
        label: task.title,
        description: t('nav:search.task', { status: task.status }),
        keywords: `${task.title} ${task.status} ${task.priority}`,
        icon: Target,
        execute: () => onNavigate('missions'),
      })),
      ...tools.map((tool) => ({
        id: `tool:${tool.name}`,
        label: tool.name,
        description: tool.description,
        keywords: `${tool.name} ${tool.description} ${tool.category}`,
        icon: Wrench,
        execute: () => onNavigate('tools'),
      })),
      ...files.map((file) => ({
        id: `file:${file.path}`,
        label: file.name,
        description: file.path,
        keywords: `${file.name} ${file.path}`,
        icon: Folder,
        execute: () => onNavigate('workspace'),
      })),
    ];
  }, [agents, files, onEditAgent, onNavigate, onOpenChat, t, tasks, tools]);

  const normalized = query.trim().toLocaleLowerCase();
  const filtered = commands
    .filter((command) => !normalized || `${command.label} ${command.keywords}`.toLocaleLowerCase().includes(normalized))
    .slice(0, 18);

  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        setActiveIndex((index) => filtered.length ? (index + 1) % filtered.length : 0);
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault();
        setActiveIndex((index) => filtered.length ? (index - 1 + filtered.length) % filtered.length : 0);
      }
      if (event.key === 'Enter' && filtered[activeIndex]) {
        event.preventDefault();
        run(filtered[activeIndex].execute);
      }
      if (event.key === 'Tab' && dialogRef.current) {
        const focusable = dialogRef.current.querySelectorAll<HTMLElement>('input, button:not([disabled])');
        if (!focusable.length) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeIndex, filtered, isOpen, onClose]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-[70] flex items-start justify-center bg-deep-ink/45 px-3 pt-[10vh] backdrop-blur-sm" onMouseDown={onClose}>
      <section
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={t('nav:search.title')}
        className="w-full max-w-2xl overflow-hidden rounded-[24px] border border-onyx/10 bg-canvas"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-onyx/10 px-4">
          <Search className="h-5 w-5 shrink-0 text-slate" />
          <input
            ref={inputRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('nav:search.placeholder')}
            className="density-control min-w-0 flex-1 bg-transparent text-body text-deep-ink outline-none placeholder:text-slate hover:outline-none focus:outline-none"
          />
          <kbd className="rounded-full border border-onyx/10 bg-soft-meadow px-2 py-1 text-caption text-slate">{t('nav:search.escapeKey')}</kbd>
        </div>
        <div className="max-h-[60vh] overflow-y-auto p-2">
          {filtered.length === 0 ? (
            <p className="px-4 py-10 text-center text-body-sm text-slate">{t('nav:search.empty')}</p>
          ) : (
            filtered.map((command, index) => {
              const Icon = command.icon;
              return (
                <button
                  key={command.id}
                  type="button"
                  onClick={() => run(command.execute)}
                  aria-selected={index === activeIndex}
                  className={`density-row flex w-full items-center gap-3 rounded-[16px] px-3 text-left hover:bg-soft-meadow focus-visible:bg-soft-meadow ${index === activeIndex ? 'bg-soft-meadow' : ''}`}
                >
                  <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-soft-meadow text-deep-ink">
                    <Icon className="h-4 w-4" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-body-sm font-semibold text-deep-ink">{command.label}</span>
                    {command.description && <span className="block truncate text-caption text-slate">{command.description}</span>}
                  </span>
                </button>
              );
            })
          )}
        </div>
      </section>
    </div>
  );
}
