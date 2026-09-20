import { useEffect, useState } from 'react';
import { getCalibrationSummary } from '../../api/color-proof';
import type { CalibrationSummary as CalibrationSummaryData } from '../../types/domain';
import { formatMetric } from '../../utils/format';

export function CalibrationSummaryPanel({ refreshKey }: { refreshKey: number }) {
  const [summary, setSummary] = useState<CalibrationSummaryData | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    getCalibrationSummary()
      .then((result) => { if (active) setSummary(result.data); })
      .catch((reason) => { if (active) setError(reason instanceof Error ? reason.message : String(reason)); });
    return () => { active = false; };
  }, [refreshKey]);

  if (error) return <div className="alert" role="alert">校准摘要刷新失败：{error}</div>;
  if (!summary) return <div className="calibration-panel" aria-busy="true">正在读取校准摘要…</div>;

  return <section className="calibration-panel" aria-label="校准漂移摘要">
    <header>
      <div>
        <span className="eyebrow">DRIFT GATE</span>
        <h2>校准漂移摘要</h2>
      </div>
      <small>按关联编码与类别取最近五份已接受校样中位数为基准</small>
    </header>
    <div className="calibration-metrics">
      <span><strong>{summary.totalProofs}</strong> 校样总数</span>
      <span><strong>{summary.acceptedProofs}</strong> 已接受</span>
      <span className="is-warning"><strong>{summary.pendingProofs}</strong> 漂移待复核</span>
      <span className="is-danger"><strong>{summary.supersededProofs}</strong> 已被更新校样取代</span>
    </div>
    <div className="calibration-table">
      <table>
        <thead><tr><th>关联编码</th><th>类别</th><th>基准中位数</th><th>容差</th><th>取样</th><th>待复核</th><th>已取代</th><th>最新偏差</th></tr></thead>
        <tbody>
          {summary.groups.map((group) => (
            <tr key={`${group.relatedCode}-${group.category}`} className={group.latestBlocked ? 'is-blocked' : ''}>
              <td><strong>{group.relatedCode || '-'}</strong></td>
              <td>{group.category}</td>
              <td>{group.baseline === undefined ? '基准不足' : group.baseline.toFixed(2)}</td>
              <td>{group.tolerance.toFixed(1)}</td>
              <td>{group.sampleSize}/5</td>
              <td>{group.pendingProofs}</td>
              <td>{group.supersededProofs}</td>
              <td>{formatMetric(group.latestDeviation)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  </section>;
}
