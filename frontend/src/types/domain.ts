
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
  driftBaseline?: number;
  driftDeviation?: number;
  driftTolerance?: number;
  driftSampleSize?: number;
  driftBlocked?: boolean;
  driftBlockReason?: string;
  driftEvaluatedAt?: string;
  createdAt: string;
  updatedAt: string;
  revisions?: RevisionRecord[];
}

export interface CalibrationGroupSummary {
  relatedCode: string;
  category: string;
  baseline?: number;
  tolerance: number;
  sampleSize: number;
  pendingProofs: number;
  supersededProofs: number;
  latestProofId: number;
  latestDeviation?: number;
  latestBlocked: boolean;
}

export interface CalibrationSummary {
  totalProofs: number;
  acceptedProofs: number;
  pendingProofs: number;
  blockedProofs: number;
  supersededProofs: number;
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
