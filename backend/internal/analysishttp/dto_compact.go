package analysishttp

// Compact DTOs retain report facts without returning metadata snapshots or logs.
type compactResultDTO struct {
	Plan         parserResultDTO                         `json:"plan"`
	Analysis     analyzerResultDTO                       `json:"analysis"`
	Reports      []compactReportDTO                      `json:"reports"`
	Repositories map[string]repositorymanagerRevisionDTO `json:"repositories"`
}

type compactReportDTO struct {
	Report       compactChangeReportDTO `json:"report"`
	BeforeLookup compactLookupDTO       `json:"before_lookup"`
	AfterLookup  compactLookupDTO       `json:"after_lookup"`
}

type compactChangeReportDTO struct {
	EntryIndex               int                             `json:"entry_index"`
	SourceLines              []int                           `json:"source_lines"`
	Gentoo                   changereportGentooSectionDTO    `json:"gentoo"`
	EffectiveConfig          changereportAuxiliarySectionDTO `json:"effective_config"`
	Upstream                 changereportAuxiliarySectionDTO `json:"upstream"`
	InterpretationLimitation string                          `json:"interpretation_limitation"`
}

type compactLookupDTO struct {
	Query      *metadataproviderQueryDTO `json:"query"`
	Result     *compactLookupResultDTO   `json:"result"`
	Error      *apiErrorDTO              `json:"error"`
	SkipReason string                    `json:"skip_reason"`
}

type compactLookupResultDTO struct {
	Status string `json:"status"`
}

func compactLookup(source lookupDTO) compactLookupDTO {
	result := compactLookupDTO{Query: source.Query, Error: source.Error, SkipReason: source.SkipReason}
	if source.Result != nil {
		result.Result = &compactLookupResultDTO{Status: source.Result.Status}
	}
	return result
}

func compactResult(source *resultDTO) compactResultDTO {
	result := compactResultDTO{Plan: source.Plan, Analysis: source.Analysis, Repositories: source.Repositories, Reports: make([]compactReportDTO, len(source.Reports))}
	for i, view := range source.Reports {
		r := view.Report
		result.Reports[i] = compactReportDTO{
			Report:       compactChangeReportDTO{EntryIndex: r.EntryIndex, SourceLines: r.SourceLines, Gentoo: r.Gentoo, EffectiveConfig: r.EffectiveConfig, Upstream: r.Upstream, InterpretationLimitation: r.InterpretationLimitation},
			BeforeLookup: compactLookup(view.BeforeLookup), AfterLookup: compactLookup(view.AfterLookup),
		}
	}
	return result
}
