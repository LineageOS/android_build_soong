// package proc contains functionality to read proc status files.
package proc

import (
	"strconv"
	"strings"
)

// ProcStatus holds information regarding the memory usage of
// an executing process. The memory sizes in each of the field
// is in bytes.
type ProcStatus struct {
	// Process PID.
	pid int

	// Peak virtual memory size.
	VmPeak uint64

	// Virtual memory size.
	VmSize uint64

	// Locked Memory size.
	VmLck uint64

	// Pinned memory size.
	VmPin uint64

	// Peak resident set size.
	VmHWM uint64

	// Resident set size (sum of RssAnon, RssFile and RssShmem).
	VmRss uint64

	// Size of resident anonymous memory.
	RssAnon uint64

	// Size of resident shared memory.
	RssShmem uint64

	// Size of data segments.
	VmData uint64

	// Size of stack segments.
	VmStk uint64

	//Size of text segments.
	VmExe uint64

	//Shared library code size.
	VmLib uint64

	// Page table entries size.
	VmPTE uint64

	// Size of second-level page tables.
	VmPMD uint64

	// Swapped-out virtual memory size by anonymous private.
	VmSwap uint64

	// Size of hugetlb memory page size.
	HugetlbPages uint64
}

// fillProcStatus takes the key and value, converts the value
// to the proper size unit and is stored in the ProcStatus.
func fillProcStatus(s *ProcStatus, key, value string) {
	v := strToUint64(value)
	switch key {
	case "VmPeak":
		s.VmPeak = v
	case "VmSize":
		s.VmSize = v
	case "VmLck":
		s.VmLck = v
	case "VmPin":
		s.VmPin = v
	case "VmHWM":
		s.VmHWM = v
	case "VmRSS":
		s.VmRss = v
	case "RssAnon":
		s.RssAnon = v
	case "RssShmem":
		s.RssShmem = v
	case "VmData":
		s.VmData = v
	case "VmStk":
		s.VmStk = v
	case "VmExe":
		s.VmExe = v
	case "VmLib":
		s.VmLib = v
	case "VmPTE":
		s.VmPTE = v
	case "VmPMD":
		s.VmPMD = v
	case "VmSwap":
		s.VmSwap = v
	case "HugetlbPages":
		s.HugetlbPages = v
	}
}

// strToUint64 takes the string and converts to unsigned 64-bit integer.
// If the string contains a memory unit such as kB and is converted to
// bytes.
func strToUint64(v string) uint64 {
	// v could be "1024 kB" so scan for the empty space and
	// split between the value and the unit.
	var separatorIndex int
	if separatorIndex = strings.IndexAny(v, " "); separatorIndex < 0 {
		separatorIndex = len(v)
	}
	value, err := strconv.ParseUint(v[:separatorIndex], 10, 64)
	if err != nil {
		return 0
	}

	var scale uint64 = 1
	switch strings.TrimSpace(v[separatorIndex:]) {
	case "kB", "KB":
		scale = 1024
	case "mB", "MB":
		scale = 1024 * 1024
	}
	return value * scale
}
