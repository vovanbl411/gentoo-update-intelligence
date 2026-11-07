package analysishttp

func convertParserResult(source storedParserResult) parserResultDTO {
	return parserResultDTO{
		Status:   source.Status,
		Entries:  v1Slice(source.Entries, func(value storedParserEntry) parserEntryDTO { return convertParserEntry(value) }),
		Blockers: v1Slice(source.Blockers, func(value storedParserBlocker) parserBlockerDTO { return convertParserBlocker(value) }),
		Removals: v1Slice(source.Removals, func(value storedParserRemoval) parserRemovalDTO { return convertParserRemoval(value) }),
		ConflictSummary: v1Pointer(source.ConflictSummary, func(value storedParserConflictSummary) parserConflictSummaryDTO {
			return convertParserConflictSummary(value)
		}),
		Summary:     v1Pointer(source.Summary, func(value storedParserSummary) parserSummaryDTO { return convertParserSummary(value) }),
		Diagnostics: v1Slice(source.Diagnostics, func(value storedParserDiagnostic) parserDiagnosticDTO { return convertParserDiagnostic(value) }),
	}
}

func convertParserEntry(source storedParserEntry) parserEntryDTO {
	return parserEntryDTO{
		Package:        convertParserPackage(source.Package),
		Operation:      source.Operation,
		NewSlot:        source.NewSlot,
		UnstableMarker: source.UnstableMarker,
		Target:         convertParserTarget(source.Target),
		Installed:      convertParserInstalledContext(source.Installed),
		FlagGroups:     v1Slice(source.FlagGroups, func(value storedParserFlagGroup) parserFlagGroupDTO { return convertParserFlagGroup(value) }),
		Download:       v1Pointer(source.Download, func(value storedParserSize) parserSizeDTO { return convertParserSize(value) }),
		SourceLines:    v1Slice(source.SourceLines, func(value int) int { return value }),
	}
}

func convertParserPackage(source storedParserPackage) parserPackageDTO {
	return parserPackageDTO{
		Category: source.Category,
		Name:     source.Name,
	}
}

func convertParserTarget(source storedParserTarget) parserTargetDTO {
	return parserTargetDTO{
		Version:    source.Version,
		Repository: source.Repository,
		Slot:       source.Slot,
		Subslot:    source.Subslot,
	}
}

func convertParserInstalledContext(source storedParserInstalledContext) parserInstalledContextDTO {
	return parserInstalledContextDTO{
		Versions: v1Slice(source.Versions, func(value storedParserInstalledVersion) parserInstalledVersionDTO {
			return convertParserInstalledVersion(value)
		}),
	}
}

func convertParserInstalledVersion(source storedParserInstalledVersion) parserInstalledVersionDTO {
	return parserInstalledVersionDTO{
		Version:    source.Version,
		Repository: source.Repository,
		Slot:       source.Slot,
		Subslot:    source.Subslot,
	}
}

func convertParserFlagGroup(source storedParserFlagGroup) parserFlagGroupDTO {
	return parserFlagGroupDTO{
		Name:   source.Name,
		Values: v1Slice(source.Values, func(value storedParserFlagValue) parserFlagValueDTO { return convertParserFlagValue(value) }),
	}
}

func convertParserFlagValue(source storedParserFlagValue) parserFlagValueDTO {
	return parserFlagValueDTO{
		Name:     source.Name,
		State:    source.State,
		RawToken: source.RawToken,
		Markers:  v1Slice(source.Markers, func(value string) string { return value }),
	}
}

func convertParserSize(source storedParserSize) parserSizeDTO {
	return parserSizeDTO{
		Value: source.Value,
		Unit:  source.Unit,
	}
}

func convertParserBlocker(source storedParserBlocker) parserBlockerDTO {
	return parserBlockerDTO{
		Atom:           source.Atom,
		ConstraintAtom: source.ConstraintAtom,
		Marker:         source.Marker,
		Resolution:     source.Resolution,
		Strength:       source.Strength,
		Parents:        v1Slice(source.Parents, func(value string) string { return value }),
		RawLine:        source.RawLine,
		SourceLines:    v1Slice(source.SourceLines, func(value int) int { return value }),
	}
}

func convertParserRemoval(source storedParserRemoval) parserRemovalDTO {
	return parserRemovalDTO{
		Package:     convertParserPackage(source.Package),
		Target:      convertParserTarget(source.Target),
		FlagGroups:  v1Slice(source.FlagGroups, func(value storedParserFlagGroup) parserFlagGroupDTO { return convertParserFlagGroup(value) }),
		RawLine:     source.RawLine,
		SourceLines: v1Slice(source.SourceLines, func(value int) int { return value }),
	}
}

