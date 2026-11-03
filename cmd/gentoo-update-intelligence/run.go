package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analyzer"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/changereport"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/planservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
)

const (
	exitSuccess       = 0
	exitFailure       = 1
	exitInvalidArgs   = 2
	localSourcePrefix = "local://"
)

var versionSuffix = regexp.MustCompile(`-[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$`)

type packageListFlag []parser.Package

func (packages *packageListFlag) String() string {
	values := make([]string, len(*packages))
	for index, pkg := range *packages {
		values[index] = pkg.Category + "/" + pkg.Name
	}
	return strings.Join(values, ",")
}

func (packages *packageListFlag) Set(value string) error {
	if !validPackagePair(value) {
		return fmt.Errorf("invalid package pair %q; expected exact category/name", value)
	}
	category, name, _ := strings.Cut(value, "/")
	*packages = append(*packages, parser.Package{Category: category, Name: name})
	return nil
}

type cliResult struct {
	Plan     parser.Result
	Analysis analyzer.Result
	Reports  []cliReport
}

type cliReport struct {
	Report       changereport.Report
	BeforeLookup cliLookupOutcome
	AfterLookup  cliLookupOutcome
}

type cliLookupOutcome struct {
	Query      *metadataprovider.Query
	Result     *metadataprovider.LookupResult
	Err        *string
	SkipReason reportservice.SkipReason
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gentoo-update-intelligence", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: gentoo-update-intelligence -plan PATH -repo NAME -repo-path PATH -commit FULL-SHA [-local-patched category/name ...]")
		flags.PrintDefaults()
	}

	var planPath, repository, repositoryPath, commit string
	var localPatched packageListFlag
	flags.StringVar(&planPath, "plan", "", "text plan file; use - to read stdin")
	flags.StringVar(&repository, "repo", "", "logical repository name for Metadata Provider")
	flags.StringVar(&repositoryPath, "repo-path", "", "path to local Git repository with metadata cache")
	flags.StringVar(&commit, "commit", "", "full commit SHA selected by the user")
	flags.Var(&localPatched, "local-patched", "exact category/name with local patches; may be repeated")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitSuccess
		}
		return exitInvalidArgs
	}
	if flags.NArg() != 0 {
		return argumentError(stderr, flags, "positional arguments are not supported")
	}
	if planPath == "" || repository == "" || repositoryPath == "" || commit == "" {
		return argumentError(stderr, flags, "-plan, -repo, -repo-path, and -commit require non-empty values")
	}

	var input []byte
	var err error
	if planPath == "-" {
		input, err = io.ReadAll(stdin)
	} else {
		input, err = os.ReadFile(planPath)
	}
	if err != nil {
		return executionError(stderr, "read plan", err)
	}

	provider, err := metadataprovider.Open(metadataprovider.Config{
		Repository: repository,
		SourceURL:  localSourcePrefix + repository,
		LocalPath:  repositoryPath,
		Commit:     commit,
	})
	if err != nil {
		return executionError(stderr, "open metadata provider", err)
	}

	result, err := planservice.Analyze(string(input), analyzer.Context{LocallyPatchedPackages: localPatched}, provider)
	if err != nil {
		return executionError(stderr, "analyze plan", err)
	}

	encoded, err := json.MarshalIndent(presentResult(result), "", "  ")
	if err != nil {
		return executionError(stderr, "serialize result", err)
	}
	encoded = append(encoded, '\n')
	written, err := stdout.Write(encoded)
	if err != nil {
		return executionError(stderr, "write result", err)
	}
	if written != len(encoded) {
		return executionError(stderr, "write result", io.ErrShortWrite)
	}
	return exitSuccess
}

func presentResult(result planservice.Result) cliResult {
	out := cliResult{
		Plan:     result.Plan,
		Analysis: result.Analysis,
		Reports:  make([]cliReport, len(result.Reports)),
	}
	for index, report := range result.Reports {
		out.Reports[index] = cliReport{
			Report:       report.Report,
			BeforeLookup: presentLookupOutcome(report.BeforeLookup),
			AfterLookup:  presentLookupOutcome(report.AfterLookup),
		}
	}
	return out
}

func presentLookupOutcome(outcome reportservice.LookupOutcome) cliLookupOutcome {
	view := cliLookupOutcome{
		Query:      outcome.Query,
		Result:     outcome.Result,
		SkipReason: outcome.SkipReason,
	}
	if outcome.Err != nil {
		message := outcome.Err.Error()
		view.Err = &message
	}
	return view
}

func argumentError(stderr io.Writer, flags *flag.FlagSet, message string) int {
	fmt.Fprintf(stderr, "error: %s\n", message)
	flags.Usage()
	return exitInvalidArgs
}

func executionError(stderr io.Writer, operation string, err error) int {
	fmt.Fprintf(stderr, "error: %s: %v\n", operation, err)
	return exitFailure
}

func validPackagePair(value string) bool {
	if strings.Count(value, "/") != 1 {
		return false
	}
	category, name, _ := strings.Cut(value, "/")
	if category == "" || name == "" || !validCategory(category) || !validPackageName(name) {
		return false
	}
	return !versionSuffix.MatchString(name)
}

func validCategory(value string) bool {
	if value[0] == '-' || value[0] == '.' || value[0] == '+' {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !asciiAlphanumeric(character) && character != '+' && character != '_' && character != '.' && character != '-' {
			return false
		}
	}
	return true
}

func validPackageName(value string) bool {
	if value[0] == '-' || value[0] == '+' {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !asciiAlphanumeric(character) && character != '+' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func asciiAlphanumeric(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}
