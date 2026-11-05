// Public HTTP API v1 types; independent of the backend domain model.
export interface ApiResult {
  plan: ApiPlan;
  analysis: ApiAnalysis;
  reports: (ApiReport)[];
  repositories: Record<string, RepositorymanagerRevision>;
}
export interface ApiPlan {
  status: string;
  entries: (ApiEntry)[];
  blockers: (ParserBlocker)[];
  removals: (ParserRemoval)[];
  conflict_summary: ParserConflictSummary | null;
  summary: ParserSummary | null;
  diagnostics: (ParserDiagnostic)[];
}
export interface ApiEntry {
  package: ParserPackage;
  operation: string;
  new_slot: boolean;
  unstable_marker: boolean;
  target: ParserTarget;
  installed: ParserInstalledContext;
  flag_groups: (ParserFlagGroup)[];
  download: ParserSize | null;
  source_lines: (number)[];
}
export interface ParserPackage {
  category: string;
  name: string;
}
export interface ParserTarget {
  version: string;
  repository: string | null;
  slot: string | null;
  subslot: string | null;
}
export interface ParserInstalledContext {
  versions: (ParserInstalledVersion)[];
}
export interface ParserInstalledVersion {
  version: string;
  repository: string | null;
  slot: string | null;
  subslot: string | null;
}
export interface ParserFlagGroup {
  name: string;
  values: (ParserFlagValue)[];
}
export interface ParserFlagValue {
  name: string;
  state: string;
  raw_token: string;
  markers: (string)[];
}
export interface ParserSize {
  value: number;
  unit: string;
}
export interface ParserBlocker {
  atom: string;
  constraint_atom: string | null;
  marker: string;
  resolution: string;
  strength: string;
  parents: (string)[];
  raw_line: string;
  source_lines: (number)[];
}
export interface ParserRemoval {
  package: ParserPackage;
  target: ParserTarget;
  flag_groups: (ParserFlagGroup)[];
  raw_line: string;
  source_lines: (number)[];
}
export interface ParserConflictSummary {
  blockers: number;
  unresolved: number;
  source_line: number;
}
export interface ParserSummary {
  packages: number;
  upgrades: number;
  downgrades: number;
  uninstalls: number;
  new_slots: number;
  download: ParserSize;
  source_line: number;
}
export interface ParserDiagnostic {
  code: string;
  message: string;
  source_lines: (number)[];
}
export interface ApiAnalysis {
  status: string;
  diagnostics: (ParserDiagnostic)[];
  findings: (ApiFinding)[];
}
export interface ApiFinding {
  rule_id: string;
  category: string;
  message: string;
  entry_index: number | null;
  evidence: FindingEvidence | null;
}
export interface ApiReport {
  report: ChangereportReport;
  before_lookup: ApiLookup;
  after_lookup: ApiLookup;
}
export interface ChangereportReport {
  entry_index: number;
  source_lines: (number)[];
  parser_status: string;
  parser_diagnostics: (ParserDiagnostic)[];
  gentoo: ChangereportGentooSection;
  effective_config: ChangereportAuxiliarySection;
  upstream: ChangereportAuxiliarySection;
  interpretation_limitation: string;
}
export interface ChangereportGentooSection {
  status: string;
  comparison: MetadatadiffResult | null;
  diagnostics: (ChangereportBindingDiagnostic)[];
}
export interface MetadatadiffResult {
  status: string;
  compared_fields: (string)[];
  changes: MetadatadiffChanges;
  limitations: (MetadatadiffLimitation)[];
  before: MetadatadiffSnapshotLink;
  after: MetadatadiffSnapshotLink;
}
export interface MetadatadiffChanges {
  eapi: MetadatadiffValueChange | null;
  slot: MetadatadiffValueChange | null;
  subslot: MetadatadiffValueChange | null;
  iuse: MetadatadiffIUSEChanges;
}
export interface MetadatadiffValueChange {
  before: string;
  after: string;
  evidence: MetadatadiffEvidencePair;
}
export interface MetadatadiffEvidencePair {
  before: MetadatadiffEvidenceRef;
  after: MetadatadiffEvidenceRef;
}
export interface MetadatadiffEvidenceRef {
  side: string;
  field: string;
  lines: (number)[];
  whole_snapshot: boolean;
}
export interface MetadatadiffIUSEChanges {
  added: (MetadatadiffIUSEFlagChange)[];
  removed: (MetadatadiffIUSEFlagChange)[];
  default_changes: (MetadatadiffIUSEDefaultChange)[];
}
export interface MetadatadiffIUSEFlagChange {
  name: string;
  default: string;
  evidence: MetadatadiffEvidencePair;
}
export interface MetadatadiffIUSEDefaultChange {
  name: string;
  before: string;
  after: string;
  evidence: MetadatadiffEvidencePair;
}
export interface MetadatadiffLimitation {
  code: string;
  field: string;
  side: string;
  evidence: (MetadatadiffEvidenceRef)[];
}
export interface MetadatadiffSnapshotLink {
  source: MetadatadiffSource;
  sha256: string;
}
export interface MetadatadiffSource {
  category: string;
  name: string;
  version: string;
  repository: string;
  source_url: string;
  commit: string;
  cache_path: string;
}
export interface ChangereportBindingDiagnostic {
  code: string;
  side: string | null;
  field: string | null;
  expected: string | null;
  actual: string | null;
}
export interface ChangereportAuxiliarySection {
  status: string;
  reason: string;
}
export interface ApiLookup {
  query: MetadataproviderQuery | null;
  result: MetadataproviderLookupResult | null;
  error: ApiError | null;
  skip_reason: string;
}
export interface MetadataproviderQuery {
  repository: string;
  category: string;
  name: string;
  version: string;
}
export interface MetadataproviderLookupResult {
  status: string;
  snapshot: MetadatadiffSnapshot | null;
  provenance: MetadatageneratorProvenance | null;
}
export interface MetadatadiffSnapshot {
  source: MetadatadiffSource;
  content: string;
  sha256: string;
  eapi: MetadatadiffEAPI;
  slot: MetadatadiffSlot;
  iuse: MetadatadiffIUSE;
  diagnostics: (MetadatadiffDiagnostic)[];
}
export interface MetadatadiffEAPI {
  state: string;
  value: string;
  supported: boolean;
  lines: (number)[];
}
export interface MetadatadiffSlot {
  state: string;
  raw: string;
  name: string;
  subslot: string;
  explicit_subslot: boolean;
  lines: (number)[];
}
export interface MetadatadiffIUSE {
  state: string;
  flags: (MetadatadiffIUSEFlag)[];
  lines: (number)[];
}
export interface MetadatadiffIUSEFlag {
  name: string;
  default: string;
  declarations: (MetadatadiffIUSEDeclaration)[];
}
export interface MetadatadiffIUSEDeclaration {
  token: string;
  line: number;
}
export interface MetadatadiffDiagnostic {
  code: string;
  field: string;
  lines: (number)[];
}
export interface MetadatageneratorProvenance {
  reference_commit: string;
  selected_commit: string;
  selection_reason: string;
  ebuild_path: string;
  ebuild_blob_id: string;
  profile: string;
  portage_version: string;
  cache_path: string;
  cache_sha256: string;
  egencache_path: string;
  egencache_args: (string)[];
  egencache_stdout: string;
  egencache_stderr: string;
  repositories_configuration: string;
  make_conf: string;
  path: string;
  portage_username: string | null;
  portage_group_name: string | null;
  master_revisions: (MetadatageneratorMasterRevision)[];
}
export interface MetadatageneratorMasterRevision {
  name: string;
  commit: string;
}
export interface RepositorymanagerRevision {
  name: string;
  url: string;
  commit: string;
  masters: (string)[];
}
export interface AnalyzerLocalPatchedPackageEvidence {
  context_index: number;
}
export interface AnalyzerUseChangedEvidence {
  tokens: (AnalyzerUseTokenRef)[];
}
export interface AnalyzerUseTokenRef {
  group_index: number;
  value_index: number;
}
export interface AnalyzerSlotChangedEvidence {
  installed_version_index: number;
}
export interface AnalyzerDowngradePlannedEvidence {
  entry_index: number;
  operation: string;
  installed_versions: (string)[];
  target_version: string;
}
export interface AnalyzerUnresolvedBlockerEvidence {
  blocker_index: number;
}
export type FindingEvidence = AnalyzerLocalPatchedPackageEvidence | AnalyzerUseChangedEvidence | AnalyzerSlotChangedEvidence | AnalyzerDowngradePlannedEvidence | AnalyzerUnresolvedBlockerEvidence;
export interface ApiError { code: string; message: string }
