
export function formatDate(value: string): string {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-';
}
export function nextStatus(current: string, statuses: readonly string[]): string | null {
  const index = statuses.indexOf(current);
  return index >= 0 && index < statuses.length - 1 ? statuses[index + 1] : null;
}
export function statusTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (/approved|accepted|released|completed|signed|closed|pass|ready|online|cleared|succeeded/.test(status)) return 'success';
  if (/failed|rejected|critical|scrap|discard|revoked|urgent/.test(status)) return 'danger';
  if (/hold|warning|review|pending|restricted|limited|quarantine/.test(status)) return 'warning';
  return 'neutral';
}

export const DRIFT_TOLERANCE: Record<string, number> = { '常规': 1.5, '重点': 1.0, '复核': 0.8 };

export function driftTolerance(category: string): number {
  return DRIFT_TOLERANCE[category] ?? DRIFT_TOLERANCE['常规'];
}

export function formatMetric(value: number | undefined, unit?: string): string {
  if (value === undefined || Number.isNaN(value)) return '-';
  return `${value.toFixed(2)}${unit ? ` ${unit}` : ''}`;
}

export const PROOF_STATUS_LABEL: Record<string, string> = {
  captured: '已采集',
  review: '待复核',
  review_pending: '漂移待复核',
  accepted: '已接受',
  rejected: '已拒绝',
};
