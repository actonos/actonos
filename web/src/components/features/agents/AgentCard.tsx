import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { Bot, Play, Square, MessageSquare, Trash2, Edit } from 'lucide-react';
import type { AgentManifest } from '@/lib/types';

export interface AgentCardProps {
  agent: AgentManifest;
  onChat: (id: string) => void;
  onEdit: (agent: AgentManifest) => void;
  onDelete: (id: string) => void;
  onToggleStatus: (id: string, currentStatus: string) => void;
}

export function AgentCard({ agent, onChat, onEdit, onDelete, onToggleStatus }: AgentCardProps) {
  const { t } = useTranslation('agents');
  const { t: tCommon } = useTranslation('common');

  const isActive = agent.status === 'active';

  return (
    <Card className="flex flex-col justify-between h-full group hover:border-onyx/20 transition-all border border-transparent">
      <div>
        {/* Top bar: Icon + Model tag + Status */}
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <div className="w-11 h-11 rounded-full bg-canvas flex items-center justify-center text-deep-ink border border-onyx shadow-xs">
              <Bot className="w-6 h-6" />
            </div>
            <div>
              <span className="text-caption uppercase tracking-wider text-slate font-semibold block">
                {agent.model_config.primary_model || 'Standard LLM'}
              </span>
              <Badge variant={isActive ? 'active' : 'stopped'}>
                {isActive ? tCommon('status.active') : tCommon('status.stopped')}
              </Badge>
            </div>
          </div>

          <div className="flex items-center gap-1 opacity-80 group-hover:opacity-100 transition-opacity">
            <button
              onClick={() => onEdit(agent)}
              className="p-1.5 rounded-full hover:bg-canvas text-slate hover:text-deep-ink cursor-pointer"
              title={t('actions.editAgent')}
            >
              <Edit className="w-4 h-4" />
            </button>
            <button
              onClick={() => onDelete(agent.agent_id)}
              className="p-1.5 rounded-full hover:bg-canvas text-slate hover:text-red-600 cursor-pointer"
              title={tCommon('buttons.delete')}
            >
              <Trash2 className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Name and description */}
        <h3 className="font-serif text-heading-sm text-deep-ink mb-2 leading-tight">
          {agent.name}
        </h3>

        <p className="font-sans text-body-sm text-slate line-clamp-3 mb-4">
          {agent.description || agent.system_instructions}
        </p>

        {/* Metadata badges */}
        <div className="flex flex-wrap gap-2 pt-2 border-t border-canvas mb-6">
          <span className="text-caption bg-canvas px-3 py-1 rounded-full text-slate border border-onyx/10">
            {t('card.toolsCount', { count: agent.authorized_tools?.length || 0 })}
          </span>
          {agent.delegation_scope?.max_monthly_budget_usd > 0 && (
            <span className="text-caption bg-canvas px-3 py-1 rounded-full text-slate border border-onyx/10">
              {t('card.budget', { amount: agent.delegation_scope.max_monthly_budget_usd })}
            </span>
          )}
          {agent.delegation_scope?.require_human_approval_level && (
            <span className="text-caption bg-canvas px-3 py-1 rounded-full text-slate border border-onyx/10">
              {t('card.approval', { level: agent.delegation_scope.require_human_approval_level })}
            </span>
          )}
        </div>
      </div>

      {/* Action CTA pair */}
      <div className="flex items-center gap-2 pt-4 border-t border-canvas">
        <Button
          variant={isActive ? 'secondary' : 'primary'}
          size="sm"
          className="flex-1"
          icon={<MessageSquare className="w-4 h-4" />}
          onClick={() => onChat(agent.agent_id)}
        >
          {t('card.openChat')}
        </Button>

        <Button
          variant="ghost"
          size="sm"
          icon={isActive ? <Square className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
          onClick={() => onToggleStatus(agent.agent_id, agent.status)}
        >
          {isActive ? tCommon('buttons.stop') : tCommon('buttons.start')}
        </Button>
      </div>
    </Card>
  );
}
