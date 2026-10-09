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

export type InputTokenDeductionFormValues = {
  input_token_deduction: string
  input_token_deduction_by_group: Record<string, string>
}

function parseSettingObject(
  setting: string | null | undefined
): Record<string, unknown> {
  if (!setting) return {}
  try {
    const parsed: unknown = JSON.parse(setting)
    if (
      typeof parsed === 'object' &&
      parsed !== null &&
      !Array.isArray(parsed)
    ) {
      return parsed as Record<string, unknown>
    }
  } catch {
    // Invalid legacy settings are handled by the existing JSON validation.
  }
  return {}
}

export function isNonnegativeIntegerInput(
  value: unknown,
  allowEmpty = false
): boolean {
  if (value === '' || value === null || value === undefined) return allowEmpty
  const numericValue = typeof value === 'number' ? value : Number(value)
  return Number.isSafeInteger(numericValue) && numericValue >= 0
}

export function parseInputTokenDeductionFormValues(
  setting: string | null | undefined
): InputTokenDeductionFormValues {
  const parsed = parseSettingObject(setting)
  const defaultValue = isNonnegativeIntegerInput(parsed.input_token_deduction)
    ? String(parsed.input_token_deduction)
    : '0'
  const groupValues: Record<string, string> = {}
  const rawGroupValues = parsed.input_token_deduction_by_group

  if (
    typeof rawGroupValues === 'object' &&
    rawGroupValues !== null &&
    !Array.isArray(rawGroupValues)
  ) {
    for (const [group, value] of Object.entries(rawGroupValues)) {
      if (isNonnegativeIntegerInput(value)) groupValues[group] = String(value)
    }
  }

  return {
    input_token_deduction: defaultValue,
    input_token_deduction_by_group: groupValues,
  }
}

export function buildChannelSettingWithInputTokenDeduction(
  existingSetting: string | null | undefined,
  settingUpdates: Record<string, unknown>,
  defaultInput: unknown,
  groupInputs: Record<string, unknown>
): string {
  if (!isNonnegativeIntegerInput(defaultInput, true)) {
    throw new Error('Input token deduction must be a nonnegative integer')
  }

  const input_token_deduction =
    defaultInput === '' || defaultInput == null ? 0 : Number(defaultInput)
  const input_token_deduction_by_group: Record<string, number> = {}

  for (const [group, value] of Object.entries(groupInputs)) {
    if (value === '' || value === null || value === undefined) continue
    if (!isNonnegativeIntegerInput(value)) {
      throw new Error(
        'Group input token deductions must be nonnegative integers'
      )
    }
    input_token_deduction_by_group[group] = Number(value)
  }

  return JSON.stringify({
    ...parseSettingObject(existingSetting),
    ...settingUpdates,
    input_token_deduction,
    input_token_deduction_by_group,
  })
}