func convertParserConflictSummary(source storedParserConflictSummary) parserConflictSummaryDTO {
	return parserConflictSummaryDTO{
		Blockers:   source.Blockers,
		Unresolved: source.Unresolved,
		SourceLine: source.SourceLine,
	}
}

func convertParserSummary(source storedParserSummary) parserSummaryDTO {
	return parserSummaryDTO{
		Packages:   source.Packages,
		Upgrades:   source.Upgrades,
		Reinstalls: source.Reinstalls,
		Downgrades: source.Downgrades,
		Uninstalls: source.Uninstalls,
		NewSlots:   source.NewSlots,
		Download:   convertParserSize(source.Download),
		SourceLine: source.SourceLine,
	}
}

func convertParserDiagnostic(source storedParserDiagnostic) parserDiagnosticDTO {
	return parserDiagnosticDTO{
		Code:        source.Code,
		Message:     source.Message,
		SourceLines: v1Slice(source.SourceLines, func(value int) int { return value }),
	}
}

func convertAnalyzerResult(source storedAnalyzerResult) analyzerResultDTO {
	return analyzerResultDTO{
		Status:      source.Status,
		Diagnostics: v1Slice(source.Diagnostics, func(value storedParserDiagnostic) parserDiagnosticDTO { return convertParserDiagnostic(value) }),
		Findings:    v1Slice(source.Findings, func(value storedAnalyzerFinding) analyzerFindingDTO { return convertAnalyzerFinding(value) }),
	}
}

func convertAnalyzerFinding(source storedAnalyzerFinding) analyzerFindingDTO {
	return analyzerFindingDTO{
		RuleID:     source.RuleID,
		Category:   source.Category,
		Message:    source.Message,
		EntryIndex: source.EntryIndex,
		Evidence:   nil,
	}
}

func convertReport(source storedReport) reportDTO {
	return reportDTO{
		Report:       convertChangereportReport(source.Report),
		BeforeLookup: convertLookup(source.BeforeLookup),
		AfterLookup:  convertLookup(source.AfterLookup),
	}
}

func convertChangereportReport(source storedChangereportReport) changereportReportDTO {
	return changereportReportDTO{
		EntryIndex:               source.EntryIndex,
		SourceLines:              v1Slice(source.SourceLines, func(value int) int { return value }),
		ParserStatus:             source.ParserStatus,
		ParserDiagnostics:        v1Slice(source.ParserDiagnostics, func(value storedParserDiagnostic) parserDiagnosticDTO { return convertParserDiagnostic(value) }),
		Gentoo:                   convertChangereportGentooSection(source.Gentoo),
		EffectiveConfig:          convertChangereportAuxiliarySection(source.EffectiveConfig),
		Upstream:                 convertChangereportAuxiliarySection(source.Upstream),
		InterpretationLimitation: source.InterpretationLimitation,
	}
}

func convertChangereportGentooSection(source storedChangereportGentooSection) changereportGentooSectionDTO {
	return changereportGentooSectionDTO{
		Status:     source.Status,
		Comparison: v1Pointer(source.Comparison, func(value storedMetadatadiffResult) metadatadiffResultDTO { return convertMetadatadiffResult(value) }),
		Diagnostics: v1Slice(source.Diagnostics, func(value storedChangereportBindingDiagnostic) changereportBindingDiagnosticDTO {
			return convertChangereportBindingDiagnostic(value)
		}),
	}
}

func convertMetadatadiffResult(source storedMetadatadiffResult) metadatadiffResultDTO {
	return metadatadiffResultDTO{
		Status:         source.Status,
		ComparedFields: v1Slice(source.ComparedFields, func(value string) string { return value }),
		Changes:        convertMetadatadiffChanges(source.Changes),
		Limitations: v1Slice(source.Limitations, func(value storedMetadatadiffLimitation) metadatadiffLimitationDTO {
			return convertMetadatadiffLimitation(value)
		}),
		Before: convertMetadatadiffSnapshotLink(source.Before),
		After:  convertMetadatadiffSnapshotLink(source.After),
	}
}

