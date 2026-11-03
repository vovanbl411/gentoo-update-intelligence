package metadatagenerator

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/ebuildresolver"
)

func TestGenerateReadsExactVersionAndReturnsProvenance(t *testing.T) {
	fixture := newFixture(t)
	fixture.stubEgencache(t, `#!/bin/sh
mkdir -p metadata/md5-cache/dev-libs
printf 'EAPI=8\nDEPEND=exact\n' > metadata/md5-cache/dev-libs/libei-1.6.0-r1
printf 'EAPI=8\nDEPEND=neighbor\n' > metadata/md5-cache/dev-libs/libei-1.6.0
printf '%s\n' "$@" > `+shellQuote(filepath.Join(fixture.root, "args.log"))+`
`)
	result, err := fixture.generator().Generate(fixture.request())
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Content) != "EAPI=8\nDEPEND=exact\n" {
		t.Fatalf("content = %q, want exact requested version", result.Content)
	}
	if result.SHA256 == "" || result.SHA256 != result.Provenance.CacheSHA256 {
		t.Fatalf("SHA-256 not preserved in provenance: %#v", result)
	}
	if result.Provenance.ReferenceCommit != fixture.commit || result.Provenance.SelectedCommit != fixture.commit || result.Provenance.EbuildBlobID != fixture.blobID {
		t.Fatalf("source provenance = %#v", result.Provenance)
	}
	if result.Provenance.PortageVersion != "test-version" || result.Provenance.Profile != "profiles/test" {
		t.Fatalf("runtime provenance = %#v", result.Provenance)
	}
	if result.Provenance.MakeConf != "" {
		t.Fatalf("default make.conf contains implicit settings: %q", result.Provenance.MakeConf)
	}
	if result.Provenance.EgencachePath != fixture.command {
		t.Fatalf("egencache executable = %q, want %q", result.Provenance.EgencachePath, fixture.command)
	}
	args, err := os.ReadFile(filepath.Join(fixture.root, "args.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--ignore-default-opts", "--repo\ngentoo", "--update\ndev-libs/libei", "--config-root", "--cache-dir", "--repositories-configuration\n[gentoo]\nlocation = " + fixture.root} {
		if !strings.Contains(string(args), expected) {
			t.Errorf("egencache args %q do not include %q", args, expected)
		}
	}
	if strings.Contains(string(args), "--external-cache-only") || strings.Contains(string(args), "--tolerant") {
		t.Fatalf("unexpected tolerant/external-only flag: %q", args)
	}
}

func TestGenerateReportsEgencacheFailure(t *testing.T) {
	fixture := newFixture(t)
	fixture.stubEgencache(t, "#!/bin/sh\necho failed >&2\nexit 7\n")
	_, err := fixture.generator().Generate(fixture.request())
	if !errors.Is(err, ErrEgencache) || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("Generate() error = %v, want egencache failure with output", err)
	}
}

func TestGenerateRequiresExactOutputVersion(t *testing.T) {
	fixture := newFixture(t)
	fixture.stubEgencache(t, "#!/bin/sh\nmkdir -p metadata/md5-cache/dev-libs\nprintf 'DEPEND=neighbor\\n' > metadata/md5-cache/dev-libs/libei-1.6.0\n")
	_, err := fixture.generator().Generate(fixture.request())
	if !errors.Is(err, ErrCacheMissing) {
		t.Fatalf("Generate() error = %v, want exact-cache missing", err)
	}
}

func TestGenerateRejectsWrongSelectedSHA(t *testing.T) {
	fixture := newFixture(t)
	fixture.stubEgencache(t, "#!/bin/sh\nexit 0\n")
	request := fixture.request()
	request.Resolution.SelectedCommit = strings.Repeat("0", 40)
	_, err := fixture.generator().Generate(request)
	if !errors.Is(err, ErrCheckout) {
		t.Fatalf("Generate() error = %v, want selected SHA mismatch", err)
	}
}

