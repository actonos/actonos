import { useState, useEffect } from 'react';
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
        .catch((err) => error('Failed to load SOUL.md', err.message))
        .finally(() => setLoading(false));
    }
  }, [isOpen, agentID]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.saveSoul(soul, agentID);
      success('Agent Soul Updated', `SOUL.md synchronized for agent ${agentName || agentID}.`);
      onClose();
    } catch (err: any) {
      error('Failed to save SOUL.md', err.message);
    } finally {
      setSaving(false);
    }
  };

  const displayName = agentName || (agentID === 'agent_system_core' ? 'Nova (Root System)' : agentID);

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={`Agent Soul & Persona: ${displayName}`}>
      <div className="space-y-4">
        <p className="text-body-sm text-slate font-sans">
          The <strong>SOUL.md</strong> defines the isolated personality, core mission, emotional tone, and behavioral standards for <strong>{displayName}</strong>.
        </p>

        {loading ? (
          <div className="py-12 text-center text-slate font-sans">Loading SOUL.md...</div>
        ) : (
          <textarea
            rows={12}
            value={soul}
            onChange={(e) => setSoul(e.target.value)}
            placeholder="# ActonOS Agent Soul&#10;Define personality, core mission, and tone of voice..."
            className="w-full bg-canvas text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink"
          />
        )}

        <div className="flex items-center justify-end gap-2.5 pt-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            icon={<Save className="w-3.5 h-3.5" />}
            onClick={handleSave}
            disabled={saving || loading}
          >
            {saving ? 'Saving...' : 'Save SOUL.md'}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
