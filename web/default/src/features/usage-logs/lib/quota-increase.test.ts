// @ts-expect-error Bun supplies its test runner at runtime.
import { expect, test } from 'bun:test'

import en from '@/i18n/locales/en.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import { LOG_TYPE_ENUM, LOG_TYPE_FILTERS, LOG_TYPES } from '../constants'
import { buildApiParams, getLogTypeConfig } from './utils'

test('quota increase has its own display and filter without changing existing types', () => {
  expect(LOG_TYPE_ENUM.QUOTA_INCREASE).toBe(8)
  expect(LOG_TYPE_ENUM.MANAGE).toBe(3)
  expect(LOG_TYPE_ENUM.TOPUP).toBe(1)
  expect(getLogTypeConfig(8).label).toBe('Quota Increase')
  expect(LOG_TYPE_FILTERS.filter((option) => option.value === '8')).toEqual([
    { label: 'Quota Increase', value: '8' },
  ])
  expect(LOG_TYPES.filter((option) => option.value === 8)).toHaveLength(1)
  expect(en.translation['Quota Increase']).toBe('Quota Increase')
  expect(zh.translation['Quota Increase']).toBe('额度增加')
  expect(zhTW.translation['Quota Increase']).toBe('額度增加')
})

test('quota increase search and toolbar request the real type with pagination', () => {
  for (const isAdmin of [false, true]) {
    const search = buildApiParams({
      page: 2,
      pageSize: 20,
      isAdmin,
      searchParams: { type: ['8'], requestId: 'quota-audit-request' },
    })
    expect(search.type).toBe(8)
    expect(search.zero_reply).toBeUndefined()
    expect(search.request_id).toBe('quota-audit-request')
    expect(search.p).toBe(2)
    expect(search.page_size).toBe(20)
    const toolbar = buildApiParams({
      page: 1,
      pageSize: 10,
      isAdmin,
      searchParams: {},
      columnFilters: [{ id: 'type', value: ['8'] }],
    })
    expect(toolbar.type).toBe(8)
    expect(toolbar.zero_reply).toBeUndefined()
  }
})
