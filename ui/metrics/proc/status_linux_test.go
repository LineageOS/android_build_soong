package proc

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"android/soong/finder/fs"
)

func TestNewProcStatus(t *testing.T) {
	fs := fs.NewMockFs(nil)

	pid := 4032827
	procDir := filepath.Join("/proc", strconv.Itoa(pid))
	if err := fs.MkDirs(procDir); err != nil {
		t.Fatalf("failed to create proc pid dir %s: %v", procDir, err)
	}
	statusFilename := filepath.Join(procDir, "status")

	if err := fs.WriteFile(statusFilename, statusData, 0644); err != nil {
		t.Fatalf("failed to write proc file %s: %v", statusFilename, err)
	}

	status, err := NewProcStatus(pid, fs)
	if err != nil {
		t.Fatalf("got %v, want nil for error", err)
	}

	fmt.Printf("%d %d\b", status.VmPeak, expectedStatus.VmPeak)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("got %v, expecting %v for ProcStatus", status, expectedStatus)
	}
}

var statusData = []byte(`Name:   fake_process
Umask:  0022
State:  S (sleeping)
Tgid:   4032827
Ngid:   0
Pid:    4032827
PPid:   1
TracerPid:      0
Uid:    0       0       0       0
Gid:    0       0       0       0
FDSize: 512
Groups:
NStgid: 4032827
NSpid:  4032827
NSpgid: 4032827
NSsid:  4032827
VmPeak:   733232 kB
VmSize:   733232 kB
VmLck:       132 kB
VmPin:       130 kB
VmHWM:     69156 kB
VmRSS:     69156 kB
RssAnon:           50896 kB
RssFile:           18260 kB
RssShmem:            122 kB
VmData:   112388 kB
VmStk:       132 kB
VmExe:      9304 kB
VmLib:         8 kB
VmPTE:       228 kB
VmSwap:        10 kB
HugetlbPages:          22 kB
CoreDumping:    0
THP_enabled:    1
Threads:        46
SigQ:   2/767780
SigPnd: 0000000000000000
ShdPnd: 0000000000000000
SigBlk: fffffffe3bfa3a00
SigIgn: 0000000000000000
SigCgt: fffffffe7fc1feff
CapInh: 0000000000000000
CapPrm: 0000003fffffffff
CapEff: 0000003fffffffff
CapBnd: 0000003fffffffff
CapAmb: 0000000000000000
NoNewPrivs:     0
Seccomp:        0
Speculation_Store_Bypass:       thread vulnerable
Cpus_allowed:   ff,ffffffff,ffffffff
Cpus_allowed_list:      0-71
Mems_allowed:   00000000,00000003
Mems_allowed_list:      0-1
voluntary_ctxt_switches:        1635
nonvoluntary_ctxt_switches:     32
`)

var expectedStatus = &ProcStatus{
	pid:          4032827,
	VmPeak:       750829568,
	VmSize:       750829568,
	VmLck:        135168,
	VmPin:        133120,
	VmHWM:        70815744,
	VmRss:        70815744,
	RssAnon:      52117504,
	RssShmem:     124928,
	VmData:       115085312,
	VmStk:        135168,
	VmExe:        9527296,
	VmLib:        8192,
	VmPTE:        233472,
	VmSwap:       10240,
	HugetlbPages: 22528,
}