func convertMetadatadiffChanges(source storedMetadatadiffChanges) metadatadiffChangesDTO {
	return metadatadiffChangesDTO{
		EAPI: v1Pointer(source.EAPI, func(value storedMetadatadiffValueChange) metadatadiffValueChangeDTO {
			return convertMetadatadiffValueChange(value)
		}),
		Slot: v1Pointer(source.Slot, func(value storedMetadatadiffValueChange) metadatadiffValueChangeDTO {
			return convertMetadatadiffValueChange(value)
		}),
		Subslot: v1Pointer(source.Subslot, func(value storedMetadatadiffValueChange) metadatadiffValueChangeDTO {
			return convertMetadatadiffValueChange(value)
		}),
		IUSE: convertMetadatadiffIUSEChanges(source.IUSE),
	}
}

func convertMetadatadiffValueChange(source storedMetadatadiffValueChange) metadatadiffValueChangeDTO {
	return metadatadiffValueChangeDTO{
		Before:   source.Before,
		After:    source.After,
		Evidence: convertMetadatadiffEvidencePair(source.Evidence),
	}
}

func convertMetadatadiffEvidencePair(source storedMetadatadiffEvidencePair) metadatadiffEvidencePairDTO {
	return metadatadiffEvidencePairDTO{
		Before: convertMetadatadiffEvidenceRef(source.Before),
		After:  convertMetadatadiffEvidenceRef(source.After),
	}
}

func convertMetadatadiffEvidenceRef(source storedMetadatadiffEvidenceRef) metadatadiffEvidenceRefDTO {
	return metadatadiffEvidenceRefDTO{
		Side:          source.Side,
		Field:         source.Field,
		Lines:         v1Slice(source.Lines, func(value int) int { return value }),
		WholeSnapshot: source.WholeSnapshot,
	}
}

func convertMetadatadiffIUSEChanges(source storedMetadatadiffIUSEChanges) metadatadiffIUSEChangesDTO {
	return metadatadiffIUSEChangesDTO{
		Added: v1Slice(source.Added, func(value storedMetadatadiffIUSEFlagChange) metadatadiffIUSEFlagChangeDTO {
			return convertMetadatadiffIUSEFlagChange(value)
		}),
		Removed: v1Slice(source.Removed, func(value storedMetadatadiffIUSEFlagChange) metadatadiffIUSEFlagChangeDTO {
			return convertMetadatadiffIUSEFlagChange(value)
		}),
		DefaultChanges: v1Slice(source.DefaultChanges, func(value storedMetadatadiffIUSEDefaultChange) metadatadiffIUSEDefaultChangeDTO {
			return convertMetadatadiffIUSEDefaultChange(value)
		}),
	}
}

func convertMetadatadiffIUSEFlagChange(source storedMetadatadiffIUSEFlagChange) metadatadiffIUSEFlagChangeDTO {
	return metadatadiffIUSEFlagChangeDTO{
		Name:     source.Name,
		Default:  source.Default,
		Evidence: convertMetadatadiffEvidencePair(source.Evidence),
	}
}

func convertMetadatadiffIUSEDefaultChange(source storedMetadatadiffIUSEDefaultChange) metadatadiffIUSEDefaultChangeDTO {
	return metadatadiffIUSEDefaultChangeDTO{
		Name:     source.Name,
		Before:   source.Before,
		After:    source.After,
		Evidence: convertMetadatadiffEvidencePair(source.Evidence),
	}
}

func convertMetadatadiffLimitation(source storedMetadatadiffLimitation) metadatadiffLimitationDTO {
	return metadatadiffLimitationDTO{
		Code:  source.Code,
		Field: source.Field,
		Side:  source.Side,
		Evidence: v1Slice(source.Evidence, func(value storedMetadatadiffEvidenceRef) metadatadiffEvidenceRefDTO {
			return convertMetadatadiffEvidenceRef(value)
		}),
	}
}

func convertMetadatadiffSnapshotLink(source storedMetadatadiffSnapshotLink) metadatadiffSnapshotLinkDTO {
	return metadatadiffSnapshotLinkDTO{
		Source: convertMetadatadiffSource(source.Source),
		SHA256: source.SHA256,
	}
}

func convertMetadatadiffSource(source storedMetadatadiffSource) metadatadiffSourceDTO {
	return metadatadiffSourceDTO{
		Category:   source.Category,
		Name:       source.Name,
		Version:    source.Version,
		Repository: source.Repository,
		SourceURL:  source.SourceURL,
		Commit:     source.Commit,
		CachePath:  source.CachePath,
	}
}

