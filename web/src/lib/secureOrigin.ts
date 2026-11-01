const LOOPBACK = new Set(['localhost', '127.0.0.1', '::1', '[::1]']);

/**
 * OAuth providers only accept https redirect addresses, except for this machine. True when the page the
 * user is on cannot complete a sign-in, so the UI can tell them to open ActonOS over HTTPS first.
 */
export function needsHttpsForSignIn(protocol: string, hostname: string): boolean {
  return protocol !== 'https:' && !LOOPBACK.has(hostname.toLowerCase());
}
