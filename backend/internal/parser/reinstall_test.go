package parser

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestRebuildPlanFixture(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/parser/ebuild-updates-3.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := Parse(string(data))
	if got.Status != StatusParsed || len(got.Entries) != 43 || len(got.Diagnostics) != 0 {
		t.Fatalf("status=%s entries=%d diagnostics=%+v", got.Status, len(got.Entries), got.Diagnostics)
	}
	// Independent expected facts in original order: package, target, installed, KiB, repository.
	expected := strings.Fields(`
dev-lang/go 1.27.3 1.27.2 34311 gentoo
app-containers/docker-cli 29.9.3 29.9.0 6984 gentoo
app-admin/terraform 1.17.1 1.16.4 16807 gentoo
dev-util/gitlab-cli 1.128.0 1.123.0 29573 gentoo
app-containers/docker-buildx 0.38.2 0.37.1 15318 gentoo
sys-libs/libxcrypt 4.5.2-r1 4.5.2 656 gentoo
dev-libs/nss 3.133 3.132 85786 gentoo
dev-python/uv 0.13.4-r1 0.12.7 51693 gentoo
dev-lang/python 3.14.10_p1 3.14.10 23726 gentoo
app-misc/ca-certificates 20260816.3.131.1-r1 20260816.3.131.1 86260 gentoo
dev-util/bpftool 7.7.0-r2 - 0 gentoo
sys-libs/libcap-ng 0.9.6 0.9.3 182 gentoo
dev-python/pyjwt 2.15.1 2.13.0 129 gentoo
dev-python/rfc3161-client 1.0.9 1.0.8 57272 gentoo
dev-python/uv-build 0.13.4 0.12.7 454 gentoo
dev-python/pyasn1 0.6.5 0.6.4 152 gentoo
dev-python/flit-core 4.1.0 4.0.2 63 gentoo
dev-python/platformdirs 4.12.3 4.12.1 68 gentoo
dev-python/sigstore 4.5.1 4.5.0 414 gentoo
dev-util/bindgen 0.73.2 0.72.1 7357 gentoo
dev-python/pyclip 0.7.0-r2 - 0 guru
dev-python/markupsafe 3.0.4 3.0.3 160 gentoo
dev-util/github-cli 2.103.3 2.102.0 78051 gentoo
dev-python/wcwidth 0.9.2 0.9.1 933 gentoo
dev-python/setuptools-scm 10.3.2 10.2.3 83 gentoo
dev-python/mako 1.4.3 1.4.1 404 gentoo
app-containers/slirp4netns 1.3.5 1.3.4 64 gentoo
dev-libs/blake3 1.8.7 1.8.5 263 gentoo
sys-apps/portage 3.0.83-r1 3.0.82.2-r1 1327 gentoo
dev-util/mesa_clc 26.2.6 - 67094 gentoo
net-vpn/openvpn 2.7.6 2.7.5 2058 gentoo
dev-python/gbinder 1.3.1 - 0 guru
net-misc/openssh 10.6_p2 10.5_p1 2301 gentoo
app-containers/docker 29.9.3 29.9.0 21915 gentoo
media-libs/libva-intel-media-driver 26.3.5-r2 26.3.5-r1 14 gentoo
net-vpn/networkmanager-openvpn 1.12.5 1.12.3 714 gentoo
virtual/linux-sources 3-r13 3-r12 0 gentoo-zh
app-containers/waydroid 1.6.2 - 0 guru
media-libs/mesa 26.2.6 - 0 gentoo
dev-util/perf 7.2 - 0 gentoo
app-editors/vscode 1.145.0 1.144.0-r1 369924 gentoo
app-editors/zed 1.27.0 1.26.2 15166 zed-overlay
gui-wm/umbriel 0.1.0_pre20261104132027 0.1.0_pre20261103013457 2072 umbriel-overlay
`)
	if len(expected) != 43*5 {
		t.Fatal("invalid expected facts")
	}
	upgrades, reinstalls := 0, 0
	unstable := map[int]bool{2: true, 3: true, 4: true, 7: true, 21: true, 23: true, 30: true, 32: true, 34: true, 35: true, 38: true, 39: true, 40: true, 42: true, 43: true}
	for i, e := range got.Entries {
		f := expected[i*5 : i*5+5]
		size, _ := strconv.ParseInt(f[3], 10, 64)
		if e.Package.Category+"/"+e.Package.Name != f[0] || e.Target.Version != f[1] || e.Target.Repository == nil || *e.Target.Repository != f[4] || e.Download == nil || *e.Download != (Size{size, "KiB"}) || !reflect.DeepEqual(e.SourceLines, []int{i + 1}) || e.NewSlot || e.UnstableMarker != unstable[i+1] {
			t.Errorf("row %d: %+v", i+1, e)
		}
		if i != 0 && i != 5 && i != 8 {
			if e.Target.Slot != nil || e.Target.Subslot != nil {
				t.Errorf("invented target slot at %d", i+1)
			}
			for _, v := range e.Installed.Versions {
				if v.Slot != nil || v.Subslot != nil {
					t.Errorf("invented installed slot at %d", i+1)
				}
			}
		}
		if f[2] == "-" {
			reinstalls++
			if e.Operation != OperationReinstall || len(e.Installed.Versions) != 0 {
				t.Errorf("rebuild row %d: %+v", i+1, e)
			}
		} else {
			upgrades++
			if e.Operation != OperationUpgrade || len(e.Installed.Versions) != 1 {
				t.Fatalf("upgrade row %d: %+v", i+1, e)
			}
			v := e.Installed.Versions[0]
			if v.Version != f[2] || v.Repository == nil || *v.Repository != f[4] {
				t.Errorf("installed row %d: %+v", i+1, v)
			}
		}
	}
	if upgrades != 36 || reinstalls != 7 || got.Summary != nil {
		t.Fatalf("counts %d %d summary %+v", upgrades, reinstalls, got.Summary)
	}
	for _, tc := range []struct {
		index                                              int
		targetSlot, targetSub, installedSlot, installedSub string
	}{
		{0, "0", "1.27.3", "0", "1.27.2"}, {5, "0", "1", "0", "1"}, {8, "3.14", "", "3.14", ""},
	} {
		e := got.Entries[tc.index]
		v := e.Installed.Versions[0]
		for _, pair := range []struct {
			got  *string
			want string
		}{{e.Target.Slot, tc.targetSlot}, {e.Target.Subslot, tc.targetSub}, {v.Slot, tc.installedSlot}, {v.Subslot, tc.installedSub}} {
			if pair.want == "" {
				if pair.got != nil {
					t.Errorf("unexpected subslot %+v", e)
				}
			} else if pair.got == nil || *pair.got != pair.want {
				t.Errorf("slot mismatch %+v", e)
			}
		}
	}
	summary := Parse(string(data) + "\nTotal: 43 packages (36 upgrades, 7 reinstalls), Size of downloads: 1,079,819 KiB")
	if summary.Status != StatusParsed || summary.Summary.Reinstalls != 7 || summary.Summary.Upgrades != 36 {
		t.Fatalf("%+v", summary)
	}

}

