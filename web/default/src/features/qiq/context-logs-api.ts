import { api } from '@/lib/api'

export type ContextLogRule = {
  id: number
  name: string
  user_id: number | null
  model_pattern: string
  decision: 'capture' | 'skip'
  enabled: boolean
  priority: number
  created_at: number
  updated_at: number
}
export type ContextLogItem = {
  id: number
  user_id: number
  username: string
  created_at: number
  request_id: string
  method: string
  path: string
  model_name: string
  is_stream: boolean
  status_code: number
  latency_ms: number
  error: string
  channel_id: number
  channel_name: string
  channel_type: number
  rule_id: number
  rule_name: string
  decision_source: string
  request_body_size: number
  request_body_truncated: boolean
  response_body_size: number
  response_body_truncated: boolean
}
export type ContextLogDetail = ContextLogItem & {
  request_headers: string
  response_headers: string
  request_body: string
  request_body_encoding: string
  request_body_truncated: boolean
  response_body: string
  response_body_encoding: string
  response_body_truncated: boolean
}
const base = '/api/qiqi/context-logs'
export async function getRules() {
  return (await api.get(`${base}/rules`)).data
}
export async function saveRule(rule: Partial<ContextLogRule>) {
  return rule.id
    ? (await api.put(`${base}/rules/${rule.id}`, rule)).data
    : (await api.post(`${base}/rules`, rule)).data
}
export async function deleteRule(id: number) {
  return (await api.delete(`${base}/rules/${id}`)).data
}
export async function getLogs(params: Record<string, string | number>) {
  return (await api.get(`${base}/logs`, { params })).data
}
export async function getLog(id: number) {
  return (await api.get(`${base}/logs/${id}`)).data
}
export async function deleteLogs(ids: number[]) {
  return (await api.post(`${base}/logs/batch-delete`, { ids })).data
}
