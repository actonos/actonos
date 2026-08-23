import { useState, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import { Upload, CheckCircle2, AlertCircle, Sparkles, FileCode, Shield, Bot, Radio, Network } from 'lucide-react';
import { getErrorMessage } from '@/lib/errors';

export interface PluginUploadModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  initialManifest?: string;
  initialID?: string;
}

export function PluginUploadModal({
  isOpen,
  onClose,
  onSuccess,
  initialManifest,
  initialID,
}: PluginUploadModalProps) {
  const { t } = useTranslation('plugins');
  const { success, error } = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [pluginID, setPluginID] = useState(initialID || '');
  const [manifestJSON, setManifestJSON] = useState(initialManifest || '');
  const [isDragOver, setIsDragOver] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const [manifestError, setManifestError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      if (initialID) setPluginID(initialID);
      if (initialManifest) setManifestJSON(initialManifest);
    }
  }, [isOpen, initialID, initialManifest]);

  const handleFileSelect = (file: File) => {
    if (!file.name.endsWith('.wasm')) {
      error('Invalid File', 'Please select a compiled WebAssembly file (.wasm)');
      return;
    }
    setSelectedFile(file);
    if (!pluginID) {
      setPluginID(file.name.replace(/\.wasm$/, '').toLowerCase().replace(/[^a-z0-9_.-]/g, '_'));
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
    const file = e.dataTransfer.files?.[0];
    if (file) {
      handleFileSelect(file);
    }
  };

  const handleFillTemplate = (type: 'channel' | 'connector' | 'tool' | 'scraper') => {
    let template = {};
    if (type === 'channel') {
      template = {
        id: pluginID || 'telegram_bot_channel',
        name: 'Telegram Bot Gateway',
        version: '1.0.0',
        capabilities: ['channel'],
        permissions: {
          net_outbound: ['api.telegram.org'],
          secrets: ['telegram_bot_token'],
          bus_events: ['channel:message:inbound', 'channel:message:outbound'],
        },
        config: {
          channel_name: 'telegram',
        },
      };
    } else if (type === 'connector') {
      template = {
        id: pluginID || 'github_connector',
        name: 'GitHub SaaS Connector',
        version: '1.0.0',
        capabilities: ['connector', 'tool'],
        permissions: {
          net_outbound: ['api.github.com'],
          secrets: ['github_token'],
          storage: true,
        },
        tools: [
          {
            name: 'github_list_issues',
            description: 'List repository issues with state filtering',
            parameters: {
              type: 'object',
              properties: {
                repo: { type: 'string', description: 'owner/repository name' },
                state: { type: 'string', enum: ['open', 'closed', 'all'] },
              },
              required: ['repo'],
            },
          },
        ],
      };
    } else if (type === 'scraper') {
      template = {
        id: pluginID || 'web_scraper_tool',
        name: 'Web Content Scraper',
        version: '1.0.0',
        capabilities: ['tool'],
        permissions: {
          net_outbound: ['*'],
          storage: true,
        },
        tools: [
          {
            name: 'scrape_page',
            description: 'Fetch and clean HTML content from URL',
            parameters: {
              type: 'object',
              properties: {
                url: { type: 'string', description: 'Target URL to fetch' },
              },
              required: ['url'],
            },
          },
        ],
      };
    } else {
      template = {
        id: pluginID || 'custom_compute_tool',
        name: 'Sandboxed Compute Tool',
        version: '1.0.0',
        capabilities: ['tool'],
        permissions: {
          storage: true,
        },
        tools: [
          {
            name: 'compute_data',
            description: 'Execute high-speed sandboxed data calculation',
            parameters: {
              type: 'object',
              properties: {
                expression: { type: 'string', description: 'Mathematical or text expression' },
              },
              required: ['expression'],
            },
          },
        ],
      };
    }
    setManifestJSON(JSON.stringify(template, null, 2));
    setManifestError(null);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedFile) {
      error('Missing File', 'Please choose a .wasm file to install.');
      return;
    }

    if (manifestJSON.trim()) {
      try {
        JSON.parse(manifestJSON);
      } catch (err) {
        setManifestError(getErrorMessage(err));
        return;
      }
    }

    try {
      setIsUploading(true);
      await api.uploadPlugin(selectedFile, pluginID.trim() || undefined, manifestJSON.trim() || undefined);
      success('Plugin Installed', `WASM Plugin ${pluginID || selectedFile.name} is now active.`);
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
    setPluginID('');
    setManifestJSON('');
    setManifestError(null);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={t('modals.uploadTitle', 'Install WASM Plugin')}
      maxWidth="max-w-2xl"
    >
      <form onSubmit={handleSubmit} className="space-y-5">
        {/* Dropzone Container */}
        <div>
          <label className="block text-body-sm font-semibold text-deep-ink dark:text-cream mb-2">
            {t('modals.wasmFileLabel', 'WebAssembly Binary (.wasm)')} <span className="text-red-500">*</span>
          </label>
          <div
            onDragOver={(e) => {
              e.preventDefault();
              setIsDragOver(true);
            }}
            onDragLeave={() => setIsDragOver(false)}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
            className={`relative flex flex-col items-center justify-center p-6 border-2 border-dashed rounded-[20px] cursor-pointer transition-all ${
              isDragOver
                ? 'border-deep-ink bg-hi-yellow/10 dark:border-hi-yellow dark:bg-hi-yellow/5'
                : selectedFile
                ? 'border-emerald-500 bg-emerald-500/5 dark:border-emerald-400 dark:bg-emerald-500/10'
                : 'border-onyx/20 bg-soft-meadow/50 hover:border-onyx/40 dark:border-white/10 dark:bg-soft-meadow/30'
            }`}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".wasm"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) handleFileSelect(file);
              }}
              className="hidden"
            />

            {selectedFile ? (
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center">
                  <CheckCircle2 className="w-6 h-6" />
                </div>
                <div className="text-left">
                  <p className="font-semibold text-deep-ink dark:text-cream text-body-sm">{selectedFile.name}</p>
                  <p className="text-caption text-slate">{(selectedFile.size / 1024).toFixed(1)} KB • WebAssembly</p>
                </div>
              </div>
            ) : (
              <div className="text-center">
                <div className="w-12 h-12 mx-auto mb-3 rounded-full bg-deep-ink/5 dark:bg-white/10 flex items-center justify-center text-deep-ink dark:text-cream">
                  <Upload className="w-6 h-6" />
                </div>
                <p className="text-body-sm font-semibold text-deep-ink dark:text-cream mb-1">
                  {t('modals.wasmDropzone', 'Drag & drop your .wasm file here, or click to browse')}
                </p>
                <p className="text-caption text-slate">Supports compiled WASM modules from Rust, TinyGo, Zig, C, and AssemblyScript</p>
              </div>
            )}
          </div>
        </div>

        {/* Plugin ID input */}
        <div>
          <label className="block text-body-sm font-semibold text-deep-ink dark:text-cream mb-1">
            Plugin Identifier (Optional ID)
          </label>
          <input
            type="text"
            placeholder="e.g. telegram_channel, github_sync, math_calculator"
            value={pluginID}
            onChange={(e) => setPluginID(e.target.value)}
            className="w-full bg-soft-meadow dark:bg-canvas text-deep-ink dark:text-cream px-3.5 py-2.5 rounded-xl border border-onyx/15 dark:border-white/10 text-body-sm font-mono focus:outline-none focus:ring-2 focus:ring-deep-ink dark:focus:ring-hi-yellow transition-all"
          />
          <p className="mt-1 text-caption text-slate">Unique ID used in routing and SQLite storage isolation. Auto-inferred if omitted.</p>
        </div>

        {/* Template helper buttons */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="text-body-sm font-semibold text-deep-ink dark:text-cream flex items-center gap-1.5">
              <FileCode className="w-4 h-4 text-slate" />
              {t('modals.manifestLabel', 'Custom Manifest (Optional JSON)')}
            </label>
            <div className="flex items-center gap-1.5">
              <span className="text-caption text-slate font-medium mr-1">{t('modals.templates', 'Quick Templates')}:</span>
              <button
                type="button"
                onClick={() => handleFillTemplate('channel')}
                className="px-2 py-0.5 rounded-md bg-purple-500/10 hover:bg-purple-500/20 text-purple-700 dark:text-purple-300 text-caption font-semibold transition-all flex items-center gap-1 cursor-pointer"
              >
                <Radio className="w-3 h-3" /> Channel
              </button>
              <button
                type="button"
                onClick={() => handleFillTemplate('connector')}
                className="px-2 py-0.5 rounded-md bg-blue-500/10 hover:bg-blue-500/20 text-blue-700 dark:text-blue-300 text-caption font-semibold transition-all flex items-center gap-1 cursor-pointer"
              >
                <Network className="w-3 h-3" /> Connector
              </button>
              <button
                type="button"
                onClick={() => handleFillTemplate('tool')}
                className="px-2 py-0.5 rounded-md bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 text-caption font-semibold transition-all flex items-center gap-1 cursor-pointer"
              >
                <Bot className="w-3 h-3" /> Tool
              </button>
            </div>
          </div>

          <div className="relative">
            <textarea
              rows={8}
              placeholder={t('modals.manifestPlaceholder')}
              value={manifestJSON}
              onChange={(e) => {
                setManifestJSON(e.target.value);
                setManifestError(null);
              }}
              className="w-full bg-soft-meadow dark:bg-canvas text-deep-ink dark:text-cream p-3 rounded-xl border border-onyx/15 dark:border-white/10 text-caption font-mono focus:outline-none focus:ring-2 focus:ring-deep-ink dark:focus:ring-hi-yellow transition-all"
            />
          </div>

          {manifestError && (
            <div className="mt-2 flex items-center gap-2 text-caption text-red-600 dark:text-red-400">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>JSON syntax error: {manifestError}</span>
            </div>
          )}
        </div>

        {/* Security Sandbox notice */}
        <div className="flex items-start gap-3 p-3.5 rounded-2xl bg-amber-500/10 border border-amber-500/20">
          <Shield className="w-4 h-4 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
          <p className="text-caption text-slate leading-relaxed">
            <strong className="text-deep-ink dark:text-cream">Fail-Closed Sandbox:</strong> Outbound network requests and Vault secrets not explicitly declared in this manifest will be blocked by the kernel security gate.
          </p>
        </div>

        {/* Modal Actions */}
        <div className="flex items-center justify-end gap-3 pt-3 border-t border-onyx/10 dark:border-white/10">
          <Button variant="ghost" type="button" onClick={handleClose} disabled={isUploading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            type="submit"
            disabled={!selectedFile || isUploading}
            icon={isUploading ? <Sparkles className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
          >
            {isUploading ? t('modals.installing', 'Validating & Installing...') : t('modals.installBtn', 'Install & Activate')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
