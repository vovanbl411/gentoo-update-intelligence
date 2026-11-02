package parser

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "parser", "ebuild-updates.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseRealFixture(t *testing.T) {
	// This test reads the accepted real fixture unchanged; all expectations are
	// facts listed in testdata/parser/ebuild-updates.expected.md.
	result := Parse(fixture(t))
	if result.Status != StatusParsed || len(result.Entries) != 6 || len(result.Diagnostics) != 0 {
		t.Fatalf("got status=%q entries=%d diagnostics=%+v", result.Status, len(result.Entries), result.Diagnostics)
	}
	wantPackages := []Package{
		{"dev-libs", "libei"}, {"app-admin", "chezmoi"}, {"app-emulation", "qemu"},
		{"app-editors", "vscode"}, {"sys-kernel", "gentoo-kernel-bin"}, {"virtual", "dist-kernel"},
	}
	wantRepositories := []string{"gentoo", "gentoo-zh", "gentoo", "gentoo", "gentoo", "gentoo"}
	wantOperations := []Operation{OperationUpgrade, OperationUpgrade, OperationUpgrade, OperationUpgrade, OperationInstall, OperationUpgrade}
	wantTargets := []string{"1.6.0-r1", "2.74.0", "10.2.3-r4", "1.144.0-r1", "6.18.60", "6.18.60"}
	wantInstalled := []string{"1.6.0", "2.73.3", "10.2.3-r3", "1.144.0", "6.18.57", "6.18.57"}
	wantSizes := []Size{{0, "KiB"}, {11897, "KiB"}, {0, "KiB"}, {0, "KiB"}, {469316, "KiB"}, {0, "KiB"}}
	wantLines := []int{6, 7, 8, 9, 10, 11}
	for i, entry := range result.Entries {
		if entry.Package != wantPackages[i] || entry.Target.Version != wantTargets[i] || !reflect.DeepEqual(entry.SourceLines, []int{wantLines[i]}) {
			t.Errorf("entry %d identity/target/line mismatch: %+v", i+1, entry)
		}
		if entry.Operation != wantOperations[i] || entry.NewSlot != (i == 4) {
			t.Errorf("entry %d operation/new-slot mismatch: %+v", i+1, entry)
		}
		if entry.Target.Repository == nil || *entry.Target.Repository != wantRepositories[i] {
			t.Errorf("entry %d target repository: %+v", i+1, entry.Target.Repository)
		}
		if entry.Download == nil || *entry.Download != wantSizes[i] {
			t.Errorf("entry %d size: %+v", i+1, entry.Download)
		}
		if len(entry.Installed.Versions) != 1 || entry.Installed.Versions[0].Version != wantInstalled[i] || entry.Installed.Versions[0].Repository == nil {
			t.Errorf("entry %d installed context: %+v", i+1, entry.Installed)
		} else {
			wantRepo := "gentoo"
			if i == 1 {
				wantRepo = "gentoo-zh"
			}
			if *entry.Installed.Versions[0].Repository != wantRepo {
				t.Errorf("entry %d installed repository: %q", i+1, *entry.Installed.Versions[0].Repository)
			}
		}
	}

	if !result.Entries[1].UnstableMarker || result.Entries[0].UnstableMarker {
		t.Errorf("unstable marker was not kept independently: %+v", result.Entries[:2])
	}
	if result.Entries[0].Target.Slot != nil || result.Entries[0].Target.Subslot != nil {
		t.Errorf("missing SLOT must remain unknown: %+v", result.Entries[0].Target)
	}
	if result.Entries[4].Target.Slot == nil || *result.Entries[4].Target.Slot != "6.18.60" || result.Entries[4].Target.Subslot != nil {
		t.Errorf("NS target SLOT/SUBSLOT: %+v", result.Entries[4].Target)
	}
	if result.Entries[4].Installed.Versions[0].Slot == nil || *result.Entries[4].Installed.Versions[0].Slot != "6.18.57" {
		t.Errorf("NS installed SLOT context: %+v", result.Entries[4].Installed)
	}
	if result.Entries[5].Target.Slot == nil || *result.Entries[5].Target.Slot != "0" || result.Entries[5].Target.Subslot == nil || *result.Entries[5].Target.Subslot != "6.18.60" {
		t.Errorf("explicit SLOT/SUBSLOT must be retained: %+v", result.Entries[5].Target)
	}
	if result.Entries[5].Installed.Versions[0].Slot == nil || *result.Entries[5].Installed.Versions[0].Slot != "0" || result.Entries[5].Installed.Versions[0].Subslot == nil || *result.Entries[5].Installed.Versions[0].Subslot != "6.18.57" {
		t.Errorf("installed SLOT/SUBSLOT must be retained: %+v", result.Entries[5].Installed)
	}
	if len(result.Entries[1].FlagGroups) != 0 || len(result.Entries[5].FlagGroups) != 0 {
		t.Errorf("groups absent in fixture rows 2 and 6: %v / %v", result.Entries[1].FlagGroups, result.Entries[5].FlagGroups)
	}
	if got := groupTokens(result.Entries[0].FlagGroups); !reflect.DeepEqual(got, []string{"USE:(systemd) (-elogind) -test"}) {
		t.Errorf("libei groups: %v", got)
	}
	if got := groupTokens(result.Entries[2].FlagGroups); !reflect.DeepEqual(got, []string{
		`USE:(X) aio bpf bzip2 capstone curl fdt filecaps gnutls io-uring jpeg ncurses nls opengl pam passt pin-upstream-blobs pipewire png python seccomp slirp spice udev usb usbredir verify-sig vhost-net virgl virtfs vnc wayland xattr zstd -accessibility -alsa -debug -doc -fuse -glusterfs -gtk -infiniband -iscsi -jack -keyutils -lzo -multipath -nfs -numa -oss -plugins -pulseaudio -rbd -sasl -sdl -sdl-image (-selinux) -smartcard -snappy -ssh -static-user -systemtap -test -valgrind -vde -vte -xdp -xen`,
		`PYTHON_TARGETS:python3_14 -python3_13`,
		`QEMU_SOFTMMU_TARGETS:x86_64 -aarch64 -alpha -arm -avr -hppa -i386 -loongarch64 -m68k -microblaze -microblazeel -mips -mips64 -mips64el -mipsel -or1k -ppc -ppc64 -riscv32 -riscv64 -rx -s390x -sh4 -sh4eb -sparc -sparc64 -tricore -xtensa -xtensaeb`,
		`QEMU_USER_TARGETS:-aarch64 -aarch64_be -alpha -arm -armeb -hexagon -hppa -i386 -loongarch64 -m68k -microblaze -microblazeel -mips -mips64 -mips64el -mipsel -mipsn32 -mipsn32el -or1k -ppc -ppc64 -ppc64le -riscv32 -riscv64 -s390x -sh4 -sh4eb -sparc -sparc32plus -sparc64 -x86_64 -xtensa -xtensaeb`,
	}) {
		t.Errorf("qemu groups/tokens differ from expected fixture facts: %v", got)
	}
	wantVSCode := []string{
		"USE:egl wayland -webkit",
		"L10N:af am ar bg bn ca cs da de el en-GB es es-419 et fa fi fil fr gu he hi hr hu id it ja kn ko lt lv ml mr ms nb nl pl pt-BR pt-PT ro ru sk sl sr sv sw ta te th tr uk ur vi zh-CN zh-TW",
	}
	if got := groupTokens(result.Entries[3].FlagGroups); !reflect.DeepEqual(got, wantVSCode) {
		difference := -1
		for i := 0; i < len(got) && i < len(wantVSCode) && difference < 0; i++ {
			limit := len(got[i])
			if len(wantVSCode[i]) < limit {
				limit = len(wantVSCode[i])
			}
			for j := 0; j < limit; j++ {
				if got[i][j] != wantVSCode[i][j] {
					difference = j
					break
				}
			}
		}
		t.Errorf("vscode groups: got=%q want=%q first-difference=%d", got, wantVSCode, difference)
	}
	if got := groupTokens(result.Entries[4].FlagGroups); !reflect.DeepEqual(got, []string{"USE:initramfs verify-sig -debug (-generic-uki) -modules-compress -test"}) {
		t.Errorf("kernel groups: %v", got)
	}
	if result.Entries[0].FlagGroups[0].Values[0].Name != "systemd" || result.Entries[0].FlagGroups[0].Values[0].State != FlagEnabled || !reflect.DeepEqual(result.Entries[0].FlagGroups[0].Values[0].Markers, []string{"(", ")"}) || result.Entries[0].FlagGroups[0].Values[0].RawToken != "(systemd)" || result.Entries[0].FlagGroups[0].Values[1].Name != "elogind" || result.Entries[0].FlagGroups[0].Values[1].State != FlagDisabled || !reflect.DeepEqual(result.Entries[0].FlagGroups[0].Values[1].Markers, []string{"(", ")", "-"}) || result.Entries[0].FlagGroups[0].Values[1].RawToken != "(-elogind)" {
		t.Errorf("flag state/raw markers were not preserved: %+v", result.Entries[0].FlagGroups[0].Values)
	}
	if result.Summary == nil || result.Summary.Packages != 6 || result.Summary.Upgrades != 5 || result.Summary.NewSlots != 1 || result.Summary.Download != (Size{481212, "KiB"}) || result.Summary.SourceLine != 13 {
		t.Errorf("summary: %+v", result.Summary)
	}
}

