import { useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { AgentManifest, MemoryRecord, MemoryImportanceTier, MemoryLayer } from '@/lib/types';
import {
  Brain,
  Pin,
  PinOff,
  RotateCw,
  Search,
  Clock,
  Sparkles,
  ShieldCheck,
  Zap,
} from 'lucide-react';

export interface MemoryManagerModalProps {
  isOpen: boolean;
  onClose: () => void;
  agent: AgentManifest | null;
}

export function MemoryManagerModal({
  isOpen,
  onClose,
  agent,
}: MemoryManagerModalProps) {
  const { t } = useTranslation('agents');
  const { success, error } = useToast();

  const [memories, setMemories] = useState<MemoryRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [activeLayer, setActiveLayer] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [actingId, setActingId] = useState<string | null>(null);

  const loadMemories = useCallback(async (isBackground = false) => {
    if (!agent) return;
    try {
      if (!isBackground) setLoading(true);
      const res = await api.listMemories(agent.agent_id, {
        layer: activeLayer === 'all' ? undefined : activeLayer,
        limit: 100,
      });
      setMemories(res.memories || []);
    } catch (err) {
      if (!isBackground) {
        error(t('memory.loadFailed', 'Không thể tải danh sách ký ức'), err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (!isBackground) setLoading(false);
    }
  }, [agent, activeLayer, error, t]);

  useEffect(() => {
    if (isOpen && agent) {
      loadMemories(false);
    }
  }, [isOpen, agent, loadMemories]);

  const handleTogglePin = async (mem: MemoryRecord) => {
    if (!agent || actingId) return;
    const isPinned = mem.pinned || mem.user_pinned;
    try {
      setActingId(mem.id);
      if (isPinned) {
        await api.unpinMemory(agent.agent_id, mem.id);
        success(t('memory.unpinSuccess', 'Đã bỏ ghim ký ức'));
      } else {
        await api.pinMemory(agent.agent_id, mem.id);
        success(t('memory.pinSuccess', 'Đã ghim ký ức (Miễn trừ suy giảm Ebbinghaus)'));
      }
      await loadMemories(true);
    } catch (err) {
      error(t('memory.actionFailed', 'Thao tác thất bại'), err instanceof Error ? err.message : String(err));
    } finally {
      setActingId(null);
    }
  };

  const handleSetImportance = async (mem: MemoryRecord, tier: MemoryImportanceTier) => {
    if (!agent || actingId || mem.importance === tier) return;
    try {
      setActingId(mem.id);
      await api.setMemoryImportance(agent.agent_id, mem.id, tier);
      success(t('memory.importanceUpdated', 'Đã cập nhật mức độ quan trọng'));
      await loadMemories(true);
    } catch (err) {
      error(t('memory.actionFailed', 'Thao tác thất bại'), err instanceof Error ? err.message : String(err));
    } finally {
      setActingId(null);
    }
  };

  const filteredMemories = useMemo(() => {
    return memories.filter((m) => {
      if (activeLayer !== 'all' && m.layer !== activeLayer) return false;
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        return m.content.toLowerCase().includes(q) || (m.id && m.id.toLowerCase().includes(q));
      }
      return true;
    });
  }, [memories, activeLayer, searchQuery]);

  const getTierVariant = (tier?: MemoryImportanceTier): 'neutral' | 'success' | 'warning' | 'danger' | 'info' => {
    switch (tier) {
      case 'critical':
        return 'danger';
      case 'user_preference':
        return 'warning';
      case 'high':
        return 'info';
      case 'normal':
        return 'neutral';
      case 'low':
        return 'neutral';
      default:
        return 'neutral';
    }
  };

  const modalTitleText = agent
    ? `${t('memory.modalTitle', 'Quản lý Bộ nhớ & Ký ức có Trọng số')} • ${agent.name}`
    : t('memory.modalTitle', 'Quản lý Bộ nhớ & Ký ức có Trọng số');

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={modalTitleText}
      maxWidth="max-w-4xl"
    >
      <div className="flex flex-col gap-4 max-h-[75vh]">
        {/* Header toolbar */}
        <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 pb-3 border-b border-deep-ink/10">
          {/* Layer Filter Pills */}
          <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0">
            {(['all', 'semantic', 'episodic', 'procedural', 'user_profile'] as (MemoryLayer | 'all')[]).map((layer) => (
              <button
                key={layer}
                type="button"
                onClick={() => setActiveLayer(layer)}
                className={`px-3 py-1 text-xs font-medium rounded-full transition-all whitespace-nowrap ${
                  activeLayer === layer
                    ? 'bg-deep-ink text-white shadow-sm'
                    : 'bg-soft-meadow text-slate hover:text-deep-ink border border-deep-ink/5'
                }`}
              >
                {t(`memory.layers.${layer}`, layer)}
              </button>
            ))}
          </div>

          <div className="flex items-center gap-2">
            <div className="relative flex-1 sm:w-60">
              <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate pointer-events-none" />
              <Input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder={t('memory.searchPlaceholder', 'Tìm kiếm ký ức...')}
                className="pl-9 pr-3 py-1.5 text-xs rounded-full bg-canvas"
              />
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => loadMemories(false)}
              disabled={loading}
              className="rounded-full shrink-0"
              icon={<RotateCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
            >
              {t('common.refresh', 'Làm mới')}
            </Button>
          </div>
        </div>

        {/* Ebbinghaus Protection Notice Banner */}
        <div className="flex items-center gap-3 p-3 rounded-2xl bg-hi-yellow/15 border border-hi-yellow/40 text-deep-ink text-xs">
          <ShieldCheck className="w-4 h-4 shrink-0 text-deep-ink" />
          <div className="flex-1">
            <span className="font-semibold">{t('memory.pinningNoticeTitle', 'Bảo tồn Ký ức Ebbinghaus')}:</span>{' '}
            {t(
              'memory.pinningNoticeDesc',
              'Ký ức được Ghim (Pinned) hoặc thuộc nhóm "critical" sẽ hoàn toàn miễn nhiễm với sự suy giảm thời gian và không bao giờ bị xóa trong các chu kỳ phản chiếu.'
            )}
          </div>
        </div>

        {/* Memory Items List */}
        <div className="flex-1 overflow-y-auto space-y-3 pr-1">
          {loading && memories.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-slate gap-2">
              <RotateCw className="w-6 h-6 animate-spin text-deep-ink" />
              <p className="text-xs">{t('common.loading', 'Đang tải...')}</p>
            </div>
          ) : filteredMemories.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-slate gap-2 text-center bg-soft-meadow/50 rounded-2xl border border-dashed border-deep-ink/10">
              <Brain className="w-8 h-8 text-slate/40" />
              <p className="text-sm font-medium text-deep-ink">{t('memory.emptyTitle', 'Chưa có mẩu ký ức nào')}</p>
              <p className="text-xs max-w-sm text-slate">
                {t('memory.emptyDesc', 'Ký ức sẽ được tự động tích lũy và cập nhật qua các phiên hội thoại, nhiệm vụ tự chủ và chu kỳ phản chiếu.')}
              </p>
            </div>
          ) : (
            filteredMemories.map((mem) => {
              const isPinned = mem.pinned || mem.user_pinned;
              const isBusy = actingId === mem.id;
              const currentTier = mem.importance || 'normal';

              return (
                <div
                  key={mem.id}
                  className={`p-4 rounded-2xl transition-all border ${
                    isPinned
                      ? 'bg-canvas border-hi-yellow/80 shadow-sm'
                      : 'bg-soft-meadow/70 border-deep-ink/10 hover:border-deep-ink/20'
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex-1 space-y-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="neutral" className="bg-canvas text-deep-ink text-[11px] font-mono border border-deep-ink/10">
                          {mem.layer}
                        </Badge>
                        <Badge variant={getTierVariant(mem.importance)} className="text-[11px] uppercase tracking-wider">
                          {mem.importance || 'normal'}
                        </Badge>
                        {isPinned && (
                          <Badge variant="warning" className="gap-1 text-[11px] bg-hi-yellow text-deep-ink font-semibold">
                            <Pin className="w-3 h-3 fill-current" />
                            {t('memory.pinned', 'Đã ghim')}
                          </Badge>
                        )}
                        <span className="text-[11px] text-slate flex items-center gap-1 ml-auto font-mono">
                          <Zap className="w-3 h-3" />
                          {t('memory.accessCount', 'Truy cập')}: {mem.access_count || 0}
                        </span>
                      </div>

                      {/* Memory Content Text */}
                      <p className="text-sm text-deep-ink leading-relaxed whitespace-pre-wrap select-text font-normal">
                        {mem.content}
                      </p>

                      {/* Metadata & Timestamps */}
                      <div className="flex flex-wrap items-center gap-4 text-[11px] text-slate pt-1 border-t border-deep-ink/5">
                        <span className="flex items-center gap-1 font-mono">
                          <Clock className="w-3 h-3" />
                          {t('memory.created', 'Tạo')}: {new Date(mem.created_at).toLocaleDateString()}
                        </span>
                        {mem.last_accessed_at && (
                          <span className="flex items-center gap-1 font-mono">
                            <Sparkles className="w-3 h-3" />
                            {t('memory.lastAccessed', 'Dùng gần nhất')}: {new Date(mem.last_accessed_at).toLocaleDateString()}
                          </span>
                        )}
                      </div>
                    </div>

                    {/* Quick Action Controls */}
                    <div className="flex flex-col items-end gap-2 shrink-0">
                      {/* Pin Toggle Button */}
                      <Button
                        variant={isPinned ? 'primary' : 'ghost'}
                        size="sm"
                        onClick={() => handleTogglePin(mem)}
                        disabled={isBusy}
                        className={`rounded-full gap-1.5 text-xs transition-all ${
                          isPinned ? 'bg-hi-yellow hover:bg-hi-yellow/90 text-deep-ink font-medium' : ''
                        }`}
                        icon={
                          isPinned ? (
                            <PinOff className="w-3.5 h-3.5" />
                          ) : (
                            <Pin className="w-3.5 h-3.5 text-slate" />
                          )
                        }
                      >
                        {isPinned ? t('memory.unpin', 'Bỏ ghim') : t('memory.pin', 'Ghim')}
                      </Button>

                      {/* Tier Selector Dropdown */}
                      <div className="flex items-center gap-1">
                        <select
                          value={currentTier}
                          onChange={(e) => handleSetImportance(mem, e.target.value as MemoryImportanceTier)}
                          disabled={isBusy}
                          className="text-xs bg-canvas text-deep-ink rounded-lg border border-deep-ink/10 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-deep-ink font-medium cursor-pointer"
                        >
                          <option value="critical">{t('memory.tiers.critical', 'Critical (3.0x)')}</option>
                          <option value="user_preference">{t('memory.tiers.user_preference', 'Preference (2.5x)')}</option>
                          <option value="high">{t('memory.tiers.high', 'High (1.5x)')}</option>
                          <option value="normal">{t('memory.tiers.normal', 'Normal (1.0x)')}</option>
                          <option value="low">{t('memory.tiers.low', 'Low (0.5x)')}</option>
                        </select>
                      </div>
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>
    </Modal>
  );
}
