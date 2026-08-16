import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';

export interface McpServerModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConnect: (cfg: { id: string; command: string; args?: string[] }) => Promise<void>;
}

export function McpServerModal({ isOpen, onClose, onConnect }: McpServerModalProps) {
  const { t } = useTranslation('tools');
  const { t: tCommon } = useTranslation('common');

  const [id, setId] = useState('');
  const [command, setCommand] = useState('npx');
  const [argsStr, setArgsStr] = useState('-y @modelcontextprotocol/server-github');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      const args = argsStr.trim() ? argsStr.split(' ') : [];
      await onConnect({ id, command, args });
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to connect MCP server');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('mcpModal.title')}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        {error && (
          <div className="p-3 bg-red-100 text-red-700 rounded-full text-body-sm px-5">
            {error}
          </div>
        )}

        <Input
          label={t('mcpModal.idLabel')}
          placeholder={t('mcpModal.idPlaceholder')}
          value={id}
          onChange={(e) => setId(e.target.value)}
          required
        />

        <Input
          label={t('mcpModal.commandLabel')}
          placeholder={t('mcpModal.commandPlaceholder')}
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          required
        />

        <Input
          label={t('mcpModal.argsLabel')}
          placeholder={t('mcpModal.argsPlaceholder')}
          value={argsStr}
          onChange={(e) => setArgsStr(e.target.value)}
        />

        <div className="flex items-center justify-end gap-3 pt-4 border-t border-soft-meadow">
          <Button type="button" variant="ghost" onClick={onClose}>
            {tCommon('buttons.cancel')}
          </Button>
          <Button type="submit" variant="primary" disabled={loading}>
            {loading ? '...' : tCommon('buttons.connect')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