func TestParseFlagTokenNormalization(t *testing.T) {
	tests := []struct {
		token   string
		name    string
		state   FlagState
		markers []string
	}{
		{"foo", "foo", FlagEnabled, nil},
		{"-foo", "foo", FlagDisabled, []string{"-"}},
		{"(foo)", "foo", FlagEnabled, []string{"(", ")"}},
		{"(-foo)", "foo", FlagDisabled, []string{"(", ")", "-"}},
		{"foo*", "foo", FlagEnabled, []string{"*"}},
		{"-foo*", "foo", FlagDisabled, []string{"-", "*"}},
		{"foo%", "foo", FlagEnabled, []string{"%"}},
		{"-foo%", "foo", FlagDisabled, []string{"-", "%"}},
		{"foo%*", "foo", FlagEnabled, []string{"%", "*"}},
		{"(-foo%)", "foo", FlagDisabled, []string{"(", ")", "-", "%"}},
	}
	for _, test := range tests {
		t.Run(test.token, func(t *testing.T) {
			result := Parse(`[ebuild U] cat/pkg-1::repo USE="` + test.token + `" 1 KiB`)
			if result.Status != StatusParsed || len(result.Entries) != 1 || len(result.Entries[0].FlagGroups) != 1 || len(result.Entries[0].FlagGroups[0].Values) != 1 {
				t.Fatalf("synthetic row did not parse: %+v", result)
			}
			value := result.Entries[0].FlagGroups[0].Values[0]
			if value.Name != test.name || value.State != test.state || value.RawToken != test.token || !reflect.DeepEqual(value.Markers, test.markers) {
				t.Errorf("value = %+v; want Name=%q State=%q RawToken=%q Markers=%v", value, test.name, test.state, test.token, test.markers)
			}
		})
	}
}

