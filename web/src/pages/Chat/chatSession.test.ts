import { describe, expect, it } from 'vitest';
import { isPersistedChatSession, NEW_CHAT_SESSION_PARAM, parseChatSessionRoute } from './chatSession';

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
