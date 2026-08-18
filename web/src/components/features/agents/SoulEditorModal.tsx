import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { getErrorMessage } from '@/lib/errors';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Save } from 'lucide-react';
import { api } from '@/lib/api';
import { useToast } from '@/components/ui/Toast';

interface SoulEditorModalProps {
  isOpen: boolean;
  onClose: () => void;
  agentID?: string;
  agentName?: string;
}

export function SoulEditorModal({ isOpen, onClose, agentID = 'agent_system_core', agentName }: SoulEditorModalProps) {
  const { t } = useTranslation('agents');
  const { success, error } = useToast();
  const [soul, setSoul] = useState('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setLoading(true);
      api
        .getSoul(agentID)
        .then((res) => setSoul(res.soul || ''))
        .catch((err) => error(t('soulEditor.loadFailed'), getErrorMessage(err)))
        .finally(() => setLoading(false));
    }
  }, [isOpen, agentID]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.saveSoul(soul, agentID);
      success(t('soulEditor.saved'), t('soulEditor.savedDescription', { agent: agentName || agentID }));
      onClose();
    } catch (err) {
      error(t('soulEditor.saveFailed'), getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const displayName = agentName || (agentID === 'agent_system_core' ? t('soulEditor.rootName') : agentID);

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('soulEditor.title', { agent: displayName })}>
      <div className="space-y-4">
        <p className="text-body-sm text-slate font-sans">
          {t('soulEditor.descriptionPrefix')} <strong>{t('soulEditor.fileName')}</strong> {t('soulEditor.description', { agent: displayName })}
        </p>

        {loading ? (
          <div className="py-12 text-center text-slate font-sans">{t('soulEditor.loading')}</div>
        ) : (
          <textarea
            rows={12}
            value={soul}
            onChange={(e) => setSoul(e.target.value)}
            placeholder={t('soulEditor.placeholder')}
            className="w-full bg-canvas text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink"
          />
        )}

        <div className="flex items-center justify-end gap-2.5 pt-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            {t('soulEditor.cancel')}
          </Button>
          <Button
            variant="primary"
            size="sm"
            icon={<Save className="w-3.5 h-3.5" />}
            onClick={handleSave}
            disabled={saving || loading}
          >
            {saving ? t('soulEditor.saving') : t('soulEditor.save')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
