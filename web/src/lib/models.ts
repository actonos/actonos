/**
 * ActonOS Dynamic Model & Provider Catalog
 *
 * Backend-driven single source of truth fetched from `/api/system/models`.
 * Frontend does not store hardcoded model catalogs; it fetches from the backend API once
 * on startup and caches in the global store.
 */

import { api } from '@/lib/api';
import type { LLMProviderInfo, ModelSpec, ProviderSpec } from '@/lib/types';
import { Sparkles, Zap, Cpu, Server, Bot, Layers, Wind, Box } from 'lucide-react';
import type React from 'react';

export interface ModelOption {
  id: string; // e.g. "anthropic/claude-sonnet-4-6"
  name: string; // e.g. "Claude Sonnet 4.6"
  providerId: string; // e.g. "anthropic"
  providerName: string; // e.g. "Anthropic Claude"
  badge?: string; // e.g. "Hybrid Reasoning", "Ultra Fast", "Frontier Coding"
  contextWindow?: string; // e.g. "256k", "512k", "1M+", "2M+"
  category: 'Cloud Frontier' | 'Reasoning' | 'Ultra Fast' | 'Aggregator' | 'Custom' | string;
  promptPer1M?: number;
  completionPer1M?: number;
  isDefault?: boolean;
  supportsTools?: boolean;
  supportsVision?: boolean;
}

export interface ProviderMeta {
  id: string;
  name: string;
  category: string;
  description: string;
  defaultBaseURL: string;
  modelPresets: { id: string; label: string }[];
  accentColor: string;
  icon: React.ElementType;
}

export const PROVIDER_ICONS: Record<string, React.ElementType> = {
  anthropic: Sparkles,
  openai: Bot,
  gemini: Sparkles,
  deepseek: Cpu,
  groq: Zap,
  openrouter: Layers,
  mistral: Wind,
  ollama: Server,
  custom_openai: Cpu,
};

export function getProviderIcon(providerId: string): React.ElementType {
  return PROVIDER_ICONS[providerId] || Box;
}

// In-Memory Global Store for cached catalog
let globalModels: ModelOption[] = [];
let globalProviders: ProviderMeta[] = [];
let catalogFetchPromise: Promise<{ models: ModelOption[]; providers: ProviderMeta[] }> | null = null;
const catalogListeners = new Set<() => void>();

export function mapBackendModelToOption(m: ModelSpec): ModelOption {
  return {
    id: m.id,
    name: m.name,
    providerId: m.provider_id,
    providerName: m.provider_name,
    badge: m.badge,
    contextWindow: m.context_window,
    category: m.category || 'Cloud Frontier',
    promptPer1M: m.prompt_per_1m,
    completionPer1M: m.completion_per_1m,
    isDefault: m.is_default,
    supportsTools: m.supports_tools,
    supportsVision: m.supports_vision,
  };
}

export function mapBackendProviderToMeta(p: ProviderSpec): ProviderMeta {
  return {
    id: p.id,
    name: p.name,
    category: p.category,
    description: p.description,
    defaultBaseURL: p.default_base_url,
    accentColor: p.accent_color || '#4F46E5',
    modelPresets: (p.model_presets || []).map((m) => ({
      id: m.id,
      label: m.name,
    })),
    icon: getProviderIcon(p.id),
  };
}

/**
 * Updates the in-memory global catalog store and notifies active listeners.
 */
export function setGlobalModelCatalog(models: ModelOption[], providers: ProviderMeta[]): void {
  globalModels = models;
  globalProviders = providers;
  catalogListeners.forEach((listener) => listener());
}

/**
 * Returns currently cached models from the global store.
 */
export function getGlobalModelCatalog(): ModelOption[] {
  return globalModels;
}

/**
 * Returns currently cached provider metadata from the global store.
 */
export function getGlobalProviderMetas(): ProviderMeta[] {
  return globalProviders;
}

/**
 * Subscribe to catalog updates.
 */
export function subscribeModelCatalog(listener: () => void): () => void {
  catalogListeners.add(listener);
  return () => {
    catalogListeners.delete(listener);
  };
}

/**
 * Dynamically fetches models and providers catalog from the backend API.
 * Deduplicates concurrent requests so it runs only once per page session.
 */
