import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Wrench, Terminal, Cpu, Sparkles, Play } from 'lucide-react';
import type { ToolInfo } from '@/lib/types';

export interface ToolCardProps {
  tool: ToolInfo;
  onTest: (tool: ToolInfo) => void;
}

export function ToolCard({ tool, onTest }: ToolCardProps) {
  const { t } = useTranslation('tools');

  const getCategoryIcon = (category: string) => {
    switch (category) {
      case 'mcp':
        return <Terminal className="w-5 h-5" />;
      case 'wasm':
        return <Cpu className="w-5 h-5" />;
      case 'skill':
        return <Sparkles className="w-5 h-5" />;
      default:
        return <Wrench className="w-5 h-5" />;
    }
  };

  const getCategoryBadgeVariant = (category: string) => {
    switch (category) {
      case 'mcp':
        return 'accent';
      case 'wasm':
        return 'active';
      case 'skill':
        return 'neutral';
      default:
        return 'neutral';
    }
  };

  return (
    <Card className="flex flex-col justify-between h-full border border-transparent hover:border-onyx/20 transition-all">
      <div>
        <div className="flex items-center justify-between mb-3">
          <div className="w-10 h-10 rounded-full bg-canvas flex items-center justify-center text-deep-ink border border-onyx shadow-xs">
            {getCategoryIcon(tool.category)}
          </div>
          <Badge variant={getCategoryBadgeVariant(tool.category)}>
            {tool.category.toUpperCase()}
          </Badge>
        </div>

        <h3 className="font-serif text-heading-sm text-deep-ink mb-1.5 break-all">
          {tool.name}
        </h3>

        <p className="font-sans text-body-sm text-slate line-clamp-3 mb-4">
          {tool.description || 'No description available.'}
        </p>
      </div>

      <div className="pt-3 border-t border-canvas flex items-center justify-between">
        <span className="text-caption text-slate font-mono truncate max-w-[140px]">
          {tool.name}
        </span>
        <Button
          variant="ghost"
          size="sm"
          icon={<Play className="w-3.5 h-3.5" />}
          onClick={() => onTest(tool)}
        >
          {t('actions.testTool')}
        </Button>
      </div>
    </Card>
  );
}
