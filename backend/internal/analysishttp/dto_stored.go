package analysishttp

import "encoding/json"

// Stored types read the pre-v1 result format; they are never serialized to clients.
type storedResult struct {
	Plan         *storedParserResult                        `json:"Plan"`
	Analysis     *storedAnalyzerResult                      `json:"Analysis"`
	Reports      []storedReport                             `json:"Reports"`
	Repositories map[string]storedRepositorymanagerRevision `json:""`
}

type storedParserResult struct {
	Status          string                       `json:"Status"`
	Entries         []storedParserEntry          `json:"Entries"`
	Blockers        []storedParserBlocker        `json:"Blockers"`
	Removals        []storedParserRemoval        `json:"Removals"`
	ConflictSummary *storedParserConflictSummary `json:"ConflictSummary"`
	Summary         *storedParserSummary         `json:"Summary"`
	Diagnostics     []storedParserDiagnostic     `json:"Diagnostics"`
}

type storedParserEntry struct {
	Package        storedParserPackage          `json:"Package"`
	Operation      string                       `json:"Operation"`
	NewSlot        bool                         `json:"NewSlot"`
	UnstableMarker bool                         `json:"UnstableMarker"`
	Target         storedParserTarget           `json:"Target"`
	Installed      storedParserInstalledContext `json:"Installed"`
	FlagGroups     []storedParserFlagGroup      `json:"FlagGroups"`
	Download       *storedParserSize            `json:"Download"`
	SourceLines    []int                        `json:"SourceLines"`
}

type storedParserPackage struct {
	Category string `json:"Category"`
	Name     string `json:"Name"`
}

type storedParserTarget struct {
	Version    string  `json:"Version"`
	Repository *string `json:"Repository"`
	Slot       *string `json:"Slot"`
	Subslot    *string `json:"Subslot"`
}

type storedParserInstalledContext struct {
	Versions []storedParserInstalledVersion `json:"Versions"`
}

type storedParserInstalledVersion struct {
	Version    string  `json:"Version"`
	Repository *string `json:"Repository"`
	Slot       *string `json:"Slot"`
	Subslot    *string `json:"Subslot"`
}

type storedParserFlagGroup struct {
	Name   string                  `json:"Name"`
	Values []storedParserFlagValue `json:"Values"`
}

type storedParserFlagValue struct {
	Name     string   `json:"Name"`
	State    string   `json:"State"`
	RawToken string   `json:"RawToken"`
	Markers  []string `json:"Markers"`
}

type storedParserSize struct {
	Value int64  `json:"Value"`
	Unit  string `json:"Unit"`
}

type storedParserBlocker struct {
	Atom           string   `json:"Atom"`
	ConstraintAtom *string  `json:"ConstraintAtom"`
	Marker         string   `json:"Marker"`
	Resolution     string   `json:"Resolution"`
	Strength       string   `json:"Strength"`
	Parents        []string `json:"Parents"`
	RawLine        string   `json:"RawLine"`
	SourceLines    []int    `json:"SourceLines"`
}

type storedParserRemoval struct {
	Package     storedParserPackage     `json:"Package"`
	Target      storedParserTarget      `json:"Target"`
	FlagGroups  []storedParserFlagGroup `json:"FlagGroups"`
	RawLine     string                  `json:"RawLine"`
	SourceLines []int                   `json:"SourceLines"`
}

type storedParserConflictSummary struct {
	Blockers   int `json:"Blockers"`
	Unresolved int `json:"Unresolved"`
	SourceLine int `json:"SourceLine"`
}

type storedParserSummary struct {
	Packages   int              `json:"Packages"`
	Upgrades   int              `json:"Upgrades"`
	Reinstalls int              `json:"Reinstalls"`
	Downgrades int              `json:"Downgrades"`
	Uninstalls int              `json:"Uninstalls"`
	NewSlots   int              `json:"NewSlots"`
	Download   storedParserSize `json:"Download"`
	SourceLine int              `json:"SourceLine"`
}

type storedParserDiagnostic struct {
	Code        string `json:"Code"`
	Message     string `json:"Message"`
	SourceLines []int  `json:"SourceLines"`
}

type storedAnalyzerResult struct {
	Status      string                   `json:"Status"`
	Diagnostics []storedParserDiagnostic `json:"Diagnostics"`
	Findings    []storedAnalyzerFinding  `json:"Findings"`
}

type storedAnalyzerFinding struct {
	RuleID     string          `json:"RuleID"`
	Category   string          `json:"Category"`
	Message    string          `json:"Message"`
	EntryIndex *int            `json:"EntryIndex"`
	Evidence   json.RawMessage `json:"Evidence"`
}