func TestGroupedDownloadSizes(t *testing.T) {
	for _, tc := range []struct {
		text  string
		value int64
	}{
		{"34311", 34311}, {"34,311", 34311}, {"1,234,567", 1234567}, {"0", 0}, {"9,223,372,036,854,775,807", 9223372036854775807},
	} {
		t.Run(tc.text, func(t *testing.T) {
			result := Parse("[ebuild U ] dev-test/pkg-2::gentoo [1::gentoo] " + tc.text + " KiB\nTotal: 1 package (1 upgrade), Size of downloads: " + tc.text + " KiB")
			if result.Status != StatusParsed || result.Entries[0].Download.Value != tc.value || result.Summary.Download.Value != tc.value {
				t.Fatalf("%+v", result)
			}
		})
	}
	for _, text := range []string{"34,31", "1,23,456", "1234,567", "1,,234", ",123", "123,", "-1", "1.5", "9223372036854775808", "9,223,372,036,854,775,808"} {
		t.Run("invalid_"+text, func(t *testing.T) {
			row := Parse("[ebuild U ] dev-test/pkg-2 " + text + " KiB")
			if len(row.Entries) != 0 || len(row.Diagnostics) != 1 || row.Diagnostics[0].Code != "MALFORMED_ENTRY" {
				t.Fatalf("%+v", row)
			}
			summary := Parse("Total: 0 packages (), Size of downloads: " + text + " KiB")
			if summary.Summary != nil || len(summary.Diagnostics) != 1 || summary.Diagnostics[0].Code != "MALFORMED_SUMMARY" {
				t.Fatalf("%+v", summary)
			}
		})
	}
}

