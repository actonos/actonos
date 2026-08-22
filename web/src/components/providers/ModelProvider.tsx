import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import {
  type ModelOption,
  type ProviderMeta,
  fetchModelCatalog,
  getGlobalModelCatalog,
  getGlobalProviderMetas,
  subscribeModelCatalog,
  getModelInfo,
  getCategorizedModels,
} from '@/lib/models';
import type { LLMProviderInfo } from '@/lib/types';

interface ModelContextValue {
  models: ModelOption[];
  providers: ProviderMeta[];
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
  getModelInfo: (modelId: string) => ModelOption | undefined;
  getCategorizedModels: (configuredProviders?: LLMProviderInfo[]) => {
    readyModels: ModelOption[];
    otherModels: ModelOption[];
    categories: Record<string, ModelOption[]>;
  };
}

const ModelContext = createContext<ModelContextValue | null>(null);

export function ModelProvider({ children }: { children: React.ReactNode }) {
  const [models, setModels] = useState<ModelOption[]>(getGlobalModelCatalog);
  const [providers, setProviders] = useState<ProviderMeta[]>(getGlobalProviderMetas);
  const [loading, setLoading] = useState<boolean>(() => getGlobalModelCatalog().length === 0);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (force = false) => {
    try {
      setLoading(true);
      setError(null);
      const res = await fetchModelCatalog(force);
      setModels(res.models);
      setProviders(res.providers);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load model catalog');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Initial fetch once on app load
    load(false);

    // Sync with any global store updates
    const unsubscribe = subscribeModelCatalog(() => {
      setModels(getGlobalModelCatalog());
      setProviders(getGlobalProviderMetas());
    });
    return unsubscribe;
  }, [load]);

  const value: ModelContextValue = {
    models,
    providers,
    loading,
    error,
    refetch: () => load(true),
    getModelInfo: (id: string) => getModelInfo(id, models),
    getCategorizedModels: (configured?: LLMProviderInfo[]) => getCategorizedModels(configured, models),
  };

  return <ModelContext.Provider value={value}>{children}</ModelContext.Provider>;
}

export function useModelCatalog(): ModelContextValue {
  const context = useContext(ModelContext);
  if (!context) {
    // Fallback to global store if used outside provider
    const globalMods = getGlobalModelCatalog();
    const globalProvs = getGlobalProviderMetas();
    return {
      models: globalMods,
      providers: globalProvs,
      loading: false,
      error: null,
      refetch: async () => {
        await fetchModelCatalog(true);
      },
      getModelInfo: (id: string) => getModelInfo(id, globalMods),
      getCategorizedModels: (configured?: LLMProviderInfo[]) => getCategorizedModels(configured, globalMods),
    };
  }
  return context;
}
