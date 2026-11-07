export const DEFAULT_MODEL_REMOVAL_KEYWORDS =
  'This Model is marked by provider as Legacy'

export function normalizeModelRemovalKeywords(value: string): string {
  return value
    .replaceAll('\r\n', '\n')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .join('\n')
}

// Persist the keyword list before enabling destructive matching. Disabling wins first.
export function orderModelRemovalUpdates<T extends string>(
  keys: T[],
  enabled: boolean
): T[] {
  const toggle = keys.filter((key) => key === 'AutomaticRemoveModelEnabled')
  const remaining = keys.filter((key) => key !== 'AutomaticRemoveModelEnabled')
  return enabled ? [...remaining, ...toggle] : [...toggle, ...remaining]
}
