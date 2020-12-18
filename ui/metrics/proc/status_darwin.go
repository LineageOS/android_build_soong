package proc

import (
	"android/soong/finder/fs"
)

// NewProcStatus returns a zero filled value of ProcStatus as it
// is not supported for darwin distribution based.
func NewProcStatus(pid int, _ fs.FileSystem) (*ProcStatus, error) {
	return &ProcStatus{}, nil
}
