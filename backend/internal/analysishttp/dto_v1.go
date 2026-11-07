// Package-local public DTOs deliberately do not embed domain types.
package analysishttp

// API v1 uses explicit names, empty collections and nullable pointers.
type resultDTO struct {
	Plan         parserResultDTO                         `json:"plan"`
	Analysis     analyzerResultDTO                       `json:"analysis"`
	Reports      []reportDTO                             `json:"reports"`
	Repositories map[string]repositorymanagerRevisionDTO `json:"repositories"`
}

type parserResultDTO struct {
	Status          string                    `json:"status"`
	Entries         []parserEntryDTO          `json:"entries"`
	Blockers        []parserBlockerDTO        `json:"blockers"`
	Removals        []parserRemovalDTO        `json:"removals"`
	ConflictSummary *parserConflictSummaryDTO `json:"conflict_summary"`
	Summary         *parserSummaryDTO         `json:"summary"`
	Diagnostics     []parserDiagnosticDTO     `json:"diagnostics"`
}

type parserEntryDTO struct {
	Package        parserPackageDTO          `json:"package"`
	Operation      string                    `json:"operation"`
	NewSlot        bool                      `json:"new_slot"`
	UnstableMarker bool                      `json:"unstable_marker"`
	Target         parserTargetDTO           `json:"target"`
	Installed      parserInstalledContextDTO `json:"installed"`
	FlagGroups     []parserFlagGroupDTO      `json:"flag_groups"`
	Download       *parserSizeDTO            `json:"download"`
	SourceLines    []int                     `json:"source_lines"`
}

type parserPackageDTO struct {
	Category string `json:"category"`
	Name     string `json:"name"`
}

type parserTargetDTO struct {
	Version    string  `json:"version"`
	Repository *string `json:"repository"`
	Slot       *string `json:"slot"`
	Subslot    *string `json:"subslot"`
}

type parserInstalledContextDTO struct {
	Versions []parserInstalledVersionDTO `json:"versions"`
}

type parserInstalledVersionDTO struct {
	Version    string  `json:"version"`
	Repository *string `json:"repository"`
	Slot       *string `json:"slot"`
	Subslot    *string `json:"subslot"`
}

type parserFlagGroupDTO struct {
	Name   string               `json:"name"`
	Values []parserFlagValueDTO `json:"values"`
}

type parserFlagValueDTO struct {
	Name     string   `json:"name"`
	State    string   `json:"state"`
	RawToken string   `json:"raw_token"`
	Markers  []string `json:"markers"`
}

type parserSizeDTO struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

type parserBlockerDTO struct {
	Atom           string   `json:"atom"`
	ConstraintAtom *string  `json:"constraint_atom"`
	Marker         string   `json:"marker"`
	Resolution     string   `json:"resolution"`
	Strength       string   `json:"strength"`
	Parents        []string `json:"parents"`
	RawLine        string   `json:"raw_line"`
	SourceLines    []int    `json:"source_lines"`
}

type parserRemovalDTO struct {
	Package     parserPackageDTO     `json:"package"`
	Target      parserTargetDTO      `json:"target"`
	FlagGroups  []parserFlagGroupDTO `json:"flag_groups"`
	RawLine     string               `json:"raw_line"`
	SourceLines []int                `json:"source_lines"`
}

type parserConflictSummaryDTO struct {
	Blockers   int `json:"blockers"`
	Unresolved int `json:"unresolved"`
	SourceLine int `json:"source_line"`
}

type parserSummaryDTO struct {
	Packages   int           `json:"packages"`
	Upgrades   int           `json:"upgrades"`
	Reinstalls int           `json:"reinstalls"`
	Downgrades int           `json:"downgrades"`
	Uninstalls int           `json:"uninstalls"`
	NewSlots   int           `json:"new_slots"`
	Download   parserSizeDTO `json:"download"`
	SourceLine int           `json:"source_line"`
}

type parserDiagnosticDTO struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	SourceLines []int  `json:"source_lines"`
}

type analyzerResultDTO struct {
	Status      string                `json:"status"`
	Diagnostics []parserDiagnosticDTO `json:"diagnostics"`
	Findings    []analyzerFindingDTO  `json:"findings"`
}

type analyzerFindingDTO struct {
	RuleID     string             `json:"rule_id"`
	Category   string             `json:"category"`
	Message    string             `json:"message"`
	EntryIndex *int               `json:"entry_index"`
	Evidence   findingEvidenceDTO `json:"evidence"`
}

type reportDTO struct {
	Report       changereportReportDTO `json:"report"`
	BeforeLookup lookupDTO             `json:"before_lookup"`
	AfterLookup  lookupDTO             `json:"after_lookup"`
}

