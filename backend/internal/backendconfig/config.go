// Package backendconfig loads backend settings once, before external services
// are opened. It deliberately never searches for files or logs their contents.
package backendconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
	"go.yaml.in/yaml/v3"
)

const Prefix = "GENTOO_UPDATE_INTELLIGENCE_"

type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag != "!!str" {
		return fmt.Errorf("duration must be a string with units at line %d", node.Line)
	}
	value, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration at line %d", node.Line)
	}
	*d = Duration(value)
	return nil
}

type Limit int

func (limit *Limit) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag != "!!int" {
		return fmt.Errorf("analysis limit must be an integer at line %d", node.Line)
	}
	var value int
	if err := node.Decode(&value); err != nil {
		return fmt.Errorf("analysis limit is out of range at line %d", node.Line)
	}
	*limit = Limit(value)
	return nil
}

type HTTP struct {
	ListenAddress     string   `yaml:"listen_address"`
	ReadHeaderTimeout Duration `yaml:"read_header_timeout"`
	IdleTimeout       Duration `yaml:"idle_timeout"`
}

type Analysis struct {
	ConcurrencyLimit Limit `yaml:"concurrency_limit"`
}

type Repository struct {
	Name      string `yaml:"name"`
	SourceURL string `yaml:"source_url"`
	Path      string `yaml:"path"`
	Commit    string `yaml:"commit"`
}

type Overlays struct {
	Path    string                         `yaml:"path"`
	Catalog []repositorymanager.Repository `yaml:"catalog"`
}

type Metadata struct {
	WorkerURL     string   `yaml:"worker_url"`
	Profile       string   `yaml:"profile"`
	WorkerTimeout Duration `yaml:"worker_timeout"`
}

type Config struct {
	StartupTimeout Duration   `yaml:"startup_timeout"`
	HTTP           HTTP       `yaml:"http"`
	Analysis       Analysis   `yaml:"analysis"`
	Repository     Repository `yaml:"repository"`
	Overlays       Overlays   `yaml:"overlays"`
	Metadata       Metadata   `yaml:"metadata"`
	DatabaseURL    string     `yaml:"-"`
}

func Defaults() Config {
	return Config{
		StartupTimeout: Duration(15 * time.Second),
		HTTP:           HTTP{ListenAddress: "127.0.0.1:8080", ReadHeaderTimeout: Duration(5 * time.Second), IdleTimeout: Duration(60 * time.Second)},
		Analysis:       Analysis{ConcurrencyLimit: 1},
		Repository:     Repository{Name: "gentoo", SourceURL: "https://anongit.gentoo.org/git/repo/gentoo.git", Path: "/metadata/gentoo.git"},
		Metadata:       Metadata{Profile: "profiles/default/linux/amd64/23.0", WorkerTimeout: Duration(20 * time.Minute)},
	}
}

// Load applies defaults, an explicitly selected YAML file, then nonempty env.
// getenv is injected so tests do not depend on the operator's environment.
func Load(filename string, getenv func(string) string) (Config, error) {
	config := Defaults()
	if filename != "" {
		data, err := os.ReadFile(filename)
		if err != nil {
			return Config{}, fmt.Errorf("config file %q cannot be read", filename)
		}
		if err := decodeYAML(data, &config); err != nil {
			return Config{}, fmt.Errorf("config file %q: %w", filename, err)
		}
	}
	if err := applyEnvironment(&config, getenv); err != nil {
		return Config{}, err
	}
	if config.Metadata.Profile == "" {
		config.Metadata.Profile = Defaults().Metadata.Profile
	}
	if err := config.validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func decodeYAML(data []byte, config *Config) error {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return yamlError(err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("expected one YAML mapping")
	}
	if err := validateNodeTypes(document.Content[0], reflect.TypeFor[Config](), "", make(map[*yaml.Node]bool)); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected exactly one YAML document")
	}
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(config); err != nil {
		return yamlError(err)
	}
	return nil
}

// yaml.v3 otherwise coerces numeric/bool scalars into strings. Check the YAML
// tags against the same typed schema before decoding, without quoting values.
func validateNodeTypes(node *yaml.Node, schema reflect.Type, path string, active map[*yaml.Node]bool) error {
	if node == nil || active[node] {
		return fmt.Errorf("%s: cyclic or invalid YAML alias", path)
	}
	active[node] = true
	defer delete(active, node)
	if node.Kind == yaml.AliasNode {
		return validateNodeTypes(node.Alias, schema, path, active)
	}
	invalid := func() error { return fmt.Errorf("%s: invalid YAML type at line %d", path, node.Line) }
	if schema == reflect.TypeFor[Duration]() {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return invalid()
		}
		return nil
	}
	switch schema.Kind() {
	case reflect.String:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return invalid()
		}
	case reflect.Int:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			return invalid()
		}
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return invalid()
		}
		for i, child := range node.Content {
			if err := validateNodeTypes(child, schema.Elem(), fmt.Sprintf("%s[%d]", path, i), active); err != nil {
				return err
			}
		}
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return invalid()
		}
		fields := make(map[string]reflect.Type, schema.NumField())
		for i := 0; i < schema.NumField(); i++ {
			field := schema.Field(i)
			name := strings.Split(field.Tag.Get("yaml"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = strings.ToLower(field.Name)
			}
			fields[name] = field.Type
		}
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Tag == "!!merge" {
				merged := []*yaml.Node{value}
				if value.Kind == yaml.SequenceNode {
					merged = value.Content
				}
				for _, child := range merged {
					if err := validateNodeTypes(child, schema, path, active); err != nil {
						return err
					}
				}
				continue
			}
			field, ok := fields[key.Value]
			name := key.Value
			if path != "" {
				name = path + "." + name
			}
			if key.Tag != "!!str" || !ok {
				return fmt.Errorf("unknown YAML field at line %d", key.Line)
			}
			if err := validateNodeTypes(value, field, name, active); err != nil {
				return err
			}
		}
	default:
		return invalid()
	}
	return nil
}

