import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Folder,
  FolderOpen,
  ChevronRight,
  ChevronDown,
  FileText,
  FileCode,
  FileSpreadsheet,
  Image as ImageIcon,
  FileArchive,
  File,
  MoreVertical,
  Loader2,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { WorkspaceFile } from '@/lib/types';

interface WorkspaceTreeItemProps {
  file: WorkspaceFile;
  level?: number;
  selectedPath: string | null;
  selectedPaths: Set<string>;
  onSelect: (file: WorkspaceFile) => void;
  onToggleSelect: (path: string, e: React.MouseEvent) => void;
  onContextMenu: (file: WorkspaceFile, e: React.MouseEvent) => void;
  onNavigateDir: (dir: string) => void;
}

function getFileIcon(name: string, isDir: boolean, isOpen: boolean) {
  if (isDir) {
    return isOpen ? (
      <FolderOpen className="w-4 h-4 text-hi-yellow shrink-0 fill-hi-yellow/20" />
    ) : (
      <Folder className="w-4 h-4 text-hi-yellow shrink-0 fill-hi-yellow/20" />
    );
  }

  const ext = name.split('.').pop()?.toLowerCase() || '';
  switch (ext) {
    case 'py':
    case 'go':
    case 'js':
    case 'ts':
    case 'tsx':
    case 'jsx':
    case 'sh':
    case 'rs':
    case 'c':
    case 'cpp':
    case 'html':
    case 'css':
      return <FileCode className="w-4 h-4 text-deep-ink shrink-0" />;
    case 'csv':
    case 'tsv':
    case 'xlsx':
    case 'xls':
      return <FileSpreadsheet className="w-4 h-4 text-moss-green shrink-0" />;
    case 'png':
    case 'jpg':
    case 'jpeg':
    case 'svg':
    case 'webp':
    case 'gif':
      return <ImageIcon className="w-4 h-4 text-fuchsia shrink-0" />;
    case 'zip':
    case 'tar':
    case 'gz':
    case '7z':
      return <FileArchive className="w-4 h-4 text-status-warning shrink-0" />;
    case 'md':
    case 'txt':
    case 'pdf':
    case 'docx':
    case 'doc':
      return <FileText className="w-4 h-4 text-slate shrink-0" />;
    default:
      return <File className="w-4 h-4 text-slate shrink-0" />;
  }
}

export function WorkspaceTreeItem({
  file,
  level = 0,
  selectedPath,
  selectedPaths,
  onSelect,
  onToggleSelect,
  onContextMenu,
  onNavigateDir,
}: WorkspaceTreeItemProps) {
  const { t } = useTranslation('workspace');
  const [isOpen, setIsOpen] = useState(false);
  const [children, setChildren] = useState<WorkspaceFile[] | null>(null);
  const [loadingChildren, setLoadingChildren] = useState(false);

  const isSelected = selectedPath === file.id;
  const isChecked = selectedPaths.has(file.id);

  const fetchChildren = async () => {
    setLoadingChildren(true);
    try {
      const res = await api.listWorkspaceFiles(file.id);
      setChildren(res.files || []);
    } catch {
      setChildren([]);
    } finally {
      setLoadingChildren(false);
    }
  };

  const toggleExpand = async (e?: React.MouseEvent) => {
    if (e) {
      e.stopPropagation();
    }
    if (!file.is_dir) return;

    const nextOpen = !isOpen;
    setIsOpen(nextOpen);

    if (nextOpen && children === null) {
      await fetchChildren();
    }
  };

  const handleRowClick = () => {
    if (file.is_dir) {
      toggleExpand();
      onNavigateDir(file.id);
    } else {
      onSelect(file);
    }
  };

  const handleCheckboxClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleSelect(file.id, e);
  };

  const handleMoreClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onContextMenu(file, e);
  };

  return (
    <div className="select-none flex flex-col">
      <div
        onClick={handleRowClick}
        onContextMenu={(e) => {
          e.preventDefault();
          onContextMenu(file, e);
        }}
        style={{ paddingLeft: `${level * 16 + 8}px` }}
        className={`group flex items-center gap-2 py-1.5 pr-2 rounded-xl text-body-sm cursor-pointer transition-colors ${
          isSelected
            ? 'bg-deep-ink text-canvas font-semibold'
            : isChecked
            ? 'bg-soft-meadow/80 text-deep-ink font-medium'
            : 'text-deep-ink hover:bg-soft-meadow/50 font-normal'
        }`}
      >
        {/* Multi-select checkbox */}
        <input
          type="checkbox"
          checked={isChecked}
          onClick={handleCheckboxClick}
          onChange={() => {}}
          className="w-3.5 h-3.5 rounded border-deep-ink/20 text-deep-ink focus:ring-0 cursor-pointer accent-deep-ink"
        />

        {/* Directory expand chevron */}
        {file.is_dir ? (
          <button
            onClick={toggleExpand}
            className="p-0.5 rounded hover:bg-deep-ink/10 transition-colors cursor-pointer"
            title={isOpen ? 'Collapse' : 'Expand'}
          >
            {loadingChildren ? (
              <Loader2 className="w-3.5 h-3.5 text-slate animate-spin" />
            ) : isOpen ? (
              <ChevronDown className="w-3.5 h-3.5 text-slate" />
            ) : (
              <ChevronRight className="w-3.5 h-3.5 text-slate" />
            )}
          </button>
        ) : (
          <div className="w-3.5" />
        )}

        {/* File / folder icon */}
        {getFileIcon(file.name, file.is_dir, isOpen)}

        {/* Name */}
        <span className="truncate flex-1 text-body-sm">{file.name}</span>

        {/* AI Indexed Status Indicator */}
        {!file.is_dir && file.ai_state && (
          <span
            title={`AI Memory: ${file.ai_state}`}
            className={`w-2 h-2 rounded-full shrink-0 ${
              file.ai_state === 'active'
                ? 'bg-status-success'
                : file.ai_state === 'indexing'
                ? 'bg-status-warning animate-pulse'
                : file.ai_state === 'unsupported'
                ? 'bg-slate/30'
                : 'bg-transparent'
            }`}
          />
        )}

        {/* Hover action button */}
        <button
          onClick={handleMoreClick}
          className={`p-1 rounded-lg opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer ${
            isSelected ? 'hover:bg-canvas/20 text-canvas' : 'hover:bg-deep-ink/10 text-slate'
          }`}
        >
          <MoreVertical className="w-3.5 h-3.5" />
        </button>
      </div>

      {/* Nested Children Tree */}
      {file.is_dir && isOpen && (
        <div className="flex flex-col">
          {loadingChildren ? (
            <div
              style={{ paddingLeft: `${(level + 1) * 16 + 24}px` }}
              className="py-1 text-caption text-slate italic flex items-center gap-1.5"
            >
              <Loader2 className="w-3 h-3 animate-spin" />
              <span>{t('tree.loading')}</span>
            </div>
          ) : children && children.length > 0 ? (
            children.map((child) => (
              <WorkspaceTreeItem
                key={child.id}
                file={child}
                level={level + 1}
                selectedPath={selectedPath}
                selectedPaths={selectedPaths}
                onSelect={onSelect}
                onToggleSelect={onToggleSelect}
                onContextMenu={onContextMenu}
                onNavigateDir={onNavigateDir}
              />
            ))
          ) : (
            <div
              style={{ paddingLeft: `${(level + 1) * 16 + 24}px` }}
              className="py-1 text-caption text-slate italic"
            >
              {t('tree.empty')}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