type changereportReportDTO struct {
	EntryIndex               int                             `json:"entry_index"`
	SourceLines              []int                           `json:"source_lines"`
	ParserStatus             string                          `json:"parser_status"`
	ParserDiagnostics        []parserDiagnosticDTO           `json:"parser_diagnostics"`
	Gentoo                   changereportGentooSectionDTO    `json:"gentoo"`
	EffectiveConfig          changereportAuxiliarySectionDTO `json:"effective_config"`
	Upstream                 changereportAuxiliarySectionDTO `json:"upstream"`
	InterpretationLimitation string                          `json:"interpretation_limitation"`
}

type changereportGentooSectionDTO struct {
	Status      string                             `json:"status"`
	Comparison  *metadatadiffResultDTO             `json:"comparison"`
	Diagnostics []changereportBindingDiagnosticDTO `json:"diagnostics"`
}

type metadatadiffResultDTO struct {
	Status         string                      `json:"status"`
	ComparedFields []string                    `json:"compared_fields"`
	Changes        metadatadiffChangesDTO      `json:"changes"`
	Limitations    []metadatadiffLimitationDTO `json:"limitations"`
	Before         metadatadiffSnapshotLinkDTO `json:"before"`
	After          metadatadiffSnapshotLinkDTO `json:"after"`
}

type metadatadiffChangesDTO struct {
	EAPI    *metadatadiffValueChangeDTO `json:"eapi"`
	Slot    *metadatadiffValueChangeDTO `json:"slot"`
	Subslot *metadatadiffValueChangeDTO `json:"subslot"`
	IUSE    metadatadiffIUSEChangesDTO  `json:"iuse"`
}

type metadatadiffValueChangeDTO struct {
	Before   string                      `json:"before"`
	After    string                      `json:"after"`
	Evidence metadatadiffEvidencePairDTO `json:"evidence"`
}

type metadatadiffEvidencePairDTO struct {
	Before metadatadiffEvidenceRefDTO `json:"before"`
	After  metadatadiffEvidenceRefDTO `json:"after"`
}

type metadatadiffEvidenceRefDTO struct {
	Side          string `json:"side"`
	Field         string `json:"field"`
	Lines         []int  `json:"lines"`
	WholeSnapshot bool   `json:"whole_snapshot"`
}

type metadatadiffIUSEChangesDTO struct {
	Added          []metadatadiffIUSEFlagChangeDTO    `json:"added"`
	Removed        []metadatadiffIUSEFlagChangeDTO    `json:"removed"`
	DefaultChanges []metadatadiffIUSEDefaultChangeDTO `json:"default_changes"`
}

type metadatadiffIUSEFlagChangeDTO struct {
	Name     string                      `json:"name"`
	Default  string                      `json:"default"`
	Evidence metadatadiffEvidencePairDTO `json:"evidence"`
}

type metadatadiffIUSEDefaultChangeDTO struct {
	Name     string                      `json:"name"`
	Before   string                      `json:"before"`
	After    string                      `json:"after"`
	Evidence metadatadiffEvidencePairDTO `json:"evidence"`
}

type metadatadiffLimitationDTO struct {
	Code     string                       `json:"code"`
	Field    string                       `json:"field"`
	Side     string                       `json:"side"`
	Evidence []metadatadiffEvidenceRefDTO `json:"evidence"`
}

type metadatadiffSnapshotLinkDTO struct {
	Source metadatadiffSourceDTO `json:"source"`
	SHA256 string                `json:"sha256"`
}

type metadatadiffSourceDTO struct {
	Category   string `json:"category"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository"`
	SourceURL  string `json:"source_url"`
	Commit     string `json:"commit"`
	CachePath  string `json:"cache_path"`
}

type changereportBindingDiagnosticDTO struct {
	Code     string  `json:"code"`
	Side     *string `json:"side"`
	Field    *string `json:"field"`
	Expected *string `json:"expected"`
	Actual   *string `json:"actual"`
}

type changereportAuxiliarySectionDTO struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type lookupDTO struct {
	Query      *metadataproviderQueryDTO        `json:"query"`
	Result     *metadataproviderLookupResultDTO `json:"result"`
	Error      *apiErrorDTO                     `json:"error"`
	SkipReason string                           `json:"skip_reason"`
}

