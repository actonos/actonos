import type { LLMProviderInfo } from './types';

export interface ModelOption {
  id: string; // e.g. "anthropic/claude-3-7-sonnet"
  name: string; // e.g. "Claude 3.7 Sonnet"
  providerId: string; // e.g. "anthropic"
  providerName: string; // e.g. "Anthropic Claude"
  badge?: string; // e.g. "Hybrid Reasoning", "Ultra Fast", "Code Specialist"
  contextWindow?: string; // e.g. "200k", "1M+", "128k"
  category: 'Cloud Frontier' | 'Reasoning' | 'Ultra Fast' | 'Local / Private' | 'Aggregator';
}

export const LATEST_MODEL_CATALOG: ModelOption[] = [
  // Anthropic
  {
    id: 'anthropic/claude-3-7-sonnet',
    name: 'Claude 3.7 Sonnet',
    providerId: 'anthropic',
    providerName: 'Anthropic',
    badge: 'Hybrid Reasoning (2025/2026 Flagship)',
    contextWindow: '200k',
    category: 'Cloud Frontier',
  },
  {
    id: 'anthropic/claude-3-5-sonnet',
    name: 'Claude 3.5 Sonnet',
    providerId: 'anthropic',
    providerName: 'Anthropic',
    badge: 'Coding Benchmark Leader',
    contextWindow: '200k',
    category: 'Cloud Frontier',
  },
  {
    id: 'anthropic/claude-3-5-haiku',
    name: 'Claude 3.5 Haiku',
    providerId: 'anthropic',
    providerName: 'Anthropic',
    badge: 'Ultra-Fast & Smart',
    contextWindow: '200k',
    category: 'Ultra Fast',
  },
  {
    id: 'anthropic/claude-3-opus',
    name: 'Claude 3 Opus',
    providerId: 'anthropic',
    providerName: 'Anthropic',
    badge: 'Deep Knowledge',
    contextWindow: '200k',
    category: 'Cloud Frontier',
  },

  // OpenAI
  {
    id: 'openai/gpt-4.5-preview',
    name: 'GPT-4.5 (Orion)',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Massive World Knowledge',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },
  {
    id: 'openai/gpt-4o',
    name: 'GPT-4o',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Omni Multimodal Flagship',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },
  {
    id: 'openai/gpt-4o-mini',
    name: 'GPT-4o Mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Fast & Efficient',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'openai/o3-mini',
    name: 'o3-mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'High STEM & Code Reasoning',
    contextWindow: '200k',
    category: 'Reasoning',
  },
  {
    id: 'openai/o1',
    name: 'o1',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Deep Deliberate Reasoning',
    contextWindow: '200k',
    category: 'Reasoning',
  },
  {
    id: 'openai/o1-mini',
    name: 'o1-mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Fast Reasoning',
    contextWindow: '128k',
    category: 'Reasoning',
  },

  // Google Gemini
  {
    id: 'google/gemini-2.5-flash',
    name: 'Gemini 2.5 Flash',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: '1M+ Context & Native Tools',
    contextWindow: '1M+',
    category: 'Cloud Frontier',
  },
  {
    id: 'google/gemini-2.0-flash',
    name: 'Gemini 2.0 Flash',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: 'Next-Gen Multimodal',
    contextWindow: '1M+',
    category: 'Cloud Frontier',
  },
  {
    id: 'google/gemini-2.0-flash-thinking-exp',
    name: 'Gemini 2.0 Flash Thinking',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: 'Visual & Math Reasoning',
    contextWindow: '1M+',
    category: 'Reasoning',
  },
  {
    id: 'google/gemini-2.0-pro-exp-02-05',
    name: 'Gemini 2.0 Pro',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: 'Complex Coding & Logic',
    contextWindow: '2M+',
    category: 'Cloud Frontier',
  },
  {
    id: 'google/gemini-1.5-pro',
    name: 'Gemini 1.5 Pro',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: '2M Context Window',
    contextWindow: '2M+',
    category: 'Cloud Frontier',
  },
  {
    id: 'google/gemini-1.5-flash',
    name: 'Gemini 1.5 Flash',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: 'Lightweight Fast',
    contextWindow: '1M+',
    category: 'Ultra Fast',
  },

  // DeepSeek
  {
    id: 'deepseek/deepseek-chat',
    name: 'DeepSeek-V3 Chat',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: '671B MoE Flagship',
    contextWindow: '64k',
    category: 'Cloud Frontier',
  },
  {
    id: 'deepseek/deepseek-reasoner',
    name: 'DeepSeek-R1 (Reasoner)',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: 'Open Reasoning Champion',
    contextWindow: '64k',
    category: 'Reasoning',
  },

  // Groq Cloud
  {
    id: 'groq/llama-3.3-70b-versatile',
    name: 'Llama 3.3 70B Versatile (Groq)',
    providerId: 'groq',
    providerName: 'Groq Cloud',
    badge: '500+ tok/s Ultra Fast',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'groq/deepseek-r1-distill-llama-70b',
    name: 'DeepSeek R1 Distill 70B (Groq)',
    providerId: 'groq',
    providerName: 'Groq Cloud',
    badge: 'LPU Reasoning Speed',
    contextWindow: '128k',
    category: 'Reasoning',
  },
  {
    id: 'groq/qwen-2.5-32b',
    name: 'Qwen 2.5 32B (Groq)',
    providerId: 'groq',
    providerName: 'Groq Cloud',
    badge: 'High Accuracy Code',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'groq/llama-3.1-8b-instant',
    name: 'Llama 3.1 8B Instant (Groq)',
    providerId: 'groq',
    providerName: 'Groq Cloud',
    badge: '800+ tok/s Realtime',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'groq/mixtral-8x7b-32768',
    name: 'Mixtral 8x7B (Groq)',
    providerId: 'groq',
    providerName: 'Groq Cloud',
    badge: 'Fast MoE',
    contextWindow: '32k',
    category: 'Ultra Fast',
  },

  // OpenRouter
  {
    id: 'openrouter/anthropic/claude-3.7-sonnet',
    name: 'Claude 3.7 Sonnet (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '200k',
    category: 'Aggregator',
  },
  {
    id: 'openrouter/openai/gpt-4o',
    name: 'GPT-4o (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '128k',
    category: 'Aggregator',
  },
  {
    id: 'openrouter/openai/o3-mini',
    name: 'o3-mini (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '200k',
    category: 'Aggregator',
  },
  {
    id: 'openrouter/google/gemini-2.5-flash',
    name: 'Gemini 2.5 Flash (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '1M+',
    category: 'Aggregator',
  },
  {
    id: 'openrouter/deepseek/deepseek-r1',
    name: 'DeepSeek-R1 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '64k',
    category: 'Aggregator',
  },
  {
    id: 'openrouter/meta-llama/llama-3.3-70b-instruct',
    name: 'Llama 3.3 70B (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '128k',
    category: 'Aggregator',
  },

  // Mistral AI
  {
    id: 'mistral/mistral-large-latest',
    name: 'Mistral Large 2',
    providerId: 'mistral',
    providerName: 'Mistral AI',
    badge: 'Flagship European Model',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },
  {
    id: 'mistral/codestral-latest',
    name: 'Codestral 2501',
    providerId: 'mistral',
    providerName: 'Mistral AI',
    badge: 'Code & Fill-In-Middle Specialist',
    contextWindow: '256k',
    category: 'Cloud Frontier',
  },
  {
    id: 'mistral/mistral-small-latest',
    name: 'Mistral Small 3',
    providerId: 'mistral',
    providerName: 'Mistral AI',
    badge: 'Efficient & Multilingual',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'mistral/pixtral-large-latest',
    name: 'Pixtral Large',
    providerId: 'mistral',
    providerName: 'Mistral AI',
    badge: 'Vision & Document Intelligence',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },

  // Local Ollama / vLLM
  {
    id: 'ollama/llama3.3',
    name: 'Llama 3.3 (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Offline Private GPU',
    contextWindow: '128k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/deepseek-r1:70b',
    name: 'DeepSeek-R1 70B (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Local Private Reasoning',
    contextWindow: '64k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/deepseek-r1:14b',
    name: 'DeepSeek-R1 14B (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Balanced Local Reasoning',
    contextWindow: '64k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/deepseek-r1:8b',
    name: 'DeepSeek-R1 8B (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Lightweight Local Reasoning',
    contextWindow: '32k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/qwen2.5-coder:32b',
    name: 'Qwen 2.5 Coder 32B (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Local Coding Powerhouse',
    contextWindow: '32k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/phi4',
    name: 'Phi-4 (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'High Quality 14B',
    contextWindow: '16k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/mistral-nemo',
    name: 'Mistral NeMo 12B (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'General Local Assistant',
    contextWindow: '128k',
    category: 'Local / Private',
  },

  // Custom OpenAI Compatible
  {
    id: 'custom_openai/default-model',
    name: 'Custom Endpoint Model',
    providerId: 'custom_openai',
    providerName: 'Custom OpenAI-Compatible',
    badge: 'LM Studio / LocalAI / Azure',
    contextWindow: 'Custom',
    category: 'Local / Private',
  },
];

/**
 * Returns models grouped by availability based on active provider keys.
 */
export function getCategorizedModels(configuredProviders: LLMProviderInfo[]) {
  const configuredProviderIds = new Set(
    configuredProviders
      .filter((p) => p.configured && p.enabled)
      .map((p) => p.id)
  );

  const readyModels: ModelOption[] = [];
  const otherModels: ModelOption[] = [];

  for (const m of LATEST_MODEL_CATALOG) {
    if (configuredProviderIds.has(m.providerId) || m.providerId === 'ollama') {
      readyModels.push(m);
    } else {
      otherModels.push(m);
    }
  }

  return { readyModels, otherModels };
}
