import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import {
  RefreshCw,
  Filter,
  Layers,
  Bot,
  TrendingUp,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { TokenUsageSummary, TokenUsageRecord, AgentManifest } from '@/lib/types';

interface TokenLedgerModalProps {
  isOpen: boolean;
  onClose: () => void;
}

interface TokenLedgerPanelProps {
  active?: boolean;
  onClose?: () => void;
}

export function TokenLedgerPanel({ active = true, onClose }: TokenLedgerPanelProps) {
  const { t } = useTranslation('settings');
  const [activeTab, setActiveTab] = useState<'overview' | 'transactions'>('overview');
  const [summary, setSummary] = useState<TokenUsageSummary | null>(null);
  const [history, setHistory] = useState<TokenUsageRecord[]>([]);
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [selectedAgent, setSelectedAgent] = useState<string>('all');
  const [selectedSource, setSelectedSource] = useState<string>('all');
  const [loading, setLoading] = useState(false);

  const loadData = async () => {
    if (!active) return;
    setLoading(true);
    try {
      const [sumRes, histRes, agsRes] = await Promise.all([
        api.getTokenUsage().catch(() => null),
        api.getTokenHistory({
          agent_id: selectedAgent !== 'all' ? selectedAgent : undefined,
          source: selectedSource !== 'all' ? selectedSource : undefined,
        }).catch(() => []),
        api.listAgents().catch(() => ({ agents: [], count: 0 })),
      ]);
      if (sumRes) setSummary(sumRes);
      setHistory(histRes || []);
      if (agsRes && agsRes.agents) setAgents(agsRes.agents);
    } catch (err) {
      console.error('Failed to load token ledger:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (active) {
      loadData();
    }
  }, [active, selectedAgent, selectedSource]);

  const maxDailyTokens = summary?.daily_trend && summary.daily_trend.length > 0
    ? Math.max(...summary.daily_trend.map((d) => d.total_tokens), 1)
    : 1;

  const getSourceBadgeColor = (source: string) => {
    switch (source) {
      case 'cron':
        return 'bg-amber-500/10 text-amber-600 border-amber-500/20';
      case 'heartbeat':
        return 'bg-rose-500/10 text-rose-600 border-rose-500/20';
      case 'channel':
        return 'bg-sky-500/10 text-sky-600 border-sky-500/20';
      case 'stream':
      case 'chat':
      default:
        return 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20';
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Tabs & Controls */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-3 border-b border-onyx/10">
        <div className="flex items-center gap-1.5 p-1 bg-soft-meadow rounded-full border border-onyx/10">
          <button
            type="button"
            onClick={() => setActiveTab('overview')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans transition-all cursor-pointer ${activeTab === 'overview'
              ? 'bg-deep-ink text-white font-semibold shadow-xs'
              : 'text-slate hover:text-deep-ink'
              }`}
          >
            {t('tokenLedger.overview')}
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('transactions')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans transition-all cursor-pointer ${activeTab === 'transactions'
              ? 'bg-deep-ink text-white font-semibold shadow-xs'
              : 'text-slate hover:text-deep-ink'
              }`}
          >
            {t('tokenLedger.transactions', { count: history.length })}
          </button>
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
            onClick={loadData}
            disabled={loading}
          >
            {t('tokenLedger.refresh')}
          </Button>
        </div>
      </div>

      {/* TAB 1: Overview & Breakdown */}
      {activeTab === 'overview' && (
        <div className="space-y-6">
          {/* 4 Summary Metric Cards */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-1">
              <span className="text-[11px] font-semibold uppercase text-slate block">{t('tokenLedger.today')}</span>
              <div className="text-heading-sm font-serif font-bold text-deep-ink">
                {(summary?.today_tokens || 0).toLocaleString()}
              </div>
              <span className="text-[11px] font-mono text-emerald-700 font-semibold block">
                {t('tokenLedger.currency', { value: (summary?.today_cost_usd || 0).toFixed(4) })}
              </span>
            </div>

            <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-1">
              <span className="text-[11px] font-semibold uppercase text-slate block">{t('tokenLedger.month')}</span>
              <div className="text-heading-sm font-serif font-bold text-deep-ink">
                {(summary?.month_tokens || 0).toLocaleString()}
              </div>
              <span className="text-[11px] font-mono text-emerald-700 font-semibold block">
                {t('tokenLedger.currency', { value: (summary?.month_cost_usd || 0).toFixed(4) })}
              </span>
            </div>

            <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-1">
              <span className="text-[11px] font-semibold uppercase text-slate block">{t('tokenLedger.lifetime')}</span>
              <div className="text-heading-sm font-serif font-bold text-deep-ink">
                {(summary?.total_tokens || 0).toLocaleString()}
              </div>
              <span className="text-[11px] font-mono text-slate block">
                {t('tokenLedger.inputOutput', {
                  input: (summary?.total_prompt_tokens || 0).toLocaleString(),
                  output: (summary?.total_completion_tokens || 0).toLocaleString(),
                })}
              </span>
            </div>

            <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-1">
              <span className="text-[11px] font-semibold uppercase text-slate block">{t('tokenLedger.totalCost')}</span>
              <div className="text-heading-sm font-serif font-bold text-emerald-700">
                ${(summary?.total_cost_usd || 0).toFixed(4)}
              </div>
              <span className="text-[11px] font-mono text-slate block">
                {t('tokenLedger.marketCatalog')}
              </span>
            </div>
          </div>

          {/* 14-Day Daily Trend Visual Bar Chart */}
          <div className="p-5 rounded-2xl bg-canvas border border-onyx/10 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <TrendingUp className="w-4 h-4 text-deep-ink" />
                <h4 className="font-serif text-body-sm font-semibold text-deep-ink">
                  {t('tokenLedger.trend')}
                </h4>
              </div>
              <span className="text-[11px] text-slate font-mono">{t('tokenLedger.dailyRollup')}</span>
            </div>

            {summary?.daily_trend && summary.daily_trend.length > 0 ? (
              <div className="space-y-2 pt-2">
                <div className="flex items-end gap-1.5 h-32 pt-4 px-2 bg-soft-meadow/50 rounded-xl border border-onyx/5">
                  {summary.daily_trend.map((d) => {
                    const heightPercent = Math.max(8, (d.total_tokens / maxDailyTokens) * 100);
                    return (
                      <div key={d.date} className="flex-1 flex flex-col items-center gap-1 group relative h-full justify-end">
                        {/* Tooltip on hover */}
                        <div className="absolute -top-10 bg-deep-ink text-white text-[10px] py-1 px-2 rounded-lg opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none whitespace-nowrap z-10 font-mono shadow-md">
                          {t('tokenLedger.tooltip', {
                            date: d.date,
                            tokens: d.total_tokens.toLocaleString(),
                            cost: d.cost_usd.toFixed(4),
                          })}
                        </div>
                        <div
                          className="w-full bg-deep-ink rounded-t-md hover:bg-emerald-600 transition-all duration-300 min-h-[4px]"
                          style={{ height: `${heightPercent}%` }}
                        />
                        <span className="text-[9px] text-slate font-mono truncate w-full text-center">
                          {d.date.slice(5)}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : (
              <div className="p-8 text-center text-caption text-slate bg-soft-meadow rounded-xl">
                {t('tokenLedger.noTrend')}
              </div>
            )}
          </div>

          {/* Model & Agent Breakdown Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Model Breakdown */}
            <div className="p-5 rounded-2xl bg-canvas border border-onyx/10 space-y-3">
              <div className="flex items-center gap-2">
                <Layers className="w-4 h-4 text-deep-ink" />
                <h4 className="font-serif text-body-sm font-semibold text-deep-ink">
                  {t('tokenLedger.byModel')}
                </h4>
              </div>

              {summary?.by_model && summary.by_model.length > 0 ? (
                <div className="space-y-2.5">
                  {summary.by_model.map((m) => (
                    <div key={m.model} className="space-y-1">
                      <div className="flex items-center justify-between text-caption">
                        <span className="font-mono font-semibold text-deep-ink truncate">{m.model}</span>
                        <span className="font-mono text-slate text-[11px]">
                          {t('tokenLedger.usage', {
                            tokens: m.total_tokens.toLocaleString(),
                            cost: m.cost_usd.toFixed(4),
                          })} <strong className="text-emerald-700">{m.percentage.toFixed(1)}%</strong>
                        </span>
                      </div>
                      <div className="w-full bg-onyx/10 h-1.5 rounded-full overflow-hidden">
                        <div
                          className="bg-deep-ink h-full rounded-full transition-all duration-500"
                          style={{ width: `${m.percentage}%` }}
                        />
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-caption text-slate">{t('tokenLedger.noModels')}</p>
              )}
            </div>

            {/* Agent Breakdown */}
            <div className="p-5 rounded-2xl bg-canvas border border-onyx/10 space-y-3">
              <div className="flex items-center gap-2">
                <Bot className="w-4 h-4 text-deep-ink" />
                <h4 className="font-serif text-body-sm font-semibold text-deep-ink">
                  {t('tokenLedger.byAgent')}
                </h4>
              </div>

              {summary?.by_agent && summary.by_agent.length > 0 ? (
                <div className="space-y-2.5">
                  {summary.by_agent.map((a) => (
                    <div key={a.agent_id} className="space-y-1">
                      <div className="flex items-center justify-between text-caption">
                        <span className="font-mono font-semibold text-deep-ink truncate">{a.agent_id}</span>
                        <span className="font-mono text-slate text-[11px]">
                          {t('tokenLedger.usage', {
                            tokens: a.total_tokens.toLocaleString(),
                            cost: a.cost_usd.toFixed(4),
                          })} <strong className="text-emerald-700">{a.percentage.toFixed(1)}%</strong>
                        </span>
                      </div>
                      <div className="w-full bg-onyx/10 h-1.5 rounded-full overflow-hidden">
                        <div
                          className="bg-emerald-600 h-full rounded-full transition-all duration-500"
                          style={{ width: `${a.percentage}%` }}
                        />
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-caption text-slate">{t('tokenLedger.noAgents')}</p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* TAB 2: Transaction Ledger Table */}
      {activeTab === 'transactions' && (
        <div className="space-y-4">
          {/* Filter Bar */}
          <div className="flex flex-wrap items-center gap-3 p-3 bg-soft-meadow rounded-2xl border border-onyx/10">
            <div className="flex items-center gap-2 text-caption font-semibold text-deep-ink">
              <Filter className="w-3.5 h-3.5 text-slate" />
              <span>{t('tokenLedger.filters')}</span>
            </div>

            <select
              value={selectedAgent}
              onChange={(e) => setSelectedAgent(e.target.value)}
              className="bg-canvas text-deep-ink text-[12px] font-sans px-3 py-1.5 rounded-full border border-onyx/10 focus:outline-none"
            >
              <option value="all">{t('tokenLedger.allAgents')}</option>
              {agents.map((ag) => (
                <option key={ag.agent_id} value={ag.agent_id}>
                  {ag.name} ({ag.agent_id})
                </option>
              ))}
            </select>

            <select
              value={selectedSource}
              onChange={(e) => setSelectedSource(e.target.value)}
              className="bg-canvas text-deep-ink text-[12px] font-sans px-3 py-1.5 rounded-full border border-onyx/10 focus:outline-none"
            >
              <option value="all">{t('tokenLedger.allSources')}</option>
              <option value="chat">{t('tokenLedger.sourceChat')}</option>
              <option value="stream">{t('tokenLedger.sourceStream')}</option>
              <option value="cron">{t('tokenLedger.sourceCron')}</option>
              <option value="heartbeat">{t('tokenLedger.sourceHeartbeat')}</option>
              <option value="channel">{t('tokenLedger.sourceChannel')}</option>
            </select>
          </div>

          {/* Ledger Table */}
          <div className="border border-onyx/10 rounded-2xl overflow-hidden bg-canvas">
            <div className="max-h-96 overflow-y-auto">
              <table className="w-full text-left border-collapse text-body-sm font-sans">
                <thead>
                  <tr className="border-b border-onyx/10 bg-soft-meadow/50 text-[11px] font-semibold uppercase tracking-wider text-slate sticky top-0 bg-soft-meadow z-10">
                    <th className="py-2.5 px-3">{t('tokenLedger.columns.time')}</th>
                    <th className="py-2.5 px-3">{t('tokenLedger.columns.agent')}</th>
                    <th className="py-2.5 px-3">{t('tokenLedger.columns.model')}</th>
                    <th className="py-2.5 px-3">{t('tokenLedger.columns.source')}</th>
                    <th className="py-2.5 px-3 text-right">{t('tokenLedger.columns.prompt')}</th>
                    <th className="py-2.5 px-3 text-right">{t('tokenLedger.columns.completion')}</th>
                    <th className="py-2.5 px-3 text-right">{t('tokenLedger.columns.total')}</th>
                    <th className="py-2.5 px-3 text-right">{t('tokenLedger.columns.cost')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-onyx/5 font-mono text-[12px]">
                  {history.length > 0 ? (
                    history.map((rec) => (
                      <tr key={rec.id} className="hover:bg-soft-meadow/30 transition-colors">
                        <td className="py-2.5 px-3 whitespace-nowrap text-slate text-[11px]">
                          {new Date(rec.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                        </td>
                        <td className="py-2.5 px-3 font-semibold text-deep-ink max-w-[120px] truncate">
                          {rec.agent_id}
                        </td>
                        <td className="py-2.5 px-3 text-deep-ink max-w-[130px] truncate">
                          {rec.model}
                        </td>
                        <td className="py-2.5 px-3">
                          <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold border ${getSourceBadgeColor(rec.source)}`}>
                            {rec.source}
                          </span>
                        </td>
                        <td className="py-2.5 px-3 text-right text-slate">
                          {rec.prompt_tokens.toLocaleString()}
                        </td>
                        <td className="py-2.5 px-3 text-right text-slate">
                          {rec.completion_tokens.toLocaleString()}
                        </td>
                        <td className="py-2.5 px-3 text-right font-semibold text-deep-ink">
                          {rec.total_tokens.toLocaleString()}
                        </td>
                        <td className="py-2.5 px-3 text-right text-emerald-700 font-semibold">
                          ${rec.estimated_cost_usd.toFixed(5)}
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={8} className="py-8 text-center text-caption text-slate font-sans">
                        {t('tokenLedger.noTransactions')}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* Modal Footer */}
      {onClose && <div className="flex items-center justify-end pt-3 border-t border-onyx/10">
        <Button variant="primary" size="md" onClick={onClose}>
          {t('tokenLedger.close')}
        </Button>
      </div>}
    </div>
  );
}

export function TokenLedgerModal({ isOpen, onClose }: TokenLedgerModalProps) {
  const { t } = useTranslation('settings');
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('tokenLedger.title')}
      maxWidth="max-w-4xl"
    >
      <TokenLedgerPanel active={isOpen} onClose={onClose} />
    </Modal>
  );
}
