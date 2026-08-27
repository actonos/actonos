import { describe, expect, it } from 'vitest';
import {
  isPersistedChatSession,
  NEW_CHAT_SESSION_PARAM,
  parseChatSessionRoute,
  shouldHydrateChatSession,
} from './chatSession';

describe('parseChatSessionRoute', () => {
  it('treats a missing session_id as the sessions hub', () => {
    expect(parseChatSessionRoute(null)).toEqual({ mode: 'sessions' });
    expect(parseChatSessionRoute('')).toEqual({ mode: 'sessions' });
  });

  it('does not load a draft new-chat hash from the API', () => {
    expect(isPersistedChatSession(NEW_CHAT_SESSION_PARAM)).toBe(false);
    expect(parseChatSessionRoute(NEW_CHAT_SESSION_PARAM)).toEqual({ mode: 'draft' });
  });

  it('loads only persisted conversation ids', () => {
    expect(isPersistedChatSession('conv_abc')).toBe(true);
    expect(parseChatSessionRoute('conv_abc')).toEqual({ mode: 'load', sessionId: 'conv_abc' });
  });
});

describe('shouldHydrateChatSession', () => {
  it('hydrates when opening a different persisted session', () => {
    expect(
      shouldHydrateChatSession({ mode: 'load', sessionId: 'conv_b' }, 'conv_a')
    ).toBe(true);
  });

  it('does not refetch the session already on screen', () => {
    expect(
      shouldHydrateChatSession({ mode: 'load', sessionId: 'conv_a' }, 'conv_a')
    ).toBe(false);
  });

  it('does not hydrate a draft stream that just received a server conversation id', () => {
    expect(
      shouldHydrateChatSession({ mode: 'load', sessionId: 'conv_new' }, null, { streaming: true })
    ).toBe(false);
    expect(
      shouldHydrateChatSession({ mode: 'load', sessionId: 'conv_new' }, NEW_CHAT_SESSION_PARAM, {
        streaming: true,
      })
    ).toBe(false);
  });

  it('still hydrates a different persisted session while another turn is streaming', () => {
    expect(
      shouldHydrateChatSession({ mode: 'load', sessionId: 'conv_b' }, 'conv_a', { streaming: true })
    ).toBe(true);
  });

  it('ignores the sessions hub and draft composer', () => {
    expect(shouldHydrateChatSession({ mode: 'sessions' }, null)).toBe(false);
    expect(shouldHydrateChatSession({ mode: 'draft' }, null)).toBe(false);
  });
});
