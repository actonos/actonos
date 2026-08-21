import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  FileEdit,
  Copy,
  Download,
  Trash2,
  Sparkles,
  MessageSquare,
  ClipboardCopy,
  ExternalLink,
} from 'lucide-react';
import type { WorkspaceFile } from '@/lib/types';

interface WorkspaceContextMenuProps {
  x: number;
  y: number;
  file: WorkspaceFile;
  onClose: () => void;
  onOpen: (file: WorkspaceFile) => void;
  onRename: (file: WorkspaceFile) => void;
  onDuplicate: (file: WorkspaceFile) => void;
  onCopyPath: (file: WorkspaceFile) => void;
  onDownload: (file: WorkspaceFile) => void;
  onReindex: (file: WorkspaceFile) => void;
  onChatWithFile: (file: WorkspaceFile) => void;
  onDelete: (file: WorkspaceFile) => void;
}

export function WorkspaceContextMenu({
  x,
  y,
  file,
  onClose,
  onOpen,
  onRename,
  onDuplicate,
  onCopyPath,
  onDownload,
  onReindex,
  onChatWithFile,
  onDelete,
}: WorkspaceContextMenuProps) {
  const { t } = useTranslation('workspace');
  const menuRef = useRef<HTMLDivElement>(null);
  const [coords, setCoords] = useState<{ x: number; y: number }>({ x, y });

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        onClose();
      }
    }
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onClose();
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [onClose]);

  // Dynamically calculate position based on actual menu dimensions and viewport bounds
  useLayoutEffect(() => {
    if (menuRef.current) {
      const rect = menuRef.current.getBoundingClientRect();
      const padding = 12;
      let newX = x;
      let newY = y;

      // Adjust horizontal overflow
      if (x + rect.width > window.innerWidth - padding) {
        newX = Math.max(padding, window.innerWidth - rect.width - padding);
      } else {
        newX = Math.max(padding, x);
      }

      // Adjust vertical overflow: if menu exceeds bottom edge, flip upward or clamp
      if (y + rect.height > window.innerHeight - padding) {
        if (y - rect.height >= padding) {
          // Flip upward
          newY = y - rect.height;
        } else {
          // Clamp inside viewport
          newY = Math.max(padding, window.innerHeight - rect.height - padding);
        }
      } else {
        newY = Math.max(padding, y);
      }

      setCoords({ x: newX, y: newY });
    }
  }, [x, y]);

  return (
    <div
      ref={menuRef}
      style={{ left: `${coords.x}px`, top: `${coords.y}px` }}
      className="fixed z-50 w-56 max-h-[calc(100vh-24px)] overflow-y-auto rounded-2xl border border-deep-ink/10 bg-canvas p-1.5 text-deep-ink backdrop-blur-md shadow-md focus:outline-none"
    >
      <div className="px-3 py-1.5 text-caption font-semibold text-slate truncate border-b border-deep-ink/5 mb-1">
        {file.name}
      </div>

      <button
        onClick={() => {
          onOpen(file);
          onClose();
        }}
        className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-soft-meadow transition-colors text-left font-medium cursor-pointer"
      >
        <ExternalLink className="w-4 h-4 text-slate" />
        <span>{t('table.actions')}</span>
      </button>

      <button
        onClick={() => {
          onRename(file);
          onClose();
        }}
        className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-soft-meadow transition-colors text-left font-medium cursor-pointer"
      >
        <FileEdit className="w-4 h-4 text-slate" />
        <span>{t('actions.rename')}</span>
      </button>

      {!file.is_dir && (
        <button
          onClick={() => {
            onDuplicate(file);
            onClose();
          }}
          className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-soft-meadow transition-colors text-left font-medium cursor-pointer"
        >
          <Copy className="w-4 h-4 text-slate" />
          <span>{t('actions.duplicate')}</span>
        </button>
      )}

      <button
        onClick={() => {
          onCopyPath(file);
          onClose();
        }}
        className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-soft-meadow transition-colors text-left font-medium cursor-pointer"
      >
        <ClipboardCopy className="w-4 h-4 text-slate" />
        <span>{t('actions.copyPath')}</span>
      </button>

      <button
        onClick={() => {
          onDownload(file);
          onClose();
        }}
        className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-soft-meadow transition-colors text-left font-medium cursor-pointer"
      >
        <Download className="w-4 h-4 text-slate" />
        <span>{file.is_dir ? t('actions.downloadZip') : t('actions.download')}</span>
      </button>

      <div className="h-px bg-deep-ink/5 my-1" />

      {!file.is_dir && (
        <>
          <button
            onClick={() => {
              onReindex(file);
              onClose();
            }}
            className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-soft-meadow transition-colors text-left font-medium text-deep-ink cursor-pointer"
          >
            <Sparkles className="w-4 h-4 text-status-success" />
            <span>{t('actions.reindex')}</span>
          </button>

          <button
            onClick={() => {
              onChatWithFile(file);
              onClose();
            }}
            className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-soft-meadow transition-colors text-left font-medium text-deep-ink cursor-pointer"
          >
            <MessageSquare className="w-4 h-4 text-fuchsia" />
            <span>{t('actions.chatWithFile')}</span>
          </button>
          <div className="h-px bg-deep-ink/5 my-1" />
        </>
      )}

      <button
        onClick={() => {
          onDelete(file);
          onClose();
        }}
        className="w-full flex items-center gap-2.5 px-3 py-2 text-body-sm rounded-xl hover:bg-status-danger-soft text-status-danger transition-colors text-left font-medium cursor-pointer"
      >
        <Trash2 className="w-4 h-4" />
        <span>{t('actions.delete')}</span>
      </button>
    </div>
  );
}
