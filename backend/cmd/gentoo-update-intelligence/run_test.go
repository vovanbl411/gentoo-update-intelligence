package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/changereport"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
)

type syntheticFile struct {
	content string
	symlink string
}

type repository struct {
	path   string
	commit string
}

type brokenReader struct{ err error }

func (reader brokenReader) Read([]byte) (int, error) { return 0, reader.err }

type brokenWriter struct{ err error }

func (writer brokenWriter) Write([]byte) (int, error) { return 0, writer.err }

type decodedResult struct {
	Plan     parser.Result
	Analysis decodedAnalysis
	Reports  []cliReport
}

type decodedAnalysis struct {
	Status      parser.Status
	Diagnostics []parser.Diagnostic
	Findings    []decodedFinding
}

type decodedFinding struct {
	RuleID     analyzer.RuleID
	Category   analyzer.Category
	Message    string
	EntryIndex *int
	Evidence   json.RawMessage
}

func TestFileAndStdinProduceIdenticalJSON(t *testing.T) {
	repo := newRepository(t, nil)
	input := "[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]\n"
	planPath := filepath.Join(t.TempDir(), "plan.txt")
	if err := os.WriteFile(planPath, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	common := []string{"-repo", "gentoo", "-repo-path", repo.path, "-commit", repo.commit}
	fileArgs := append([]string{"-plan", planPath}, common...)
	stdinArgs := append([]string{"-plan", "-"}, common...)

	fileStdout, fileStderr, fileCode := runCLI(t, fileArgs, strings.NewReader(""))
	stdinStdout, stdinStderr, stdinCode := runCLI(t, stdinArgs, strings.NewReader(input))
	if fileCode != exitSuccess || stdinCode != exitSuccess || fileStderr.Len() != 0 || stdinStderr.Len() != 0 {
		t.Fatalf("file code/stderr=%d/%q; stdin code/stderr=%d/%q", fileCode, fileStderr.String(), stdinCode, stdinStderr.String())
	}
	if !bytes.Equal(fileStdout.Bytes(), stdinStdout.Bytes()) {
		t.Fatalf("file and stdin JSON differ:\nfile: %s\nstdin: %s", fileStdout, stdinStdout)
	}
	var result decodedResult
	if err := json.Unmarshal(fileStdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, fileStdout)
	}
	if result.Plan.Status != parser.StatusParsed || len(result.Plan.Entries) != 1 || len(result.Reports) != 1 || result.Reports[0].Report.EntryIndex != 0 {
		t.Fatalf("incomplete result: %#v", result)
	}
	if result.Reports[0].BeforeLookup.Err != nil || result.Reports[0].AfterLookup.Err != nil || result.Reports[0].BeforeLookup.Result.Status != metadataprovider.StatusNotFound {
		t.Fatalf("normal Lookup outcomes = %#v / %#v", result.Reports[0].BeforeLookup, result.Reports[0].AfterLookup)
	}
}

