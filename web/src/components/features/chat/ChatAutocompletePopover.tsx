import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Sparkles,
  Wrench,
  HelpCircle,
  RotateCcw,
  Plus,
  FileCode,
  FileText,
  FileSpreadsheet,
  Image as ImageIcon,
  Archive,
  File,
  Folder,
  X,
} from 'lucide-react';
import type { ToolInfo } from '@/lib/types';

export interface AutocompleteSkillItem {
  id: string;
  name: string;
  command: string;
  description: string;
  category?: string;
  type: 'skill' | 'system';
}

export interface AutocompleteFileItem {
  id: string;
  name: string;
  path: string;
  size?: number;
  isDir?: boolean;
}

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function getFileIcon(filename: string, isDir?: boolean) {
  if (isDir) return Folder;
  const ext = filename.split('.').pop()?.toLowerCase() || '';
  if (['png', 'jpg', 'jpeg', 'webp', 'gif', 'svg'].includes(ext)) return ImageIcon;
  if (['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'c', 'cpp', 'json', 'yaml', 'yml', 'html', 'css'].includes(ext)) {
    return FileCode;
  }
  if (['md', 'txt', 'log', 'env'].includes(ext)) return FileText;
  if (['csv', 'tsv', 'xlsx'].includes(ext)) return FileSpreadsheet;
  if (['zip', 'tar', 'gz', 'wasm'].includes(ext)) return Archive;
  return File;
}

export const SYSTEM_COMMANDS: AutocompleteSkillItem[] = [
  {
    id: 'cmd-help',
    name: 'help',
    command: '/help',
    description: 'Show available commands, tools, and usage guide',
    type: 'system',
  },
  {
    id: 'cmd-clear',
    name: 'clear',
    command: '/clear',
    description: 'Clear current conversation messages',
    type: 'system',
  },
  {
    id: 'cmd-new',
    name: 'new',
    command: '/new',
    description: 'Start a fresh conversation session',
    type: 'system',
  },
];

export function getFilteredSkills(skills: ToolInfo[], query: string): AutocompleteSkillItem[] {
  const q = query.toLowerCase().trim();

  // Map installed skills
  const installed: AutocompleteSkillItem[] = skills.map((s) => ({
    id: s.name,
    name: s.name.replace(/^skill[-_]/, ''),
    command: `/${s.name.replace(/^skill[-_]/, '')}`,
    description: s.description || s.name,
    category: s.category || 'skill',
    type: 'skill' as const,
  }));

  const combined = [...SYSTEM_COMMANDS, ...installed];
  if (!q) return combined;

  return combined.filter(
    (item) =>
      item.name.toLowerCase().includes(q) ||
      item.command.toLowerCase().includes(q) ||
      item.description.toLowerCase().includes(q)
  );
}

export function getFilteredFiles(files: AutocompleteFileItem[], query: string): AutocompleteFileItem[] {
  const q = query.toLowerCase().trim();
  if (!q) return files;
  return files.filter(
    (f) =>
      f.name.toLowerCase().includes(q) ||
      f.path.toLowerCase().includes(q)
  );
}

export interface ChatAutocompletePopoverProps {
  type: 'slash' | 'mention' | null;
  query: string;
  filteredSkills: AutocompleteSkillItem[];
  filteredFiles: AutocompleteFileItem[];
  selectedIndex: number;
  onSelectSkill: (item: AutocompleteSkillItem) => void;
  onSelectFile: (file: AutocompleteFileItem) => void;
  onHoverIndex?: (index: number) => void;
  onClose: () => void;
}

export function ChatAutocompletePopover({
  type,
  filteredSkills,
  filteredFiles,
  selectedIndex,
  onSelectSkill,
  onSelectFile,
  onHoverIndex,
  onClose,
}: ChatAutocompletePopoverProps) {
  const { t } = useTranslation('chat');
  const containerRef = useRef<HTMLDivElement>(null);

  const itemCount = type === 'slash' ? filteredSkills.length : filteredFiles.length;

  // Auto-scroll selected item into view
  useEffect(() => {
    if (!containerRef.current) return;
    const selectedEl = containerRef.current.querySelector('[data-selected="true"]');
    if (selectedEl) {
      selectedEl.scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIndex]);

  if (!type || itemCount === 0) {
    if (!type) return null;
    return (
      <div className="absolute bottom-full left-0 right-0 mb-2 z-50 rounded-[20px] border border-onyx/15 bg-canvas/95 backdrop-blur-md p-4 shadow-xl text-center">
        <p className="font-sans text-body-sm text-slate">
          {type === 'slash' ? t('popover.noSkillsFound') : t('popover.noFilesFound')}
        </p>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      className="absolute bottom-full left-0 right-0 mb-2 z-50 max-h-64 overflow-y-auto rounded-[20px] border border-onyx/15 bg-canvas/95 backdrop-blur-md p-1.5 shadow-xl transition-all"
    >
      <div className="px-3 py-1.5 border-b border-onyx/10 flex items-center justify-between mb-1">
        <span className="font-sans text-[11px] font-semibold uppercase tracking-wider text-slate">
          {type === 'slash' ? t('popover.skills') : t('popover.workspaceFiles')}
        </span>
        <div className="flex items-center gap-2">
          <span className="font-sans text-[10px] text-slate/70">
            {type === 'slash' ? t('popover.slashHint') : t('popover.mentionHint')}
          </span>
          <button
            type="button"
            onClick={onClose}
            className="p-0.5 rounded-full text-slate hover:text-deep-ink hover:bg-onyx/10 transition-colors"
            title="Close"
          >
            <X className="w-3 h-3" />
          </button>
        </div>
      </div>

      <div className="space-y-0.5" role="listbox">
        {type === 'slash' &&
          filteredSkills.map((item, idx) => {
            const isSelected = idx === selectedIndex;
            const Icon =
              item.name === 'help'
                ? HelpCircle
                : item.name === 'clear'
                  ? RotateCcw
                  : item.name === 'new'
                    ? Plus
                    : item.type === 'skill'
                      ? Sparkles
                      : Wrench;

            return (
              <button
                key={item.id}
                type="button"
                role="option"
                aria-selected={isSelected}
                data-selected={isSelected}
                onMouseEnter={() => onHoverIndex?.(idx)}
                onClick={() => onSelectSkill(item)}
                className={`w-full flex items-center justify-between gap-3 px-3 py-2 rounded-xl text-left transition-colors font-sans text-body-sm ${
                  isSelected
                    ? 'bg-soft-meadow text-deep-ink font-medium shadow-xs'
                    : 'text-deep-ink/80 hover:bg-canvas/80 hover:text-deep-ink'
                }`}
              >
                <div className="flex items-center gap-2.5 min-w-0 flex-1">
                  <div
                    className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                      isSelected ? 'bg-hi-yellow text-deep-ink' : 'bg-onyx/5 text-slate'
                    }`}
                  >
                    <Icon className="w-3.5 h-3.5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-xs font-semibold text-deep-ink">
                        {item.command}
                      </span>
                      <span
                        className={`text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded-full font-semibold ${
                          item.type === 'skill'
                            ? 'bg-hi-yellow/20 text-deep-ink'
                            : 'bg-onyx/10 text-slate'
                        }`}
                      >
                        {item.type === 'skill' ? t('popover.installedSkill') : t('popover.systemCommand')}
                      </span>
                    </div>
                    <p className="text-[11px] text-slate truncate mt-0.5">
                      {item.description}
                    </p>
                  </div>
                </div>
              </button>
            );
          })}

        {type === 'mention' &&
          filteredFiles.map((file, idx) => {
            const isSelected = idx === selectedIndex;
            const Icon = getFileIcon(file.name, file.isDir);

            return (
              <button
                key={file.id || file.path}
                type="button"
                role="option"
                aria-selected={isSelected}
                data-selected={isSelected}
                onMouseEnter={() => onHoverIndex?.(idx)}
                onClick={() => onSelectFile(file)}
                className={`w-full flex items-center justify-between gap-3 px-3 py-2 rounded-xl text-left transition-colors font-sans text-body-sm ${
                  isSelected
                    ? 'bg-soft-meadow text-deep-ink font-medium shadow-xs'
                    : 'text-deep-ink/80 hover:bg-canvas/80 hover:text-deep-ink'
                }`}
              >
                <div className="flex items-center gap-2.5 min-w-0 flex-1">
                  <div
                    className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                      isSelected ? 'bg-hi-yellow text-deep-ink' : 'bg-onyx/5 text-slate'
                    }`}
                  >
                    <Icon className="w-3.5 h-3.5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <span className="font-mono text-xs font-semibold text-deep-ink truncate block">
                      {file.name}
                    </span>
                    <span className="text-[11px] text-slate truncate block">
                      {file.path}
                    </span>
                  </div>
                </div>
                {file.size ? (
                  <span className="text-[11px] text-slate/70 shrink-0 font-mono">
                    {formatBytes(file.size)}
                  </span>
                ) : null}
              </button>
            );
          })}
      </div>
    </div>
  );
}
