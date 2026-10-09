/*
Copyright (C) 2025 QuantumNous

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

function parseSettingObject(setting) {
  if (!setting) return {};
  try {
    const parsed = JSON.parse(setting);
    if (
      typeof parsed === 'object' &&
      parsed !== null &&
      !Array.isArray(parsed)
    ) {
      return parsed;
    }
  } catch {
    // Invalid legacy settings are handled by the existing form validation.
  }
  return {};
}

export function isNonnegativeIntegerInput(value, allowEmpty = false) {
  if (value === '' || value === null || value === undefined) return allowEmpty;
  const numericValue = typeof value === 'number' ? value : Number(value);
  return Number.isSafeInteger(numericValue) && numericValue >= 0;
}

export function parseInputTokenDeductionFormValues(setting) {
  const parsed = parseSettingObject(setting);
  const inputTokenDeduction = isNonnegativeIntegerInput(
    parsed.input_token_deduction,
  )
    ? String(parsed.input_token_deduction)
    : '0';
  const inputTokenDeductionByGroup = {};
  const rawGroupValues = parsed.input_token_deduction_by_group;

  if (
    typeof rawGroupValues === 'object' &&
    rawGroupValues !== null &&
    !Array.isArray(rawGroupValues)
  ) {
    Object.entries(rawGroupValues).forEach(([group, value]) => {
      if (isNonnegativeIntegerInput(value)) {
        inputTokenDeductionByGroup[group] = String(value);
      }
    });
  }

  return {
    input_token_deduction: inputTokenDeduction,
    input_token_deduction_by_group: inputTokenDeductionByGroup,
  };
}

export function buildChannelSettingWithInputTokenDeduction(
  existingSetting,
  settingUpdates,
  defaultInput,
  groupInputs,
) {
  if (!isNonnegativeIntegerInput(defaultInput, true)) {
    throw new Error('Input token deduction must be a nonnegative integer');
  }

  const inputTokenDeduction =
    defaultInput === '' || defaultInput == null ? 0 : Number(defaultInput);
  const inputTokenDeductionByGroup = {};

  Object.entries(groupInputs).forEach(([group, value]) => {
    if (value === '' || value === null || value === undefined) return;
    if (!isNonnegativeIntegerInput(value)) {
      throw new Error(
        'Group input token deductions must be nonnegative integers',
      );
    }
    inputTokenDeductionByGroup[group] = Number(value);
  });

  return JSON.stringify({
    ...parseSettingObject(existingSetting),
    ...settingUpdates,
    input_token_deduction: inputTokenDeduction,
    input_token_deduction_by_group: inputTokenDeductionByGroup,
  });
}
