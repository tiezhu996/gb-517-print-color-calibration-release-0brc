import { useEffect, useMemo, useState } from 'react';
import { request } from '../api/client';
import { roleAtLeast, useAuth } from '../hooks/useAuth';
import { usePagination } from '../hooks/usePagination';
import type { EntityConfig, DomainRecord } from '../types/domain';
import type { RunState } from '../types/status';
import type { EntityStore } from '../stores/factory';
import { formatDate } from '../utils/format';
import { StatusBadge } from './common/StatusBadge';
import { RunStateBadge } from './common/RunStateBadge';
import { ColorTable } from './common/ColorTable';
import { CalibrationSummaryPanel } from './common/CalibrationSummaryPanel';
import { EmptyState } from './common/EmptyState';
import { MetricCard } from './common/MetricCard';
import { ConfirmDialog } from './common/ConfirmDialog';
import { UiButton } from './common/UiButton';

function decisionRunState(status: string): RunState {
  if (status === 'release') return 'released';
  if (status === 'rework' || status === 'quarantine') return 'hold';
  return 'proofing';
}

function nextPermittedStatus(config: EntityConfig, current: string, reviewer: boolean): string | null {
  const transitions: Record<string, Record<string, string | null>> = {
    pressUnit: { ready: 'setup', setup: 'printing', printing: 'maintenance', maintenance: 'printing' },
    printRun: { setup: 'printing', printing: 'proofing', proofing: reviewer ? 'released' : 'hold', hold: 'proofing', released: reviewer ? 'hold' : null },
    colorProof: { captured: 'review', review: reviewer ? 'accepted' : null, accepted: reviewer ? 'review' : null, rejected: reviewer ? 'review' : null },
    releaseDecision: { draft: reviewer ? 'release' : 'rework', release: reviewer ? 'rework' : null, rework: reviewer ? 'release' : null, quarantine: reviewer ? 'rework' : null },
  };
  return transitions[config.key]?.[current] ?? null;
}