func convertChangereportBindingDiagnostic(source storedChangereportBindingDiagnostic) changereportBindingDiagnosticDTO {
	return changereportBindingDiagnosticDTO{
		Code:     source.Code,
		Side:     source.Side,
		Field:    source.Field,
		Expected: source.Expected,
		Actual:   source.Actual,
	}
}

func convertChangereportAuxiliarySection(source storedChangereportAuxiliarySection) changereportAuxiliarySectionDTO {
	return changereportAuxiliarySectionDTO{
		Status: source.Status,
		Reason: source.Reason,
	}
}

func convertLookup(source storedLookup) lookupDTO {
	return lookupDTO{
		Query: v1Pointer(source.Query, func(value storedMetadataproviderQuery) metadataproviderQueryDTO {
			return convertMetadataproviderQuery(value)
		}),
		Result: v1Pointer(source.Result, func(value storedMetadataproviderLookupResult) metadataproviderLookupResultDTO {
			return convertMetadataproviderLookupResult(value)
		}),
		Error:      lookupError(source.Err),
		SkipReason: source.SkipReason,
	}
}

func convertMetadataproviderQuery(source storedMetadataproviderQuery) metadataproviderQueryDTO {
	return metadataproviderQueryDTO{
		Repository: source.Repository,
		Category:   source.Category,
		Name:       source.Name,
		Version:    source.Version,
	}
}

func convertMetadataproviderLookupResult(source storedMetadataproviderLookupResult) metadataproviderLookupResultDTO {
	return metadataproviderLookupResultDTO{
		Status: source.Status,
		Snapshot: v1Pointer(source.Snapshot, func(value storedMetadatadiffSnapshot) metadatadiffSnapshotDTO {
			return convertMetadatadiffSnapshot(value)
		}),
		Provenance: v1Pointer(source.Provenance, func(value storedMetadatageneratorProvenance) metadatageneratorProvenanceDTO {
			return convertMetadatageneratorProvenance(value)
		}),
	}
}

func convertMetadatadiffSnapshot(source storedMetadatadiffSnapshot) metadatadiffSnapshotDTO {
	return metadatadiffSnapshotDTO{
		Source:  convertMetadatadiffSource(source.Source),
		Content: source.Content,
		SHA256:  source.SHA256,
		EAPI:    convertMetadatadiffEAPI(source.EAPI),
		Slot:    convertMetadatadiffSlot(source.Slot),
		IUSE:    convertMetadatadiffIUSE(source.IUSE),
		Diagnostics: v1Slice(source.Diagnostics, func(value storedMetadatadiffDiagnostic) metadatadiffDiagnosticDTO {
			return convertMetadatadiffDiagnostic(value)
		}),
	}
}

func convertMetadatadiffEAPI(source storedMetadatadiffEAPI) metadatadiffEAPIDTO {
	return metadatadiffEAPIDTO{
		State:     source.State,
		Value:     source.Value,
		Supported: source.Supported,
		Lines:     v1Slice(source.Lines, func(value int) int { return value }),
	}
}

func convertMetadatadiffSlot(source storedMetadatadiffSlot) metadatadiffSlotDTO {
	return metadatadiffSlotDTO{
		State:           source.State,
		Raw:             source.Raw,
		Name:            source.Name,
		Subslot:         source.Subslot,
		ExplicitSubslot: source.ExplicitSubslot,
		Lines:           v1Slice(source.Lines, func(value int) int { return value }),
	}
}

func convertMetadatadiffIUSE(source storedMetadatadiffIUSE) metadatadiffIUSEDTO {
	return metadatadiffIUSEDTO{
		State: source.State,
		Flags: v1Slice(source.Flags, func(value storedMetadatadiffIUSEFlag) metadatadiffIUSEFlagDTO {
			return convertMetadatadiffIUSEFlag(value)
		}),
		Lines: v1Slice(source.Lines, func(value int) int { return value }),
	}
}

func convertMetadatadiffIUSEFlag(source storedMetadatadiffIUSEFlag) metadatadiffIUSEFlagDTO {
	return metadatadiffIUSEFlagDTO{
		Name:    source.Name,
		Default: source.Default,
		Declarations: v1Slice(source.Declarations, func(value storedMetadatadiffIUSEDeclaration) metadatadiffIUSEDeclarationDTO {
			return convertMetadatadiffIUSEDeclaration(value)
		}),
	}
}

