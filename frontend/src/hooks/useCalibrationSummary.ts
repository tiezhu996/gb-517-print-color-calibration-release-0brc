import { useCallback, useEffect, useState } from 'react';
import { getCalibrationSummary } from '../api/color-proof';
import type { CalibrationSummary } from '../types/domain';

const EMPTY_SUMMARY: CalibrationSummary = {
  totalProofs: 0,
  acceptedProofs: 0,
  openBlocked: 0,
  groups: [],
};

// useCalibrationSummary 读取漂移门禁校准摘要；刷新工作台或手动触发 refresh 后
// 重新拉取，保证复核员看到最新的基准与待复核阻断计数。
export function useCalibrationSummary(enabled = true) {
  const [summary, setSummary] = useState<CalibrationSummary>(EMPTY_SUMMARY);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const result = await getCalibrationSummary();
      setSummary(result.data);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : String(loadError));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) {
      void refresh();
    }
  }, [enabled, refresh]);

  return { summary, loading, error, refresh };
}
