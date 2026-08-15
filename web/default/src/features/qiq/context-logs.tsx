import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
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
import { getSystemOptions } from '@/features/system-settings/api'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'

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

function formatDetailText(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

const detailSections = [
  { key: 'request_headers', label: '请求 Headers', kind: 'headers' },
  { key: 'response_headers', label: '响应 Headers', kind: 'headers' },
  { key: 'request_body', label: '请求正文', kind: 'body' },
  { key: 'response_body', label: '响应正文', kind: 'body' },
] as const
export function ContextLogs() {
  const qc = useQueryClient()
  const [filters, setFilters] = useState<Record<string, string | number>>({
    p: 1,
    page_size: 20,
  })
  const [editing, setEditing] = useState<Partial<ContextLogRule> | null>(null)
  const [detailId, setDetailId] = useState<number | null>(null)
  const [retentionDays, setRetentionDays] = useState<number | null>(null)
  const updateOption = useUpdateOption()
  const options = useQuery({
    queryKey: ['system-options'],
    queryFn: getSystemOptions,
  })
  const configuredRetentionDays = Number(
    options.data?.data?.find(
      (option) =>
        option.key === 'qiqi_setting.context_request_log_retention_days'
    )?.value ?? 0
  )
  const displayedRetentionDays = retentionDays ?? configuredRetentionDays
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
    <SectionPageLayout>
      <SectionPageLayout.Title>上下文日志</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='min-w-0 space-y-4'>
          <p className='text-muted-foreground text-sm'>
            仅 Root 可访问。正文按每侧 2 MiB 上限捕获，截断项会明确标记。
          </p>
      <div className='rounded-lg border p-4'>
        <div className='flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between'>
          <div className='min-w-0 flex-1 space-y-1'>
            <Label htmlFor='context-log-retention'>自动清理保留天数</Label>
            <p className='text-muted-foreground text-sm'>
              0 表示永久保留；N 表示自动删除 N
              天前的上下文日志。仅影响上下文日志，不影响普通系统日志。 SQL
              日志库每日分批清理；ClickHouse 使用该设置动态维护本表 TTL。
            </p>
          </div>
          <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto sm:flex-nowrap'>
            <Input
              id='context-log-retention'
              className='min-w-28 flex-1 sm:w-32 sm:flex-none'
              type='number'
              min={0}
              max={3650}
              value={displayedRetentionDays}
              onChange={(e) => setRetentionDays(Number(e.target.value))}
            />
            <Button
              className='shrink-0'
              disabled={
                updateOption.isPending ||
                !Number.isInteger(displayedRetentionDays) ||
                displayedRetentionDays < 0 ||
                displayedRetentionDays > 3650
              }
              onClick={async () => {
                await updateOption.mutateAsync({
                  key: 'qiqi_setting.context_request_log_retention_days',
                  value: String(displayedRetentionDays),
                })
                setRetentionDays(null)
              }}
            >
              保存保留策略
            </Button>
          </div>
        </div>
      </div>
      <Tabs defaultValue='logs'>
        <TabsList>
          <TabsTrigger value='logs'>日志</TabsTrigger>
          <TabsTrigger value='rules'>保存规则</TabsTrigger>
        </TabsList>
        <TabsContent value='logs' className='space-y-3'>
          <div className='grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-4'>
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
          <div className='max-w-full overflow-x-auto rounded border'>
            <table className='w-full min-w-[62rem] text-sm'>
              <thead>
                <tr className='border-b text-left'>
                  <th className='whitespace-nowrap p-2'>时间</th>
                  <th className='px-2'>用户</th>
                  <th className='px-2'>模型/渠道</th>
                  <th className='whitespace-nowrap px-2'>状态</th>
                  <th className='whitespace-nowrap px-2'>大小</th>
                  <th className='px-2'>命中原因</th>
                  <th className='bg-background sticky right-0 px-2'>操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr className='border-b' key={item.id}>
                    <td className='p-2 whitespace-nowrap'>
                      {new Date(item.created_at * 1000).toLocaleString()}
                    </td>
                    <td className='px-2'>{item.username || item.user_id}</td>
                    <td className='px-2'>
                      {item.model_name}
                      <br />
                      <span className='text-muted-foreground'>
                        {item.channel_name || item.channel_id}
                      </span>
                    </td>
                    <td className='px-2 whitespace-nowrap'>
                      {item.status_code} · {item.latency_ms}ms
                    </td>
                    <td className='px-2 whitespace-nowrap'>
                      {item.request_body_size}/{item.response_body_size}
                      {item.request_body_truncated ||
                      item.response_body_truncated
                        ? '（截断）'
                        : ''}
                    </td>
                    <td className='px-2'>
                      {item.rule_name || item.decision_source}
                    </td>
                    <td className='bg-background sticky right-0 space-x-1 px-2 whitespace-nowrap shadow-[-8px_0_8px_-8px_rgb(0_0_0_/_0.2)]'>
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
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <span>共 {logs.data?.data?.total ?? 0} 条</span>
            <div className='flex gap-2'>
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
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <p className='text-muted-foreground min-w-0 flex-1 text-sm'>
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
                className='flex flex-wrap items-center justify-between gap-3 border-b p-3'
              >
                <div className='min-w-0 flex-1'>
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
        <DialogContent className='flex h-[min(90vh,56rem)] w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] flex-col overflow-hidden p-3 sm:w-[96vw] sm:max-w-6xl sm:p-5'>
          <DialogHeader className='min-w-0 shrink-0 pr-10'>
            <DialogTitle className='truncate'>请求详情</DialogTitle>
          </DialogHeader>
          {detail.isPending && (
            <div className='text-muted-foreground flex min-h-32 items-center justify-center'>
              正在读取详情…
            </div>
          )}
          {detail.isError && (
            <div className='bg-destructive/10 text-destructive rounded-lg p-3'>
              读取详情失败，请稍后重试。
            </div>
          )}
          {selected && (
            <div className='min-h-0 flex-1 space-y-4 overflow-y-auto pr-1'>
              <div className='bg-muted/20 grid min-w-0 gap-2 rounded-lg border p-3 text-sm sm:grid-cols-2 xl:grid-cols-4'>
                <div className='min-w-0'>
                  <span className='text-muted-foreground'>请求 ID</span>
                  <code className='mt-1 block font-mono text-xs break-all'>
                    {selected.request_id}
                  </code>
                </div>
                <div className='min-w-0'>
                  <span className='text-muted-foreground'>模型 / 渠道</span>
                  <span className='mt-1 block break-words'>
                    {selected.model_name} /{' '}
                    {selected.channel_name || selected.channel_id}
                  </span>
                </div>
                <div>
                  <span className='text-muted-foreground'>状态 / 延迟</span>
                  <span className='mt-1 block'>
                    {selected.status_code} / {selected.latency_ms}ms
                  </span>
                </div>
                <div className='min-w-0'>
                  <span className='text-muted-foreground'>命中规则</span>
                  <span className='mt-1 block break-words'>
                    {selected.rule_name || selected.decision_source}
                  </span>
                </div>
              </div>
              {selected.error && (
                <div className='min-w-0'>
                  <Label className='mb-2 block'>错误</Label>
                  <pre className='bg-destructive/10 max-h-48 overflow-auto rounded-lg p-3 font-mono text-xs break-words whitespace-pre-wrap'>
                    {selected.error}
                  </pre>
                </div>
              )}
              <div className='grid min-w-0 gap-4 xl:grid-cols-2'>
                {detailSections.map(({ key, label, kind }) => {
                  const raw = String(selected[key] ?? '')
                  const display =
                    kind === 'headers' ? formatDetailText(raw) : raw
                  const truncated =
                    (key === 'request_body' &&
                      selected.request_body_truncated) ||
                    (key === 'response_body' &&
                      selected.response_body_truncated)
                  return (
                    <section
                      key={key}
                      className='bg-muted/10 min-w-0 overflow-hidden rounded-lg border'
                    >
                      <div className='flex min-w-0 items-center justify-between gap-2 border-b px-3 py-2'>
                        <Label className='min-w-0 truncate'>
                          {label}
                          {truncated ? '（已截断）' : ''}
                        </Label>
                        <Button
                          className='shrink-0'
                          size='sm'
                          variant='ghost'
                          onClick={() => copy(display)}
                        >
                          复制
                        </Button>
                      </div>
                      <pre
                        className={`min-h-32 overflow-auto p-3 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap ${kind === 'body' ? 'max-h-[min(42vh,36rem)]' : 'max-h-[min(34vh,28rem)]'}`}
                      >
                        {display || '（空）'}
                      </pre>
                    </section>
                  )
                })}
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
