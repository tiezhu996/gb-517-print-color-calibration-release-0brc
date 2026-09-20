
export interface DomainRecord {
  id: number;
  code: string;
  name: string;
  status: string;
  version: number;
  description: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence: string;
  relatedCode: string;
  createdAt: string;
  updatedAt: string;
  revisions?: RevisionRecord[];
  // 漂移门禁快照，由后端在创建、编辑与接受前统一重算。
  gateBaseline?: number | null;
  gateDeviation?: number | null;
  gateTolerance?: number | null;
  gateSampleSize?: number;
  gateStatus?: 'pending' | 'passed' | 'blocked' | string;
  gateBlockReason?: string;
  supersededById?: number | null;
  superseded?: boolean;
}

export interface CalibrationGroupSummary {
  relatedCode: string;
  category: string;
  tolerance: number;
  baseline: number;
  sampleSize: number;
  accepted: number;
  openBlocked: number;
}

export interface CalibrationSummary {
  totalProofs: number;
  acceptedProofs: number;
  openBlocked: number;
  groups: CalibrationGroupSummary[];
}

export interface RevisionRecord {
  id: number; version: number; status: string; name: string; metricValue: number;
  metricUnit: string; evidence: string; actor: string; requestId: string; reason: string; createdAt: string;
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta }
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; detail: string; createdAt: string;
}
export interface EntityConfig { key: string; path: string; label: string; statuses: readonly string[] }