type storedReport struct {
	Report       storedChangereportReport `json:"Report"`
	BeforeLookup storedLookup             `json:"BeforeLookup"`
	AfterLookup  storedLookup             `json:"AfterLookup"`
}

type storedChangereportReport struct {
	EntryIndex               int                                `json:"EntryIndex"`
	SourceLines              []int                              `json:"SourceLines"`
	ParserStatus             string                             `json:"ParserStatus"`
	ParserDiagnostics        []storedParserDiagnostic           `json:"ParserDiagnostics"`
	Gentoo                   storedChangereportGentooSection    `json:"Gentoo"`
	EffectiveConfig          storedChangereportAuxiliarySection `json:"EffectiveConfig"`
	Upstream                 storedChangereportAuxiliarySection `json:"Upstream"`
	InterpretationLimitation string                             `json:"InterpretationLimitation"`
}

type storedChangereportGentooSection struct {
	Status      string                                `json:"Status"`
	Comparison  *storedMetadatadiffResult             `json:"Comparison"`
	Diagnostics []storedChangereportBindingDiagnostic `json:"Diagnostics"`
}

type storedMetadatadiffResult struct {
	Status         string                         `json:"Status"`
	ComparedFields []string                       `json:"ComparedFields"`
	Changes        storedMetadatadiffChanges      `json:"Changes"`
	Limitations    []storedMetadatadiffLimitation `json:"Limitations"`
	Before         storedMetadatadiffSnapshotLink `json:"Before"`
	After          storedMetadatadiffSnapshotLink `json:"After"`
}

type storedMetadatadiffChanges struct {
	EAPI    *storedMetadatadiffValueChange `json:"EAPI"`
	Slot    *storedMetadatadiffValueChange `json:"Slot"`
	Subslot *storedMetadatadiffValueChange `json:"Subslot"`
	IUSE    storedMetadatadiffIUSEChanges  `json:"IUSE"`
}

type storedMetadatadiffValueChange struct {
	Before   string                         `json:"Before"`
	After    string                         `json:"After"`
	Evidence storedMetadatadiffEvidencePair `json:"Evidence"`
}

type storedMetadatadiffEvidencePair struct {
	Before storedMetadatadiffEvidenceRef `json:"Before"`
	After  storedMetadatadiffEvidenceRef `json:"After"`
}

type storedMetadatadiffEvidenceRef struct {
	Side          string `json:"Side"`
	Field         string `json:"Field"`
	Lines         []int  `json:"Lines"`
	WholeSnapshot bool   `json:"WholeSnapshot"`
}

type storedMetadatadiffIUSEChanges struct {
	Added          []storedMetadatadiffIUSEFlagChange    `json:"Added"`
	Removed        []storedMetadatadiffIUSEFlagChange    `json:"Removed"`
	DefaultChanges []storedMetadatadiffIUSEDefaultChange `json:"DefaultChanges"`
}

type storedMetadatadiffIUSEFlagChange struct {
	Name     string                         `json:"Name"`
	Default  string                         `json:"Default"`
	Evidence storedMetadatadiffEvidencePair `json:"Evidence"`
}

type storedMetadatadiffIUSEDefaultChange struct {
	Name     string                         `json:"Name"`
	Before   string                         `json:"Before"`
	After    string                         `json:"After"`
	Evidence storedMetadatadiffEvidencePair `json:"Evidence"`
}

type storedMetadatadiffLimitation struct {
	Code     string                          `json:"Code"`
	Field    string                          `json:"Field"`
	Side     string                          `json:"Side"`
	Evidence []storedMetadatadiffEvidenceRef `json:"Evidence"`
}

type storedMetadatadiffSnapshotLink struct {
	Source storedMetadatadiffSource `json:"Source"`
	SHA256 string                   `json:"SHA256"`
}

type storedMetadatadiffSource struct {
	Category   string `json:"Category"`
	Name       string `json:"Name"`
	Version    string `json:"Version"`
	Repository string `json:"Repository"`
	SourceURL  string `json:"SourceURL"`
	Commit     string `json:"Commit"`
	CachePath  string `json:"CachePath"`
}

type storedChangereportBindingDiagnostic struct {
	Code     string  `json:"Code"`
	Side     *string `json:"Side"`
	Field    *string `json:"Field"`
	Expected *string `json:"Expected"`
	Actual   *string `json:"Actual"`
}

type storedChangereportAuxiliarySection struct {
	Status string `json:"Status"`
	Reason string `json:"Reason"`
}

type storedLookup struct {
	Query      *storedMetadataproviderQuery        `json:"Query"`
	Result     *storedMetadataproviderLookupResult `json:"Result"`
	Err        *string                             `json:"Err"`
	SkipReason string                              `json:"SkipReason"`
}