func TestReinstallMarkersAndSummary(t *testing.T) {
	for _, marker := range []string{"R", "R ~"} {
		for _, word := range []string{"reinstall", "reinstalls"} {
			got := Parse("[ebuild " + marker + "] dev-test/pkg-1::gentoo 0 KiB\nTotal: 1 package (1 " + word + "), Size of downloads: 0 KiB")
			if got.Status != StatusParsed || got.Summary.Reinstalls != 1 || string(got.Entries[0].Operation) != "reinstall" || len(got.Entries[0].Installed.Versions) != 0 {
				t.Fatalf("%+v", got)
			}
		}
	}
	for _, marker := range []string{"RU", "UR", "RN", "R~", "R ~ ~", "~ R", "R U"} {
		got := Parse("[ebuild " + marker + "] dev-test/pkg-1 0 KiB")
		if len(got.Entries) != 0 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "UNSUPPORTED_OPERATION" {
			t.Fatalf("%s: %+v", marker, got)
		}
	}
	got := Parse("[ebuild R ] dev-test/pkg-1:0/2::gentoo [1:0/1::gentoo] USE=\"foo* -bar%\" 0 KiB")
	if got.Status != StatusParsed || len(got.Entries[0].Installed.Versions) != 1 {
		t.Fatalf("%+v", got)
	}
	for _, count := range []string{"9999999999999999999999999999"} {
		for _, input := range []string{"Total: " + count + " packages (), Size of downloads: 0 KiB", "Total: 0 packages (" + count + " reinstalls), Size of downloads: 0 KiB"} {
			got := Parse(input)
			if got.Summary != nil || !strings.Contains(got.Diagnostics[0].Code, "MALFORMED_SUMMARY") {
				t.Fatalf("%+v", got)
			}
		}
	}
}

func TestRebuildFlagGroups(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/parser/ebuild-updates-3.txt")
	if err != nil {
		t.Fatal(err)
	}
	plan := Parse(string(data))
	if len(plan.Entries) != 43 {
		t.Fatal(plan)
	}
	expected := map[int][]string{
		10: {"USE=clang -caps -llvm", "LLVM_SLOT=22 -19 -20 -21 (-23) (-18%)"},
		20: {"USE=wayland -X -test", "PYTHON_TARGETS=python3_14 -python3_13 (-python3_12%)"},
		29: {"USE=-debug", "LLVM_SLOT=22 -19 -20 -21 (-18%)", "VIDEO_CARDS=-asahi -panfrost"},
		31: {"USE=-debug -test", "PYTHON_TARGETS=python3_14 -python3_13 (-python3_12%)"},
		37: {"USE=apparmor clipboard", "PYTHON_SINGLE_TARGET=python3_14 -python3_13 (-python3_12%)"},
		38: {"USE=X llvm opencl (opengl) proprietary-codecs vulkan wayland zstd -debug -lm-sensors -sysprof -test -unwind -vaapi -valgrind", "CPU_FLAGS_X86=sse2", "LLVM_SLOT=22 -19 -20 -21 (-18%)", "VIDEO_CARDS=intel zink -asahi -d3d12 (-freedreno) -i915 (-imagination) -lavapipe (-lima) -nouveau -nvk (-panfrost) -r300 -r600 -radeon -radeonsi (-v3d) (-vc4) -virgl (-vivante) -vmware"},
		39: {"USE=bpf libtraceevent libtracefs python slang -babeltrace (-big-endian) -caps -capstone -crypt -debug -gtk -java -libpfm -lzma -numa -perl -systemtap -tcmalloc", "LLVM_SLOT=22 -19 -20 -21 (-18%)", "PYTHON_SINGLE_TARGET=python3_14 -python3_12 -python3_13 -python3_14t"},
	}
	for index, want := range expected {
		groups := plan.Entries[index].FlagGroups
		actual := make([]string, 0, len(groups))
		for _, g := range groups {
			tokens := make([]string, 0, len(g.Values))
			for _, v := range g.Values {
				tokens = append(tokens, v.RawToken)
			}
			actual = append(actual, g.Name+"="+strings.Join(tokens, " "))
		}
		if !reflect.DeepEqual(actual, want) {
			t.Errorf("row %d groups: %v", index+1, actual)
		}
	}
	value := plan.Entries[10].FlagGroups[1].Values[5]
	if value.Name != "18" || value.State != FlagDisabled || !reflect.DeepEqual(value.Markers, []string{"(", ")", "-", "%"}) {
		t.Fatalf("complex token: %+v", value)
	}
}

func TestDuplicateReinstallSummaryCounter(t *testing.T) {
	got := Parse("[ebuild R] dev-test/pkg-1\nTotal: 1 package (1 reinstall, 1 reinstalls), Size of downloads: 0 KiB")
	if got.Status != StatusPartial || got.Summary != nil || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "MALFORMED_SUMMARY" {
		t.Fatalf("%+v", got)
	}
}
