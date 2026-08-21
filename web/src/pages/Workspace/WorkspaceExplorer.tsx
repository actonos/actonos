import { useState, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Folder,
  FileText,
  Search,
  Plus,
  FolderPlus,
  RefreshCw,
  Download,
  Trash2,
  Upload,
  ChevronRight,
  ArrowUpDown,
  LayoutGrid,
  List,
  FolderTree,
  MoreVertical,
} from 'lucide-react';
import type { WorkspaceBreadcrumb, WorkspaceFile } from '@/lib/types';
import { WorkspaceTreeItem } from './WorkspaceTreeItem';

interface WorkspaceExplorerProps {
  files: WorkspaceFile[];
  breadcrumbs: WorkspaceBreadcrumb[];
  selectedPath: string | null;
  selectedPaths: Set<string>;
  loading: boolean;
  onSelectFile: (file: WorkspaceFile) => void;
  onToggleSelectPath: (path: string, e: React.MouseEvent) => void;
  onNavigateDir: (dir: string) => void;
  onUpload: (files: FileList) => void;
  onNewFile: () => void;
  onNewFolder: () => void;
  onRefresh: () => void;
  onDownloadZip: () => void;
  onBatchDelete: () => void;
  onBatchDownload: () => void;
  onContextMenu: (file: WorkspaceFile, e: React.MouseEvent) => void;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

export function WorkspaceExplorer({
  files,
  breadcrumbs,
  selectedPath,
  selectedPaths,
  loading,
  onSelectFile,
  onToggleSelectPath,
  onNavigateDir,
  onUpload,
  onNewFile,
  onNewFolder,
  onRefresh,
  onDownloadZip,
  onBatchDelete,
  onBatchDownload,
  onContextMenu,
}: WorkspaceExplorerProps) {
  const { t } = useTranslation('workspace');
  const [viewMode, setViewMode] = useState<'tree' | 'list' | 'grid'>('tree');
  const [searchQuery, setSearchQuery] = useState('');
  const [categoryFilter, setCategoryFilter] = useState<'all' | 'documents' | 'code' | 'data' | 'media'>('all');
  const [sortKey, setSortKey] = useState<'name' | 'size' | 'date'>('name');
  const [sortAsc, setSortAsc] = useState(true);
  const [isDragOver, setIsDragOver] = useState(false);
  const dragCounter = useRef(0);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Filtering & Sorting
  const filteredFiles = useMemo(() => {
    let result = [...files];

    // Search query
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      result = result.filter((f) => f.name.toLowerCase().includes(q) || f.path.toLowerCase().includes(q));
    }

    // Category filter
    if (categoryFilter !== 'all') {
      result = result.filter((f) => {
        if (f.is_dir) return true;
        const ext = f.name.split('.').pop()?.toLowerCase() || '';
        switch (categoryFilter) {
          case 'documents':
            return ['md', 'txt', 'pdf', 'docx', 'doc', 'rtf'].includes(ext);
          case 'code':
            return ['py', 'go', 'ts', 'tsx', 'js', 'jsx', 'sh', 'rs', 'c', 'cpp', 'html', 'css', 'json', 'yaml', 'yml'].includes(ext);
          case 'data':
            return ['csv', 'tsv', 'json', 'xlsx', 'xls', 'sql', 'db', 'sqlite'].includes(ext);
          case 'media':
            return ['png', 'jpg', 'jpeg', 'svg', 'webp', 'gif', 'mp3', 'wav', 'mp4'].includes(ext);
          default:
            return true;
        }
      });
    }

    // Sort
    result.sort((a, b) => {
      // Directories first
      if (a.is_dir && !b.is_dir) return -1;
      if (!a.is_dir && b.is_dir) return 1;

      if (sortKey === 'size') {
        return sortAsc ? a.size - b.size : b.size - a.size;
      }
      if (sortKey === 'date') {
        return sortAsc ? a.mod_time.localeCompare(b.mod_time) : b.mod_time.localeCompare(a.mod_time);
      }
      return sortAsc ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name);
    });

    return result;
  }, [files, searchQuery, categoryFilter, sortKey, sortAsc]);

  // Drag & drop upload handlers (flicker-free with dragCounter)
  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current++;
    if (e.dataTransfer.items && e.dataTransfer.items.length > 0) {
      setIsDragOver(true);
    }
  };
  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = 'copy';
  };
  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current--;
    if (dragCounter.current <= 0) {
      dragCounter.current = 0;
      setIsDragOver(false);
    }
  };
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current = 0;
    setIsDragOver(false);
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      onUpload(e.dataTransfer.files);
    }
  };

  return (
    <div
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className="flex flex-col h-full bg-canvas text-deep-ink border-r border-deep-ink/10 relative select-none w-80 shrink-0"
    >
      {/* Drag & Drop Visual Overlay with pointer-events-none */}
      {isDragOver && (
        <div className="pointer-events-none absolute inset-0 z-40 bg-hi-yellow/90 backdrop-blur-sm border-2 border-dashed border-deep-ink flex flex-col items-center justify-center p-6 text-center text-deep-ink animate-fadeIn">
          <Upload className="w-12 h-12 mb-2 animate-bounce" />
          <h4 className="text-subheading font-bold">{t('dragDrop.active')}</h4>
          <p className="text-caption text-deep-ink/70">{t('dragDrop.hint')}</p>
        </div>
      )}

      {/* Explorer Top Action Bar */}
      <div className="p-3 border-b border-deep-ink/10 flex items-center justify-between gap-1.5 bg-soft-meadow/30">
        <div className="flex items-center gap-1">
          <button
            onClick={onNewFile}
            className="p-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-deep-ink transition-colors"
            title={t('actions.newFile')}
          >
            <Plus className="w-4 h-4" />
          </button>
          <button
            onClick={onNewFolder}
            className="p-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-deep-ink transition-colors"
            title={t('actions.newFolder')}
          >
            <FolderPlus className="w-4 h-4" />
          </button>
          <button
            onClick={() => fileInputRef.current?.click()}
            className="p-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-deep-ink transition-colors"
            title={t('actions.upload')}
          >
            <Upload className="w-4 h-4" />
          </button>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => {
              if (e.target.files && e.target.files.length > 0) {
                onUpload(e.target.files);
                e.target.value = '';
              }
            }}
          />
        </div>

        <div className="flex items-center gap-1">
          <button
            onClick={onDownloadZip}
            className="p-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors"
            title={t('actions.downloadZip')}
          >
            <Download className="w-4 h-4" />
          </button>
          <button
            onClick={onRefresh}
            className={`p-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors ${
              loading ? 'animate-spin' : ''
            }`}
            title={t('actions.refresh')}
          >
            <RefreshCw className="w-4 h-4" />
          </button>

          {/* View Mode Toggle */}
          <div className="flex items-center p-0.5 rounded-full bg-soft-meadow border border-deep-ink/10 ml-1">
            <button
              onClick={() => setViewMode('tree')}
              className={`p-1 rounded-full transition-colors ${
                viewMode === 'tree' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
              }`}
              title={t('views.tree')}
            >
              <FolderTree className="w-3.5 h-3.5" />
            </button>
            <button
              onClick={() => setViewMode('list')}
              className={`p-1 rounded-full transition-colors ${
                viewMode === 'list' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
              }`}
              title={t('views.list')}
            >
              <List className="w-3.5 h-3.5" />
            </button>
            <button
              onClick={() => setViewMode('grid')}
              className={`p-1 rounded-full transition-colors ${
                viewMode === 'grid' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
              }`}
              title={t('views.grid')}
            >
              <LayoutGrid className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>

      {/* Breadcrumbs Navigation */}
      <div className="flex items-center gap-1 px-3 py-2 border-b border-deep-ink/5 bg-canvas text-caption font-medium overflow-x-auto">
        <button
          onClick={() => onNavigateDir('')}
          className="text-deep-ink hover:underline shrink-0"
        >
          {t('navigation.root')}
        </button>
        {breadcrumbs.map((segment) => {
          return (
            <div key={segment.id} className="flex items-center gap-1 shrink-0">
              <ChevronRight className="w-3 h-3 text-slate" />
              <button
                onClick={() => onNavigateDir(segment.id)}
                className="text-deep-ink hover:underline font-semibold"
              >
                {segment.name}
              </button>
            </div>
          );
        })}
      </div>

      {/* Search Input & Sort Toggle */}
      <div className="p-2 border-b border-deep-ink/5 flex items-center gap-1.5">
        <div className="relative flex-1">
          <Search className="w-3.5 h-3.5 text-slate absolute left-2.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('navigation.searchPlaceholder')}
            className="w-full pl-8 pr-2.5 py-1 rounded-full border border-deep-ink/10 bg-soft-meadow/40 text-caption focus:outline-none focus:border-deep-ink"
          />
        </div>
        <button
          onClick={() => {
            if (sortKey === 'name') {
              setSortKey('date');
            } else if (sortKey === 'date') {
              setSortKey('size');
            } else {
              setSortKey('name');
            }
            setSortAsc(!sortAsc);
          }}
          className="p-1.5 rounded-full border border-deep-ink/10 hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors shrink-0"
          title={`Sort: ${sortKey} (${sortAsc ? 'asc' : 'desc'})`}
        >
          <ArrowUpDown className="w-3.5 h-3.5" />
        </button>
      </div>

      {/* Category Pills */}
      <div className="flex items-center gap-1 px-2 py-1.5 border-b border-deep-ink/5 overflow-x-auto text-caption font-semibold">
        {(['all', 'documents', 'code', 'data', 'media'] as const).map((cat) => (
          <button
            key={cat}
            onClick={() => setCategoryFilter(cat)}
            className={`px-2.5 py-0.5 rounded-full transition-colors whitespace-nowrap ${
              categoryFilter === cat
                ? 'bg-deep-ink text-canvas'
                : 'bg-soft-meadow/50 text-slate hover:text-deep-ink'
            }`}
          >
            {t(`categories.${cat}`)}
          </button>
        ))}
      </div>

      {/* Batch Operations Bar */}
      {selectedPaths.size > 0 && (
        <div className="flex items-center justify-between px-3 py-1.5 bg-hi-yellow/40 border-b border-deep-ink/10 text-caption font-semibold">
          <span>{t('actions.selectedCount', { count: selectedPaths.size })}</span>
          <div className="flex items-center gap-1.5">
            <button
              onClick={onBatchDownload}
              className="p-1 rounded-full bg-canvas hover:bg-soft-meadow border border-deep-ink/10 text-deep-ink"
              title={t('actions.batchDownload')}
            >
              <Download className="w-3.5 h-3.5" />
            </button>
            <button
              onClick={onBatchDelete}
              className="p-1 rounded-full bg-status-danger-soft hover:bg-status-danger/20 text-status-danger"
              title={t('actions.batchDelete', { count: selectedPaths.size })}
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      )}

      {/* Files Content Container */}
      <div className="flex-1 overflow-y-auto p-2">
        {loading ? (
          <div className="space-y-2 p-2 animate-pulse">
            <div className="h-6 bg-soft-meadow rounded-xl" />
            <div className="h-6 bg-soft-meadow rounded-xl" />
            <div className="h-6 bg-soft-meadow rounded-xl" />
          </div>
        ) : filteredFiles.length === 0 ? (
          <div className="text-center py-12 text-slate text-body-sm">
            <Folder className="w-10 h-10 mx-auto mb-2 opacity-30 text-slate" />
            <p>{t('table.empty')}</p>
          </div>
        ) : viewMode === 'tree' ? (
          /* Tree View */
          <div className="space-y-0.5">
            {filteredFiles.map((file) => (
              <WorkspaceTreeItem
                key={file.id}
                file={file}
                level={0}
                selectedPath={selectedPath}
                selectedPaths={selectedPaths}
                onSelect={onSelectFile}
                onToggleSelect={onToggleSelectPath}
                onContextMenu={onContextMenu}
                onNavigateDir={onNavigateDir}
              />
            ))}
          </div>
        ) : viewMode === 'list' ? (
          /* List View */
          <div className="space-y-1">
            {filteredFiles.map((file) => {
              const isSelected = selectedPath === file.id;
              const isChecked = selectedPaths.has(file.id);
              return (
                <div
                  key={file.id}
                  onClick={() => (file.is_dir ? onNavigateDir(file.id) : onSelectFile(file))}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    onContextMenu(file, e);
                  }}
                  className={`group flex items-center justify-between p-2 rounded-xl text-body-sm cursor-pointer transition-colors ${
                    isSelected
                      ? 'bg-deep-ink text-canvas font-semibold'
                      : isChecked
                      ? 'bg-soft-meadow/80 text-deep-ink font-medium'
                      : 'hover:bg-soft-meadow/50 text-deep-ink'
                  }`}
                >
                  <div className="flex items-center gap-2 truncate">
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onClick={(e) => {
                        e.stopPropagation();
                        onToggleSelectPath(file.id, e);
                      }}
                      onChange={() => {}}
                      className="w-3.5 h-3.5 rounded border-deep-ink/20 text-deep-ink cursor-pointer accent-deep-ink"
                    />
                    {file.is_dir ? (
                      <Folder className="w-4 h-4 text-hi-yellow shrink-0 fill-hi-yellow/20" />
                    ) : (
                      <FileText className="w-4 h-4 text-slate shrink-0" />
                    )}
                    <span className="truncate">{file.name}</span>
                  </div>

                  <div className="flex items-center gap-2 text-caption opacity-70 shrink-0">
                    {!file.is_dir && <span>{formatBytes(file.size)}</span>}
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onContextMenu(file, e);
                      }}
                      className="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-deep-ink/10"
                    >
                      <MoreVertical className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          /* Grid / Card Tile View */
          <div className="grid grid-cols-2 gap-2">
            {filteredFiles.map((file) => {
              const isSelected = selectedPath === file.id;
              const isChecked = selectedPaths.has(file.id);
              return (
                <div
                  key={file.id}
                  onClick={() => (file.is_dir ? onNavigateDir(file.id) : onSelectFile(file))}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    onContextMenu(file, e);
                  }}
                  className={`p-3 rounded-2xl border transition-all cursor-pointer flex flex-col justify-between gap-2 relative group ${
                    isSelected
                      ? 'bg-deep-ink text-canvas border-deep-ink shadow-xs'
                      : isChecked
                      ? 'bg-soft-meadow border-deep-ink/20 text-deep-ink'
                      : 'bg-soft-meadow/30 border-deep-ink/5 hover:border-deep-ink/20 text-deep-ink'
                  }`}
                >
                  <div className="flex items-center justify-between">
                    {file.is_dir ? (
                      <Folder className="w-6 h-6 text-hi-yellow fill-hi-yellow/20" />
                    ) : (
                      <FileText className="w-6 h-6 text-slate" />
                    )}

                    <input
                      type="checkbox"
                      checked={isChecked}
                      onClick={(e) => {
                        e.stopPropagation();
                        onToggleSelectPath(file.id, e);
                      }}
                      onChange={() => {}}
                      className="w-3.5 h-3.5 rounded border-deep-ink/20 text-deep-ink cursor-pointer accent-deep-ink"
                    />
                  </div>

                  <div>
                    <div className="text-body-sm font-semibold truncate" title={file.name}>
                      {file.name}
                    </div>
                    <div className="text-caption opacity-60">
                      {file.is_dir ? t('stats.folders') : formatBytes(file.size)}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
