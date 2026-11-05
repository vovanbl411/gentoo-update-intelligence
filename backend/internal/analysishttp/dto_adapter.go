package analysishttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type apiErrorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type createRequestDTO struct {
	Plan                   string             `json:"plan"`
	LocallyPatchedPackages []parserPackageDTO `json:"locally_patched_packages"`
}

func lookupError(message *string) *apiErrorDTO {
	if message == nil {
		return nil
	}
	return &apiErrorDTO{Code: "METADATA_LOOKUP_FAILED", Message: *message}
}

func v1Slice[S, D any](source []S, convert func(S) D) []D {
	result := make([]D, len(source))
	for i, value := range source {
		result[i] = convert(value)
	}
	return result
}

func v1Pointer[S, D any](source *S, convert func(S) D) *D {
	if source == nil {
		return nil
	}
	result := convert(*source)
	return &result
}

func v1Map[S, D any](source map[string]S, convert func(S) D) map[string]D {
	result := make(map[string]D, len(source))
	for key, value := range source {
		result[key] = convert(value)
	}
	return result
}

// decodeStoredResult is the only bridge from the existing persistence format
// to the public v1 schema. It never changes the stored JSON or reruns analysis.
func decodeStoredResult(data []byte) (*resultDTO, error) {
	var stored storedResult
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	if stored.Plan == nil || stored.Analysis == nil || !validParserStatus(stored.Plan.Status) || !validParserStatus(stored.Analysis.Status) {
		return nil, errors.New("stored result is missing a valid plan or analysis")
	}
	result := &resultDTO{
		Plan:         convertParserResult(*stored.Plan),
		Analysis:     convertAnalyzerResult(*stored.Analysis),
		Reports:      v1Slice(stored.Reports, convertReport),
		Repositories: v1Map(stored.Repositories, convertRepositorymanagerRevision),
	}
	for i, finding := range stored.Analysis.Findings {
		evidence, err := decodeStoredEvidence(finding.RuleID, finding.Evidence)
		if err != nil {
			return nil, fmt.Errorf("finding %d: %w", i, err)
		}
		result.Analysis.Findings[i].Evidence = evidence
	}
	return result, nil
}

func validParserStatus(value string) bool {
	return value == "parsed" || value == "partial" || value == "unsupported"
}

func readEvidence[S any, D findingEvidenceDTO](data []byte, convert func(S) D, required ...string) (findingEvidenceDTO, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, name := range required {
		value, present := fields[name]
		if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("stored evidence is missing %s", name)
		}
	}
	var stored S
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	return convert(stored), nil
}

func decodeStoredEvidence(rule string, data []byte) (findingEvidenceDTO, error) {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	switch rule {
	case "LOCAL_PATCHED_PACKAGE":
		return readEvidence(data, convertAnalyzerLocalPatchedPackageEvidence, "ContextIndex")
	case "USE_CHANGED":
		return readEvidence(data, convertAnalyzerUseChangedEvidence)
	case "SLOT_CHANGED":
		return readEvidence(data, convertAnalyzerSlotChangedEvidence, "InstalledVersionIndex")
	case "DOWNGRADE_PLANNED":
		return readEvidence(data, convertAnalyzerDowngradePlannedEvidence, "EntryIndex", "Operation", "TargetVersion")
	case "UNRESOLVED_BLOCKER":
		return readEvidence(data, convertAnalyzerUnresolvedBlockerEvidence, "BlockerIndex")
	default:
		return nil, fmt.Errorf("unsupported stored evidence rule %q", rule)
	}
}
