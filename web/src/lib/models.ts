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
    badge: 'Hybrid Reasoning & Frontier Coding',
    contextWindow: '200k',
    category: 'Cloud Frontier',
  },
  {
    id: 'anthropic/claude-opus-4-8',
    name: 'Claude Opus 4.8',
    providerId: 'anthropic',
    providerName: 'Anthropic',
    badge: 'Supreme Intelligence & Complex Math',
    contextWindow: '200k',
    category: 'Cloud Frontier',
  },
  {
    id: 'anthropic/claude-sonnet-4-6',
    name: 'Claude Sonnet 4.6',
    providerId: 'anthropic',
    providerName: 'Anthropic',
    badge: 'Advanced Multi-Agent Specialist',
    contextWindow: '200k',
    category: 'Cloud Frontier',
  },
  {
    id: 'anthropic/claude-haiku-4-5',
    name: 'Claude Haiku 4.5',
    providerId: 'anthropic',
    providerName: 'Anthropic',
    badge: 'Ultra-Fast Sub-Agent Worker',
    contextWindow: '200k',
    category: 'Ultra Fast',
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
    badge: 'Fast & Cost Efficient',
    contextWindow: '200k',
    category: 'Ultra Fast',
  },

  // OpenAI
  {
    id: 'openai/gpt-5.6',
    name: 'GPT-5.6',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: '2026 Autonomous Frontier Flagship',
    contextWindow: '256k',
    category: 'Cloud Frontier',
  },
  {
    id: 'openai/gpt-5.5',
    name: 'GPT-5.5',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'General Cognitive Multimodal',
    contextWindow: '256k',
    category: 'Cloud Frontier',
  },
  {
    id: 'openai/gpt-5.4-pro',
    name: 'GPT-5.4 Pro',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Enterprise Deep Reasoning',
    contextWindow: '200k',
    category: 'Reasoning',
  },
  {
    id: 'openai/gpt-5.4-mini',
    name: 'GPT-5.4 Mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'High-Throughput Light Engine',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'openai/o3',
    name: 'o3',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Next-Gen Frontier Reasoning',
    contextWindow: '200k',
    category: 'Reasoning',
  },
  {
    id: 'openai/o3-mini',
    name: 'o3-mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Fast STEM & Code Reasoning',
    contextWindow: '200k',
    category: 'Reasoning',
  },
  {
    id: 'openai/gpt-4o',
    name: 'GPT-4o',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Omni Multimodal Standard',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },
  {
    id: 'openai/gpt-4o-mini',
    name: 'GPT-4o Mini',
    providerId: 'openai',
    providerName: 'OpenAI',
    badge: 'Fast & Lightweight',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },

  // Google Gemini
  {
    id: 'google/gemini-3.1-pro',
    name: 'Gemini 3.1 Pro',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: '2M+ Context & Native System Agent',
    contextWindow: '2M+',
    category: 'Cloud Frontier',
  },
  {
    id: 'google/gemini-3-flash',
    name: 'Gemini 3 Flash',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: '1M+ Context Real-time Streaming',
    contextWindow: '1M+',
    category: 'Ultra Fast',
  },
  {
    id: 'google/gemini-2.5-pro',
    name: 'Gemini 2.5 Pro',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: 'Deep Multi-Modal Code Analysis',
    contextWindow: '2M+',
    category: 'Cloud Frontier',
  },
  {
    id: 'google/gemini-2.5-flash',
    name: 'Gemini 2.5 Flash',
    providerId: 'gemini',
    providerName: 'Google Gemini',
    badge: '1M Context & Fast Execution',
    contextWindow: '1M+',
    category: 'Ultra Fast',
  },

  // xAI Grok
  {
    id: 'xai/grok-4.5',
    name: 'Grok 4.5',
    providerId: 'xai',
    providerName: 'xAI Grok',
    badge: 'Real-Time Web & Deep Logic',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },
  {
    id: 'xai/grok-4.1-fast',
    name: 'Grok 4.1 Fast',
    providerId: 'xai',
    providerName: 'xAI Grok',
    badge: 'Rapid Realtime Analysis',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'xai/grok-code-fast',
    name: 'Grok Code Fast',
    providerId: 'xai',
    providerName: 'xAI Grok',
    badge: 'Dedicated Code Generation Engine',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },
  {
    id: 'xai/grok-3',
    name: 'Grok 3',
    providerId: 'xai',
    providerName: 'xAI Grok',
    badge: 'Supercomputing Reasoning',
    contextWindow: '128k',
    category: 'Reasoning',
  },

  // DeepSeek
  {
    id: 'deepseek/deepseek-v4-pro',
    name: 'DeepSeek-V4 Pro',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: '1M Context MoE Architecture',
    contextWindow: '1M',
    category: 'Cloud Frontier',
  },
  {
    id: 'deepseek/deepseek-v4-flash',
    name: 'DeepSeek-V4 Flash',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: 'Fast Cost-Effective MoE',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'deepseek/deepseek-r1',
    name: 'DeepSeek-R1 (Reasoner)',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: 'Open Reasoning Standard',
    contextWindow: '128k',
    category: 'Reasoning',
  },
  {
    id: 'deepseek/deepseek-v3.2',
    name: 'DeepSeek-V3.2',
    providerId: 'deepseek',
    providerName: 'DeepSeek',
    badge: 'High Accuracy MoE Chat',
    contextWindow: '128k',
    category: 'Cloud Frontier',
  },

  // Open-Source / Local / Ollama / Haimaker
  {
    id: 'ollama/qwen3-coder',
    name: 'Qwen3 Coder (Local/Ollama)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'State of the Art Open Coding',
    contextWindow: '128k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/gemma4:latest',
    name: 'Gemma 4 (Local/Ollama)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Google Open Hardware-Optimized',
    contextWindow: '64k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/llama-3.3-70b',
    name: 'Llama 3.3 70B (Local/Ollama)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Open Weight Production Standard',
    contextWindow: '128k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/deepseek-r1:70b',
    name: 'DeepSeek-R1 70B (Local)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Private Hardware Deliberate Reasoning',
    contextWindow: '64k',
    category: 'Local / Private',
  },
  {
    id: 'ollama/minimax-m3',
    name: 'MiniMax M3 (Local/Cloud)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: '4M Context Agentic Long-Horizon',
    contextWindow: '4M',
    category: 'Local / Private',
  },
  {
    id: 'ollama/kimi-k2.7',
    name: 'Kimi K2.7 (Local/Cloud)',
    providerId: 'ollama',
    providerName: 'Local Ollama',
    badge: 'Multi-Step Agentic Reasoning',
    contextWindow: '256k',
    category: 'Local / Private',
  },

  // Groq Cloud
  {
    id: 'groq/llama-3.3-70b-versatile',
    name: 'Llama 3.3 70B (Groq LPU)',
    providerId: 'groq',
    providerName: 'Groq Cloud',
    badge: '500+ tok/s Ultra Fast',
    contextWindow: '128k',
    category: 'Ultra Fast',
  },
  {
    id: 'groq/deepseek-r1-distill-llama-70b',
    name: 'DeepSeek R1 70B (Groq LPU)',
    providerId: 'groq',
    providerName: 'Groq Cloud',
    badge: 'High-Speed LPU Reasoning',
    contextWindow: '128k',
    category: 'Reasoning',
  },

  // OpenRouter Gateway
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
    id: 'openrouter/openai/gpt-5.5',
    name: 'GPT-5.5 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '256k',
    category: 'Aggregator',
  },
  {
    id: 'openrouter/deepseek/deepseek-r1',
    name: 'DeepSeek-R1 (OpenRouter)',
    providerId: 'openrouter',
    providerName: 'OpenRouter',
    badge: 'Aggregated Gateway',
    contextWindow: '128k',
    category: 'Aggregator',
  },
];

/**
 * Returns models grouped by category or filtered by active configured API keys.
 */
export function getCategorizedModels(configuredProviders?: any[]) {
  const activeProviderIds = new Set<string>();
  configuredProviders?.forEach((p) => {
    if (p.is_configured || p.configured) {
      const pid = p.provider || p.id || '';
      activeProviderIds.add(pid);
      if (pid === 'gemini') activeProviderIds.add('google');
      if (pid === 'google') activeProviderIds.add('gemini');
    }
  });

  const readyModels: ModelOption[] = [];
  const otherModels: ModelOption[] = [];

  const categories: Record<string, ModelOption[]> = {
    'Configured Providers': [],
    'Cloud Frontier': [],
    Reasoning: [],
    'Ultra Fast': [],
    'Local / Private': [],
    Aggregator: [],
  };

  LATEST_MODEL_CATALOG.forEach((model) => {
    const isReady =
      activeProviderIds.has(model.providerId) ||
      model.providerId === 'ollama' ||
      model.providerId === 'groq';

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
