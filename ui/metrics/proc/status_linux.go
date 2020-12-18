package proc

import (
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"android/soong/finder/fs"
)

// NewProcStatus returns an instance of the ProcStatus that contains memory
// information of the process. The memory information is extracted from the
// "/proc/<pid>/status" text file. This is only available for Linux
// distribution that supports /proc.
func NewProcStatus(pid int, fileSystem fs.FileSystem) (*ProcStatus, error) {
	statusFname := filepath.Join("/proc", strconv.Itoa(pid), "status")
	r, err := fileSystem.Open(statusFname)
	if err != nil {
		return &ProcStatus{}, err
	}
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return &ProcStatus{}, err
	}

	s := &ProcStatus{
		pid: pid,
	}

	for _, l := range strings.Split(string(data), "\n") {
		// If the status file does not contain "key: values", just skip the line
		// as the information we are looking for is not needed.
		if !strings.Contains(l, ":") {
			continue
		}

		// At this point, we're only considering entries that has key, single value pairs.
		kv := strings.SplitN(l, ":", 2)
		fillProcStatus(s, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
	}

	return s, nil
}
