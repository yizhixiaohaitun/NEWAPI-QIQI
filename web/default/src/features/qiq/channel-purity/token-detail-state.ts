/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { TokenSimilarityDetail } from './types'

export function tokenMetricAvailability(detail?: TokenSimilarityDetail) {
  return {
    available: detail?.score_available === true && detail.paired_count > 0,
    partial: Boolean(detail && (detail.baseline_valid_samples !== detail.paired_count || detail.target_valid_samples !== detail.paired_count)),
    outside: detail?.outside_count ?? 0,
  }
}