export async function fetchModelCatalog(force = false): Promise<{ models: ModelOption[]; providers: ProviderMeta[] }> {
  if (!force && globalModels.length > 0 && globalProviders.length > 0) {
    return { models: globalModels, providers: globalProviders };
  }

  if (catalogFetchPromise && !force) {
    return catalogFetchPromise;
  }

  catalogFetchPromise = (async () => {
    try {
      const res = await api.getModelsCatalog();
      const mappedModels: ModelOption[] = (res?.models || []).map(mapBackendModelToOption);
      const mappedProviders: ProviderMeta[] = (res?.providers || []).map(mapBackendProviderToMeta);

      setGlobalModelCatalog(mappedModels, mappedProviders);
      return { models: mappedModels, providers: mappedProviders };
    } catch (err) {
      console.warn('Failed fetching live model catalog from backend:', err);
      return { models: globalModels, providers: globalProviders };
    } finally {
      catalogFetchPromise = null;
    }
  })();

  return catalogFetchPromise;
}

/**
 * Proxy export for backward-compatibility with components expecting a static catalog array.
 */
export const LATEST_MODEL_CATALOG: ModelOption[] = new Proxy([] as ModelOption[], {
  get(target, prop, receiver) {
    const active = globalModels.length > 0 ? globalModels : target;
    return Reflect.get(active, prop, receiver);
  },
  has(target, prop) {
    const active = globalModels.length > 0 ? globalModels : target;
    return Reflect.has(active, prop);
  },
  ownKeys(target) {
    const active = globalModels.length > 0 ? globalModels : target;
    return Reflect.ownKeys(active);
  },
  getOwnPropertyDescriptor(target, prop) {
    const active = globalModels.length > 0 ? globalModels : target;
    return Reflect.getOwnPropertyDescriptor(active, prop);
  },
});

/**
 * Proxy export for backward-compatibility with components expecting static provider metadata.
 */
export const PROVIDER_METAS: ProviderMeta[] = new Proxy([] as ProviderMeta[], {
  get(target, prop, receiver) {
    const active = globalProviders.length > 0 ? globalProviders : target;
    return Reflect.get(active, prop, receiver);
  },
  has(target, prop) {
    const active = globalProviders.length > 0 ? globalProviders : target;
    return Reflect.has(active, prop);
  },
  ownKeys(target) {
    const active = globalProviders.length > 0 ? globalProviders : target;
    return Reflect.ownKeys(active);
  },
  getOwnPropertyDescriptor(target, prop) {
    const active = globalProviders.length > 0 ? globalProviders : target;
    return Reflect.getOwnPropertyDescriptor(active, prop);
  },
});

/**
 * Returns models grouped by category or filtered by active configured API keys.
 */
export function getCategorizedModels(
  configuredProviders?: LLMProviderInfo[],
  catalog: ModelOption[] = globalModels
) {
  const activeProviderIds = new Set<string>();
  configuredProviders?.forEach((p) => {
    if (p.configured) {
      activeProviderIds.add(p.id);
    }
  });

  const readyModels: ModelOption[] = [];
  const otherModels: ModelOption[] = [];

  const categories: Record<string, ModelOption[]> = {
    'Configured Providers': [],
    'Cloud Frontier': [],
    Reasoning: [],
    'Ultra Fast': [],
    Aggregator: [],
    Custom: [],
  };

  catalog.forEach((model) => {
    const isReady =
      activeProviderIds.has(model.providerId) ||
      model.providerId === 'custom_openai';

    if (isReady) {
      readyModels.push(model);
      categories['Configured Providers'].push(model);
    } else {
      otherModels.push(model);
    }
    if (categories[model.category]) {
      categories[model.category].push(model);
    } else {
      categories[model.category] = [model];
    }
  });

  return { readyModels, otherModels, categories };
}

/**
 * Resolves full metadata for a given model ID.
 */
export function getModelInfo(
  modelId: string,
  catalog: ModelOption[] = globalModels
): ModelOption | undefined {
  return (
    catalog.find((m) => m.id === modelId) ||
    catalog.find((m) => m.id.endsWith('/' + modelId)) ||
    catalog.find((m) => modelId.includes(m.id))
  );
}
