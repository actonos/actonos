import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Info,
  Sparkles,
  RefreshCw,
  MessageSquare,
  FileEdit,
  Trash2,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
  Database,
  Calendar,
} from 'lucide-react';
import { api } from '@/lib/api';
import { useToast } from '@/components/ui/Toast';
import type { WorkspaceChunksResponse } from '@/lib/types';

interface WorkspaceInspectorProps {
  fileId: string | null;
  filePath: string | null;
  fileKind: string;
  fileSize: number;
  mimeType: string;
  content: string;
  onRename: () => void;
  onDelete: () => void;
  onChatWithFile: () => void;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

export function WorkspaceInspector({
  fileId,
  filePath,
  fileKind,
  fileSize,
  mimeType,
  content,
  onRename,
  onDelete,
  onChatWithFile,
}: WorkspaceInspectorProps) {
  const { t } = useTranslation('workspace');
  const { success, error: toastError } = useToast();
  const [activeTab, setActiveTab] = useState<'overview' | 'ai'>('overview');
  const [chunksData, setChunksData] = useState<WorkspaceChunksResponse | null>(null);
  const [loadingChunks, setLoadingChunks] = useState(false);
  const [reindexing, setReindexing] = useState(false);
  const [expandedChunks, setExpandedChunks] = useState<Set<string>>(new Set());
  const [copiedChunkId, setCopiedChunkId] = useState<string | null>(null);

  useEffect(() => {
    if (!fileId) {
      setChunksData(null);
      return;
    }
    let isMounted = true;
    setLoadingChunks(true);
    api
      .getWorkspaceChunks(fileId)
      .then((data) => {
        if (isMounted) setChunksData(data);
      })
      .catch(() => {
        if (isMounted) setChunksData(null);
      })
      .finally(() => {
        if (isMounted) setLoadingChunks(false);
      });
    return () => {
      isMounted = false;
    };
  }, [fileId]);

  if (!filePath) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-6 text-center text-slate bg-canvas border-l border-deep-ink/10">
        <Info className="w-8 h-8 opacity-30 mb-2" />
        <p className="text-body-sm">{t('editor.noSelection')}</p>
      </div>
    );
  }

  const linesCount = content ? content.split('\n').length : 0;
  const wordsCount = content ? content.trim().split(/\s+/).filter(Boolean).length : 0;
  const tokenEstimate = Math.max(1, Math.round(wordsCount * 1.3));

  const handleReindex = async () => {
	if (!fileId) return;
    setReindexing(true);
    try {
		await api.reindexWorkspaceFile(fileId);
      success(t('toasts.reindexSuccess', { path: filePath }));
      // Reload chunks
		const data = await api.getWorkspaceChunks(fileId);
      setChunksData(data);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      toastError(t('toasts.reindexFailed', { error: msg }));
    } finally {
      setReindexing(false);
    }
  };

  const toggleChunk = (id: string) => {
    setExpandedChunks((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleCopyChunk = async (id: string, text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedChunkId(id);
      setTimeout(() => setCopiedChunkId(null), 2000);
    } catch {}
  };

  const stateKey = (chunksData?.state || 'none') as 'active' | 'indexing' | 'unsupported' | 'none';

  return (
    <div className="h-full flex flex-col bg-canvas border-l border-deep-ink/10 text-deep-ink w-80 shrink-0">
      {/* Header Tabs */}
      <div className="flex items-center justify-between p-3 border-b border-deep-ink/10 bg-soft-meadow/30">
        <div className="flex items-center p-0.5 rounded-full bg-soft-meadow border border-deep-ink/10 w-full">
          <button
            onClick={() => setActiveTab('overview')}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1 rounded-full text-caption font-semibold transition-colors ${
              activeTab === 'overview' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
            }`}
          >
            <Info className="w-3.5 h-3.5" />
            <span>{t('inspector.overview')}</span>
          </button>
          <button
            onClick={() => setActiveTab('ai')}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1 rounded-full text-caption font-semibold transition-colors ${
              activeTab === 'ai' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
            }`}
          >
            <Sparkles className="w-3.5 h-3.5 text-hi-yellow" />
            <span>{t('inspector.aiMemory')}</span>
          </button>
        </div>
      </div>

      {/* Tab Content */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {activeTab === 'overview' ? (
          <>
            {/* File Path & Main Meta */}
            <div className="p-3 rounded-2xl bg-soft-meadow/40 border border-deep-ink/5 space-y-2">
              <div className="text-caption text-slate font-medium">{t('inspector.path')}</div>
              <div className="text-body-sm font-mono font-semibold break-all">{filePath}</div>
            </div>

            {/* Metrics Grid */}
            <div className="grid grid-cols-2 gap-2 text-caption">
              <div className="p-3 rounded-2xl bg-soft-meadow/20 border border-deep-ink/5">
                <div className="text-slate font-medium">{t('inspector.size')}</div>
                <div className="text-body-sm font-bold mt-0.5">{formatBytes(fileSize)}</div>
              </div>
              <div className="p-3 rounded-2xl bg-soft-meadow/20 border border-deep-ink/5">
                <div className="text-slate font-medium">{t('inspector.kind')}</div>
                <div className="text-body-sm font-bold capitalize mt-0.5">{fileKind}</div>
              </div>
              {content && (
                <>
                  <div className="p-3 rounded-2xl bg-soft-meadow/20 border border-deep-ink/5">
                    <div className="text-slate font-medium">{t('inspector.lines')}</div>
                    <div className="text-body-sm font-bold mt-0.5">{linesCount}</div>
                  </div>
                  <div className="p-3 rounded-2xl bg-soft-meadow/20 border border-deep-ink/5">
                    <div className="text-slate font-medium">{t('inspector.tokens')}</div>
                    <div className="text-body-sm font-bold mt-0.5 text-status-success">
                      ~{tokenEstimate}
                    </div>
                  </div>
                </>
              )}
            </div>

            {/* MIME type */}
            {mimeType && (
              <div className="p-3 rounded-2xl bg-soft-meadow/20 border border-deep-ink/5 text-caption">
                <div className="text-slate font-medium">{t('inspector.mime')}</div>
                <div className="text-body-sm font-mono mt-0.5 truncate">{mimeType}</div>
              </div>
            )}

            {/* Action Buttons */}
            <div className="space-y-2 pt-2 border-t border-deep-ink/5">
              <button
                onClick={onChatWithFile}
                className="w-full flex items-center justify-center gap-2 py-2 rounded-full bg-deep-ink text-canvas hover:bg-deep-ink/90 text-body-sm font-semibold transition-colors"
              >
                <MessageSquare className="w-4 h-4 text-hi-yellow" />
                <span>{t('actions.chatWithFile')}</span>
              </button>

              <div className="grid grid-cols-2 gap-2">
                <button
                  onClick={onRename}
                  className="flex items-center justify-center gap-1.5 py-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-body-sm font-medium transition-colors"
                >
                  <FileEdit className="w-3.5 h-3.5 text-slate" />
                  <span>{t('actions.rename')}</span>
                </button>
                <button
                  onClick={onDelete}
                  className="flex items-center justify-center gap-1.5 py-1.5 rounded-full border border-status-danger/20 bg-status-danger-soft hover:bg-status-danger/20 text-status-danger text-body-sm font-medium transition-colors"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>{t('actions.delete')}</span>
                </button>
              </div>
            </div>
          </>
        ) : (
          /* AI Semantic Memory Tab */
          <div className="space-y-4">
            {/* Status Banner */}
            <div className="p-3 rounded-2xl bg-soft-meadow/40 border border-deep-ink/5 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-caption text-slate font-medium">{t('inspector.aiMemory')}</span>
                <span
                  className={`px-2.5 py-0.5 rounded-full text-caption font-semibold ${
                    stateKey === 'active'
                      ? 'bg-status-success-soft text-status-success'
                      : stateKey === 'indexing'
                      ? 'bg-status-warning-soft text-status-warning animate-pulse'
                      : 'bg-slate/10 text-slate'
                  }`}
                >
                  {t(`inspector.aiStatus.${stateKey}`)}
                </span>
              </div>

              {chunksData?.indexed_at && (
                <div className="flex items-center gap-1 text-caption text-slate">
                  <Calendar className="w-3 h-3" />
                  <span>{chunksData.indexed_at}</span>
                </div>
              )}
            </div>

            {/* Model & Chunker Details */}
            {chunksData && (
              <div className="p-3 rounded-2xl bg-soft-meadow/20 border border-deep-ink/5 space-y-1.5 text-caption">
                <div className="flex items-center justify-between">
                  <span className="text-slate">{t('inspector.model')}</span>
                  <span className="font-mono font-semibold">{chunksData.model_id || 'e5-small'}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-slate">{t('inspector.chunkerVersion')}</span>
                  <span className="font-mono">{chunksData.chunker_version || 'paragraph-v2'}</span>
                </div>
              </div>
            )}

            {/* Reindex Action Button */}
            <button
              disabled={reindexing}
              onClick={handleReindex}
              className="w-full flex items-center justify-center gap-2 py-2 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-body-sm font-semibold text-deep-ink transition-colors disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 text-status-success ${reindexing ? 'animate-spin' : ''}`} />
              <span>{reindexing ? t('actions.reindexing') : t('actions.reindex')}</span>
            </button>

            {/* Chunks List */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-caption font-semibold text-slate">
                  {t('inspector.chunksTitle', { count: chunksData?.chunk_count || 0 })}
                </span>
              </div>

              {loadingChunks ? (
                <div className="space-y-2 animate-pulse">
                  <div className="h-16 bg-soft-meadow rounded-xl" />
                  <div className="h-16 bg-soft-meadow rounded-xl" />
                </div>
              ) : chunksData?.chunks && chunksData.chunks.length > 0 ? (
                <div className="space-y-2">
                  {chunksData.chunks.map((chunk) => {
                    const isExpanded = expandedChunks.has(chunk.id);
                    return (
                      <div
                        key={chunk.id}
                        className="rounded-xl border border-deep-ink/10 bg-soft-meadow/30 overflow-hidden text-caption"
                      >
                        <div
                          onClick={() => toggleChunk(chunk.id)}
                          className="flex items-center justify-between p-2 cursor-pointer hover:bg-soft-meadow/60 transition-colors select-none"
                        >
                          <div className="flex items-center gap-1.5 font-semibold text-deep-ink">
                            {isExpanded ? (
                              <ChevronDown className="w-3.5 h-3.5 text-slate" />
                            ) : (
                              <ChevronRight className="w-3.5 h-3.5 text-slate" />
                            )}
                            <span>{t('inspector.chunkOrdinal', { num: chunk.ordinal })}</span>
                          </div>
                          <span className="text-slate font-mono">
                            {t('inspector.chunkTokens', { count: chunk.token_count })}
                          </span>
                        </div>

                        {isExpanded && (
                          <div className="p-2.5 border-t border-deep-ink/5 bg-canvas font-mono text-caption text-deep-ink relative group">
                            <button
                              onClick={() => handleCopyChunk(chunk.id, chunk.content)}
                              className="absolute top-2 right-2 p-1 rounded bg-soft-meadow hover:bg-soft-meadow/80 border border-deep-ink/10 text-slate"
                              title={t('jsonViewer.copy')}
                            >
                              {copiedChunkId === chunk.id ? (
                                <Check className="w-3 h-3 text-status-success" />
                              ) : (
                                <Copy className="w-3 h-3" />
                              )}
                            </button>
                            <div className="whitespace-pre-wrap break-words pr-6">
                              {chunk.content}
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="text-center py-6 text-slate text-caption">
                  <Database className="w-8 h-8 mx-auto mb-1.5 opacity-30" />
                  <p>{t('inspector.noChunks')}</p>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
