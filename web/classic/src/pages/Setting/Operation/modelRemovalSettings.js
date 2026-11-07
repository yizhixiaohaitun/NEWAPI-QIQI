export const normalizeModelRemovalKeywords = (value) =>
  value.replaceAll('\r\n', '\n').split('\n').map((line) => line.trim()).filter(Boolean).join('\n');

export function orderModelRemovalUpdates(updates, enabled) {
  const toggle = updates.filter((item) => item.key === 'AutomaticRemoveModelEnabled');
  const remaining = updates.filter((item) => item.key !== 'AutomaticRemoveModelEnabled');
  return enabled ? [...remaining, ...toggle] : [...toggle, ...remaining];
}