func TestParseNegativeInputs(t *testing.T) {
	base := fixture(t)
	first := strings.Split(base, "\n")[5]
	tests := []struct {
		name   string
		input  string
		status Status
		code   string
	}{
		{"empty synthetic input", "", StatusUnsupported, "EMPTY_INPUT"},
		{"whitespace synthetic input", " \n\t\n", StatusUnsupported, "EMPTY_INPUT"},
		{"arbitrary synthetic text", "hello from an unrelated command", StatusUnsupported, "UNRECOGNIZED_LINE"},
		{"service-only synthetic input", "These are the packages that would be merged, in order:\nCalculating dependencies ... done!", StatusUnsupported, ""},
		{"fixture summary without its rows", "Total: 6 packages (5 upgrades, 1 in new slot), Size of downloads: 481212 KiB", StatusUnsupported, "SUMMARY_MISMATCH"},
		{"fixture plus unknown synthetic line", first + "\nunknown noteworthy output", StatusPartial, "UNRECOGNIZED_LINE"},
		{"fixture row with unsupported operation", strings.Replace(base, "[ebuild     U  ]", "[ebuild     R  ]", 1), StatusPartial, "UNSUPPORTED_OPERATION"},
		{"synthetic binary row", "[binary     U  ] dev-libs/libei-1.6.0::gentoo 0 KiB", StatusUnsupported, "UNSUPPORTED_OPERATION"},
		{"fixture row with unclosed group quote", strings.Replace(base, `USE="(systemd) (-elogind) -test"`, `USE="(systemd) (-elogind) -test`, 1), StatusPartial, "MALFORMED_ENTRY"},
		{"synthetic malformed row", "[ebuild     U  ] this-is-not-a-package 1 KiB", StatusUnsupported, "MALFORMED_ENTRY"},
		{"fixture summary count changed", strings.Replace(base, "Total: 6 packages", "Total: 7 packages", 1), StatusPartial, "SUMMARY_MISMATCH"},
		{"synthetic tree formatting", "emerge --tree --pretend", StatusUnsupported, "UNSUPPORTED_FORMAT"},
		{"synthetic columns formatting", "emerge --columns --pretend", StatusUnsupported, "UNSUPPORTED_FORMAT"},
		{"synthetic ANSI formatting", "\x1b[32m[ebuild U] dev-libs/libei-1.0::gentoo 0 KiB\x1b[0m", StatusUnsupported, "UNSUPPORTED_FORMAT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Parse(test.input)
			if result.Status != test.status {
				t.Fatalf("status = %q, want %q; diagnostics=%+v", result.Status, test.status, result.Diagnostics)
			}
			if test.code != "" && !hasDiagnostic(result.Diagnostics, test.code) {
				t.Errorf("diagnostic %q missing: %+v", test.code, result.Diagnostics)
			}
			for _, diagnostic := range result.Diagnostics {
				for _, line := range diagnostic.SourceLines {
					if line < 1 {
						t.Errorf("line numbers must be one-based: %+v", diagnostic)
					}
				}
			}
		})
	}
}