func TestGenerateRejectsChangedEbuildContents(t *testing.T) {
	fixture := newFixture(t)
	fixture.stubEgencache(t, "#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(filepath.Join(fixture.root, "dev-libs/libei/libei-1.6.0-r1.ebuild"), []byte("EAPI=8\n# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.git("update-index", "--assume-unchanged", "dev-libs/libei/libei-1.6.0-r1.ebuild")
	_, err := fixture.generator().Generate(fixture.request())
	if !errors.Is(err, ErrCheckout) || !strings.Contains(err.Error(), "contents") {
		t.Fatalf("Generate() error = %v, want dirty/changed checkout rejection", err)
	}
}

func TestGenerateRejectsIgnoredCheckoutFilesBeforeEgencache(t *testing.T) {
	fixture := newFixture(t)
	marker := filepath.Join(fixture.temp, "egencache-ran")
	fixture.stubEgencache(t, "#!/bin/sh\ntouch "+shellQuote(marker)+"\nexit 0\n")
	profile := filepath.Join(fixture.root, "profiles/local")
	if err := os.Mkdir(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "local-profile-data"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	infoExclude := filepath.Join(fixture.root, ".git/info/exclude")
	if err := os.WriteFile(infoExclude, []byte("profiles/local/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := fixture.request()
	request.Profile = profile
	_, err := fixture.generator().Generate(request)
	if !errors.Is(err, ErrCheckout) {
		t.Fatalf("Generate() error = %v, want ignored checkout file rejection", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("egencache ran before rejecting ignored checkout file: stat error = %v", err)
	}
}

func TestGenerateRejectsExistingRequestedCache(t *testing.T) {
	fixture := newFixture(t)
	fixture.stubEgencache(t, "#!/bin/sh\nexit 0\n")
	cache := filepath.Join(fixture.root, "metadata/md5-cache/dev-libs/libei-1.6.0-r1")
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("ready"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := fixture.generator().Generate(fixture.request())
	if !errors.Is(err, ErrCheckout) {
		t.Fatalf("Generate() error = %v, want existing cache rejection", err)
	}
}

func TestGenerateWritesOnlyExplicitPortageIdentityOverrides(t *testing.T) {
	fixture := newFixture(t)
	fixture.stubEgencache(t, "#!/bin/sh\nmkdir -p metadata/md5-cache/dev-libs\nprintf 'EAPI=8\\n' > metadata/md5-cache/dev-libs/libei-1.6.0-r1\n")
	emerge := filepath.Join(filepath.Dir(fixture.root), "emerge")
	if err := os.WriteFile(emerge, []byte("#!/bin/sh\nprintf 'Portage test-version\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := New(Config{
		EgencachePath: fixture.command, EmergePath: emerge,
		PortageUserName: "builder", PortageGroupName: "builders",
	}).Generate(fixture.request())
	if err != nil {
		t.Fatal(err)
	}
	makeConf, err := os.ReadFile(filepath.Join(fixture.temp, "config-root/etc/portage/make.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(makeConf), "PORTAGE_USERNAME=\"builder\"\nPORTAGE_GRPNAME=\"builders\"\n"; got != want {
		t.Fatalf("make.conf = %q, want %q", got, want)
	}
}

type testFixture struct {
	t       *testing.T
	root    string
	temp    string
	commit  string
	blobID  string
	command string
}

func newFixture(t *testing.T) testFixture {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "checkout")
	command := filepath.Join(parent, "egencache")
	fixture := testFixture{t: t, root: root, temp: filepath.Join(parent, "tmp"), command: command}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.temp, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.git("init", "-q")
	fixture.git("config", "user.name", "Test")
	fixture.git("config", "user.email", "test@example.invalid")
	fixture.git("config", "commit.gpgsign", "false")
	if err := os.MkdirAll(filepath.Join(root, "dev-libs/libei"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "profiles/test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dev-libs/libei/libei-1.6.0-r1.ebuild"), []byte("EAPI=8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles/test/eapi"), []byte("8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.git("add", ".")
	fixture.git("commit", "-qm", "fixture")
	fixture.commit = strings.TrimSpace(fixture.git("rev-parse", "HEAD"))
	fixture.blobID = strings.TrimSpace(fixture.git("rev-parse", "HEAD:dev-libs/libei/libei-1.6.0-r1.ebuild"))
	return fixture
}

func (f testFixture) generator() *Generator {
	f.t.Helper()
	emerge := filepath.Join(filepath.Dir(f.root), "emerge")
	if err := os.WriteFile(emerge, []byte("#!/bin/sh\nprintf 'Portage test-version\\n'\n"), 0o755); err != nil {
		f.t.Fatal(err)
	}
	return New(Config{EgencachePath: f.command, EmergePath: emerge})
}

func (f testFixture) request() Request {
	return Request{
		Resolution: ebuildresolver.Result{
			Status: ebuildresolver.StatusFound, ReferenceCommit: f.commit, SelectedCommit: f.commit,
			EbuildPath: "dev-libs/libei/libei-1.6.0-r1.ebuild", BlobID: f.blobID, Reason: ebuildresolver.ReasonAtReference,
		},
		Category: "dev-libs", Name: "libei", Version: "1.6.0-r1",
		Checkout: f.root, Profile: filepath.Join(f.root, "profiles/test"), TempDir: f.temp,
	}
}

func (f testFixture) stubEgencache(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(f.command, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f testFixture) git(args ...string) string {
	f.t.Helper()
	command := exec.Command("git", append([]string{"-C", f.root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
