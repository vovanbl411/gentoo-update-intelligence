package metadatacache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatagenerator"
)

func resultFor(input Inputs) metadatagenerator.Result {
	content := []byte("EAPI=8\nSLOT=0\nIUSE=foo\n")
	digest := sha256.Sum256(content)
	sha := hex.EncodeToString(digest[:])
	masters := make([]metadatagenerator.MasterRevision, len(input.Masters))
	for i, master := range input.Masters {
		masters[i] = metadatagenerator.MasterRevision{Name: master.Name, Commit: master.Commit}
	}
	return metadatagenerator.Result{Content: content, SHA256: sha, Provenance: metadatagenerator.Provenance{
		ReferenceCommit: input.ReferenceCommit, SelectedCommit: input.SourceCommit, EbuildBlobID: input.EbuildBlob,
		EbuildPath:  input.Category + "/" + input.Name + "/" + input.Name + "-" + input.Version + ".ebuild",
		CachePath:   "metadata/md5-cache/" + input.Category + "/" + input.Name + "-" + input.Version,
		CacheSHA256: sha, Profile: input.Profile, PortageVersion: input.PortageVersion, MasterRevisions: masters,
	}}
}

func TestStoreRoundTripAndIncompatibleInputs(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := sample()
	result := resultFor(input)
	if err := store.Save(input, result); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Load(input)
	if err != nil || !found || string(got.Content) != string(result.Content) {
		t.Fatalf("load = %#v, %t, %v", got, found, err)
	}
	changed := sample()
	changed.Masters[0].Commit = changed.Masters[1].Commit
	_, found, err = store.Load(changed)
	if err != nil || found {
		t.Fatalf("incompatible = %t, %v", found, err)
	}
	result.Provenance.SelectedCommit = changed.Masters[0].Commit
	if err := store.Save(input, result); err == nil {
		t.Fatal("accepted mismatched provenance")
	}
}

func TestStoreRejectsCorruption(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	input := sample()
	if err := store.Save(input, resultFor(input)); err != nil {
		t.Fatal(err)
	}
	key, err := Key(input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved entry
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	saved.Result.Content = []byte("tampered")
	data, err = json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Load(input); err == nil || found {
		t.Fatalf("corrupted cache = %t, %v", found, err)
	}
}
