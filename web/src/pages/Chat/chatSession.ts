/** Hash sentinel for a blank composer that has no server row yet. */
export const NEW_CHAT_SESSION_PARAM = 'new';

export function isPersistedChatSession(sessionId: string | null | undefined): sessionId is string {
  return Boolean(sessionId && sessionId !== NEW_CHAT_SESSION_PARAM);
}

export type ChatSessionRoute =
  | { mode: 'sessions' }
  | { mode: 'draft' }
  | { mode: 'load'; sessionId: string };

export function parseChatSessionRoute(sessionId: string | null | undefined): ChatSessionRoute {
  if (!sessionId) {
    return { mode: 'sessions' };
  }
  if (!isPersistedChatSession(sessionId)) {
    return { mode: 'draft' };
  }
  return { mode: 'load', sessionId };
}

/** False when the hash only caught up with an in-flight first-message stream. */
export function shouldHydrateChatSession(
  route: ChatSessionRoute,
  currentSessionId: string | null | undefined,
  options?: { streaming?: boolean }
): boolean {
  if (route.mode !== 'load') {
    return false;
  }
  if (route.sessionId === currentSessionId) {
    return false;
  }
  if (options?.streaming && !isPersistedChatSession(currentSessionId)) {
    return false;
  }
  return true;
}
