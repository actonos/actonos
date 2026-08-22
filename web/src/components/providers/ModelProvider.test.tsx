import { render, screen, cleanup, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ModelProvider, useModelCatalog } from './ModelProvider';
import { api } from '@/lib/api';

vi.mock('@/lib/api', () => ({
  api: {
    getModelsCatalog: vi.fn(),
  },
}));

function TestProbe() {
  const { models, providers, loading, getModelInfo } = useModelCatalog();
  const sampleModel = getModelInfo('anthropic/claude-sonnet-4-6');

  return (
    <div>
      <span data-testid="loading">{loading ? 'true' : 'false'}</span>
      <span data-testid="models-count">{models.length}</span>
      <span data-testid="providers-count">{providers.length}</span>
      {sampleModel && <span data-testid="sample-model-name">{sampleModel.name}</span>}
    </div>
  );
}

describe('ModelProvider', () => {
  beforeEach(() => {
    vi.mocked(api.getModelsCatalog).mockResolvedValue({
      models: [
        {
          id: 'anthropic/claude-sonnet-4-6',
          name: 'Claude Sonnet 4.6',
          provider_id: 'anthropic',
          provider_name: 'Anthropic Claude',
          category: 'Cloud Frontier',
          prompt_per_1m: 3.0,
          completion_per_1m: 15.0,
          supports_tools: true,
          supports_vision: true,
        },
        {
          id: 'openai/gpt-5.4-mini',
          name: 'GPT-5.4 Mini',
          provider_id: 'openai',
          provider_name: 'OpenAI GPT',
          category: 'Ultra Fast',
          prompt_per_1m: 0.75,
          completion_per_1m: 3.0,
          supports_tools: true,
          supports_vision: true,
        },
      ],
      providers: [
        {
          id: 'anthropic',
          name: 'Anthropic Claude',
          category: 'Cloud Frontier',
          description: 'Frontier AI models',
          default_base_url: 'https://api.anthropic.com/v1',
          accent_color: '#D97706',
          model_presets: [],
        },
      ],
    });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('fetches model catalog from API on mount and provides data to consumer hooks', async () => {
    render(
      <ModelProvider>
        <TestProbe />
      </ModelProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('models-count')).toHaveTextContent('2');
    });

    expect(screen.getByTestId('providers-count')).toHaveTextContent('1');
    expect(screen.getByTestId('sample-model-name')).toHaveTextContent('Claude Sonnet 4.6');
    expect(api.getModelsCatalog).toHaveBeenCalledTimes(1);
  });
});