type storedMetadataproviderQuery struct {
	Repository string `json:"Repository"`
	Category   string `json:"Category"`
	Name       string `json:"Name"`
	Version    string `json:"Version"`
}

type storedMetadataproviderLookupResult struct {
	Status     string                             `json:"Status"`
	Snapshot   *storedMetadatadiffSnapshot        `json:"Snapshot"`
	Provenance *storedMetadatageneratorProvenance `json:"provenance"`
}

type storedMetadatadiffSnapshot struct {
	Source      storedMetadatadiffSource       `json:"Source"`
	Content     string                         `json:"Content"`
	SHA256      string                         `json:"SHA256"`
	EAPI        storedMetadatadiffEAPI         `json:"EAPI"`
	Slot        storedMetadatadiffSlot         `json:"Slot"`
	IUSE        storedMetadatadiffIUSE         `json:"IUSE"`
	Diagnostics []storedMetadatadiffDiagnostic `json:"Diagnostics"`
}

type storedMetadatadiffEAPI struct {
	State     string `json:"State"`
	Value     string `json:"Value"`
	Supported bool   `json:"Supported"`
	Lines     []int  `json:"Lines"`
}

type storedMetadatadiffSlot struct {
	State           string `json:"State"`
	Raw             string `json:"Raw"`
	Name            string `json:"Name"`
	Subslot         string `json:"Subslot"`
	ExplicitSubslot bool   `json:"ExplicitSubslot"`
	Lines           []int  `json:"Lines"`
}

type storedMetadatadiffIUSE struct {
	State string                       `json:"State"`
	Flags []storedMetadatadiffIUSEFlag `json:"Flags"`
	Lines []int                        `json:"Lines"`
}

type storedMetadatadiffIUSEFlag struct {
	Name         string                              `json:"Name"`
	Default      string                              `json:"Default"`
	Declarations []storedMetadatadiffIUSEDeclaration `json:"Declarations"`
}

type storedMetadatadiffIUSEDeclaration struct {
	Token string `json:"Token"`
	Line  int    `json:"Line"`
}

type storedMetadatadiffDiagnostic struct {
	Code  string `json:"Code"`
	Field string `json:"Field"`
	Lines []int  `json:"Lines"`
}

type storedMetadatageneratorProvenance struct {
	ReferenceCommit  string                                  `json:"reference_commit"`
	SelectedCommit   string                                  `json:"selected_commit"`
	SelectionReason  string                                  `json:"selection_reason"`
	EbuildPath       string                                  `json:"ebuild_path"`
	EbuildBlobID     string                                  `json:"ebuild_blob_id"`
	Profile          string                                  `json:"profile"`
	PortageVersion   string                                  `json:"portage_version"`
	CachePath        string                                  `json:"cache_path"`
	CacheSHA256      string                                  `json:"cache_sha256"`
	EgencachePath    string                                  `json:"egencache_path"`
	EgencacheArgs    []string                                `json:"egencache_args"`
	EgencacheStdout  string                                  `json:"egencache_stdout"`
	EgencacheStderr  string                                  `json:"egencache_stderr"`
	Repositories     string                                  `json:"repositories_configuration"`
	MakeConf         string                                  `json:"make_conf"`
	Path             string                                  `json:"path"`
	PortageUserName  *string                                 `json:"portage_username"`
	PortageGroupName *string                                 `json:"portage_group_name"`
	MasterRevisions  []storedMetadatageneratorMasterRevision `json:"master_revisions"`
}

type storedMetadatageneratorMasterRevision struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

type storedRepositorymanagerRevision struct {
	Name    string   `json:"Name"`
	URL     string   `json:"URL"`
	Commit  string   `json:"Commit"`
	Masters []string `json:"Masters"`
}

type storedAnalyzerLocalPatchedPackageEvidence struct {
	ContextIndex int `json:"ContextIndex"`
}

type storedAnalyzerUseChangedEvidence struct {
	Tokens []storedAnalyzerUseTokenRef `json:"Tokens"`
}

type storedAnalyzerUseTokenRef struct {
	GroupIndex int `json:"GroupIndex"`
	ValueIndex int `json:"ValueIndex"`
}

type storedAnalyzerSlotChangedEvidence struct {
	InstalledVersionIndex int `json:"InstalledVersionIndex"`
}

type storedAnalyzerDowngradePlannedEvidence struct {
	EntryIndex        int      `json:"EntryIndex"`
	Operation         string   `json:"Operation"`
	InstalledVersions []string `json:"InstalledVersions"`
	TargetVersion     string   `json:"TargetVersion"`
}

type storedAnalyzerUnresolvedBlockerEvidence struct {
	BlockerIndex int `json:"BlockerIndex"`
}
