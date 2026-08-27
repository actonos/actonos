import { afterEach, describe, expect, it } from 'vitest';
import { setHashParam } from './url-state';

describe('setHashParam', () => {
  afterEach(() => {
    window.location.hash = '';
  });

  it('writes a query param onto the current hash route', () => {
    window.location.hash = '#/chat?session_id=new';
    setHashParam('session_id', 'conv_abc');
    expect(window.location.hash).toBe('#/chat?session_id=conv_abc');
  });

  it('does not assign location.hash when the value is unchanged', () => {
    window.location.hash = '#/chat?session_id=conv_abc';
    let hashChanges = 0;
    const onChange = () => {
      hashChanges += 1;
    };
    window.addEventListener('hashchange', onChange);
    setHashParam('session_id', 'conv_abc');
    window.removeEventListener('hashchange', onChange);
    expect(hashChanges).toBe(0);
    expect(window.location.hash).toBe('#/chat?session_id=conv_abc');
  });
});