type metadataproviderQueryDTO struct {
	Repository string `json:"repository"`
	Category   string `json:"category"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

type metadataproviderLookupResultDTO struct {
	Status     string                          `json:"status"`
	Snapshot   *metadatadiffSnapshotDTO        `json:"snapshot"`
	Provenance *metadatageneratorProvenanceDTO `json:"provenance"`
}

type metadatadiffSnapshotDTO struct {
	Source      metadatadiffSourceDTO       `json:"source"`
	Content     string                      `json:"content"`
	SHA256      string                      `json:"sha256"`
	EAPI        metadatadiffEAPIDTO         `json:"eapi"`
	Slot        metadatadiffSlotDTO         `json:"slot"`
	IUSE        metadatadiffIUSEDTO         `json:"iuse"`
	Diagnostics []metadatadiffDiagnosticDTO `json:"diagnostics"`
}

type metadatadiffEAPIDTO struct {
	State     string `json:"state"`
	Value     string `json:"value"`
	Supported bool   `json:"supported"`
	Lines     []int  `json:"lines"`
}

type metadatadiffSlotDTO struct {
	State           string `json:"state"`
	Raw             string `json:"raw"`
	Name            string `json:"name"`
	Subslot         string `json:"subslot"`
	ExplicitSubslot bool   `json:"explicit_subslot"`
	Lines           []int  `json:"lines"`
}

type metadatadiffIUSEDTO struct {
	State string                    `json:"state"`
	Flags []metadatadiffIUSEFlagDTO `json:"flags"`
	Lines []int                     `json:"lines"`
}

type metadatadiffIUSEFlagDTO struct {
	Name         string                           `json:"name"`
	Default      string                           `json:"default"`
	Declarations []metadatadiffIUSEDeclarationDTO `json:"declarations"`
}

type metadatadiffIUSEDeclarationDTO struct {
	Token string `json:"token"`
	Line  int    `json:"line"`
}

type metadatadiffDiagnosticDTO struct {
	Code  string `json:"code"`
	Field string `json:"field"`
	Lines []int  `json:"lines"`
}

type metadatageneratorProvenanceDTO struct {
	ReferenceCommit  string                               `json:"reference_commit"`
	SelectedCommit   string                               `json:"selected_commit"`
	SelectionReason  string                               `json:"selection_reason"`
	EbuildPath       string                               `json:"ebuild_path"`
	EbuildBlobID     string                               `json:"ebuild_blob_id"`
	Profile          string                               `json:"profile"`
	PortageVersion   string                               `json:"portage_version"`
	CachePath        string                               `json:"cache_path"`
	CacheSHA256      string                               `json:"cache_sha256"`
	EgencachePath    string                               `json:"egencache_path"`
	EgencacheArgs    []string                             `json:"egencache_args"`
	EgencacheStdout  string                               `json:"egencache_stdout"`
	EgencacheStderr  string                               `json:"egencache_stderr"`
	Repositories     string                               `json:"repositories_configuration"`
	MakeConf         string                               `json:"make_conf"`
	Path             string                               `json:"path"`
	PortageUserName  *string                              `json:"portage_username"`
	PortageGroupName *string                              `json:"portage_group_name"`
	MasterRevisions  []metadatageneratorMasterRevisionDTO `json:"master_revisions"`
}

type metadatageneratorMasterRevisionDTO struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

type repositorymanagerRevisionDTO struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Commit  string   `json:"commit"`
	Masters []string `json:"masters"`
}

type analyzerLocalPatchedPackageEvidenceDTO struct {
	ContextIndex int `json:"context_index"`
}

type analyzerUseChangedEvidenceDTO struct {
	Tokens []analyzerUseTokenRefDTO `json:"tokens"`
}

type analyzerUseTokenRefDTO struct {
	GroupIndex int `json:"group_index"`
	ValueIndex int `json:"value_index"`
}

type analyzerSlotChangedEvidenceDTO struct {
	InstalledVersionIndex int `json:"installed_version_index"`
}

type analyzerDowngradePlannedEvidenceDTO struct {
	EntryIndex        int      `json:"entry_index"`
	Operation         string   `json:"operation"`
	InstalledVersions []string `json:"installed_versions"`
	TargetVersion     string   `json:"target_version"`
}

type analyzerUnresolvedBlockerEvidenceDTO struct {
	BlockerIndex int `json:"blocker_index"`
}

type findingEvidenceDTO interface{ v1Evidence() }

func (analyzerLocalPatchedPackageEvidenceDTO) v1Evidence() {}
func (analyzerUseChangedEvidenceDTO) v1Evidence()          {}
func (analyzerSlotChangedEvidenceDTO) v1Evidence()         {}
func (analyzerDowngradePlannedEvidenceDTO) v1Evidence()    {}
func (analyzerUnresolvedBlockerEvidenceDTO) v1Evidence()   {}
