import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

import {
  deleteLogs,
  deleteRule,
  getLog,
  getLogs,
  getRules,
  saveRule,
  type ContextLogDetail,
  type ContextLogItem,
  type ContextLogRule,
} from './context-logs-api'

const emptyRule: Partial<ContextLogRule> = {
  name: '',
  user_id: null,
  model_pattern: '',
  decision: 'capture',
  enabled: true,
  priority: 0,
}
function copy(text: string) {
  void navigator.clipboard.writeText(text)
  toast.success('已复制')
}
export function ContextLogs() {
  const qc = useQueryClient()
  const [filters, setFilters] = useState<Record<string, string | number>>({
    p: 1,
    page_size: 20,
  })
  const [editing, setEditing] = useState<Partial<ContextLogRule> | null>(null)
  const [detailId, setDetailId] = useState<number | null>(null)
  const rules = useQuery({ queryKey: ['context-log-rules'], queryFn: getRules })
  const logs = useQuery({
    queryKey: ['context-logs', filters],
    queryFn: () => getLogs(filters),
  })
  const detail = useQuery({
    queryKey: ['context-log', detailId],
    queryFn: () => getLog(detailId ?? 0),
    enabled: detailId !== null,
  })
  const setFilter = (key: string, value: string) =>
    setFilters((old) => ({ ...old, [key]: value, p: 1 }))
  const persist = async () => {
    if (!editing) return
    const result = await saveRule(editing)
    if (!result.success) {
      toast.error(result.message)
      return
    }
    setEditing(null)
    await qc.invalidateQueries({ queryKey: ['context-log-rules'] })
    toast.success('规则已保存')
  }
  const items = (logs.data?.data?.items ?? []) as ContextLogItem[]
  const selected = detail.data?.data as ContextLogDetail | undefined
  return (
    <div className='container mx-auto space-y-4 p-4'>
      <div>
        <h1 className='text-2xl font-semibold'>上下文日志</h1>
        <p className='text-muted-foreground text-sm'>
          仅 Root 可访问。正文按每侧 2 MiB 上限捕获，截断项会明确标记。
        </p>
      </div>
      <Tabs defaultValue='logs'>
        <TabsList>
          <TabsTrigger value='logs'>日志</TabsTrigger>
          <TabsTrigger value='rules'>保存规则</TabsTrigger>
        </TabsList>
        <TabsContent value='logs' className='space-y-3'>
          <div className='grid gap-2 md:grid-cols-4'>
            <Input
              placeholder='用户 ID'
              onChange={(e) => setFilter('user_id', e.target.value)}
            />
            <Input
              placeholder='用户名'
              onChange={(e) => setFilter('username', e.target.value)}
            />
            <Input
              placeholder='模型（精确）'
              onChange={(e) => setFilter('model', e.target.value)}
            />
            <Input
              placeholder='渠道 ID'
              onChange={(e) => setFilter('channel_id', e.target.value)}
            />
            <Input
              placeholder='状态码'
              onChange={(e) => setFilter('status', e.target.value)}
            />
            <Input
              placeholder='请求 ID'
              onChange={(e) => setFilter('request_id', e.target.value)}
            />
            <Input
              type='datetime-local'
              onChange={(e) =>
                setFilter(
                  'start_time',
                  e.target.value
                    ? Math.floor(
                        new Date(e.target.value).getTime() / 1000
                      ).toString()
                    : ''
                )
              }
            />
            <Input
              type='datetime-local'
              onChange={(e) =>
                setFilter(
                  'end_time',
                  e.target.value
                    ? Math.floor(
                        new Date(e.target.value).getTime() / 1000
                      ).toString()
                    : ''
                )
              }
            />
          </div>
          <div className='overflow-auto rounded border'>
            <table className='w-full text-sm'>
              <thead>
                <tr className='border-b text-left'>
                  <th className='p-2'>时间</th>
                  <th>用户</th>
                  <th>模型/渠道</th>
                  <th>状态</th>
                  <th>大小</th>
                  <th>命中原因</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr className='border-b' key={item.id}>
                    <td className='p-2'>
                      {new Date(item.created_at * 1000).toLocaleString()}
                    </td>
                    <td>{item.username || item.user_id}</td>
                    <td>
                      {item.model_name}
                      <br />
                      <span className='text-muted-foreground'>
                        {item.channel_name || item.channel_id}
                      </span>
                    </td>
                    <td>
                      {item.status_code} · {item.latency_ms}ms
                    </td>
                    <td>
                      {item.request_body_size}/{item.response_body_size}
                      {item.request_body_truncated ||
                      item.response_body_truncated
                        ? '（截断）'
                        : ''}
                    </td>
                    <td>{item.rule_name || item.decision_source}</td>
                    <td className='space-x-1'>
                      <Button
                        size='sm'
                        variant='outline'
                        onClick={() => setDetailId(item.id)}
                      >
                        查看
                      </Button>
                      <Button
                        size='sm'
                        variant='destructive'
                        onClick={async () => {
                          await deleteLogs([item.id])
                          await qc.invalidateQueries({
                            queryKey: ['context-logs'],
                          })
                        }}
                      >
                        删除
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className='flex items-center justify-between'>
            <span>共 {logs.data?.data?.total ?? 0} 条</span>
            <div className='space-x-2'>
              <Button
                variant='outline'
                disabled={Number(filters.p) <= 1}
                onClick={() =>
                  setFilters((v) => ({ ...v, p: Number(v.p) - 1 }))
                }
              >
                上一页
              </Button>
              <Button
                variant='outline'
                disabled={items.length < Number(filters.page_size)}
                onClick={() =>
                  setFilters((v) => ({ ...v, p: Number(v.p) + 1 }))
                }
              >
                下一页
              </Button>
            </div>
          </div>
        </TabsContent>
        <TabsContent value='rules' className='space-y-3'>
          <div className='flex justify-between'>
            <p className='text-muted-foreground text-sm'>
              优先级：用户+模型 &gt; 用户 &gt; 模型 &gt; 全局；同层 priority
              大者优先，再按 ID 小者优先。模型匹配忽略大小写，* 为通配符。
            </p>
            <Button onClick={() => setEditing({ ...emptyRule })}>
              新增规则
            </Button>
          </div>
          <div className='rounded border'>
            {((rules.data?.data ?? []) as ContextLogRule[]).map((rule) => (
              <div
                key={rule.id}
                className='flex items-center justify-between border-b p-3'
              >
                <div>
                  <b>{rule.name}</b> · {rule.decision} · priority{' '}
                  {rule.priority}
                  <br />
                  <span className='text-muted-foreground text-sm'>
                    用户 {rule.user_id ?? '任意'} / 模型{' '}
                    {rule.model_pattern || '任意'} /{' '}
                    {rule.enabled ? '启用' : '停用'}
                  </span>
                </div>
                <div className='space-x-2'>
                  <Button
                    variant='outline'
                    onClick={() => setEditing({ ...rule })}
                  >
                    编辑
                  </Button>
                  <Button
                    variant='destructive'
                    onClick={async () => {
                      await deleteRule(rule.id)
                      await qc.invalidateQueries({
                        queryKey: ['context-log-rules'],
                      })
                    }}
                  >
                    删除
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </TabsContent>
      </Tabs>
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => !open && setEditing(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>上下文保存规则</DialogTitle>
          </DialogHeader>
          {editing && (
            <div className='space-y-3'>
              <Label>名称</Label>
              <Input
                value={editing.name}
                onChange={(e) =>
                  setEditing({ ...editing, name: e.target.value })
                }
              />
              <Label>用户 ID（空为任意）</Label>
              <Input
                type='number'
                value={editing.user_id ?? ''}
                onChange={(e) =>
                  setEditing({
                    ...editing,
                    user_id: e.target.value ? Number(e.target.value) : null,
                  })
                }
              />
              <Label>模型模式（空为任意，支持 *）</Label>
              <Input
                value={editing.model_pattern}
                onChange={(e) =>
                  setEditing({ ...editing, model_pattern: e.target.value })
                }
              />
              <Label>决策</Label>
              <select
                className='border-input bg-background w-full rounded border p-2'
                value={editing.decision}
                onChange={(e) =>
                  setEditing({
                    ...editing,
                    decision: e.target.value as 'capture' | 'skip',
                  })
                }
              >
                <option value='capture'>capture</option>
                <option value='skip'>skip</option>
              </select>
              <Label>Priority</Label>
              <Input
                type='number'
                value={editing.priority}
                onChange={(e) =>
                  setEditing({ ...editing, priority: Number(e.target.value) })
                }
              />
              <div className='flex gap-2'>
                <Switch
                  checked={editing.enabled}
                  onCheckedChange={(v) =>
                    setEditing({ ...editing, enabled: v })
                  }
                />
                <Label>启用</Label>
              </div>
              <Button onClick={persist}>保存</Button>
            </div>
          )}
        </DialogContent>
      </Dialog>
      <Dialog
        open={detailId !== null}
        onOpenChange={(open) => !open && setDetailId(null)}
      >
        <DialogContent className='max-h-[90vh] max-w-5xl overflow-auto'>
          <DialogHeader>
            <DialogTitle>请求详情 {selected?.request_id}</DialogTitle>
          </DialogHeader>
          {selected && (
            <div className='space-y-3'>
              <p>
                模型：{selected.model_name} · 渠道：
                {selected.channel_name || selected.channel_id} · 状态：
                {selected.status_code} · 规则：
                {selected.rule_name || selected.decision_source}
              </p>
              {selected.error && (
                <pre className='bg-destructive/10 rounded p-2 whitespace-pre-wrap'>
                  {selected.error}
                </pre>
              )}
              {(
                [
                  'request_headers',
                  'request_body',
                  'response_headers',
                  'response_body',
                ] as const
              ).map((key) => (
                <div key={key}>
                  <div className='flex justify-between'>
                    <Label>
                      {key}
                      {((key === 'request_body' &&
                        selected.request_body_truncated) ||
                        (key === 'response_body' &&
                          selected.response_body_truncated)) &&
                        '（已截断）'}
                    </Label>
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={() => copy(String(selected[key] ?? ''))}
                    >
                      复制
                    </Button>
                  </div>
                  <Textarea
                    readOnly
                    className='min-h-32 font-mono'
                    value={selected[key] ?? ''}
                  />
                </div>
              ))}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
