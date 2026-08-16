import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { api } from '@/lib/api';
import type { ToolInfo } from '@/lib/types';

export interface ToolTestModalProps {
  tool: ToolInfo | null;
  isOpen: boolean;
  onClose: () => void;
}

export function ToolTestModal({ tool, isOpen, onClose }: ToolTestModalProps) {
  const { t } = useTranslation('tools');
  const { t: tCommon } = useTranslation('common');

  const [inputJSON, setInputJSON] = useState('{}');
  const [output, setOutput] = useState('');
  const [loading, setLoading] = useState(false);

  if (!tool) return null;

  const handleRun = async () => {
    setLoading(true);
    setOutput('');
    try {
      let parsed = {};
      try {
        parsed = JSON.parse(inputJSON);
      } catch {
        setOutput('Invalid JSON input syntax');
        return;
      }

      const res = await api.executeTool(tool.name, parsed);
      setOutput(JSON.stringify(res, null, 2));
    } catch (err: any) {
      setOutput(`Error: ${err.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('testModal.title', { name: tool.name })}
    >
      <div className="flex flex-col gap-4">
        <div>
          <label className="text-caption uppercase tracking-wider text-slate font-medium block mb-1">
            {t('testModal.inputLabel')}
          </label>
          <textarea
            rows={4}
            value={inputJSON}
            onChange={(e) => setInputJSON(e.target.value)}
            className="w-full bg-white text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
            placeholder="{}"
          />
        </div>

        <div>
          <label className="text-caption uppercase tracking-wider text-slate font-medium block mb-1">
            {t('testModal.outputLabel')}
          </label>
          <div className="w-full bg-deep-ink text-white font-mono text-body-sm p-4 rounded-[16px] min-h-[100px] max-h-[250px] overflow-y-auto whitespace-pre-wrap">
            {loading ? t('testModal.running') : output || 'Press Run Tool to execute.'}
          </div>
        </div>

        <div className="flex items-center justify-end gap-3 pt-4 border-t border-soft-meadow">
          <Button variant="ghost" onClick={onClose}>
            {tCommon('buttons.close')}
          </Button>
          <Button variant="primary" onClick={handleRun} disabled={loading}>
            {loading ? '...' : tCommon('buttons.execute')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
