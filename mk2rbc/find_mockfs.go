package mk2rbc

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Mock FS. Maps a directory name to an array of entries.
// An entry implements fs.DirEntry, fs.FIleInfo and fs.File interface
type FindMockFS struct {
	dirs map[string][]myFileInfo
}

func (m FindMockFS) locate(name string) (myFileInfo, bool) {
	if name == "." {
		return myFileInfo{".", true}, true
	}
	dir := filepath.Dir(name)
	base := filepath.Base(name)
	if entries, ok := m.dirs[dir]; ok {
		for _, e := range entries {
			if e.name == base {
				return e, true
			}
		}
	}
	return myFileInfo{}, false
}

func (m FindMockFS) create(name string, isDir bool) {
	dir := filepath.Dir(name)
	m.dirs[dir] = append(m.dirs[dir], myFileInfo{filepath.Base(name), isDir})
}

func (m FindMockFS) Stat(name string) (fs.FileInfo, error) {
	if fi, ok := m.locate(name); ok {
		return fi, nil
	}
	return nil, os.ErrNotExist
}

type myFileInfo struct {
	name  string
	isDir bool
}

func (m myFileInfo) Info() (fs.FileInfo, error) {
	panic("implement me")
}

func (m myFileInfo) Size() int64 {
	panic("implement me")
}

func (m myFileInfo) Mode() fs.FileMode {
	panic("implement me")
}

func (m myFileInfo) ModTime() time.Time {
	panic("implement me")
}

func (m myFileInfo) Sys() interface{} {
	return nil
}

func (m myFileInfo) Stat() (fs.FileInfo, error) {
	return m, nil
}

func (m myFileInfo) Read(bytes []byte) (int, error) {
	panic("implement me")
}

func (m myFileInfo) Close() error {
	panic("implement me")
}

func (m myFileInfo) Name() string {
	return m.name
}

func (m myFileInfo) IsDir() bool {
	return m.isDir
}

func (m myFileInfo) Type() fs.FileMode {
	return m.Mode()
}

func (m FindMockFS) Open(name string) (fs.File, error) {
	panic("implement me")
}

func (m FindMockFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if d, ok := m.dirs[name]; ok {
		var res []fs.DirEntry
		for _, e := range d {
			res = append(res, e)
		}
		return res, nil
	}
	return nil, os.ErrNotExist
}

func NewFindMockFS(files []string) FindMockFS {
	myfs := FindMockFS{make(map[string][]myFileInfo)}
	for _, f := range files {
		isDir := false
		for f != "." {
			if _, ok := myfs.locate(f); !ok {
				myfs.create(f, isDir)
			}
			isDir = true
			f = filepath.Dir(f)
		}
	}
	return myfs
}
