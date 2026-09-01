/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'

export function parseLogOther(other: string): LogOtherData | null {
  if (!other) return null
  try {
    return JSON.parse(other) as LogOtherData
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to parse log other field:', error)
    return null
  }
}

const parsedLogOtherCache = new WeakMap<
  UsageLog,
  { raw: string; parsed: LogOtherData | null }
>()

/** Parse a row payload once while allowing paginated row objects to be collected. */
export function parseUsageLogOther(log: UsageLog): LogOtherData | null {
  const cached = parsedLogOtherCache.get(log)
  if (cached?.raw === log.other) return cached.parsed

  const parsed = parseLogOther(log.other)
  parsedLogOtherCache.set(log, { raw: log.other, parsed })
  return parsed
}

/** Closed details must not construct a dialog subtree for every table row. */
export function shouldMountUsageLogDetails(open: boolean): boolean {
  return open
}