func convertMetadatadiffIUSEDeclaration(source storedMetadatadiffIUSEDeclaration) metadatadiffIUSEDeclarationDTO {
	return metadatadiffIUSEDeclarationDTO{
		Token: source.Token,
		Line:  source.Line,
	}
}

func convertMetadatadiffDiagnostic(source storedMetadatadiffDiagnostic) metadatadiffDiagnosticDTO {
	return metadatadiffDiagnosticDTO{
		Code:  source.Code,
		Field: source.Field,
		Lines: v1Slice(source.Lines, func(value int) int { return value }),
	}
}

func convertMetadatageneratorProvenance(source storedMetadatageneratorProvenance) metadatageneratorProvenanceDTO {
	return metadatageneratorProvenanceDTO{
		ReferenceCommit:  source.ReferenceCommit,
		SelectedCommit:   source.SelectedCommit,
		SelectionReason:  source.SelectionReason,
		EbuildPath:       source.EbuildPath,
		EbuildBlobID:     source.EbuildBlobID,
		Profile:          source.Profile,
		PortageVersion:   source.PortageVersion,
		CachePath:        source.CachePath,
		CacheSHA256:      source.CacheSHA256,
		EgencachePath:    source.EgencachePath,
		EgencacheArgs:    v1Slice(source.EgencacheArgs, func(value string) string { return value }),
		EgencacheStdout:  source.EgencacheStdout,
		EgencacheStderr:  source.EgencacheStderr,
		Repositories:     source.Repositories,
		MakeConf:         source.MakeConf,
		Path:             source.Path,
		PortageUserName:  source.PortageUserName,
		PortageGroupName: source.PortageGroupName,
		MasterRevisions: v1Slice(source.MasterRevisions, func(value storedMetadatageneratorMasterRevision) metadatageneratorMasterRevisionDTO {
			return convertMetadatageneratorMasterRevision(value)
		}),
	}
}

func convertMetadatageneratorMasterRevision(source storedMetadatageneratorMasterRevision) metadatageneratorMasterRevisionDTO {
	return metadatageneratorMasterRevisionDTO{
		Name:   source.Name,
		Commit: source.Commit,
	}
}

func convertRepositorymanagerRevision(source storedRepositorymanagerRevision) repositorymanagerRevisionDTO {
	return repositorymanagerRevisionDTO{
		Name:    source.Name,
		URL:     source.URL,
		Commit:  source.Commit,
		Masters: v1Slice(source.Masters, func(value string) string { return value }),
	}
}

func convertAnalyzerLocalPatchedPackageEvidence(source storedAnalyzerLocalPatchedPackageEvidence) analyzerLocalPatchedPackageEvidenceDTO {
	return analyzerLocalPatchedPackageEvidenceDTO{
		ContextIndex: source.ContextIndex,
	}
}

func convertAnalyzerUseChangedEvidence(source storedAnalyzerUseChangedEvidence) analyzerUseChangedEvidenceDTO {
	return analyzerUseChangedEvidenceDTO{
		Tokens: v1Slice(source.Tokens, func(value storedAnalyzerUseTokenRef) analyzerUseTokenRefDTO { return convertAnalyzerUseTokenRef(value) }),
	}
}

func convertAnalyzerUseTokenRef(source storedAnalyzerUseTokenRef) analyzerUseTokenRefDTO {
	return analyzerUseTokenRefDTO{
		GroupIndex: source.GroupIndex,
		ValueIndex: source.ValueIndex,
	}
}

func convertAnalyzerSlotChangedEvidence(source storedAnalyzerSlotChangedEvidence) analyzerSlotChangedEvidenceDTO {
	return analyzerSlotChangedEvidenceDTO{
		InstalledVersionIndex: source.InstalledVersionIndex,
	}
}

func convertAnalyzerDowngradePlannedEvidence(source storedAnalyzerDowngradePlannedEvidence) analyzerDowngradePlannedEvidenceDTO {
	return analyzerDowngradePlannedEvidenceDTO{
		EntryIndex:        source.EntryIndex,
		Operation:         source.Operation,
		InstalledVersions: v1Slice(source.InstalledVersions, func(value string) string { return value }),
		TargetVersion:     source.TargetVersion,
	}
}

func convertAnalyzerUnresolvedBlockerEvidence(source storedAnalyzerUnresolvedBlockerEvidence) analyzerUnresolvedBlockerEvidenceDTO {
	return analyzerUnresolvedBlockerEvidenceDTO{
		BlockerIndex: source.BlockerIndex,
	}
}
