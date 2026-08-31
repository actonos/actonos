import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { api, type AuditLogItem } from '@/lib/api';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { EmptyState } from '@/components/ui/EmptyState';
import { useToast } from '@/components/ui/Toast';
import { getErrorMessage } from '@/lib/errors';
import { AuditLogDetailModal } from './components/AuditLogDetailModal';
import {
  ShieldCheck,
  ShieldAlert,
  Clock,
  RefreshCw,
  Download,
  Activity,
  FileText,
  Copy,
  Check,
  ChevronRight,
  AlertOctagon,
  Wrench,
} from 'lucide-react';

export function AuditLogsPage() {
  const { t } = useTranslation('audit');
  const { success, error } = useToast();

  const [logs, setLogs] = useState<AuditLogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [verifying, setVerifying] = useState(false);
  const [selectedLog, setSelectedLog] = useState<AuditLogItem | null>(null);
  const [liveTail, setLiveTail] = useState(false);

  // Filters & Search
  const [riskFilter, setRiskFilter] = useState<'all' | 'high' | 'medium' | 'low'>('all');
  const [statusFilter, setStatusFilter] = useState<'all' | 'success' | 'blocked' | 'failed'>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [quickPreset, setQuickPreset] = useState<'all' | 'critical' | 'failures' | 'slow'>('all');
  const [copiedTraceId, setCopiedTraceId] = useState<string | null>(null);

  const fetchLogs = async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await api.getAuditLogs({
        query: searchQuery || undefined,
        risk_level: riskFilter !== 'all' ? riskFilter : undefined,
        status: statusFilter !== 'all' ? statusFilter : undefined,
        limit: 100,
      });
      setLogs(res.entries || []);
    } catch (err) {
      if (!silent) error('Failed to load audit logs', getErrorMessage(err));
    } finally {
      if (!silent) setLoading(false);
    }
  };

  useEffect(() => {
    fetchLogs();
  }, [riskFilter, statusFilter]);

  // Live Tail Auto-polling
  useEffect(() => {
    if (!liveTail) return;
    const interval = setInterval(() => {
      fetchLogs(true);
    }, 3000);
    return () => clearInterval(interval);
  }, [liveTail, riskFilter, statusFilter, searchQuery]);

  const handleVerifyChain = async () => {
    setVerifying(true);
    try {
      const res = await api.verifyAuditChain();
      success(
        t('verifiedSuccess', 'Cryptographic Hash Chain Valid'),
        res.message || t('verifiedSuccessDesc', 'All SHA-256 block hashes are intact.')
      );
    } catch (err) {
      error(t('verifyFailed', 'Hash Chain Verification Failed'), getErrorMessage(err));
    } finally {
      setVerifying(false);
    }
  };

  const handleExportCSV = () => {
    const url = api.exportAuditLogsUrl('csv', {
      query: searchQuery || undefined,
      risk_level: riskFilter !== 'all' ? riskFilter : undefined,
      status: statusFilter !== 'all' ? statusFilter : undefined,
    });
    window.open(url, '_blank');
    success('Export Initiated', 'Audit CSV download started.');
  };

  const handleExportJSON = () => {
    const url = api.exportAuditLogsUrl('json', {
      query: searchQuery || undefined,
      risk_level: riskFilter !== 'all' ? riskFilter : undefined,
      status: statusFilter !== 'all' ? statusFilter : undefined,
    });
    window.open(url, '_blank');
    success('Export Initiated', 'Audit JSON download started.');
  };

  const handleCopyTrace = (e: React.MouseEvent, traceId: string) => {
    e.stopPropagation();
    navigator.clipboard.writeText(traceId);
    setCopiedTraceId(traceId);
    setTimeout(() => setCopiedTraceId(null), 2000);
  };

  const handleApplyPreset = (preset: 'all' | 'critical' | 'failures' | 'slow') => {
    setQuickPreset(preset);
    if (preset === 'critical') {
      setRiskFilter('high');
      setStatusFilter('all');
    } else if (preset === 'failures') {
      setRiskFilter('all');
      setStatusFilter('failed');
    } else if (preset === 'slow') {
      setRiskFilter('all');
      setStatusFilter('all');
    } else {
      setRiskFilter('all');
      setStatusFilter('all');
    }
  };

  // Filtered dataset
  const filteredLogs = useMemo(() => {
    return logs.filter((entry) => {
      if (quickPreset === 'slow' && (entry.execution_time_ms || 0) < 1000) {
        return false;
      }
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const matchTool = entry.tool_name?.toLowerCase().includes(q);
        const matchAgent = entry.agent_id?.toLowerCase().includes(q);
        const matchTrace = entry.trace_id?.toLowerCase().includes(q);
        const matchError = entry.error?.toLowerCase().includes(q);
        if (!matchTool && !matchAgent && !matchTrace && !matchError) {
          return false;
        }
      }
      return true;
    });
  }, [logs, quickPreset, searchQuery]);

  // Statistics
  const stats = useMemo(() => {
    const total = logs.length;
    const highRisk = logs.filter((l) => l.risk_level?.toLowerCase() === 'high').length;
    const blockedOrFailed = logs.filter((l) => l.status?.toLowerCase() !== 'success').length;
    const totalDuration = logs.reduce((acc, curr) => acc + (curr.execution_time_ms || 0), 0);
    const avgDuration = total > 0 ? Math.round(totalDuration / total) : 0;
    return { total, highRisk, blockedOrFailed, avgDuration };
  }, [logs]);

  return (
    <PageContainer maxWidth="wide">
      {/* Page Header */}
      <PageHeader
        eyebrow={t('eyebrow', 'Governance & Security')}
        title={t('title', 'Audit Logs Explorer')}
        description={t('subtitle', 'Immutable, cryptographically-chained ledger of tool executions, permission gates, and system operations.')}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant={liveTail ? 'primary' : 'ghost'}
              onClick={() => setLiveTail(!liveTail)}
              size="sm"
              icon={
                <span className={`w-2 h-2 rounded-full ${liveTail ? 'bg-emerald-400 animate-ping' : 'bg-slate'}`} />
              }
            >
              {liveTail ? t('liveTailActive', 'Live Tail: ON') : t('liveTail', 'Live Tail')}
            </Button>
            <Button
              variant="ghost"
              onClick={handleExportCSV}
              disabled={logs.length === 0}
              size="sm"
              icon={<Download className="w-3.5 h-3.5" />}
            >
              {t('exportCSV', 'Export CSV')}
            </Button>
            <Button
              variant="ghost"
              onClick={handleExportJSON}
              disabled={logs.length === 0}
              size="sm"
              icon={<Download className="w-3.5 h-3.5" />}
            >
              {t('exportLogs', 'Export JSON')}
            </Button>
            <Button
              variant="ghost"
              onClick={handleVerifyChain}
              disabled={verifying}
              size="sm"
              icon={<ShieldCheck className="w-3.5 h-3.5 text-emerald-600" />}
            >
              {verifying ? t('verifying', 'Verifying...') : t('verifyChain', 'Verify Hash Chain')}
            </Button>
            <Button
              variant="ghost"
              onClick={() => fetchLogs()}
              disabled={loading}
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
            >
              {t('refresh', 'Refresh')}
            </Button>
          </div>
        }
      />

      <div className="space-y-6">
        {/* Metric Cards Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <Card className="p-4 border border-onyx/10 bg-canvas flex items-center justify-between">
            <div className="space-y-1">
              <span className="text-caption font-medium text-slate">{t('stats.totalEvents', 'Total Recorded Events')}</span>
              <div className="font-serif text-heading-md font-bold text-deep-ink">{stats.total}</div>
            </div>
            <div className="p-3 rounded-2xl bg-soft-meadow border border-onyx/10 text-deep-ink">
              <Activity className="w-5 h-5" />
            </div>
          </Card>

          <Card className="p-4 border border-onyx/10 bg-canvas flex items-center justify-between">
            <div className="space-y-1">
              <span className="text-caption font-medium text-slate">{t('stats.highRisk', 'High-Risk Actions')}</span>
              <div className="font-serif text-heading-md font-bold text-accent-coral">{stats.highRisk}</div>
            </div>
            <div className="p-3 rounded-2xl bg-accent-coral/10 border border-accent-coral/20 text-accent-coral">
              <AlertOctagon className="w-5 h-5" />
            </div>
          </Card>

          <Card className="p-4 border border-onyx/10 bg-canvas flex items-center justify-between">
            <div className="space-y-1">
              <span className="text-caption font-medium text-slate">{t('stats.blockedPolicies', 'Blocked / Failed Actions')}</span>
              <div className="font-serif text-heading-md font-bold text-amber-600">{stats.blockedOrFailed}</div>
            </div>
            <div className="p-3 rounded-2xl bg-amber-500/10 border border-amber-500/20 text-amber-600">
              <ShieldAlert className="w-5 h-5" />
            </div>
          </Card>

          <Card className="p-4 border border-onyx/10 bg-canvas flex items-center justify-between">
            <div className="space-y-1">
              <span className="text-caption font-medium text-slate">{t('stats.avgDuration', 'Avg Execution Latency')}</span>
              <div className="font-serif text-heading-md font-bold text-deep-ink">{stats.avgDuration} <span className="text-caption font-sans font-normal text-slate">ms</span></div>
            </div>
            <div className="p-3 rounded-2xl bg-soft-meadow border border-onyx/10 text-deep-ink">
              <Clock className="w-5 h-5" />
            </div>
          </Card>
        </div>

        {/* Filters & Search Toolbar */}
        <Card className="p-4 border border-onyx/10 bg-canvas/90 space-y-4">
          {/* Quick Presets Bar */}
          <div className="flex flex-wrap items-center gap-2 border-b border-onyx/10 pb-3">
            <span className="text-caption font-semibold text-slate uppercase tracking-wider">{t('presets.label', 'Presets')}:</span>
            {[
              { id: 'all' as const, label: t('presets.all', 'All Logs') },
              { id: 'critical' as const, label: t('presets.critical', 'High-Risk Only') },
              { id: 'failures' as const, label: t('presets.failures', 'Blocked / Failures') },
              { id: 'slow' as const, label: t('presets.slow', 'High Latency (>1s)') },
            ].map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => handleApplyPreset(p.id)}
                className={`px-3 py-1 rounded-full text-caption font-medium transition-colors cursor-pointer ${
                  quickPreset === p.id
                    ? 'bg-deep-ink text-white font-semibold shadow-2xs'
                    : 'bg-soft-meadow text-deep-ink hover:bg-soft-meadow/80'
                }`}
              >
                {p.label}
              </button>
            ))}
          </div>

          <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
            {/* Filter Pills */}
            <div className="flex flex-wrap items-center gap-3">
              {/* Risk Level Pills */}
              <div className="flex items-center gap-1 bg-soft-meadow p-1 rounded-full border border-onyx/10">
                <span className="text-caption font-medium text-slate px-2 hidden sm:inline">{t('filters.riskLevel')}:</span>
                {(['all', 'high', 'medium', 'low'] as const).map((r) => (
                  <button
                    key={r}
                    type="button"
                    onClick={() => {
                      setRiskFilter(r);
                      setQuickPreset('all');
                    }}
                    className={`px-3 py-1 rounded-full text-caption font-medium capitalize cursor-pointer transition-colors ${riskFilter === r ? 'bg-deep-ink text-white font-semibold' : 'text-deep-ink hover:text-slate'
                      }`}
                  >
                    {t(`filters.${r}`)}
                  </button>
                ))}
              </div>

              {/* Status Pills */}
              <div className="flex items-center gap-1 bg-soft-meadow p-1 rounded-full border border-onyx/10">
                <span className="text-caption font-medium text-slate px-2 hidden sm:inline">{t('filters.status')}:</span>
                {(['all', 'success', 'blocked', 'failed'] as const).map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => setStatusFilter(s)}
                    className={`px-3 py-1 rounded-full text-caption font-medium capitalize cursor-pointer transition-colors ${statusFilter === s ? 'bg-deep-ink text-white font-semibold' : 'text-deep-ink hover:text-slate'
                      }`}
                  >
                    {t(`filters.${s}`)}
                  </button>
                ))}
              </div>
            </div>

            {/* Search Input */}
            <div className="w-full lg:w-80">
              <Input
                placeholder={t('filters.searchPlaceholder', 'Search logs...')}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full text-body-sm"
              />
            </div>
          </div>
        </Card>

        {/* Audit Log Table */}
        <Card className="p-0 border border-onyx/10 bg-canvas/90 overflow-hidden shadow-xs">
          {filteredLogs.length === 0 ? (
            <div className="py-16">
              <EmptyState
                icon={<FileText className="w-10 h-10" />}
                title={t('empty')}
                description={t('empty')}
              />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-onyx/10 bg-soft-meadow text-caption font-semibold text-slate">
                    <th className="py-3 px-4">{t('columns.tool', 'Tool / Action')}</th>
                    <th className="py-3 px-4">{t('columns.agent', 'Agent')}</th>
                    <th className="py-3 px-4">{t('columns.risk', 'Risk')}</th>
                    <th className="py-3 px-4">{t('columns.status', 'Status')}</th>
                    <th className="py-3 px-4">{t('columns.duration', 'Latency')}</th>
                    <th className="py-3 px-4">{t('columns.trace', 'Trace ID')}</th>
                    <th className="py-3 px-4">{t('columns.timestamp', 'Timestamp')}</th>
                    <th className="py-3 px-4 text-right"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-onyx/5 text-body-sm">
                  {filteredLogs.map((log, idx) => (
                    <tr
                      key={log.trace_id || idx}
                      onClick={() => setSelectedLog(log)}
                      className="hover:bg-soft-meadow/50 transition-colors cursor-pointer group"
                    >
                      {/* Tool Name */}
                      <td className="py-3.5 px-4 font-mono font-bold text-deep-ink">
                        <div className="flex items-center gap-2">
                          <Wrench className="w-3.5 h-3.5 text-slate group-hover:text-deep-ink" />
                          <span>{log.tool_name}</span>
                        </div>
                      </td>

                      {/* Agent ID */}
                      <td className="py-3.5 px-4 font-mono text-caption text-slate">
                        {log.agent_id}
                      </td>

                      {/* Risk Level */}
                      <td className="py-3.5 px-4">
                        <Badge
                          variant={
                            log.risk_level === 'High'
                              ? 'accent'
                              : log.risk_level === 'Medium'
                                ? 'stopped'
                                : 'neutral'
                          }
                          className="text-[10px]"
                        >
                          {log.risk_level}
                        </Badge>
                      </td>

                      {/* Status */}
                      <td className="py-3.5 px-4">
                        <Badge
                          variant={
                            log.status === 'Success'
                              ? 'neutral'
                              : log.status === 'Blocked'
                                ? 'stopped'
                                : 'accent'
                          }
                          className="text-[10px]"
                        >
                          {log.status}
                        </Badge>
                      </td>

                      {/* Latency */}
                      <td className="py-3.5 px-4 font-mono text-caption text-slate">
                        {log.execution_time_ms} ms
                      </td>

                      {/* Trace ID */}
                      <td className="py-3.5 px-4">
                        <div className="flex items-center gap-1.5">
                          <span className="font-mono text-caption text-slate max-w-[120px] truncate" title={log.trace_id}>
                            {log.trace_id}
                          </span>
                          <button
                            type="button"
                            onClick={(e) => handleCopyTrace(e, log.trace_id)}
                            className="opacity-0 group-hover:opacity-100 p-1 hover:bg-canvas rounded text-slate hover:text-deep-ink transition-all cursor-pointer"
                            title="Copy Trace ID"
                          >
                            {copiedTraceId === log.trace_id ? (
                              <Check className="w-3 h-3 text-green-600" />
                            ) : (
                              <Copy className="w-3 h-3" />
                            )}
                          </button>
                        </div>
                      </td>

                      {/* Timestamp */}
                      <td className="py-3.5 px-4 font-mono text-caption text-slate whitespace-nowrap">
                        {new Date(log.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                      </td>

                      {/* Action Chevron */}
                      <td className="py-3.5 px-4 text-right">
                        <ChevronRight className="w-4 h-4 text-slate group-hover:text-deep-ink group-hover:translate-x-0.5 transition-all inline-block" />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      </div>

      {/* Record Details Modal */}
      <AuditLogDetailModal
        log={selectedLog}
        isOpen={Boolean(selectedLog)}
        onClose={() => setSelectedLog(null)}
      />
    </PageContainer>
  );
}
