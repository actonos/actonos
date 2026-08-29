import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { IconButton } from '@/components/ui/IconButton';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import type { HeartbeatConfigData, StructuredDirective } from '@/lib/types';
import { DirectiveModal } from './DirectiveModal';
import {
  Plus,
  Edit2,
  Trash2,
  Clock,
  Target,
  ShieldCheck,
  Zap,
  Sparkles,
} from 'lucide-react';

export interface StructuredDirectivesTabProps {
  config: HeartbeatConfigData;
  onSaveConfig: (updated: Partial<HeartbeatConfigData>) => Promise<void>;
  saving: boolean;
}

export function StructuredDirectivesTab({
  config,
  onSaveConfig,
  saving,
}: StructuredDirectivesTabProps) {
  const { t } = useTranslation('missions');
  const { success, error } = useToast();

  const [mode, setMode] = useState<'structured' | 'raw'>('structured');
  const [rawText, setRawText] = useState(config.directives || '');
  const [intervalMinutes, setIntervalMinutes] = useState(config.interval_minutes || 5);
  const [targetChannel, setTargetChannel] = useState(config.target_channel || 'all');
  const [structuredDirectives, setStructuredDirectives] = useState<StructuredDirective[]>(
    config.structured_directives || []
  );

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editingDirective, setEditingDirective] = useState<StructuredDirective | null>(null);
  const [deletingDirectiveId, setDeletingDirectiveId] = useState<string | null>(null);

  const handleSaveRaw = async () => {
    try {
      await onSaveConfig({
        directives: rawText,
        interval_minutes: intervalMinutes,
        target_channel: targetChannel,
      });
      success(t('toast.directivesSaved'));
    } catch (err) {
      error('Failed to save directives', err instanceof Error ? err.message : String(err));
    }
  };

  const handleSaveDirective = async (item: StructuredDirective) => {
    const existingIndex = structuredDirectives.findIndex((d) => d.id === item.id);
    let nextList: StructuredDirective[];
    if (existingIndex >= 0) {
      nextList = [...structuredDirectives];
      nextList[existingIndex] = item;
    } else {
      nextList = [...structuredDirectives, item];
    }
    setStructuredDirectives(nextList);

    try {
      await onSaveConfig({
        structured_directives: nextList,
        interval_minutes: intervalMinutes,
        target_channel: targetChannel,
      });
      success(t('toast.directivesSaved'));
    } catch (err) {
      error('Failed to save structured directive', err instanceof Error ? err.message : String(err));
    }
  };

  const handleDeleteDirective = async () => {
    if (!deletingDirectiveId) return;
    const nextList = structuredDirectives.filter((d) => d.id !== deletingDirectiveId);
    setStructuredDirectives(nextList);
    setDeletingDirectiveId(null);

    try {
      await onSaveConfig({
        structured_directives: nextList,
        interval_minutes: intervalMinutes,
        target_channel: targetChannel,
      });
      success(t('toast.directivesSaved'));
    } catch (err) {
      error('Failed to delete directive', err instanceof Error ? err.message : String(err));
    }
  };

  const handleToggleDirective = async (item: StructuredDirective) => {
    const updated = { ...item, enabled: !item.enabled };
    await handleSaveDirective(updated);
  };

  const getPriorityBadge = (priority?: string) => {
    switch (priority) {
      case 'p0_critical':
        return <Badge variant="stopped">P0 CRITICAL</Badge>;
      case 'p1_high':
        return <Badge variant="warning">P1 HIGH</Badge>;
      case 'p2_normal':
        return <Badge variant="active">P2 NORMAL</Badge>;
      default:
        return <Badge variant="neutral">P3 LOW</Badge>;
    }
  };

  return (
    <div className="space-y-6">
      {/* Top Controls: Interval & Mode Switcher */}
      <Card className="p-5 border border-onyx/10 bg-canvas">
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
          <div className="space-y-1">
            <h3 className="font-serif text-heading-sm font-bold text-deep-ink">
              {t('directives.title')}
            </h3>
            <p className="text-caption text-slate">{t('directives.description')}</p>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            {/* Mode Switcher */}
            <div className="flex rounded-full bg-soft-meadow p-1 border border-onyx/10">
              <button
                type="button"
                onClick={() => setMode('structured')}
                className={`px-3.5 py-1 text-caption font-semibold rounded-full transition-all cursor-pointer ${
                  mode === 'structured'
                    ? 'bg-deep-ink text-canvas shadow-xs'
                    : 'text-slate hover:text-deep-ink'
                }`}
              >
                {t('directives.modeStructured')}
              </button>
              <button
                type="button"
                onClick={() => setMode('raw')}
                className={`px-3.5 py-1 text-caption font-semibold rounded-full transition-all cursor-pointer ${
                  mode === 'raw'
                    ? 'bg-deep-ink text-canvas shadow-xs'
                    : 'text-slate hover:text-deep-ink'
                }`}
              >
                {t('directives.modeRaw')}
              </button>
            </div>

            {/* Pulse Interval Selector */}
            <select
              value={intervalMinutes}
              onChange={(e) => setIntervalMinutes(parseInt(e.target.value, 10))}
              aria-label={t('directives.interval')}
              className="rounded-full bg-soft-meadow border border-onyx/10 px-3.5 py-1.5 text-caption text-deep-ink font-semibold focus:outline-none"
            >
              <option value={1}>{t('directives.intervals.one')}</option>
              <option value={5}>{t('directives.intervals.five')}</option>
              <option value={15}>{t('directives.intervals.fifteen')}</option>
              <option value={30}>{t('directives.intervals.thirty')}</option>
              <option value={60}>{t('directives.intervals.hour')}</option>
            </select>

            {/* Target Channel Selector */}
            <select
              value={targetChannel}
              onChange={(e) => setTargetChannel(e.target.value)}
              aria-label={t('directives.channel')}
              className="rounded-full bg-soft-meadow border border-onyx/10 px-3.5 py-1.5 text-caption text-deep-ink font-semibold focus:outline-none"
            >
              <option value="all">{t('channels.all')}</option>
              <option value="telegram">Telegram</option>
              <option value="discord">Discord</option>
              <option value="whatsapp">WhatsApp</option>
              <option value="none">{t('channels.none')}</option>
            </select>
          </div>
        </div>
      </Card>

      {/* Mode 1: Structured Directives */}
      {mode === 'structured' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-caption font-semibold uppercase text-slate tracking-wide">
              {structuredDirectives.length} {t('directives.modeStructured')}
            </span>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={() => {
                setEditingDirective(null);
                setIsModalOpen(true);
              }}
            >
              {t('directives.addDirective')}
            </Button>
          </div>

          {structuredDirectives.length === 0 ? (
            <Card className="p-12 text-center border border-onyx/10 bg-canvas space-y-3">
              <div className="w-12 h-12 rounded-full bg-soft-meadow mx-auto flex items-center justify-center">
                <Sparkles className="w-6 h-6 text-slate" />
              </div>
              <p className="text-body-sm font-semibold text-deep-ink">
                {t('directives.noStructuredDirectives')}
              </p>
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-3.5 h-3.5" />}
                onClick={() => {
                  setEditingDirective(null);
                  setIsModalOpen(true);
                }}
              >
                {t('directives.addDirective')}
              </Button>
            </Card>
          ) : (
            <div className="grid grid-cols-1 gap-3.5">
              {structuredDirectives.map((item) => (
                <Card
                  key={item.id}
                  className={`p-4.5 border transition-all ${
                    item.enabled
                      ? 'border-onyx/10 bg-canvas hover:border-onyx/20'
                      : 'border-onyx/5 bg-soft-meadow/50 opacity-70'
                  }`}
                >
                  <div className="flex flex-col md:flex-row md:items-start justify-between gap-3">
                    <div className="space-y-2 flex-1 min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        {getPriorityBadge(item.priority)}
                        <h4 className="font-sans font-bold text-body text-deep-ink truncate">
                          {item.title}
                        </h4>
                        {item.schedule && (
                          <span className="text-[11px] font-mono text-slate bg-soft-meadow px-2.5 py-0.5 rounded-full border border-onyx/10 flex items-center gap-1">
                            <Clock className="w-3 h-3 text-slate" />
                            {item.schedule}
                          </span>
                        )}
                        {item.auto_create_mission && (
                          <span className="text-[10px] font-mono text-deep-ink bg-soft-meadow px-2 py-0.5 rounded-full border border-onyx/10">
                            Auto-Mission
                          </span>
                        )}
                      </div>

                      <p className="text-body-sm text-slate leading-relaxed">
                        {item.description || item.intent}
                      </p>

                      {/* Expected outcome & verification */}
                      {(item.expected_outcome || item.verification || item.verify_rule) && (
                        <div className="flex flex-wrap items-center gap-2 pt-1">
                          {item.expected_outcome && (
                            <span className="text-[11px] font-mono text-deep-ink/80 bg-soft-meadow px-2.5 py-1 rounded-lg border border-onyx/5 flex items-center gap-1.5">
                              <Target className="w-3 h-3 text-slate" />
                              {item.expected_outcome}
                            </span>
                          )}
                          {(item.verification || item.verify_rule) && (
                            <span className="text-[11px] font-mono text-emerald-800 bg-emerald-50 px-2.5 py-1 rounded-lg border border-emerald-200/50 flex items-center gap-1.5">
                              <ShieldCheck className="w-3 h-3 text-emerald-600" />
                              {item.verification || item.verify_rule}
                            </span>
                          )}
                        </div>
                      )}
                    </div>

                    {/* Actions */}
                    <div className="flex items-center gap-1.5 shrink-0 self-end md:self-start">
                      <IconButton
                        size="sm"
                        label={item.enabled ? 'Disable' : 'Enable'}
                        icon={<Zap className={`w-3.5 h-3.5 ${item.enabled ? 'text-amber-500 fill-amber-500' : 'text-slate'}`} />}
                        onClick={() => handleToggleDirective(item)}
                      />
                      <IconButton
                        size="sm"
                        label={t('actions.edit')}
                        icon={<Edit2 className="w-3.5 h-3.5" />}
                        onClick={() => {
                          setEditingDirective(item);
                          setIsModalOpen(true);
                        }}
                      />
                      <IconButton
                        size="sm"
                        tone="danger"
                        label={t('actions.delete')}
                        icon={<Trash2 className="w-3.5 h-3.5" />}
                        onClick={() => setDeletingDirectiveId(item.id)}
                      />
                    </div>
                  </div>
                </Card>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Mode 2: Raw Markdown Directives */}
      {mode === 'raw' && (
        <Card className="p-5 border border-onyx/10 bg-canvas space-y-4">
          <label className="block text-caption font-semibold text-deep-ink">
            {t('directives.instructions')}
          </label>
          <textarea
            value={rawText}
            onChange={(e) => setRawText(e.target.value)}
            placeholder={t('directives.placeholder')}
            rows={10}
            className="w-full rounded-[18px] border border-onyx/15 bg-soft-meadow p-4 font-mono text-body-sm text-deep-ink placeholder:text-slate/60 focus:outline-none focus:ring-1 focus:ring-deep-ink transition-all"
          />
          <div className="flex justify-end">
            <Button
              variant="primary"
              onClick={handleSaveRaw}
              disabled={saving}
            >
              {saving ? t('directives.saving') : t('directives.save')}
            </Button>
          </div>
        </Card>
      )}

      {/* Add / Edit Directive Modal */}
      <DirectiveModal
        isOpen={isModalOpen}
        onClose={() => {
          setIsModalOpen(false);
          setEditingDirective(null);
        }}
        onSave={handleSaveDirective}
        directive={editingDirective}
      />

      {/* Delete Confirmation Modal */}
      <ConfirmModal
        isOpen={deletingDirectiveId !== null}
        onClose={() => setDeletingDirectiveId(null)}
        onConfirm={handleDeleteDirective}
        title={t('directives.deleteDirective')}
        description={t('directives.deleteDirectiveConfirm')}
        confirmLabel={t('actions.delete')}
        variant="danger"
      />
    </div>
  );
}
