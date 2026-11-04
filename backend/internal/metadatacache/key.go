// Package metadatacache defines compatibility keys for generated metadata.
package metadatacache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

type Master struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// Inputs contains all reproducibility inputs, not just the ebuild blob.
type Inputs struct {
	Repository       string   `json:"repository"`
	SourceURL        string   `json:"source_url"`
	Category         string   `json:"category"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	ReferenceCommit  string   `json:"reference_commit"`
	SourceCommit     string   `json:"source_commit"`
	EbuildBlob       string   `json:"ebuild_blob"`
	Masters          []Master `json:"masters"`
	Profile          string   `json:"profile"`
	GeneratorVersion string   `json:"generator_version"`
	PortageVersion   string   `json:"portage_version"`
}

var objectID = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)
var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

// Key returns a stable SHA-256 key and preserves master order.
func Key(input Inputs) (string, error) {
	if !repositoryName.MatchString(input.Repository) || input.SourceURL == "" || input.Category == "" || input.Name == "" || input.Version == "" ||
		!objectID.MatchString(input.ReferenceCommit) || !objectID.MatchString(input.SourceCommit) || !objectID.MatchString(input.EbuildBlob) ||
		input.Profile == "" || input.GeneratorVersion == "" || input.PortageVersion == "" {
		return "", errors.New("incomplete or invalid metadata cache inputs")
	}
	input.Masters = append([]Master{}, input.Masters...)
	seen := make(map[string]bool, len(input.Masters))
	for _, master := range input.Masters {
		if !repositoryName.MatchString(master.Name) || master.Name == input.Repository || !objectID.MatchString(master.Commit) || seen[master.Name] {
			return "", fmt.Errorf("invalid or duplicate master %q", master.Name)
		}
		seen[master.Name] = true
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
