/**
 * ActonOS Universal Model & Provider Catalog
 * Unified Single Source of Truth for frontend components and synchronized with backend /api/models.
 */

import { api } from '@/lib/api';
import type { ModelSpec } from '@/lib/types';
import { Sparkles, Zap, Cpu, Server, Bot } from 'lucide-react';
import type React from 'react';

export interface ModelOption {
  id: string; // e.g. "anthropic/claude-sonnet-4-6"
  name: string; // e.g. "Claude Sonnet 4.6"
  providerId: string; // e.g. "anthropic"
  providerName: string; // e.g. "Anthropic Claude"
  badge?: string; // e.g. "Hybrid Reasoning", "Ultra Fast", "Frontier Coding"
  contextWindow?: string; // e.g. "256k", "512k", "1M+", "2M+"
  category: 'Cloud Frontier' | 'Reasoning' | 'Ultra Fast' | 'Aggregator' | 'Custom';
  promptPer1M?: number;
  completionPer1M?: number;
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

export const LATEST_MODEL_CATALOG: ModelOption[] = [
  // Anthropic
  {
    id: 'anthropic/claude-haiku-4-5',
    name: 'Claude Haiku 4.5',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Ultra Fast & High Efficiency Sub-Agent',
    contextWindow: '256k',
    category: 'Ultra Fast',
    promptPer1M: 0.8,
    completionPer1M: 4.0,
  },
  {
    id: 'anthropic/claude-sonnet-4-5',
    name: 'Claude Sonnet 4.5',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Autonomous Engineering Specialist',
    contextWindow: '256k',
    category: 'Cloud Frontier',
    promptPer1M: 3.0,
    completionPer1M: 15.0,
  },
  {
    id: 'anthropic/claude-sonnet-4-6',
    name: 'Claude Sonnet 4.6',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Frontier Coding & Multi-Agent Swarm',
    contextWindow: '512k',
    category: 'Cloud Frontier',
    promptPer1M: 3.0,
    completionPer1M: 15.0,
  },
  {
    id: 'anthropic/claude-opus-4-5',
    name: 'Claude Opus 4.5',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Deep Cognitive Reasoning Flagship',
    contextWindow: '256k',
    category: 'Reasoning',
    promptPer1M: 10.0,
    completionPer1M: 40.0,
  },
  {
    id: 'anthropic/claude-sonnet-5',
    name: 'Claude Sonnet 5',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Next-Gen Cognitive Architecture',
    contextWindow: '1M+',
    category: 'Cloud Frontier',
    promptPer1M: 3.5,
    completionPer1M: 17.5,
  },
  {
    id: 'anthropic/claude-opus-4-6',
    name: 'Claude Opus 4.6',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Supreme STEM & System Architecture',
    contextWindow: '512k',
    category: 'Reasoning',
    promptPer1M: 12.0,
    completionPer1M: 50.0,
  },
  {
    id: 'anthropic/claude-opus-4-7',
    name: 'Claude Opus 4.7',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Advanced Deliberate Reasoning',
    contextWindow: '512k',
    category: 'Reasoning',
    promptPer1M: 14.0,
    completionPer1M: 55.0,
  },
  {
    id: 'anthropic/claude-opus-4-8',
    name: 'Claude Opus 4.8',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Supreme Autonomous Superintelligence',
    contextWindow: '1M+',
    category: 'Reasoning',
    promptPer1M: 15.0,
    completionPer1M: 60.0,
  },
  {
    id: 'anthropic/claude-opus-5',
    name: 'Claude Opus 5',
    providerId: 'anthropic',
    providerName: 'Anthropic Claude',
    badge: 'Peak Frontier Superintelligence Flagship',
    contextWindow: '2M+',
    category: 'Cloud Frontier',
    promptPer1M: 20.0,
    completionPer1M: 80.0,
  },

  // OpenAI
  {
    id: 'openai/gpt-5-mini',
    name: 'GPT-5 Mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Lightweight Ultra-Fast Multimodal',
    contextWindow: '256k',
    category: 'Ultra Fast',
    promptPer1M: 0.2,
    completionPer1M: 0.8,
  },
  {
    id: 'openai/gpt-5',
    name: 'GPT-5',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'GPT-5 Flagship Foundation Model',
    contextWindow: '256k',
    category: 'Cloud Frontier',
    promptPer1M: 2.0,
    completionPer1M: 8.0,
  },
  {
    id: 'openai/gpt-5.1',
    name: 'GPT-5.1',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Enhanced Code & Tool Calling',
    contextWindow: '256k',
    category: 'Cloud Frontier',
    promptPer1M: 2.2,
    completionPer1M: 8.8,
  },
  {
    id: 'openai/gpt-5.2',
    name: 'GPT-5.2',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Adaptive Multi-Step Reasoning',
    contextWindow: '512k',
    category: 'Cloud Frontier',
    promptPer1M: 2.5,
    completionPer1M: 10.0,
  },
  {
    id: 'openai/gpt-5.4-mini',
    name: 'GPT-5.4 Mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'High-Throughput Sub-Agent Brain',
    contextWindow: '256k',
    category: 'Ultra Fast',
    promptPer1M: 0.3,
    completionPer1M: 1.2,
  },
  {
    id: 'openai/gpt-5.4',
    name: 'GPT-5.4',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Enterprise Cognitive Flagship',
    contextWindow: '512k',
    category: 'Cloud Frontier',
    promptPer1M: 3.0,
    completionPer1M: 12.0,
  },
  {
    id: 'openai/gpt-5.5',
    name: 'GPT-5.5',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Advanced Multimodal Deep Understanding',
    contextWindow: '1M+',
    category: 'Cloud Frontier',
    promptPer1M: 3.5,
    completionPer1M: 14.0,
  },
  {
    id: 'openai/gpt-5.6-terra',
    name: 'GPT-5.6 Terra',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Grounding & Large-Scale Data Systems',
    contextWindow: '1M+',
    category: 'Cloud Frontier',
    promptPer1M: 4.0,
    completionPer1M: 16.0,
  },
  {
    id: 'openai/gpt-5.6-sol',
    name: 'GPT-5.6 Sol',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Peak Autonomous Agent Flagship',
    contextWindow: '2M+',
    category: 'Reasoning',
    promptPer1M: 5.0,
    completionPer1M: 20.0,
  },

  // DeepSeek
  {
    id: 'deepseek/deepseek-v4-flash',
    name: 'DeepSeek-V4 Flash',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: 'Ultra-High Throughput MoE Architecture',
    contextWindow: '256k',
    category: 'Ultra Fast',
    promptPer1M: 0.1,
    completionPer1M: 0.25,
  },
  {
    id: 'deepseek/deepseek-v4-pro',
    name: 'DeepSeek-V4 Pro',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: '1M Context MoE Reasoning Leader',
    contextWindow: '1M+',
    category: 'Reasoning',
    promptPer1M: 0.45,
    completionPer1M: 1.8,
  },

  // xAI (Grok)
  {
    id: 'grok/grok-4.3',
    name: 'Grok 4.3',
    providerId: 'grok',
    providerName: 'xAI (Grok)',
    badge: 'Real-Time Knowledge & Rapid Tool Use',
    contextWindow: '256k',
    category: 'Ultra Fast',
    promptPer1M: 1.5,
    completionPer1M: 6.0,
  },
  {
    id: 'grok/grok-4.5',
    name: 'Grok 4.5',
    providerId: 'grok',
    providerName: 'xAI (Grok)',
    badge: 'Deep Cognitive Reasoning & Coding',
    contextWindow: '512k',
    category: 'Reasoning',
    promptPer1M: 3.0,
    completionPer1M: 12.0,
  },
  {
    id: 'grok/grok-4.6',
    name: 'Grok 4.6',
    providerId: 'grok',
    providerName: 'xAI (Grok)',
    badge: 'Peak Frontier Realtime Intelligence',
    contextWindow: '1M+',
    category: 'Cloud Frontier',
    promptPer1M: 4.5,
    completionPer1M: 18.0,
  },

  // OpenRouter
  {
    id: 'openrouter/anthropic/claude-sonnet-5',
    name: 'Claude Sonnet 5 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Frontier Claude via OpenRouter',
    contextWindow: '1M+',
    category: 'Aggregator',
    promptPer1M: 3.5,
    completionPer1M: 17.5,
  },
  {
    id: 'openrouter/anthropic/claude-opus-5',
    name: 'Claude Opus 5 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Superintelligence via OpenRouter',
    contextWindow: '2M+',
    category: 'Aggregator',
    promptPer1M: 20.0,
    completionPer1M: 80.0,
  },
  {
    id: 'openrouter/anthropic/claude-sonnet-4-6',
    name: 'Claude Sonnet 4.6 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Frontier Coding via OpenRouter',
    contextWindow: '512k',
    category: 'Aggregator',
    promptPer1M: 3.0,
    completionPer1M: 15.0,
  },
  {
    id: 'openrouter/openai/gpt-5.6-sol',
    name: 'GPT-5.6 Sol (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'GPT-5.6 Flagship via OpenRouter',
    contextWindow: '2M+',
    category: 'Aggregator',
    promptPer1M: 5.0,
    completionPer1M: 20.0,
  },
  {
    id: 'openrouter/openai/gpt-5.5',
    name: 'GPT-5.5 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'GPT-5.5 via OpenRouter',
    contextWindow: '1M+',
    category: 'Aggregator',
    promptPer1M: 3.5,
    completionPer1M: 14.0,
  },
  {
    id: 'openrouter/openai/gpt-5',
    name: 'GPT-5 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'GPT-5 Standard via OpenRouter',
    contextWindow: '256k',
    category: 'Aggregator',
    promptPer1M: 2.0,
    completionPer1M: 8.0,
  },
  {
    id: 'openrouter/deepseek/deepseek-v4-pro',
    name: 'DeepSeek-V4 Pro (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'DeepSeek-V4 via OpenRouter',
    contextWindow: '1M+',
    category: 'Aggregator',
    promptPer1M: 0.45,
    completionPer1M: 1.8,
  },
  {
    id: 'openrouter/x-ai/grok-4.6',
    name: 'Grok 4.6 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'xAI Grok via OpenRouter',
    contextWindow: '1M+',
    category: 'Aggregator',
    promptPer1M: 4.5,
    completionPer1M: 18.0,
  },

  // Custom OpenAI
  {
    id: 'custom_openai/default-model',
    name: 'Default Model',
    providerId: 'custom_openai',
    providerName: 'Custom Gateway',
    badge: 'Self-Hosted / Private',
    contextWindow: '128k',
    category: 'Custom',
    promptPer1M: 0.0,
    completionPer1M: 0.0,
  },
];

export const PROVIDER_METAS: ProviderMeta[] = [
  {
    id: 'anthropic',
    name: 'Anthropic Claude',
    category: 'Cloud Frontier',
    description: 'Frontier coding, hybrid reasoning, and autonomous multi-agent intelligence.',
    defaultBaseURL: 'https://api.anthropic.com/v1',
    modelPresets: [
      { id: 'claude-sonnet-4-6', label: 'Claude Sonnet 4.6 (Frontier Coding Specialist)' },
      { id: 'claude-haiku-4-5', label: 'Claude Haiku 4.5 (Ultra Fast Sub-Agent)' },
      { id: 'claude-sonnet-4-5', label: 'Claude Sonnet 4.5' },
      { id: 'claude-opus-4-5', label: 'Claude Opus 4.5' },
      { id: 'claude-sonnet-5', label: 'Claude Sonnet 5 (Next-Gen Cognitive)' },
      { id: 'claude-opus-4-6', label: 'Claude Opus 4.6' },
      { id: 'claude-opus-4-7', label: 'Claude Opus 4.7' },
      { id: 'claude-opus-4-8', label: 'Claude Opus 4.8' },
      { id: 'claude-opus-5', label: 'Claude Opus 5 (Superintelligence Flagship)' },
    ],
    accentColor: '#D97706',
    icon: Sparkles,
  },
  {
    id: 'openai',
    name: 'OpenAI',
    category: 'Cloud Frontier',
    description: 'Industry standard GPT-5 generation reasoning and agentic execution.',
    defaultBaseURL: 'https://api.openai.com/v1',
    modelPresets: [
      { id: 'gpt-5', label: 'GPT-5 (Flagship Foundation)' },
      { id: 'gpt-5-mini', label: 'GPT-5 Mini (Ultra Fast)' },
      { id: 'gpt-5.1', label: 'GPT-5.1 (Tool Calling Enhanced)' },
      { id: 'gpt-5.2', label: 'GPT-5.2 (Multi-Step Reasoning)' },
      { id: 'gpt-5.4-mini', label: 'GPT-5.4 Mini' },
      { id: 'gpt-5.4', label: 'GPT-5.4 (Enterprise Flagship)' },
      { id: 'gpt-5.5', label: 'GPT-5.5 (1M+ Multimodal)' },
      { id: 'gpt-5.6-terra', label: 'GPT-5.6 Terra' },
      { id: 'gpt-5.6-sol', label: 'GPT-5.6 Sol (2M+ Peak Autonomous)' },
    ],
    accentColor: '#10B981',
    icon: Zap,
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    category: 'Open Weights & Cloud',
    description: 'DeepSeek-V4 generation high-performance architecture.',
    defaultBaseURL: 'https://api.deepseek.com/v1',
    modelPresets: [
      { id: 'deepseek-v4-flash', label: 'DeepSeek-V4 Flash (High Throughput MoE)' },
      { id: 'deepseek-v4-pro', label: 'DeepSeek-V4 Pro (1M MoE Architecture)' },
    ],
    accentColor: '#6366F1',
    icon: Cpu,
  },
  {
    id: 'grok',
    name: 'xAI (Grok)',
    category: 'Real-Time Intelligence',
    description: 'xAI Grok generation real-time knowledge and frontier reasoning.',
    defaultBaseURL: 'https://api.x.ai/v1',
    modelPresets: [
      { id: 'grok-4.5', label: 'Grok 4.5 (Deep Cognitive Reasoning)' },
      { id: 'grok-4.3', label: 'Grok 4.3 (Real-Time Knowledge)' },
      { id: 'grok-4.6', label: 'Grok 4.6 (1M+ Peak Frontier)' },
    ],
    accentColor: '#EC4899',
    icon: Bot,
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    category: 'Unified Aggregator',
    description: 'Universal gateway aggregating Claude, GPT-5, DeepSeek-V4, and Grok.',
    defaultBaseURL: 'https://openrouter.ai/api/v1',
    modelPresets: [
      { id: 'anthropic/claude-sonnet-5', label: 'Claude Sonnet 5 (via OpenRouter)' },
      { id: 'anthropic/claude-opus-5', label: 'Claude Opus 5 (via OpenRouter)' },
      { id: 'anthropic/claude-sonnet-4-6', label: 'Claude Sonnet 4.6 (via OpenRouter)' },
      { id: 'openai/gpt-5.6-sol', label: 'GPT-5.6 Sol (via OpenRouter)' },
      { id: 'openai/gpt-5.5', label: 'GPT-5.5 (via OpenRouter)' },
      { id: 'openai/gpt-5', label: 'GPT-5 (via OpenRouter)' },
      { id: 'deepseek/deepseek-v4-pro', label: 'DeepSeek-V4 Pro (via OpenRouter)' },
      { id: 'x-ai/grok-4.6', label: 'Grok 4.6 (via OpenRouter)' },
    ],
    accentColor: '#8B5CF6',
    icon: Server,
  },
  {
    id: 'custom_openai',
    name: 'Custom OpenAI-Compatible',
    category: 'Self-Hosted / Gateway',
    description: 'Connect LM Studio, vLLM, LocalAI, Azure OpenAI, or enterprise gateway.',
    defaultBaseURL: 'http://localhost:8000/v1',
    modelPresets: [
      { id: 'default-model', label: 'Default Model' },
      { id: 'custom-model', label: 'Custom Model Tag' },
    ],
    accentColor: '#0EA5E9',
    icon: Server,
  },
];

/**
 * Dynamically synchronizes models and providers from backend /api/models.
 */
export async function fetchModelCatalog(): Promise<ModelOption[]> {
  try {
    const res = await api.getModelsCatalog();
    if (res?.models && res.models.length > 0) {
      const mapped: ModelOption[] = res.models.map((m: ModelSpec) => ({
        id: m.id,
        name: m.name,
        providerId: m.provider_id,
        providerName: m.provider_name,
        badge: m.badge,
        contextWindow: m.context_window,
        category: (m.category as ModelOption['category']) || 'Cloud Frontier',
        promptPer1M: m.prompt_per_1m,
        completionPer1M: m.completion_per_1m,
      }));
      return mapped;
    }
  } catch (err) {
    console.debug('Failed fetching live model catalog from backend, using canonical snapshot:', err);
  }
  return LATEST_MODEL_CATALOG;
}

/**
 * Returns models grouped by category or filtered by active configured API keys.
 */
export function getCategorizedModels(configuredProviders?: any[], catalog: ModelOption[] = LATEST_MODEL_CATALOG) {
  const activeProviderIds = new Set<string>();
  configuredProviders?.forEach((p) => {
    if (p.is_configured || p.configured) {
      const pid = p.provider || p.id || '';
      activeProviderIds.add(pid);
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
    }
  });

  return { readyModels, otherModels, categories };
}

export function getModelInfo(modelId: string, catalog: ModelOption[] = LATEST_MODEL_CATALOG): ModelOption | undefined {
  return (
    catalog.find((m) => m.id === modelId) ||
    catalog.find((m) => m.id.endsWith('/' + modelId)) ||
    catalog.find((m) => modelId.includes(m.id))
  );
}