func TestEmptyNamedFlagGroupIsDistinctFromAbsentGroup(t *testing.T) {
	// These two synthetic rows exercise absence versus an explicitly empty group.
	absent := Parse("[ebuild U] cat/pkg-1::repo 1 KiB")
	empty := Parse(`[ebuild U] cat/pkg-1::repo USE="" 1 KiB`)
	if absent.Status != StatusParsed || empty.Status != StatusParsed {
		t.Fatalf("synthetic rows should parse: absent=%+v empty=%+v", absent, empty)
	}
	if len(absent.Entries[0].FlagGroups) != 0 {
		t.Fatalf("absent group should have no group records: %+v", absent.Entries[0].FlagGroups)
	}
	if len(empty.Entries[0].FlagGroups) != 1 || empty.Entries[0].FlagGroups[0].Name != "USE" || empty.Entries[0].FlagGroups[0].Values == nil || len(empty.Entries[0].FlagGroups[0].Values) != 0 {
		t.Fatalf("explicit empty group should remain present: %+v", empty.Entries[0].FlagGroups)
	}
}

func TestDiagnosticsKeepSourceOrderAndLineNumbers(t *testing.T) {
	// This derived fixture has a count mismatch at line 13 and an unknown
	// significant line at line 16; diagnostics must follow source order.
	input := strings.Replace(fixture(t), "Total: 6 packages", "Total: 7 packages", 1) + "unknown noteworthy output\n"
	result := Parse(input)
	if result.Status != StatusPartial || len(result.Diagnostics) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Diagnostics[0].Code != "SUMMARY_MISMATCH" || !reflect.DeepEqual(result.Diagnostics[0].SourceLines, []int{13}) {
		t.Errorf("summary diagnostic source: %+v", result.Diagnostics[0])
	}
	if result.Diagnostics[1].Code != "UNRECOGNIZED_LINE" || !reflect.DeepEqual(result.Diagnostics[1].SourceLines, []int{16}) {
		t.Errorf("unknown line diagnostic source: %+v", result.Diagnostics[1])
	}
}

func groupTokens(groups []FlagGroup) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		tokens := make([]string, 0, len(group.Values))
		for _, value := range group.Values {
			tokens = append(tokens, value.RawToken)
		}
		result = append(result, group.Name+":"+strings.Join(tokens, " "))
	}
	return result
}

func hasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
