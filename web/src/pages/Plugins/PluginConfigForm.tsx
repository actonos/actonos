import { useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { PluginInfo, VaultSecretMeta, AgentManifest } from '@/lib/types';
import {
  Lock,
  Eye,
  EyeOff,
  Save,
  Plus,
  Trash2,
  RefreshCw,
  CheckCircle2,
  Bot,
  Sliders,
  Settings2,
} from 'lucide-react';
import { getErrorMessage } from '@/lib/errors';

export interface PluginConfigFormProps {
  plugin: PluginInfo;
  onSaved?: (updated: PluginInfo) => void;
  onCancel?: () => void;
}

interface ConfigSchemaProperty {
  type?: string;
  title?: string;
  description?: string;
  default?: unknown;
  enum?: (string | number)[];
  items?: {
    type?: string;
    properties?: Record<string, ConfigSchemaProperty>;
  };
  properties?: Record<string, ConfigSchemaProperty>;
  secret?: string;
  minimum?: number;
  maximum?: number;
  'x-secret'?: boolean;
  'x-ui-widget'?: string;
  'x-ui-group'?: string;
  'x-ui-placeholder'?: string;
  'x-order'?: number;
  [key: string]: unknown;
}

export function PluginConfigForm({ plugin, onSaved, onCancel }: PluginConfigFormProps) {
  const { t } = useTranslation('plugins');
  const { success, error } = useToast();

  const manifest = plugin.manifest;
  const configSchema = (manifest.config_schema || {}) as { properties?: Record<string, ConfigSchemaProperty> };
  const schemaProperties = (configSchema.properties || {}) as Record<string, ConfigSchemaProperty>;

  const [formData, setFormData] = useState<Record<string, unknown>>(() => {
    const initial: Record<string, unknown> = {};
    // Load defaults from config_schema
    Object.entries(schemaProperties).forEach(([key, prop]) => {
      if (prop.default !== undefined) {
        initial[key] = prop.default;
      }
    });
    // Overlay with existing manifest.config values
    if (manifest.config) {
      Object.entries(manifest.config).forEach(([key, val]) => {
        initial[key] = val;
      });
    }
    return initial;
  });

  const [showSecretMap, setShowSecretMap] = useState<Record<string, boolean>>({});
  const [vaultSecrets, setVaultSecrets] = useState<VaultSecretMeta[]>([]);
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [_loadingMeta, setLoadingMeta] = useState(true);
  const [isSaving, setIsSaving] = useState(false);

  // Fetch Vault secrets & existing Agents for dropdowns
  const fetchMetadata = useCallback(async () => {
    try {
      setLoadingMeta(true);
      const [vaultRes, agentsRes] = await Promise.allSettled([
        api.listVaultSecrets(),
        api.listAgents(),
      ]);

      if (vaultRes.status === 'fulfilled') {
        setVaultSecrets(vaultRes.value.secrets || []);
      }
      if (agentsRes.status === 'fulfilled') {
        setAgents(agentsRes.value.agents || []);
      }
    } catch {
      // Best-effort metadata fetching
    } finally {
      setLoadingMeta(false);
    }
  }, []);

  useEffect(() => {
    fetchMetadata();
  }, [fetchMetadata]);

  // Group properties strictly by x-ui-group
  const groupedProperties = useMemo(() => {
    const groups: Record<string, Array<{ key: string; prop: ConfigSchemaProperty }>> = {};
    const defaultGroup = 'General Settings';

    Object.entries(schemaProperties).forEach(([key, prop]) => {
      const groupName = (prop['x-ui-group'] as string) || defaultGroup;
      if (!groups[groupName]) {
        groups[groupName] = [];
      }
      groups[groupName].push({ key, prop });
    });

    // Sort items within each group by x-order
    Object.keys(groups).forEach((g) => {
      groups[g].sort((a, b) => ((a.prop['x-order'] as number) ?? 0) - ((b.prop['x-order'] as number) ?? 0));
    });

    return groups;
  }, [schemaProperties]);

  const handleFieldChange = (key: string, value: unknown) => {
    setFormData((prev) => ({
      ...prev,
      [key]: value,
    }));
  };

  const toggleShowSecret = (key: string) => {
    setShowSecretMap((prev) => ({
      ...prev,
      [key]: !prev[key],
    }));
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setIsSaving(true);

      const secretsToPersist: Record<string, string> = {};

      // 1. Check top-level properties marked with x-secret or password widget
      Object.entries(schemaProperties).forEach(([k, prop]) => {
        if ((prop['x-secret'] || prop['x-ui-widget'] === 'password') && formData[k] !== undefined && typeof formData[k] === 'string') {
          const val = (formData[k] as string).trim();
          if (val) {
            secretsToPersist[k] = val;
            if (k === 'bot_token' || k === 'discord_bot_token') {
              secretsToPersist['discord_bot_token'] = val;
              secretsToPersist['discord_bot_tokens.default'] = val;
            }
          }
        }
      });

      // 2. Check array items marked with x-secret (e.g. accounts[i].bot_token)
      Object.entries(schemaProperties).forEach(([k, prop]) => {
        if (prop.type === 'array' && Array.isArray(formData[k])) {
          const itemProps = (prop.items?.properties || {}) as Record<string, ConfigSchemaProperty>;
          (formData[k] as Array<Record<string, unknown>>).forEach((item, idx) => {
            Object.entries(itemProps).forEach(([subKey, subProp]) => {
              if ((subProp['x-secret'] || subProp['x-ui-widget'] === 'password') && item[subKey] && typeof item[subKey] === 'string') {
                const subVal = (item[subKey] as string).trim();
                if (subVal) {
                  const accountId = (item.account_id as string) || (idx === 0 ? 'default' : `item_${idx}`);
                  secretsToPersist[`${subKey}.${accountId}`] = subVal;
                  secretsToPersist[`discord_bot_tokens.${accountId}`] = subVal;
                  secretsToPersist[`discord_bot_token.${accountId}`] = subVal;
                  if (idx === 0 || accountId === 'default') {
                    secretsToPersist['discord_bot_token'] = subVal;
                    secretsToPersist['discord_bot_tokens.default'] = subVal;
                    secretsToPersist[subKey] = subVal;
                  }
                }
              }
            });
          });
        }
      });

      const res = await api.updatePluginConfig(manifest.id, formData, secretsToPersist);
      const updatedPlugin = 'plugin' in res ? (res.plugin as PluginInfo) : plugin;

      success(
        t('config.savedSuccess', 'Configuration Saved'),
        t('config.savedSuccessDesc', { name: manifest.name || manifest.id, defaultValue: `Configuration for ${manifest.name || manifest.id} was saved successfully.` })
      );

      await fetchMetadata();

      if (onSaved) {
        onSaved(updatedPlugin);
      }
    } catch (err) {
      error(t('config.saveFailed', 'Failed to Save Configuration'), getErrorMessage(err));
    } finally {
      setIsSaving(false);
    }
  };

  // Standard ActonOS input styles
  const inputStyle = "w-full bg-canvas text-deep-ink placeholder:text-slate px-4 py-2.5 rounded-full border border-onyx/15 focus:outline-none focus:ring-2 focus:ring-deep-ink text-body-sm font-sans transition-all";
  const selectStyle = "w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 focus:outline-none focus:ring-2 focus:ring-deep-ink text-body-sm font-sans transition-all cursor-pointer";
  const textareaStyle = "w-full bg-canvas text-deep-ink placeholder:text-slate p-3.5 rounded-2xl border border-onyx/15 focus:outline-none focus:ring-2 focus:ring-deep-ink text-body-sm font-sans transition-all resize-none";

  // Render Array of Objects (e.g. accounts list in Discord/Telegram bot)
  const renderArrayField = (key: string, prop: ConfigSchemaProperty) => {
    const items = (formData[key] || []) as Array<Record<string, unknown>>;
    const itemSchema = prop.items || {};
    const itemProps = (itemSchema.properties || {}) as Record<string, ConfigSchemaProperty>;

    const handleAddItem = () => {
      const newItem: Record<string, unknown> = {};
      Object.entries(itemProps).forEach(([subKey, subProp]) => {
        if (subProp.default !== undefined) {
          newItem[subKey] = subProp.default;
        } else if (subProp.type === 'boolean') {
          newItem[subKey] = false;
        } else {
          newItem[subKey] = '';
        }
      });
      handleFieldChange(key, [...items, newItem]);
    };

    const handleRemoveItem = (index: number) => {
      const updated = items.filter((_, idx) => idx !== index);
      handleFieldChange(key, updated);
    };

    const handleItemChange = (index: number, subKey: string, subVal: unknown) => {
      const updated = items.map((item, idx) => {
        if (idx === index) {
          return { ...item, [subKey]: subVal };
        }
        return item;
      });
      handleFieldChange(key, updated);
    };

    return (
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div>
            <label className="block text-body-sm font-semibold text-deep-ink">
              {prop.title || key}
            </label>
            {prop.description && (
              <p className="text-caption text-slate mt-0.5">{prop.description}</p>
            )}
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            icon={<Plus className="w-3.5 h-3.5" />}
            onClick={handleAddItem}
          >
            {t('config.addEntry', 'Add Entry')}
          </Button>
        </div>

        {items.length === 0 ? (
          <div className="p-4 rounded-2xl bg-canvas/60 border border-dashed border-onyx/15 text-center text-caption text-slate">
            {t('config.emptyArray', 'No entries configured yet. Click "Add Entry" to create one.')}
          </div>
        ) : (
          <div className="space-y-3">
            {items.map((item, idx) => (
              <div
                key={idx}
                className="p-4 rounded-2xl bg-canvas border border-onyx/10 shadow-2xs space-y-3 relative"
              >
                <div className="flex items-center justify-between pb-2 border-b border-onyx/10">
                  <span className="text-caption font-bold text-deep-ink flex items-center gap-2">
                    <span className="w-5 h-5 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-[10px] text-deep-ink">
                      {idx + 1}
                    </span>
                    {String(item.display_name || item.account_id || `Item #${idx + 1}`)}
                  </span>
                  <button
                    type="button"
                    onClick={() => handleRemoveItem(idx)}
                    className="p-1.5 rounded-full text-slate hover:text-status-danger hover:bg-status-danger-soft transition-colors cursor-pointer"
                    title={t('config.removeItem', 'Remove')}
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
                  {Object.entries(itemProps).map(([subKey, subProp]) => {
                    const subVal = item[subKey] !== undefined ? item[subKey] : (subProp.default || '');
                    const isSecret = subProp['x-secret'] || subProp['x-ui-widget'] === 'password';
                    const isAgentSelector = subProp['x-ui-widget'] === 'agent-selector';
                    const isBool = subProp.type === 'boolean';
                    const showSecret = !!showSecretMap[`${key}_${idx}_${subKey}`];

                    if (isBool) {
                      return (
                        <div key={subKey} className="flex items-center gap-2 sm:col-span-2 mt-1">
                          <input
                            type="checkbox"
                            id={`${key}_${idx}_${subKey}`}
                            checked={!!subVal}
                            onChange={(e) => handleItemChange(idx, subKey, e.target.checked)}
                            className="w-4 h-4 rounded text-deep-ink focus:ring-deep-ink"
                          />
                          <label
                            htmlFor={`${key}_${idx}_${subKey}`}
                            className="text-body-sm font-medium text-deep-ink cursor-pointer"
                          >
                            {subProp.title || subKey}
                          </label>
                        </div>
                      );
                    }

                    if (isAgentSelector) {
                      return (
                        <div key={subKey}>
                          <label className="block text-[11px] font-semibold uppercase tracking-wider text-slate mb-1">
                            {subProp.title || subKey}
                          </label>
                          <select
                            value={(subVal as string) || ''}
                            onChange={(e) => handleItemChange(idx, subKey, e.target.value)}
                            className="w-full bg-soft-meadow text-deep-ink px-3.5 py-2 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink cursor-pointer"
                          >
                            <option value="">-- {t('config.selectAgent', 'Select Agent')} --</option>
                            {agents.map((agent) => (
                              <option key={agent.agent_id} value={agent.agent_id}>
                                {agent.name} ({agent.agent_id})
                              </option>
                            ))}
                          </select>
                        </div>
                      );
                    }

                    return (
                      <div key={subKey}>
                        <div className="flex items-center justify-between mb-1">
                          <label className="block text-[11px] font-semibold uppercase tracking-wider text-slate">
                            {subProp.title || subKey}
                          </label>
                          {isSecret && (
                            <span className="text-[10px] text-slate font-mono flex items-center gap-1">
                              <Lock className="w-2.5 h-2.5" /> Vault
                            </span>
                          )}
                        </div>
                        <div className="relative">
                          <input
                            type={isSecret && !showSecret ? 'password' : 'text'}
                            placeholder={(subProp['x-ui-placeholder'] as string) || ''}
                            value={(subVal as string | number) || ''}
                            onChange={(e) => handleItemChange(idx, subKey, e.target.value)}
                            className={`w-full bg-soft-meadow text-deep-ink placeholder:text-slate px-3.5 py-2 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink ${
                              isSecret ? 'pr-9 font-mono' : ''
                            }`}
                          />
                          {isSecret && (
                            <button
                              type="button"
                              onClick={() =>
                                setShowSecretMap((prev) => ({
                                  ...prev,
                                  [`${key}_${idx}_${subKey}`]: !prev[`${key}_${idx}_${subKey}`],
                                }))
                              }
                              className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink cursor-pointer"
                            >
                              {showSecret ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                            </button>
                          )}
                        </div>
                        {subProp.description && (
                          <p className="mt-1 text-caption text-slate">{subProp.description}</p>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  };

  // Render Single Property Field
  const renderSingleField = (key: string, prop: ConfigSchemaProperty) => {
    const val = formData[key] !== undefined ? formData[key] : (prop.default || '');
    const isSecret = prop['x-secret'] || prop['x-ui-widget'] === 'password';
    const isAgentSelector = prop['x-ui-widget'] === 'agent-selector';
    const isTextarea = prop['x-ui-widget'] === 'textarea';
    const isBool = prop.type === 'boolean';
    const isNumber = prop.type === 'integer' || prop.type === 'number';
    const isEnum = Array.isArray(prop.enum);
    const showSecret = !!showSecretMap[key];
    const isVaultConfigured = vaultSecrets.some((s) => s.name === key);

    if (prop.type === 'array') {
      return renderArrayField(key, prop);
    }

    if (isBool) {
      return (
        <div key={key} className="flex items-start gap-3 p-3.5 rounded-2xl bg-canvas border border-onyx/10">
          <input
            type="checkbox"
            id={`field_${key}`}
            checked={!!val}
            onChange={(e) => handleFieldChange(key, e.target.checked)}
            className="w-4 h-4 mt-0.5 rounded text-deep-ink focus:ring-deep-ink"
          />
          <div>
            <label
              htmlFor={`field_${key}`}
              className="text-body-sm font-semibold text-deep-ink cursor-pointer"
            >
              {prop.title || key}
            </label>
            {prop.description && (
              <p className="text-caption text-slate mt-0.5">{prop.description}</p>
            )}
          </div>
        </div>
      );
    }

    if (isEnum && prop.enum) {
      return (
        <div key={key}>
          <label className="block text-body-sm font-semibold text-deep-ink mb-1.5">
            {prop.title || key}
          </label>
          <select
            value={(val as string | number) || ''}
            onChange={(e) => handleFieldChange(key, e.target.value)}
            className={selectStyle}
          >
            {prop.enum.map((optionVal: string | number) => (
              <option key={String(optionVal)} value={optionVal}>
                {String(optionVal)}
              </option>
            ))}
          </select>
          {prop.description && <p className="mt-1 text-caption text-slate">{prop.description}</p>}
        </div>
      );
    }

    if (isAgentSelector) {
      return (
        <div key={key}>
          <label className="block text-body-sm font-semibold text-deep-ink mb-1.5 flex items-center gap-1.5">
            <Bot className="w-4 h-4 text-slate" />
            <span>{prop.title || key}</span>
          </label>
          <select
            value={(val as string) || ''}
            onChange={(e) => handleFieldChange(key, e.target.value)}
            className={selectStyle}
          >
            <option value="">-- {t('config.selectAgent', 'Select Agent')} --</option>
            {agents.map((agent) => (
              <option key={agent.agent_id} value={agent.agent_id}>
                {agent.name} ({agent.agent_id})
              </option>
            ))}
          </select>
          {prop.description && <p className="mt-1 text-caption text-slate">{prop.description}</p>}
        </div>
      );
    }

    if (isTextarea) {
      return (
        <div key={key}>
          <label className="block text-body-sm font-semibold text-deep-ink mb-1.5">
            {prop.title || key}
          </label>
          <textarea
            rows={3}
            placeholder={(prop['x-ui-placeholder'] as string) || ''}
            value={(val as string) || ''}
            onChange={(e) => handleFieldChange(key, e.target.value)}
            className={textareaStyle}
          />
          {prop.description && <p className="mt-1 text-caption text-slate">{prop.description}</p>}
        </div>
      );
    }

    return (
      <div key={key}>
        <div className="flex items-center justify-between mb-1.5">
          <label className="block text-body-sm font-semibold text-deep-ink">
            {prop.title || key}
          </label>
          {isSecret && (
            <span className="inline-flex items-center gap-1 text-[11px] font-mono text-slate bg-canvas px-2.5 py-0.5 rounded-full border border-onyx/10 shadow-2xs">
              <Lock className="w-3 h-3 text-slate" /> Vault
              {isVaultConfigured && <CheckCircle2 className="w-3 h-3 text-status-success" />}
            </span>
          )}
        </div>
        <div className="relative">
          <input
            type={isSecret && !showSecret ? 'password' : isNumber ? 'number' : 'text'}
            min={prop.minimum}
            max={prop.maximum}
            placeholder={(prop['x-ui-placeholder'] as string) || (isVaultConfigured ? '•••••••• (Configured in Vault)' : '')}
            value={(val as string | number) || ''}
            onChange={(e) => handleFieldChange(key, isNumber ? Number(e.target.value) : e.target.value)}
            className={`${inputStyle} ${isSecret || isNumber ? 'font-mono' : 'font-sans'} ${
              isSecret ? 'pr-10' : ''
            }`}
          />
          {isSecret && (
            <button
              type="button"
              onClick={() => toggleShowSecret(key)}
              className="absolute right-3.5 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink cursor-pointer"
            >
              {showSecret ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
          )}
        </div>
        {prop.description && <p className="mt-1 text-caption text-slate">{prop.description}</p>}
      </div>
    );
  };

  return (
    <form onSubmit={handleSave} className="space-y-6">
      {/* Schema-defined Configuration Groups strictly from config_schema */}
      {Object.keys(schemaProperties).length === 0 ? (
        <div className="p-8 rounded-2xl bg-canvas/60 border border-dashed border-onyx/15 text-center">
          <Sliders className="w-8 h-8 text-slate mx-auto mb-2 opacity-50" />
          <h4 className="font-serif text-subheading font-bold text-deep-ink mb-1">
            {t('config.noConfigNeeded', 'No configuration required for this plugin.')}
          </h4>
          <p className="text-caption text-slate max-w-sm mx-auto">
            {t(
              'config.noConfigNeededDesc',
              'This plugin operates statelessly without external credentials or parameters.'
            )}
          </p>
        </div>
      ) : (
        <div className="space-y-6">
          {Object.entries(groupedProperties).map(([groupName, fields]) => (
            <div key={groupName} className="p-5 rounded-[22px] bg-soft-meadow/70 border border-onyx/10 space-y-4">
              <div className="flex items-center gap-2 pb-2 border-b border-onyx/10">
                <Settings2 className="w-4 h-4 text-deep-ink" />
                <h4 className="font-serif text-body-sm font-bold text-deep-ink">{groupName}</h4>
              </div>

              <div className="space-y-4">
                {fields.map(({ key, prop }) => renderSingleField(key, prop))}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Form Action Buttons */}
      <div className="flex items-center justify-end gap-3 pt-4 border-t border-onyx/10">
        {onCancel && (
          <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
            {t('actions.cancel', 'Cancel')}
          </Button>
        )}
        <Button
          type="submit"
          variant="primary"
          size="sm"
          disabled={isSaving}
          icon={isSaving ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
        >
          {isSaving ? t('config.saving', 'Saving Settings...') : t('config.saveBtn', 'Save Plugin Configuration')}
        </Button>
      </div>
    </form>
  );
}
