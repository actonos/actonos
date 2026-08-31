import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import { getErrorMessage } from '@/lib/errors';
import { api } from '@/lib/api';
import type { BackupManifest } from '@/lib/types';
import {
  Download,
  Upload,
  RotateCcw,
  AlertTriangle,
  HardDrive,
  FileArchive,
  RefreshCw,
  Trash2,
  ShieldAlert,
  Copy,
  Check,
} from 'lucide-react';

export function BackupRestoreSection() {
  const { t } = useTranslation('settings');
  const { success, error, info } = useToast();

  const [backups, setBackups] = useState<BackupManifest[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [restoring, setRestoring] = useState(false);
  const [includeWorkspace, setIncludeWorkspace] = useState(false);
  const [backupNotes, setBackupNotes] = useState('');

  // Restore Modal State
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [selectedRestoreSnapshot, setSelectedRestoreSnapshot] = useState<BackupManifest | null>(null);
  const [isRestoreModalOpen, setIsRestoreModalOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Delete Snapshot State
  const [deleteSnapshotConfirm, setDeleteSnapshotConfirm] = useState<BackupManifest | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Factory Reset State
  const [isFactoryResetModalOpen, setIsFactoryResetModalOpen] = useState(false);
  const [confirmResetText, setConfirmResetText] = useState('');
  const [resetting, setResetting] = useState(false);

  const [copiedHash, setCopiedHash] = useState<string | null>(null);

  const fetchBackups = async () => {
    setLoading(true);
    try {
      const res = await api.listBackups();
      setBackups(res.backups || []);
    } catch {
      // Fallback
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchBackups();
  }, []);

  const handleCreateBackup = async (downloadDirectly: boolean) => {
    setCreating(true);
    try {
      const res = await api.createBackup({
        include_workspace: includeWorkspace,
        notes: backupNotes,
        download: downloadDirectly,
      });

      if (downloadDirectly && res.manifest) {
        const link = document.createElement('a');
        link.href = api.getBackupDownloadUrl(res.manifest.id);
        link.download = res.manifest.file_name || `actonos-backup-${res.manifest.id}.actonbak`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
      }

      success(
        t('backup.createdTitle', 'Backup Created'),
        t('backup.createdDesc', 'Snapshot generated and verified with SHA-256.')
      );
      setBackupNotes('');
      fetchBackups();
    } catch (err) {
      error(t('backup.createFailed', 'Backup Creation Failed'), getErrorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setSelectedRestoreSnapshot(null);
      setRestoreFile(file);
      setIsRestoreModalOpen(true);
    }
    if (fileInputRef.current) fileInputRef.current.value = '';
  };

  const handleExecuteRestore = async () => {
    if (!restoreFile && !selectedRestoreSnapshot) return;
    setRestoring(true);
    try {
      const target = selectedRestoreSnapshot
        ? { backup_id: selectedRestoreSnapshot.id }
        : restoreFile!;
      const res = await api.restoreBackup(target);
      success(
        t('backup.restoredTitle', 'Database Restored'),
        res.message || t('backup.restoredDesc', 'Database successfully restored. Reloading system.')
      );
      setIsRestoreModalOpen(false);
      setRestoreFile(null);
      setSelectedRestoreSnapshot(null);
      setTimeout(() => window.location.reload(), 1500);
    } catch (err) {
      error(t('backup.restoreFailed', 'Restore Failed'), getErrorMessage(err));
    } finally {
      setRestoring(false);
    }
  };

  const handleDeleteSnapshot = async () => {
    if (!deleteSnapshotConfirm) return;
    setDeleting(true);
    try {
      await api.deleteBackup(deleteSnapshotConfirm.id);
      success(
        t('backup.deleteSnapshotBtn', 'Deleted'),
        t('backup.snapshotDeleted', 'Backup snapshot removed successfully.')
      );
      setDeleteSnapshotConfirm(null);
      fetchBackups();
    } catch (err) {
      error('Delete Failed', getErrorMessage(err));
    } finally {
      setDeleting(false);
    }
  };

  const handleFactoryReset = async () => {
    if (confirmResetText !== 'RESET-ACTONOS') {
      error('Invalid Token', 'You must type RESET-ACTONOS exactly to confirm.');
      return;
    }
    setResetting(true);
    try {
      await api.factoryReset(confirmResetText);
      info('Factory Reset Complete', 'All operational state reset. Reloading...');
      setIsFactoryResetModalOpen(false);
      setTimeout(() => window.location.reload(), 1500);
    } catch (err) {
      error('Factory Reset Failed', getErrorMessage(err));
    } finally {
      setResetting(false);
    }
  };

  const handleCopyHash = (hash: string) => {
    navigator.clipboard.writeText(hash);
    setCopiedHash(hash);
    setTimeout(() => setCopiedHash(null), 2000);
  };

  return (
    <div className="space-y-6">
      {/* Hidden File Input for Restore */}
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileSelect}
        accept=".actonbak,.tar.gz,.db,.sqlite"
        className="hidden"
      />

      {/* 1. Create Backup Card */}
      <Card className="p-5 border border-onyx/10 bg-canvas/90 space-y-4 shadow-xs">
        <div className="flex items-center justify-between border-b border-onyx/10 pb-3">
          <div>
            <h3 className="font-serif text-body font-bold text-deep-ink">
              {t('backup.createCardTitle', 'Create System Backup')}
            </h3>
            <p className="text-caption text-slate">
              {t('backup.createCardDesc', 'Generates a transactional SQLite snapshot with SHA-256 checksum verification.')}
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="text-caption font-semibold text-deep-ink block mb-1.5">
              {t('backup.notesLabel', 'Backup Notes / Description')}
            </label>
            <Input
              placeholder={t('backup.notesPlaceholder', 'e.g. Pre-upgrade safety snapshot')}
              value={backupNotes}
              onChange={(e) => setBackupNotes(e.target.value)}
              className="text-body-sm"
            />
          </div>

          <div className="flex items-center space-x-2 pt-6">
            <input
              type="checkbox"
              id="includeWorkspace"
              checked={includeWorkspace}
              onChange={(e) => setIncludeWorkspace(e.target.checked)}
              className="rounded border-onyx/20 text-deep-ink focus:ring-deep-ink cursor-pointer w-4 h-4"
            />
            <label htmlFor="includeWorkspace" className="text-body-sm text-deep-ink font-medium cursor-pointer">
              {t('backup.includeWorkspaceLabel', 'Include Workspace Files in Archive')}
            </label>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-end gap-3 pt-3 border-t border-onyx/5">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => fileInputRef.current?.click()}
            icon={<Upload className="w-3.5 h-3.5" />}
          >
            {t('backup.uploadRestoreBtn', 'Upload & Restore File')}
          </Button>

          <Button
            variant="secondary"
            size="sm"
            onClick={() => handleCreateBackup(false)}
            disabled={creating}
            icon={<HardDrive className="w-3.5 h-3.5" />}
          >
            {creating ? t('backup.creating', 'Creating...') : t('backup.saveLocalBtn', 'Save Local Snapshot')}
          </Button>

          <Button
            variant="primary"
            size="sm"
            onClick={() => handleCreateBackup(true)}
            disabled={creating}
            icon={<Download className="w-3.5 h-3.5" />}
          >
            {creating ? t('backup.creating', 'Creating...') : t('backup.downloadBtn', 'Download .actonbak Archive')}
          </Button>
        </div>
      </Card>

      {/* 2. Local Snapshots & History Table */}
      <Card className="p-5 border border-onyx/10 bg-canvas/90 space-y-4 shadow-xs">
        <div className="flex items-center justify-between border-b border-onyx/10 pb-3">
          <div className="flex items-center gap-2">
            <FileArchive className="w-4 h-4 text-deep-ink" />
            <h3 className="font-serif text-body font-bold text-deep-ink">
              {t('backup.historyTitle', 'Local Backup Snapshots')}
            </h3>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={fetchBackups}
            disabled={loading}
            icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
          >
            {t('common.refresh', 'Refresh')}
          </Button>
        </div>

        {backups.length === 0 ? (
          <div className="py-8 text-center text-slate text-body-sm font-medium">
            {t('backup.noBackups', 'No local backup snapshots found. Create one above.')}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-onyx/10 text-caption font-semibold text-slate">
                  <th className="py-2.5 px-3">{t('backup.cols.date', 'Timestamp')}</th>
                  <th className="py-2.5 px-3">{t('backup.cols.size', 'Size')}</th>
                  <th className="py-2.5 px-3">{t('backup.cols.records', 'Contents')}</th>
                  <th className="py-2.5 px-3">{t('backup.cols.hash', 'SHA-256 Checksum')}</th>
                  <th className="py-2.5 px-3">{t('backup.cols.notes', 'Notes')}</th>
                  <th className="py-2.5 px-3 text-right">{t('common.actions', 'Actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-onyx/5 text-body-sm">
                {backups.map((bak) => (
                  <tr key={bak.id} className="hover:bg-soft-meadow/40 transition-colors">
                    <td className="py-3 px-3 font-mono text-caption text-deep-ink whitespace-nowrap font-medium">
                      {new Date(bak.created_at).toLocaleString()}
                    </td>
                    <td className="py-3 px-3 font-mono text-caption text-slate">
                      {((bak.archive_size_bytes || bak.database_size_bytes) / 1024 / 1024).toFixed(2)} MB
                    </td>
                    <td className="py-3 px-3">
                      <div className="flex items-center gap-2 text-caption text-slate font-mono">
                        <Badge variant="neutral" className="text-[10px]">
                          {bak.agents_count || 0} agents
                        </Badge>
                        <Badge variant="neutral" className="text-[10px]">
                          {bak.tasks_count || 0} tasks
                        </Badge>
                        {bak.include_workspace && (
                          <Badge variant="neutral" className="text-[10px]">
                            +workspace
                          </Badge>
                        )}
                      </div>
                    </td>
                    <td className="py-3 px-3 font-mono text-caption text-slate">
                      <div className="flex items-center gap-1.5">
                        <span className="max-w-[100px] truncate" title={bak.checksum_sha256}>
                          {bak.checksum_sha256 ? bak.checksum_sha256.slice(0, 12) + '...' : '-'}
                        </span>
                        {bak.checksum_sha256 && (
                          <button
                            type="button"
                            onClick={() => handleCopyHash(bak.checksum_sha256)}
                            className="p-1 hover:bg-canvas rounded text-slate hover:text-deep-ink transition-colors cursor-pointer"
                            title="Copy full SHA-256"
                          >
                            {copiedHash === bak.checksum_sha256 ? (
                              <Check className="w-3 h-3 text-green-600" />
                            ) : (
                              <Copy className="w-3 h-3" />
                            )}
                          </button>
                        )}
                      </div>
                    </td>
                    <td className="py-3 px-3 text-caption text-slate italic max-w-xs truncate">
                      {bak.notes || '-'}
                    </td>
                    <td className="py-3 px-3 text-right whitespace-nowrap">
                      <div className="flex items-center justify-end gap-1.5">
                        <Button
                          variant="secondary"
                          size="sm"
                          icon={<RotateCcw className="w-3 h-3" />}
                          onClick={() => {
                            setRestoreFile(null);
                            setSelectedRestoreSnapshot(bak);
                            setIsRestoreModalOpen(true);
                          }}
                          title={t('backup.restoreSnapshotBtn', 'Restore')}
                          className="h-7 px-2.5 text-xs font-medium"
                        >
                          {t('backup.restoreSnapshotBtn', 'Restore')}
                        </Button>

                        <Button
                          variant="ghost"
                          size="sm"
                          icon={<Download className="w-3.5 h-3.5" />}
                          onClick={() => {
                            const link = document.createElement('a');
                            link.href = api.getBackupDownloadUrl(bak.id);
                            link.download = bak.file_name || `actonos-backup-${bak.id}.actonbak`;
                            document.body.appendChild(link);
                            link.click();
                            document.body.removeChild(link);
                          }}
                          title={t('backup.downloadBtn', 'Download')}
                          className="h-7 px-2 text-xs"
                        />

                        <Button
                          variant="ghost"
                          size="sm"
                          icon={<Trash2 className="w-3.5 h-3.5 text-red-600" />}
                          onClick={() => setDeleteSnapshotConfirm(bak)}
                          title={t('backup.deleteSnapshotBtn', 'Delete')}
                          className="h-7 px-2 text-xs hover:bg-red-500/10 text-red-600"
                        />
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* 3. Danger Zone: Factory Reset */}
      <Card className="p-5 border border-accent-coral/20 bg-accent-coral/5 space-y-4 shadow-xs">
        <div className="flex items-center justify-between">
          <div className="flex items-start gap-3.5">
            <div className="p-2 rounded-xl bg-accent-coral/10 text-accent-coral mt-0.5">
              <AlertTriangle className="w-5 h-5" />
            </div>
            <div>
              <h4 className="font-serif text-body-sm font-bold text-deep-ink">
                {t('backup.dangerZoneTitle', 'Danger Zone: Factory Reset')}
              </h4>
              <p className="text-caption text-slate max-w-xl">
                {t('backup.dangerZoneDesc', 'Wipes task queues, execution events, notifications, and reset memory tables. An automated safety backup is taken prior to reset.')}
              </p>
            </div>
          </div>

          <Button
            variant="danger"
            size="sm"
            onClick={() => {
              setConfirmResetText('');
              setIsFactoryResetModalOpen(true);
            }}
            icon={<Trash2 className="w-3.5 h-3.5" />}
          >
            {t('backup.factoryResetBtn', 'Factory Reset...')}
          </Button>
        </div>
      </Card>

      {/* Restore Preview & Confirmation Modal */}
      <Modal
        isOpen={isRestoreModalOpen}
        onClose={() => {
          setIsRestoreModalOpen(false);
          setRestoreFile(null);
          setSelectedRestoreSnapshot(null);
        }}
        title={t('backup.confirmRestoreTitle', 'Confirm System Restoration')}
        maxWidth="max-w-md"
      >
        <div className="space-y-4">
          <div className="p-4 rounded-xl bg-amber-500/10 border border-amber-500/20 text-deep-ink flex items-start gap-3">
            <ShieldAlert className="w-5 h-5 text-amber-600 shrink-0 mt-0.5" />
            <div className="text-body-sm space-y-1">
              <strong className="font-semibold block">Pre-restore Safety Active</strong>
              <p className="text-caption text-slate">
                ActonOS will take a safety snapshot of the existing database before applying the restore.
                The system will automatically reload once restored.
              </p>
            </div>
          </div>

          {selectedRestoreSnapshot && (
            <div className="p-3.5 rounded-xl bg-canvas border border-onyx/10 text-caption font-mono text-slate space-y-1.5">
              <div className="text-xs font-bold text-deep-ink font-sans pb-1 border-b border-onyx/5">
                {t('backup.snapshotDetails', 'Snapshot Details')}
              </div>
              <div className="flex justify-between">
                <span>Created:</span>
                <strong className="text-deep-ink">{new Date(selectedRestoreSnapshot.created_at).toLocaleString()}</strong>
              </div>
              <div className="flex justify-between">
                <span>Size:</span>
                <strong className="text-deep-ink">
                  {((selectedRestoreSnapshot.archive_size_bytes || selectedRestoreSnapshot.database_size_bytes) / 1024 / 1024).toFixed(2)} MB
                </strong>
              </div>
              <div className="flex justify-between">
                <span>Contents:</span>
                <strong className="text-deep-ink">
                  {selectedRestoreSnapshot.agents_count || 0} agents, {selectedRestoreSnapshot.tasks_count || 0} tasks
                </strong>
              </div>
              {selectedRestoreSnapshot.notes && (
                <div className="pt-1 text-slate italic">
                  "{selectedRestoreSnapshot.notes}"
                </div>
              )}
            </div>
          )}

          {restoreFile && (
            <div className="p-3.5 rounded-xl bg-canvas border border-onyx/10 text-caption font-mono text-slate space-y-1">
              <div>File: <strong className="text-deep-ink">{restoreFile.name}</strong></div>
              <div>Size: <strong className="text-deep-ink">{(restoreFile.size / 1024 / 1024).toFixed(2)} MB</strong></div>
            </div>
          )}

          <div className="flex items-center justify-end gap-3 pt-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setIsRestoreModalOpen(false);
                setRestoreFile(null);
                setSelectedRestoreSnapshot(null);
              }}
            >
              {t('common.cancel', 'Cancel')}
            </Button>

            <Button
              variant="danger"
              size="sm"
              onClick={handleExecuteRestore}
              disabled={restoring}
              icon={<RotateCcw className={`w-3.5 h-3.5 ${restoring ? 'animate-spin' : ''}`} />}
            >
              {restoring ? t('backup.restoring', 'Restoring...') : t('backup.confirmRestoreBtn', 'Confirm & Restore')}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Delete Snapshot Confirmation Modal */}
      <ConfirmModal
        isOpen={Boolean(deleteSnapshotConfirm)}
        onClose={() => setDeleteSnapshotConfirm(null)}
        onConfirm={handleDeleteSnapshot}
        title={t('backup.confirmDeleteTitle', 'Delete Backup Snapshot')}
        description={t(
          'backup.confirmDeleteDesc',
          'Are you sure you want to permanently remove this backup snapshot from local storage?'
        )}
        confirmLabel={t('backup.deleteSnapshotBtn', 'Delete')}
        variant="danger"
        loading={deleting}
      />

      {/* Factory Reset Modal */}
      <Modal
        isOpen={isFactoryResetModalOpen}
        onClose={() => setIsFactoryResetModalOpen(false)}
        title={t('backup.factoryResetModalTitle', 'Confirm Factory Reset')}
        maxWidth="max-w-md"
      >
        <div className="space-y-4">
          <p className="text-body-sm text-slate">
            To prevent accidental data loss, please type <strong className="text-accent-coral font-mono">RESET-ACTONOS</strong> below:
          </p>

          <Input
            value={confirmResetText}
            onChange={(e) => setConfirmResetText(e.target.value)}
            placeholder="RESET-ACTONOS"
            className="font-mono text-body-sm font-bold tracking-widest text-center"
          />

          <div className="flex items-center justify-end gap-3 pt-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setIsFactoryResetModalOpen(false)}
            >
              {t('common.cancel', 'Cancel')}
            </Button>

            <Button
              variant="danger"
              size="sm"
              onClick={handleFactoryReset}
              disabled={confirmResetText !== 'RESET-ACTONOS' || resetting}
              icon={<Trash2 className="w-3.5 h-3.5" />}
            >
              {resetting ? 'Resetting...' : t('backup.executeResetBtn', 'Execute Factory Reset')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
