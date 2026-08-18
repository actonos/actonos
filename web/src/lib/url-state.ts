export function readHashParams(): URLSearchParams {
  const hash = window.location.hash.replace(/^#\/?/, '');
  const [, query = ''] = hash.split('?');
  return new URLSearchParams(query);
}

export function updateHashQuery(params: URLSearchParams): void {
  const route = window.location.hash.replace(/^#\/?/, '').split('?')[0];
  const query = params.toString();
  window.location.hash = `/${route}${query ? `?${query}` : ''}`;
}

export function setHashParam(key: string, value?: string): void {
  const params = readHashParams();
  if (!value) params.delete(key);
  else params.set(key, value);
  updateHashQuery(params);
}
