import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { getErrorMessage } from '@/lib/errors';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Play, Copy, Check, Terminal } from 'lucide-react';
import { api } from '@/lib/api';
import type { ToolInfo } from '@/lib/types';

export interface ToolTestModalProps {
  tool: ToolInfo | null;
  isOpen: boolean;
  onClose: () => void;
}

export function ToolTestModal({ tool, isOpen, onClose }: ToolTestModalProps) {
  const { t } = useTranslation('tools');
  const [inputJSON, setInputJSON] = useState('{}');
  const [output, setOutput] = useState('');
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (tool?.schema?.properties) {
      const sample: Record<string, unknown> = {};
      for (const [key, rawValue] of Object.entries(tool.schema.properties as Record<string, unknown>)) {
        const val = typeof rawValue === 'object' && rawValue !== null
          ? rawValue as Record<string, unknown>
          : {};
        if (val.type === 'string') {
          sample[key] = key === 'url' ? 'https://httpbin.org/get' : 'sample_value';
        } else if (val.type === 'number' || val.type === 'integer') {
          sample[key] = 10;
        } else if (val.type === 'boolean') {
          sample[key] = true;
        } else {
          sample[key] = {};
        }
      }
      setInputJSON(JSON.stringify(sample, null, 2));
    } else {
      setInputJSON('{}');
    }
    setOutput('');
  }, [tool]);

  if (!tool) return null;

  const handleRun = async () => {
    setLoading(true);
    setOutput('');
    try {
      let parsed = {};
      try {
        parsed = JSON.parse(inputJSON);
      } catch {
        setOutput(t('test.invalidJson'));
        return;
      }

      const res = await api.executeTool(tool.name, parsed);
      setOutput(JSON.stringify(res, null, 2));
    } catch (err) {
      setOutput(t('test.executionFailed', { error: getErrorMessage(err) }));
    } finally {
      setLoading(false);
    }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(output);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('test.title', { tool: tool.name })}
    >
      <div className="flex flex-col gap-4">
        <div>
          <span className="text-caption text-slate block mb-1">
            {tool.description}
          </span>
          <label className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
            {t('test.input')}
          </label>
          <textarea
            rows={5}
            value={inputJSON}
            onChange={(e) => setInputJSON(e.target.value)}
            className="w-full bg-canvas text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink"
          />
        </div>

        <Button
          variant="primary"
          onClick={handleRun}
          disabled={loading}
          icon={<Play className="w-4 h-4" />}
          className="w-full justify-center"
        >
          {loading ? t('test.executing') : t('test.run')}
        </Button>

        {output && (
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-caption uppercase tracking-wider text-slate font-semibold flex items-center gap-1.5">
                <Terminal className="w-3.5 h-3.5" /> {t('test.output')}
              </label>
              <button
                onClick={handleCopy}
                className="text-caption text-slate hover:text-deep-ink flex items-center gap-1"
              >
                {copied ? <Check className="w-3 h-3 text-emerald-600" /> : <Copy className="w-3 h-3" />}
                <span>{copied ? t('test.copied') : t('test.copy')}</span>
              </button>
            </div>
            <pre className="w-full bg-deep-ink text-white font-mono text-caption p-4 rounded-[16px] max-h-56 overflow-auto border border-onyx/10">
              {output}
            </pre>
          </div>
        )}
      </div>
    </Modal>
  );
}
