import { useCalibrationSummary } from '../../hooks/useCalibrationSummary';
import { MetricCard } from './MetricCard';
import { EmptyState } from './EmptyState';
import { UiButton } from './UiButton';

const CATEGORY_TOLERANCES: Array<{ category: string; tolerance: number }> = [
  { category: '常规', tolerance: 1.5 },
  { category: '重点', tolerance: 1.0 },
  { category: '复核', tolerance: 0.8 },
];

function formatNumber(value: number | null | undefined): string {
  if (value === null || value === undefined) return '—';
  return value.toFixed(2);
}

// CalibrationGatePanel 展示漂移门禁的只读校准摘要：类别容差、各「关联编码 + 类别」
// 基准组的中位数基准，以及仍待复核的阻断数量。
export function CalibrationGatePanel() {
  const { summary, loading, error, refresh } = useCalibrationSummary(true);

  return <section className="color-panel gate-panel" aria-label="漂移门禁校准摘要" aria-busy={loading}>
    <header>
      <div>
        <span className="eyebrow">DRIFT GATE</span>
        <h2>漂移门禁校准摘要</h2>
        <small>按关联编码与类别取最近五份已接受校样的中位数作为基准</small>
      </div>
      <UiButton onClick={() => void refresh()}>刷新摘要</UiButton>
    </header>
    <div className="gate-body">
      <div className="metrics gate-metrics">
        <MetricCard label="校样总数" value={summary.totalProofs} detail="全部色彩校样" />
        <MetricCard label="已接受基准" value={summary.acceptedProofs} detail="进入基准池的校样" />
        <MetricCard label="待复核阻断" value={summary.openBlocked} detail="漂移超限或已被取代" />
      </div>
      <div className="gate-legend">
        {CATEGORY_TOLERANCES.map((entry) =>
          <span key={entry.category} className="gate-legend-item">{entry.category}容差 ±{entry.tolerance}</span>)}
      </div>
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="color-table">
        <table>
          <thead><tr><th>关联编码</th><th>类别</th><th>基准（中位数）</th><th>容差</th><th>基准样本</th><th>已接受</th><th>待复核阻断</th></tr></thead>
          <tbody>
            {summary.groups.map((group) =>
              <tr key={`${group.relatedCode}-${group.category}`} className={group.openBlocked > 0 ? 'gate-row--blocked' : ''}>
                <td><strong>{group.relatedCode || '—'}</strong></td>
                <td>{group.category}</td>
                <td>{group.sampleSize > 0 ? formatNumber(group.baseline) : '等待首份接受'}</td>
                <td>±{group.tolerance}</td>
                <td>{group.sampleSize || 0} 份</td>
                <td>{group.accepted}</td>
                <td>{group.openBlocked > 0 ? <span className="status status--danger">{group.openBlocked} 待复核</span> : <span className="status status--success">正常</span>}</td>
              </tr>)}
            {!summary.groups.length && !loading && <tr><td colSpan={7}><EmptyState title="暂无基准组" detail="接受校样后将自动建立漂移基准" /></td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  </section>;
}
