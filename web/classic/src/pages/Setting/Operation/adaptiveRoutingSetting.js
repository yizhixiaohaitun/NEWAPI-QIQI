export const adaptiveRoutingKey = 'adaptive_routing_setting.enabled';

export function adaptiveRoutingValue(value) {
  return value === true || value === 'true';
}

export function adaptiveRoutingOption(value) {
  return { key: adaptiveRoutingKey, value: String(value) };
}