export function EntityPage({ config, useStore }: { config: EntityConfig; useStore: EntityStore }) {
  const { session } = useAuth();
  const { items, meta, loading, error, load, createRecord, transition } = useStore();
  const [search, setSearch] = useState('');
  const [submittedSearch, setSubmittedSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [pending, setPending] = useState<{ item: DomainRecord; status: string } | null>(null);
  const [detail, setDetail] = useState<DomainRecord | null>(null);
  const [summaryRefresh, setSummaryRefresh] = useState(0);
  const { page, pageSize, pages, setPage, previous, next } = usePagination(meta.total);
  const canWrite = roleAtLeast(session?.role, 'operator');
  const canReview = roleAtLeast(session?.role, 'reviewer');
  const isColorProof = config.key === 'colorProof';

  useEffect(() => { void load(config.path, submittedSearch, page, pageSize); }, [config.path, load, page, pageSize, submittedSearch]);
  const highRisk = useMemo(() => items.filter((item) => ['high', 'critical'].includes(item.riskLevel)).length, [items]);
  const createDemo = async () => {
    const now = Date.now();
    await createRecord(config.path, { code: `${config.key.toUpperCase()}-${now.toString().slice(-6)}`, name: `新增${config.label}`,
      description: '通过前端工作台创建的业务记录', facility: '默认作业区', owner: session?.username || 'operator', category: '常规', riskLevel: 'medium',
      metricValue: 2.4, metricUnit: 'ΔE', effectiveAt: new Date().toISOString(), evidence: '已完成创建前色彩检查', relatedCode: 'PR-001' });
    setShowCreate(false);
    if (isColorProof) setSummaryRefresh((value) => value + 1);
  };
  const openDetail = async (item: DomainRecord) => {
    try { setDetail((await request<DomainRecord>(`/${config.path}/${item.id}`)).data); }
    catch { setDetail(item); }
  };
  const runTransition = async (item: DomainRecord, status: string) => {
    try {
      await transition(config.path, item, status);
      if (isColorProof) setSummaryRefresh((value) => value + 1);
    } catch { /* 错误已写入 store 并显示在告警条 */ }
  };

  return <main className="workspace">
    <header className="page-header"><div><p className="eyebrow">业务工作台</p><h1>{config.label}</h1><p>统一管理{config.label}的状态、风险、证据与责任人。</p></div>{canWrite && <UiButton onClick={() => setShowCreate(true)}>新增{config.label}</UiButton>}</header>
    <section className="metrics"><MetricCard label="记录总数" value={meta.total} detail="当前筛选范围"/><MetricCard label="高风险" value={highRisk} detail="需要优先复核"/><MetricCard label="状态种类" value={new Set(items.map((item) => item.status)).size} detail="状态机覆盖"/></section>
    {(isColorProof || config.key === 'releaseDecision') && <ColorTable records={items} title={isColorProof ? '当前校样读数' : '放行依据读数'} />}
    {isColorProof && <CalibrationSummaryPanel refreshKey={summaryRefresh} />}
    <section className="toolbar"><input aria-label="搜索" placeholder={`搜索${config.label}编码或名称`} value={search} onChange={(event) => setSearch(event.target.value)} /><UiButton onClick={() => { setPage(1); setSubmittedSearch(search); }}>查询</UiButton><button className="link-button" onClick={() => { setSearch(''); setSubmittedSearch(''); setPage(1); }}>重置</button>{isColorProof && <button className="link-button" onClick={() => setSummaryRefresh((value) => value + 1)}>刷新校准摘要</button>}</section>
    {error && <div className="alert" role="alert">{error}</div>}
    <section className="table-shell" aria-busy={loading}><table><thead><tr><th>编码</th><th>名称</th><th>状态</th><th>风险</th><th>责任人</th><th>指标</th><th>更新时间</th><th>操作</th></tr></thead><tbody>
      {items.map((item) => { const target = nextPermittedStatus(config, item.status, canReview); const driftPending = isColorProof && item.status === 'review_pending'; return <tr key={item.id} className={driftPending ? 'is-drift-pending' : ''}><td><strong>{item.code}</strong></td><td><button className="record-link" onClick={() => void openDetail(item)}>{item.name}</button><small>{item.facility}</small></td><td>{config.key === 'printRun' ? <RunStateBadge state={item.status as RunState}/> : <StatusBadge status={item.status}/>} {config.key === 'releaseDecision' && <RunStateBadge state={decisionRunState(item.status)}/>}{driftPending && item.driftBlockReason && <small className="drift-reason" title={item.driftBlockReason}>漂移阻断：{item.driftBlockReason}</small>}</td><td>{item.riskLevel}</td><td>{item.owner}</td><td>{item.metricValue} {item.metricUnit}{isColorProof && item.driftDeviation !== undefined && <small className={item.driftBlocked ? 'drift-reason' : 'drift-ok'}>偏差 {item.driftDeviation.toFixed(2)} / 容差 {item.driftTolerance?.toFixed(1)}</small>}</td><td>{formatDate(item.updatedAt)}</td><td>{canWrite && target ? <button className="table-action" onClick={() => setPending({ item, status: target })}>推进至 {target}</button> : null}{driftPending && canReview && <button className="table-action" onClick={() => void runTransition(item, 'review')}>复核前重算</button>}{(!canWrite || !target) && !driftPending && <button className="table-action" onClick={() => void openDetail(item)}>查看详情</button>}</td></tr>; })}
      {!items.length && !loading && <tr><td colSpan={8}><EmptyState title="没有匹配记录" detail="可清空搜索条件后重新查询" /></td></tr>}
    </tbody></table>{loading && <div className="loading">正在同步业务数据…</div>}</section>
    <footer className="pagination"><button onClick={previous} disabled={page <= 1}>上一页</button><span>第 {page} / {pages} 页</span><button onClick={next} disabled={page >= pages}>下一页</button></footer>
    <ConfirmDialog open={showCreate} title={`新增${config.label}`} onCancel={() => setShowCreate(false)} onConfirm={() => void createDemo()}><p>将创建一条包含完整责任人、风险和证据信息的演示记录。</p></ConfirmDialog>
    <ConfirmDialog open={Boolean(pending)} title="确认状态迁移" onCancel={() => setPending(null)} onConfirm={() => { if (pending) void runTransition(pending.item, pending.status).then(() => setPending(null)); }}><p>状态迁移会写入审计日志；色彩配置和放行决定同时生成不可变版本。</p><strong>{pending?.item.status} → {pending?.status}</strong>{pending?.item.status === 'review_pending' && <p className="drift-reason">复核接受前会按最近五份已接受校样重算漂移门禁；仍超限或已被更新校样取代时将被阻断。</p>}</ConfirmDialog>
    <ConfirmDialog open={Boolean(detail)} title={`${detail?.code || ''} 记录详情`} onCancel={() => setDetail(null)} onConfirm={() => setDetail(null)}>{detail && <div className="detail-content"><p>{detail.description}</p><dl><div><dt>证据</dt><dd>{detail.evidence || '-'}</dd></div><div><dt>当前版本</dt><dd>v{detail.version}</dd></div>{isColorProof && detail.driftEvaluatedAt && <><div><dt>漂移基准（最近五份已接受中位数）</dt><dd>{detail.driftBaseline === undefined ? '基准不足' : detail.driftBaseline.toFixed(2)}</dd></div><div><dt>漂移偏差</dt><dd>{detail.driftDeviation === undefined ? '-' : detail.driftDeviation.toFixed(2)}</dd></div><div><dt>类别容差</dt><dd>{detail.driftTolerance === undefined ? '-' : detail.driftTolerance.toFixed(1)}（取样 {detail.driftSampleSize} 份）</dd></div>{detail.driftBlocked && <div><dt>阻断原因</dt><dd className="drift-reason">{detail.driftBlockReason}</dd></div>}</>}</dl><ColorTable records={[detail]} title="记录色彩读数" />{detail.revisions?.length ? <div className="revision-list"><h3>版本链</h3>{detail.revisions.map((revision) => <article key={revision.id}><strong>v{revision.version} · {revision.status}</strong><span>{revision.actor} · {revision.reason}</span><code>{revision.requestId}</code></article>)}</div> : null}</div>}</ConfirmDialog>
  </main>;
}