func TestCLIEndToEndWithSyntheticMetadataAndCompleteJSON(t *testing.T) {
	repo := newRepository(t, map[string]syntheticFile{
		"dev-libs/widget-1": {content: "EAPI=8\nSLOT=1/1\nIUSE=old\n"},
		"dev-libs/widget-2": {content: "EAPI=8\nSLOT=2/2\nIUSE=new\n"},
		"dev-libs/gadget-1": {content: "EAPI=8\nSLOT=1/1\nIUSE=after\n"},
		"dev-libs/gadget-2": {content: "EAPI=8\nSLOT=2/2\nIUSE=before\n"},
	})
	input := strings.Join([]string{
		"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]",
		"[ebuild UD] dev-libs/gadget-1::gentoo [2::gentoo]",
		"[ebuild U] dev-libs/missing-2::gentoo [1::gentoo]",
		"[ebuild U] app-misc/other-2::overlay [1::overlay]",
		"[ebuild NS] app-misc/new-1::gentoo",
		"[blocks B] <dev-libs/old-2 (is hard blocking dev-libs/widget-2)",
		"[uninstall] app-misc/old-1",
		"WARNING: One or more updates/rebuilds have been skipped due to a dependency conflict:",
	}, "\n")
	planPath := filepath.Join(t.TempDir(), "emerge-plan.txt")
	if err := os.WriteFile(planPath, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotRepository(t, repo.path)
	stdout, stderr, code := runCLI(t, []string{
		"-plan", planPath,
		"-repo", "gentoo",
		"-repo-path", repo.path,
		"-commit", repo.commit,
		"-local-patched", "dev-libs/widget",
		"-local-patched", "dev-libs/gadget",
		"-local-patched", "dev-libs/gadget",
	}, strings.NewReader(""))
	after := snapshotRepository(t, repo.path)
	if code != exitSuccess || stderr.Len() != 0 {
		t.Fatalf("code/stderr=%d/%q", code, stderr.String())
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("CLI changed metadata repository:\nbefore=%#v\nafter=%#v", before, after)
	}
	var result decodedResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if result.Plan.Status != parser.StatusPartial || len(result.Plan.Entries) != 5 || len(result.Plan.Blockers) != 1 || len(result.Plan.Removals) != 1 || len(result.Plan.Diagnostics) != 1 || !reflect.DeepEqual(result.Plan.Diagnostics[0].SourceLines, []int{8}) {
		t.Fatalf("Parser facts were not preserved: %#v", result.Plan)
	}
	if !reflect.DeepEqual(result.Plan, parser.Parse(input)) {
		t.Fatal("JSON Plan differs from full Parser result")
	}
	if len(result.Reports) != len(result.Plan.Entries) {
		t.Fatalf("reports=%d entries=%d", len(result.Reports), len(result.Plan.Entries))
	}
	for index, report := range result.Reports {
		if report.Report.EntryIndex != index || !reflect.DeepEqual(report.Report.SourceLines, []int{index + 1}) {
			t.Errorf("report[%d] index/source lines=%d/%v", index, report.Report.EntryIndex, report.Report.SourceLines)
		}
	}

	upgrade := result.Reports[0]
	comparison := upgrade.Report.Gentoo.Comparison
	if comparison == nil || comparison.Before.Source.Commit != repo.commit || comparison.After.Source.Commit != repo.commit || comparison.Before.Source.SourceURL != "local://gentoo" {
		t.Fatalf("metadata provenance missing or misleading: %#v", comparison)
	}
	if comparison.Before.Source.Version != "1" || comparison.After.Source.Version != "2" || comparison.Changes.Slot == nil || comparison.Changes.Slot.Before != "1" || comparison.Changes.Slot.After != "2" || len(comparison.Changes.IUSE.Added) != 1 || len(comparison.Changes.IUSE.Removed) != 1 {
		t.Fatalf("upgrade diff lost: %#v", comparison)
	}
	if !reflect.DeepEqual(comparison.Changes.Slot.Evidence.Before.Lines, []int{2}) || !reflect.DeepEqual(comparison.Changes.IUSE.Added[0].Evidence.After.Lines, []int{3}) {
		t.Fatalf("metadata evidence lost: %#v", comparison.Changes)
	}
	if upgrade.Report.InterpretationLimitation == "" || upgrade.Report.EffectiveConfig.Reason != changereport.ReasonNotImplemented || upgrade.Report.Upstream.Reason != changereport.ReasonNotImplemented {
		t.Fatalf("report limitations/sections were lost: %#v", upgrade.Report)
	}
	if upgrade.BeforeLookup.Err != nil || upgrade.AfterLookup.Err != nil || upgrade.BeforeLookup.Query.Version != "1" || upgrade.AfterLookup.Query.Version != "2" {
		t.Fatalf("lookup fields not serialized: %#v", upgrade)
	}
	downgradeComparison := result.Reports[1].Report.Gentoo.Comparison
	if downgradeComparison == nil || downgradeComparison.Before.Source.Commit != repo.commit || downgradeComparison.After.Source.Commit != repo.commit || downgradeComparison.Before.Source.Version != "2" || downgradeComparison.After.Source.Version != "1" || downgradeComparison.Changes.Slot == nil || downgradeComparison.Changes.Slot.Before != "2" || downgradeComparison.Changes.Slot.After != "1" {
		t.Fatalf("downgrade direction/metadata lost: %#v", downgradeComparison)
	}
	if !reflect.DeepEqual(downgradeComparison.Changes.Slot.Evidence.Before.Lines, []int{2}) || !reflect.DeepEqual(downgradeComparison.Changes.Slot.Evidence.After.Lines, []int{2}) {
		t.Fatalf("downgrade evidence lost: %#v", downgradeComparison.Changes.Slot.Evidence)
	}
	if result.Reports[2].BeforeLookup.Result.Status != metadataprovider.StatusNotFound || result.Reports[2].BeforeLookup.Err != nil || result.Reports[2].AfterLookup.Result.Status != metadataprovider.StatusNotFound {
		t.Fatalf("not_found status or null Err lost: %#v", result.Reports[2])
	}
	if result.Reports[3].BeforeLookup.Result.Status != metadataprovider.StatusRepositoryUnavailable || result.Reports[3].AfterLookup.Result.Status != metadataprovider.StatusRepositoryUnavailable {
		t.Fatalf("repository_unavailable status lost: %#v", result.Reports[3])
	}
	if result.Reports[4].BeforeLookup.SkipReason != reportservice.SkipBindingRejected || result.Reports[4].AfterLookup.SkipReason != reportservice.SkipBindingRejected || result.Reports[4].BeforeLookup.Query != nil {
		t.Fatalf("NS skip outcome lost: %#v", result.Reports[4])
	}

	if result.Analysis.Status != parser.StatusPartial || !reflect.DeepEqual(result.Analysis.Diagnostics, result.Plan.Diagnostics) || len(result.Analysis.Findings) != 4 {
		t.Fatalf("Analysis result incomplete: %#v", result.Analysis)
	}
	findings := result.Analysis.Findings
	if findings[0].RuleID != analyzer.RuleLocalPatchedPackage || findings[0].EntryIndex == nil || *findings[0].EntryIndex != 0 || !hasJSONField(findings[0].Evidence, "ContextIndex", float64(0)) {
		t.Errorf("first package finding/evidence = %#v", findings[0])
	}
	if findings[1].RuleID != analyzer.RuleDowngradePlanned || findings[1].EntryIndex == nil || *findings[1].EntryIndex != 1 || !strings.Contains(findings[1].Message, "2 → 1") || !hasJSONField(findings[1].Evidence, "EntryIndex", float64(1)) {
		t.Errorf("downgrade finding/evidence = %#v", findings[1])
	}
	if findings[2].RuleID != analyzer.RuleLocalPatchedPackage || findings[2].EntryIndex == nil || *findings[2].EntryIndex != 1 || !hasJSONField(findings[2].Evidence, "ContextIndex", float64(1)) {
		t.Errorf("second local-patched finding/evidence = %#v", findings[2])
	}
	if findings[3].RuleID != analyzer.RuleUnresolvedBlocker || findings[3].Category != analyzer.CategoryBlocker || findings[3].EntryIndex != nil || !hasJSONField(findings[3].Evidence, "BlockerIndex", float64(0)) {
		t.Errorf("blocker finding/evidence = %#v", findings[3])
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"EntryIndex": null`)) || !bytes.Contains(stdout.Bytes(), []byte(`"Err": null`)) {
		t.Fatal("null references or errors are missing from JSON")
	}
}

func TestCLIReportsLookupErrorAsStringAndExitsSuccessfully(t *testing.T) {
	repo := newRepository(t, map[string]syntheticFile{
		"dev-libs/widget-1": {symlink: "missing-cache-target"},
		"dev-libs/widget-2": {content: "EAPI=8\nSLOT=0\nIUSE=\n"},
	})
	inputPath := writePlan(t, "[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]")
	stdout, stderr, code := runCLI(t, argsForPlan(repo, inputPath), strings.NewReader(""))
	if code != exitSuccess || stderr.Len() != 0 {
		t.Fatalf("code/stderr=%d/%q", code, stderr.String())
	}
	var result decodedResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	outcome := result.Reports[0].BeforeLookup
	if outcome.Err == nil || !strings.Contains(*outcome.Err, "unsupported metadata tree entry") || outcome.Result != nil || outcome.Query == nil {
		t.Fatalf("Lookup error not rendered as string: %#v", outcome)
	}
	if result.Reports[0].AfterLookup.Err != nil || result.Reports[0].AfterLookup.Result.Status != metadataprovider.StatusFound {
		t.Fatalf("successful side was lost: %#v", result.Reports[0].AfterLookup)
	}
}

func TestCLIUsesExistingParserFixtureWithoutChangingIt(t *testing.T) {
	repo := newRepository(t, nil)
	fixture, err := os.ReadFile("../../../testdata/parser/ebuild-updates.txt")
	if err != nil {
		t.Fatal(err)
	}
	planPath := writePlan(t, string(fixture))
	stdout, stderr, code := runCLI(t, argsForPlan(repo, planPath), strings.NewReader(""))
	if code != exitSuccess || stderr.Len() != 0 {
		t.Fatalf("code/stderr=%d/%q", code, stderr.String())
	}
	var result decodedResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !reflect.DeepEqual(result.Plan, parser.Parse(string(fixture))) || result.Plan.Status != parser.StatusParsed || len(result.Plan.Entries) != 6 || len(result.Reports) != 6 {
		t.Fatalf("fixture result differs from Parser: entries=%d reports=%d status=%q", len(result.Plan.Entries), len(result.Reports), result.Plan.Status)
	}
	if len(result.Analysis.Findings) != 0 || result.Reports[4].BeforeLookup.SkipReason != reportservice.SkipBindingRejected {
		t.Fatalf("fixture Analysis/NS report = %#v / %#v", result.Analysis, result.Reports[4])
	}
}

func TestCLIUnsupportedInputAndBlockerStillReturnJSON(t *testing.T) {
	repo := newRepository(t, nil)
	cases := []struct {
		name   string
		input  string
		status parser.Status
		block  bool
	}{
		{name: "unsupported", input: "unrecognized significant text", status: parser.StatusUnsupported},
		{name: "blocker", input: "[blocks B] cat/old", status: parser.StatusParsed, block: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			planPath := writePlan(t, test.input)
			stdout, stderr, code := runCLI(t, argsForPlan(repo, planPath), strings.NewReader(""))
			if code != exitSuccess || stderr.Len() != 0 {
				t.Fatalf("code/stderr=%d/%q", code, stderr.String())
			}
			var result decodedResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if result.Plan.Status != test.status || len(result.Reports) != 0 {
				t.Fatalf("result = %#v", result)
			}
			if test.block && (len(result.Analysis.Findings) != 1 || result.Analysis.Findings[0].Category != analyzer.CategoryBlocker || result.Analysis.Findings[0].EntryIndex != nil) {
				t.Fatalf("blocker result = %#v", result.Analysis)
			}
		})
	}
}

func TestCLIArgumentErrorsReturnTwoWithoutJSON(t *testing.T) {
	valid := []string{"-plan", "plan.txt", "-repo", "gentoo", "-repo-path", "/unused", "-commit", strings.Repeat("a", 40)}
	cases := []struct {
		name string
		args []string
	}{
		{name: "unknown option", args: append(append([]string(nil), valid...), "-unknown")},
		{name: "missing all", args: nil},
		{name: "missing plan", args: []string{"-repo", "gentoo", "-repo-path", "/unused", "-commit", strings.Repeat("a", 40)}},
		{name: "missing repository", args: []string{"-plan", "plan.txt", "-repo-path", "/unused", "-commit", strings.Repeat("a", 40)}},
		{name: "missing repository path", args: []string{"-plan", "plan.txt", "-repo", "gentoo", "-commit", strings.Repeat("a", 40)}},
		{name: "missing commit", args: []string{"-plan", "plan.txt", "-repo", "gentoo", "-repo-path", "/unused"}},
		{name: "empty plan", args: []string{"-plan=", "-repo", "gentoo", "-repo-path", "/unused", "-commit", strings.Repeat("a", 40)}},
		{name: "empty repository", args: []string{"-plan", "plan.txt", "-repo=", "-repo-path", "/unused", "-commit", strings.Repeat("a", 40)}},
		{name: "empty repository path", args: []string{"-plan", "plan.txt", "-repo", "gentoo", "-repo-path=", "-commit", strings.Repeat("a", 40)}},
		{name: "empty commit", args: []string{"-plan", "plan.txt", "-repo", "gentoo", "-repo-path", "/unused", "-commit="}},
		{name: "positional argument", args: append(append([]string(nil), valid...), "extra")},
		{name: "versioned package", args: append(append([]string(nil), valid...), "-local-patched", "cat/pkg-1.2")},
		{name: "operator", args: append(append([]string(nil), valid...), "-local-patched", "=cat/pkg")},
		{name: "wildcard", args: append(append([]string(nil), valid...), "-local-patched", "cat/pkg*")},
		{name: "slot", args: append(append([]string(nil), valid...), "-local-patched", "cat/pkg:0")},
		{name: "repository", args: append(append([]string(nil), valid...), "-local-patched", "cat/pkg::gentoo")},
		{name: "spaces", args: append(append([]string(nil), valid...), "-local-patched", "cat/pkg name")},
		{name: "multiple separators", args: append(append([]string(nil), valid...), "-local-patched", "cat/sub/pkg")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, test.args, strings.NewReader(""))
			if code != exitInvalidArgs || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("code/stdout/stderr=%d/%q/%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestCLIHelpReturnsZeroOnStderr(t *testing.T) {
	for _, help := range []string{"-h", "-help"} {
		stdout, stderr, code := runCLI(t, []string{help}, strings.NewReader(""))
		if code != exitSuccess || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage:") || !strings.Contains(stderr.String(), "-plan") {
			t.Errorf("%s: code/stdout/stderr=%d/%q/%q", help, code, stdout.String(), stderr.String())
		}
	}
}

func TestCLIExecutionErrorsReturnOneWithoutJSON(t *testing.T) {
	t.Run("file read", func(t *testing.T) {
		repo := newRepository(t, nil)
		args := argsForPlan(repo, filepath.Join(t.TempDir(), "missing-plan.txt"))
		stdout, stderr, code := runCLI(t, args, strings.NewReader(""))
		assertExecutionFailure(t, stdout, stderr, code)
	})

	t.Run("stdin read", func(t *testing.T) {
		repo := newRepository(t, nil)
		args := argsForPlan(repo, "-")
		stdout, stderr, code := runCLI(t, args, brokenReader{err: errors.New("synthetic stdin failure")})
		assertExecutionFailure(t, stdout, stderr, code)
	})

	t.Run("provider open", func(t *testing.T) {
		planPath := writePlan(t, "")
		args := []string{"-plan", planPath, "-repo", "gentoo", "-repo-path", filepath.Join(t.TempDir(), "not-a-repository"), "-commit", strings.Repeat("a", 40)}
		stdout, stderr, code := runCLI(t, args, strings.NewReader(""))
		assertExecutionFailure(t, stdout, stderr, code)
	})

	t.Run("provider still opens without entries", func(t *testing.T) {
		planPath := writePlan(t, "unrecognized significant text")
		args := []string{"-plan", planPath, "-repo", "gentoo", "-repo-path", filepath.Join(t.TempDir(), "not-a-repository"), "-commit", strings.Repeat("a", 40)}
		stdout, stderr, code := runCLI(t, args, strings.NewReader(""))
		assertExecutionFailure(t, stdout, stderr, code)
	})

	t.Run("stdout write", func(t *testing.T) {
		repo := newRepository(t, nil)
		planPath := writePlan(t, "")
		args := argsForPlan(repo, planPath)
		stderr := &bytes.Buffer{}
		code := run(args, strings.NewReader(""), brokenWriter{err: errors.New("synthetic stdout failure")}, stderr)
		if code != exitFailure || !strings.Contains(stderr.String(), "write result") {
			t.Fatalf("code/stderr=%d/%q", code, stderr.String())
		}
	})
}

func TestCLIRejectsShortStdoutWrites(t *testing.T) {
	repo := newRepository(t, nil)
	planPath := writePlan(t, "")
	args := argsForPlan(repo, planPath)
	stderr := &bytes.Buffer{}
	code := run(args, strings.NewReader(""), shortWriter{}, stderr)
	if code != exitFailure || !strings.Contains(stderr.String(), "short write") {
		t.Fatalf("code/stderr=%d/%q", code, stderr.String())
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) {
	return len(data) / 2, nil
}

func runCLI(t *testing.T, args []string, stdin io.Reader) (*bytes.Buffer, *bytes.Buffer, int) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(args, stdin, stdout, stderr)
	return stdout, stderr, code
}

func newRepository(t *testing.T, files map[string]syntheticFile) repository {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitForTest(t, root, "init", "--quiet", "--initial-branch=main")
	if err := os.MkdirAll(filepath.Join(root, "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "metadata", ".synthetic-repository"), []byte("local test data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for relative, file := range files {
		fullPath := filepath.Join(root, "metadata", "md5-cache", filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if file.symlink != "" {
			if err := os.Symlink(file.symlink, fullPath); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(fullPath, []byte(file.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitForTest(t, root, "add", "metadata")
	gitForTest(t, root,
		"-c", "user.name=Synthetic CLI Test",
		"-c", "user.email=synthetic-cli@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "synthetic metadata",
	)
	commit := strings.TrimSpace(gitOutputForTest(t, root, "rev-parse", "HEAD"))
	return repository{path: root, commit: commit}
}

func gitForTest(t *testing.T, root string, args ...string) {
	t.Helper()
	if _, err := gitOutput(t, root, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func gitOutputForTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := gitOutput(t, root, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(output)
}

func gitOutput(t *testing.T, root string, args ...string) ([]byte, error) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

type repositorySnapshot struct {
	head   string
	status string
	index  []byte
	refs   string
}

func snapshotRepository(t *testing.T, root string) repositorySnapshot {
	t.Helper()
	index, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	return repositorySnapshot{
		head:   strings.TrimSpace(gitOutputForTest(t, root, "rev-parse", "HEAD")),
		status: gitOutputForTest(t, root, "status", "--porcelain=v1", "--untracked-files=all"),
		index:  index,
		refs:   gitOutputForTest(t, root, "for-each-ref", "--format=%(refname) %(objectname)"),
	}
}

func writePlan(t *testing.T, input string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.txt")
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func argsForPlan(repo repository, plan string) []string {
	return []string{"-plan", plan, "-repo", "gentoo", "-repo-path", repo.path, "-commit", repo.commit}
}

func assertExecutionFailure(t *testing.T, stdout, stderr *bytes.Buffer, code int) {
	t.Helper()
	if code != exitFailure || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("code/stdout/stderr=%d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func hasJSONField(value json.RawMessage, name string, expected any) bool {
	fields := make(map[string]any)
	if err := json.Unmarshal(value, &fields); err != nil {
		return false
	}
	actual, ok := fields[name]
	return ok && reflect.DeepEqual(actual, expected)
}