var yamlLine = regexp.MustCompile(`line ([0-9]+)`)

// Parser errors may quote input values. Preserve position, not raw values.
func yamlError(err error) error {
	if position := yamlLine.FindStringSubmatch(err.Error()); len(position) > 1 {
		return fmt.Errorf("invalid YAML field or value at line %s (check names, types and duplicate keys)", position[1])
	}
	return errors.New("invalid YAML field or value (check names, types, units and duplicate keys)")
}

func applyEnvironment(config *Config, getenv func(string) string) error {
	for name, target := range map[string]*string{
		"LISTEN_ADDR": &config.HTTP.ListenAddress,
		"REPOSITORY":  &config.Repository.Name, "SOURCE_URL": &config.Repository.SourceURL,
		"REPOSITORY_PATH": &config.Repository.Path, "COMMIT": &config.Repository.Commit,
		"OVERLAY_PATH": &config.Overlays.Path, "METADATA_WORKER_URL": &config.Metadata.WorkerURL,
		"METADATA_PROFILE": &config.Metadata.Profile,
	} {
		if value := getenv(Prefix + name); value != "" {
			*target = value
		}
	}
	if value := getenv(Prefix + "ANALYSIS_CONCURRENCY_LIMIT"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return errors.New(Prefix + "ANALYSIS_CONCURRENCY_LIMIT: expected a non-negative integer")
		}
		config.Analysis.ConcurrencyLimit = Limit(limit)
	}
	for name, target := range map[string]*Duration{
		"STARTUP_TIMEOUT":          &config.StartupTimeout,
		"HTTP_READ_HEADER_TIMEOUT": &config.HTTP.ReadHeaderTimeout,
		"HTTP_IDLE_TIMEOUT":        &config.HTTP.IdleTimeout,
		"METADATA_WORKER_TIMEOUT":  &config.Metadata.WorkerTimeout,
	} {
		if value := getenv(Prefix + name); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("%s%s: expected a positive duration with units", Prefix, name)
			}
			*target = Duration(duration)
		}
	}
	if value := getenv(Prefix + "OVERLAY_CATALOG"); value != "" {
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.DisallowUnknownFields()
		var catalog []repositorymanager.Repository
		if err := decoder.Decode(&catalog); err != nil || catalog == nil {
			return errors.New(Prefix + "OVERLAY_CATALOG: expected a JSON array of repository entries")
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return errors.New(Prefix + "OVERLAY_CATALOG: expected one JSON array")
		}
		config.Overlays.Catalog = catalog
	}
	config.DatabaseURL = getenv(Prefix + "DATABASE_URL")
	return nil
}

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_+.-]*$`)
var commitID = regexp.MustCompile(`^([a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)

func (config Config) validate() error {
	for field, value := range map[string]Duration{
		"startup_timeout":          config.StartupTimeout,
		"http.read_header_timeout": config.HTTP.ReadHeaderTimeout,
		"http.idle_timeout":        config.HTTP.IdleTimeout,
		"metadata.worker_timeout":  config.Metadata.WorkerTimeout,
	} {
		if value <= 0 {
			return fmt.Errorf("%s: expected a positive duration", field)
		}
	}
	if config.Analysis.ConcurrencyLimit < 0 {
		return errors.New("analysis.concurrency_limit: expected a non-negative integer")
	}
	_, port, err := net.SplitHostPort(config.HTTP.ListenAddress)
	if err != nil {
		return errors.New("http.listen_address: expected host:port")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 0 || number > 65535 {
		return errors.New("http.listen_address: expected a valid numeric port")
	}
	if !repositoryName.MatchString(config.Repository.Name) {
		return errors.New("repository.name: invalid repository name")
	}
	for field, value := range map[string]string{"repository.source_url": config.Repository.SourceURL, "repository.path": config.Repository.Path} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("%s: expected a nonempty value without control characters", field)
		}
	}
	if config.Repository.Commit != "" && !commitID.MatchString(config.Repository.Commit) {
		return errors.New("repository.commit: expected a full hexadecimal commit ID or an empty string")
	}
	if config.Overlays.Path != "" && config.Repository.Name != "gentoo" {
		return errors.New("overlays.path: overlay preparation requires repository.name gentoo")
	}
	if _, err := config.OverlayManager(); err != nil {
		return err
	}
	if config.Metadata.WorkerURL != "" {
		endpoint, err := url.Parse(config.Metadata.WorkerURL)
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return errors.New("metadata.worker_url: expected an HTTP(S) URL without credentials, query or fragment")
		}
	}
	if strings.TrimSpace(config.DatabaseURL) == "" {
		return errors.New(Prefix + "DATABASE_URL is required")
	}
	return nil
}

// OverlayManager validates the full trusted catalog without network or disk I/O.
// A disabled path still validates catalog entries; the caller leaves it unused.
func (config Config) OverlayManager() (*repositorymanager.Manager, error) {
	root := config.Overlays.Path
	if root == "" {
		root = "."
	}
	catalog := append(repositorymanager.KnownOverlays(), config.Overlays.Catalog...)
	manager, err := repositorymanager.New(root, catalog)
	if err != nil {
		return nil, errors.New("overlays.catalog: invalid repository entry or master")
	}
	return manager, nil
}
