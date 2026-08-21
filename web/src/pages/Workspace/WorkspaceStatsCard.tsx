import { useTranslation } from 'react-i18next';
import { HardDrive, FileText, Folder, Sparkles } from 'lucide-react';
import type { WorkspaceStatsResponse } from '@/lib/types';

interface WorkspaceStatsCardProps {
  stats: WorkspaceStatsResponse | null;
  loading?: boolean;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

export function WorkspaceStatsCard({ stats, loading }: WorkspaceStatsCardProps) {
  const { t } = useTranslation('workspace');

  if (loading || !stats) {
    return (
      <div className="rounded-[24px] border border-deep-ink/10 bg-canvas p-4 animate-pulse">
        <div className="h-4 w-32 bg-deep-ink/10 rounded-full mb-3" />
        <div className="h-6 w-24 bg-deep-ink/10 rounded-full mb-4" />
        <div className="grid grid-cols-3 gap-2">
          <div className="h-10 bg-deep-ink/5 rounded-xl" />
          <div className="h-10 bg-deep-ink/5 rounded-xl" />
          <div className="h-10 bg-deep-ink/5 rounded-xl" />
        </div>
      </div>
    );
  }

  const total = stats.total_size || 1;
  const docsPct = Math.min(100, Math.round(((stats.breakdown?.documents || 0) / total) * 100));
  const codePct = Math.min(100, Math.round(((stats.breakdown?.code || 0) / total) * 100));
  const dataPct = Math.min(100, Math.round(((stats.breakdown?.data || 0) / total) * 100));
  const mediaPct = Math.min(100, Math.round(((stats.breakdown?.media || 0) / total) * 100));
  const otherPct = Math.max(0, 100 - (docsPct + codePct + dataPct + mediaPct));

  const indexedRatio = stats.total_files > 0
    ? Math.round((stats.indexed_files / stats.total_files) * 100)
    : 0;

  return (
    <div className="rounded-[24px] border border-deep-ink/10 bg-canvas p-4 text-deep-ink flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-full bg-hi-yellow flex items-center justify-center text-deep-ink font-semibold text-xs">
            <HardDrive className="w-4 h-4" />
          </div>
          <div>
            <div className="text-caption text-slate font-medium">{t('stats.title')}</div>
            <div className="text-subheading font-bold">{formatBytes(stats.total_size)}</div>
          </div>
        </div>
        <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-status-success-soft text-status-success text-caption font-semibold">
          <Sparkles className="w-3.5 h-3.5" />
          <span>{indexedRatio}% {t('inspector.overview')}</span>
        </div>
      </div>

      {/* Multi-category usage bar */}
      <div className="w-full h-2 rounded-full bg-soft-meadow overflow-hidden flex">
        {docsPct > 0 && <div style={{ width: `${docsPct}%` }} className="h-full bg-fuchsia" title={`Docs: ${docsPct}%`} />}
        {codePct > 0 && <div style={{ width: `${codePct}%` }} className="h-full bg-deep-ink" title={`Code: ${codePct}%`} />}
        {dataPct > 0 && <div style={{ width: `${dataPct}%` }} className="h-full bg-hi-yellow" title={`Data: ${dataPct}%`} />}
        {mediaPct > 0 && <div style={{ width: `${mediaPct}%` }} className="h-full bg-moss-green" title={`Media: ${mediaPct}%`} />}
        {otherPct > 0 && <div style={{ width: `${otherPct}%` }} className="h-full bg-slate/30" title={`Other: ${otherPct}%`} />}
      </div>

      {/* Mini metric pills */}
      <div className="grid grid-cols-3 gap-2 text-center text-caption">
        <div className="p-2 rounded-xl bg-soft-meadow/50 border border-deep-ink/5 flex flex-col">
          <div className="flex items-center justify-center gap-1 text-slate font-medium">
            <FileText className="w-3 h-3" />
            <span>{t('stats.files')}</span>
          </div>
          <span className="text-body-sm font-bold text-deep-ink mt-0.5">{stats.total_files}</span>
        </div>
        <div className="p-2 rounded-xl bg-soft-meadow/50 border border-deep-ink/5 flex flex-col">
          <div className="flex items-center justify-center gap-1 text-slate font-medium">
            <Folder className="w-3 h-3" />
            <span>{t('stats.folders')}</span>
          </div>
          <span className="text-body-sm font-bold text-deep-ink mt-0.5">{stats.total_directories}</span>
        </div>
        <div className="p-2 rounded-xl bg-soft-meadow/50 border border-deep-ink/5 flex flex-col">
          <div className="flex items-center justify-center gap-1 text-status-success font-medium">
            <Sparkles className="w-3 h-3" />
            <span>{t('stats.indexed')}</span>
          </div>
          <span className="text-body-sm font-bold text-status-success mt-0.5">{stats.indexed_files}</span>
        </div>
      </div>
    </div>
  );
}
