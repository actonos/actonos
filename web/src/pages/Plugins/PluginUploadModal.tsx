import { useState, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import { Upload, CheckCircle2, Shield, Package, FileCheck, X } from 'lucide-react';
import { getErrorMessage } from '@/lib/errors';

export interface PluginUploadModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

export function PluginUploadModal({
  isOpen,
  onClose,
  onSuccess,
}: PluginUploadModalProps) {
  const { t } = useTranslation('plugins');
  const { success, error } = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [isDragOver, setIsDragOver] = useState(false);
  const [isUploading, setIsUploading] = useState(false);

  const handleFileSelect = (file: File) => {
    if (!file.name.toLowerCase().endsWith('.actonpkg')) {
      error(t('modals.invalidFile', 'Invalid File'), t('modals.invalidFile', 'Please select an ActonOS Plugin package (.actonpkg)'));
      return;
    }
    setSelectedFile(file);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
    const file = e.dataTransfer.files?.[0];
    if (file) {
      handleFileSelect(file);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedFile) {
      error(t('modals.invalidFile', 'Missing File'), t('modals.invalidFile', 'Please select an ActonOS Plugin package (.actonpkg)'));
      return;
    }

    try {
      setIsUploading(true);
      const res = await api.uploadPlugin(selectedFile);
      const pluginName = ('plugin' in res && res.plugin?.manifest?.name) ? res.plugin.manifest.name : selectedFile.name;
      success(
        t('modals.installedSuccess', 'Plugin Installed'),
        t('modals.installedSuccessDesc', { name: pluginName, defaultValue: `Plugin ${pluginName} is now installed and active.` })
      );
      onSuccess();
      handleClose();
    } catch (err) {
      error('Installation Failed', getErrorMessage(err));
    } finally {
      setIsUploading(false);
    }
  };

  const handleClose = () => {
    setSelectedFile(null);
    setIsDragOver(false);
    setIsUploading(false);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={t('modals.uploadTitle', 'Install Plugin Package')}
      maxWidth="max-w-xl"
    >
      <form onSubmit={handleSubmit} className="space-y-5">
        {/* Dropzone Container */}
        <div>
          <label className="block text-body-sm font-semibold text-deep-ink mb-2">
            {t('modals.packageFileLabel', 'ActonOS Plugin Package (.actonpkg)')} <span className="text-red-500">*</span>
          </label>
          <div
            onDragOver={(e) => {
              e.preventDefault();
              setIsDragOver(true);
            }}
            onDragLeave={() => setIsDragOver(false)}
            onDrop={handleDrop}
            onClick={() => !selectedFile && fileInputRef.current?.click()}
            className={`relative flex flex-col items-center justify-center p-6 border-2 border-dashed rounded-[20px] transition-all ${
              isDragOver
                ? 'border-deep-ink bg-hi-yellow/10 dark:border-hi-yellow dark:bg-hi-yellow/5 cursor-pointer'
                : selectedFile
                ? 'border-emerald-500 bg-emerald-500/5 dark:border-emerald-400 dark:bg-emerald-500/10'
                : 'border-onyx/20 bg-soft-meadow/50 hover:border-onyx/40 dark:border-white/10 dark:bg-soft-meadow/30 cursor-pointer'
            }`}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".actonpkg"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) handleFileSelect(file);
              }}
              className="hidden"
            />

            {selectedFile ? (
              <div className="flex items-center justify-between w-full p-2">
                <div className="flex items-center gap-3.5">
                  <div className="w-11 h-11 rounded-2xl bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 flex items-center justify-center shrink-0">
                    <Package className="w-6 h-6" />
                  </div>
                  <div className="text-left">
                    <p className="font-semibold text-deep-ink text-body-sm flex items-center gap-2">
                      <span>{selectedFile.name}</span>
                      <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0" />
                    </p>
                    <p className="text-caption text-slate mt-0.5">
                      {(selectedFile.size / 1024).toFixed(1)} KB • {t('modals.packageBadge', 'ActonOS Plugin Bundle')}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    setSelectedFile(null);
                    if (fileInputRef.current) fileInputRef.current.value = '';
                  }}
                  className="p-1.5 rounded-full hover:bg-onyx/10 dark:hover:bg-white/10 text-slate hover:text-deep-ink transition-colors"
                  title="Remove file"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            ) : (
              <div className="text-center py-2">
                <div className="w-12 h-12 mx-auto mb-3 rounded-2xl bg-deep-ink/5 dark:bg-white/10 flex items-center justify-center text-deep-ink">
                  <Upload className="w-6 h-6" />
                </div>
                <p className="text-body-sm font-semibold text-deep-ink mb-1">
                  {t('modals.packageDropzone', 'Drag & drop your .actonpkg file here, or click to browse')}
                </p>
                <p className="text-caption text-slate max-w-sm mx-auto">
                  {t('modals.packageHint', "Packaged plugin bundle created with 'acton-plugin pack' containing manifest.json, plugin.wasm, and signatures.")}
                </p>
              </div>
            )}
          </div>
        </div>

        {/* Bundle Content Description */}
        <div className="p-4 rounded-2xl bg-soft-meadow/80 dark:bg-soft-meadow/40 border border-onyx/15 dark:border-white/10 space-y-2 shadow-xs">
          <div className="flex items-center gap-2 text-deep-ink text-caption font-semibold">
            <FileCheck className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />
            <span>Plugin Package Verification</span>
          </div>
          <p className="text-caption text-slate leading-relaxed">
            ActonOS will automatically unpack the bundle, parse <code className="px-1.5 py-0.5 rounded bg-white dark:bg-black/40 border border-onyx/15 dark:border-white/15 font-mono text-[11px] font-semibold text-deep-ink shadow-2xs">manifest.json</code>, compile <code className="px-1.5 py-0.5 rounded bg-white dark:bg-black/40 border border-onyx/15 dark:border-white/15 font-mono text-[11px] font-semibold text-deep-ink shadow-2xs">plugin.wasm</code> into the Wazero runtime, and register exported Tools, Channels, or Connectors.
          </p>
        </div>

        {/* Security Sandbox notice */}
        <div className="flex items-start gap-3 p-4 rounded-2xl bg-amber-500/10 border border-amber-500/30 shadow-2xs">
          <Shield className="w-4.5 h-4.5 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
          <p className="text-caption text-slate leading-relaxed">
            <strong className="text-deep-ink">{t('modals.sandboxNoticeTitle', 'Fail-Closed Sandbox')}: </strong>
            {t('modals.sandboxNotice', 'Outbound network domains, Hardware Vault secrets, and KV storage permissions declared in the package manifest will be strictly enforced by the kernel security gate.')}
          </p>
        </div>

        {/* Modal Actions */}
        <div className="flex items-center justify-end gap-3 pt-3 border-t border-onyx/10 dark:border-white/10">
          <Button variant="ghost" type="button" onClick={handleClose} disabled={isUploading}>
            {t('modals.cancel', 'Cancel')}
          </Button>
          <Button
            variant="primary"
            type="submit"
            disabled={!selectedFile || isUploading}
            icon={isUploading ? <Package className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
          >
            {isUploading ? t('modals.installing', 'Validating & Installing...') : t('modals.installBtn', 'Install & Activate')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
