// Copyright 2017 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"
)

var OsFs FileSystem = osFs{}

func NewMockFs(files map[string][]byte) *MockFs {
	workDir := "/cwd"
	fs := &MockFs{
		Clock:   NewClock(time.Unix(2, 2)),
		workDir: workDir,
	}
	fs.root = *fs.newDir()
	fs.MkDirs(workDir)

	for path, bytes := range files {
		dir := filepath.Dir(path)
		fs.MkDirs(dir)
		fs.WriteFile(path, bytes, 0777)
	}

	return fs
}

type FileSystem interface {
	// getting information about files
	Open(name string) (file io.ReadCloser, err error)
	Lstat(path string) (stats os.FileInfo, err error)
	ReadDir(path string) (contents []DirEntryInfo, err error)

	InodeNumber(info os.FileInfo) (number uint64, err error)
	DeviceNumber(info os.FileInfo) (number uint64, err error)
	PermTime(info os.FileInfo) (time time.Time, err error)

	// changing contents of the filesystem
	Rename(oldPath string, newPath string) (err error)
	WriteFile(path string, data []byte, perm os.FileMode) (err error)
	Remove(path string) (err error)
	RemoveAll(path string) (err error)

	// metadata about the filesystem
	ViewId() (id string) // Some unique id of the user accessing the filesystem
}

// DentryInfo is a subset of the functionality available through os.FileInfo that might be able
// to be gleaned through only a syscall.Getdents without requiring a syscall.Lstat of every file.
type DirEntryInfo interface {
	Name() string
	Mode() os.FileMode // the file type encoded as an os.FileMode
	IsDir() bool
}

type dirEntryInfo struct {
	name       string
	mode       os.FileMode
	modeExists bool
}

var _ DirEntryInfo = os.FileInfo(nil)

func (d *dirEntryInfo) Name() string      { return d.name }
func (d *dirEntryInfo) Mode() os.FileMode { return d.mode }
func (d *dirEntryInfo) IsDir() bool       { return d.mode.IsDir() }
func (d *dirEntryInfo) String() string    { return d.name + ": " + d.mode.String() }

// osFs implements FileSystem using the local disk.
type osFs struct{}

var _ FileSystem = (*osFs)(nil)

func (osFs) Open(name string) (io.ReadCloser, error) { return os.Open(name) }

func (osFs) Lstat(path string) (stats os.FileInfo, err error) {
	return os.Lstat(path)
}

func (osFs) ReadDir(path string) (contents []DirEntryInfo, err error) {
	entries, err := readdir(path)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		contents = append(contents, entry)
	}

	return contents, nil
}

func (osFs) Rename(oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (osFs) WriteFile(path string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(path, data, perm)
}

func (osFs) Remove(path string) error {
	return os.Remove(path)
}

func (osFs) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (osFs) ViewId() (id string) {
	user, err := user.Current()
	if err != nil {
		return ""
	}
	username := user.Username

	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}

	return username + "@" + hostname
}

type Clock struct {
	time time.Time
}

func NewClock(startTime time.Time) *Clock {
	return &Clock{time: startTime}

}

func (c *Clock) Tick() {
	c.time = c.time.Add(time.Microsecond)
}

func (c *Clock) Time() time.Time {
	return c.time
}

// given "/a/b/c/d", pathSplit returns ("/a/b/c", "d")
func pathSplit(path string) (dir string, leaf string) {
	dir, leaf = filepath.Split(path)
	if dir != "/" && len(dir) > 0 {
		dir = dir[:len(dir)-1]
	}
	return dir, leaf
}

// MockFs supports singlethreaded writes and multithreaded reads
type MockFs struct {
	// configuration
	viewId       string //
	deviceNumber uint64

	// implementation
	root            mockDir
	Clock           *Clock
	workDir         string
	nextInodeNumber uint64

	// history of requests, for tests to check
	StatCalls      []string
	ReadDirCalls   []string
	aggregatesLock sync.Mutex
}

var _ FileSystem = (*MockFs)(nil)

type mockInode struct {
	modTime     time.Time
	permTime    time.Time
	sys         interface{}
	inodeNumber uint64
	readErr     error
}

func (m mockInode) ModTime() time.Time {
	return m.modTime
}

func (m mockInode) Sys() interface{} {
	return m.sys
}

type mockFile struct {
	bytes []byte

	mockInode
}

type mockLink struct {
	target string

	mockInode
}

type mockDir struct {
	mockInode

	subdirs  map[string]*mockDir
	files    map[string]*mockFile
	symlinks map[string]*mockLink
}

func (m *MockFs) resolve(path string, followLastLink bool) (result string, err error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.workDir, path)
	}
	path = filepath.Clean(path)

	return m.followLinks(path, followLastLink, 10)
}

// note that followLinks can return a file path that doesn't exist
func (m *MockFs) followLinks(path string, followLastLink bool, count int) (canonicalPath string, err error) {
	if path == "/" {
		return path, nil
	}

	parentPath, leaf := pathSplit(path)
	if parentPath == path {
		err = fmt.Errorf("Internal error: %v yields itself as a parent", path)
		panic(err.Error())
	}

	parentPath, err = m.followLinks(parentPath, true, count)
	if err != nil {
		return "", err
	}

	parentNode, err := m.getDir(parentPath, false)
	if err != nil {
		return "", err
	}
	if parentNode.readErr != nil {
		return "", &os.PathError{
			Op:   "read",
			Path: path,
			Err:  parentNode.readErr,
		}
	}

	link, isLink := parentNode.symlinks[leaf]
	if isLink && followLastLink {
		if count <= 0 {
			// probably a loop
			return "", &os.PathError{
				Op:   "read",
				Path: path,
				Err:  fmt.Errorf("too many levels of symbolic links"),
			}
		}

		if link.readErr != nil {
			return "", &os.PathError{
				Op:   "read",
				Path: path,
				Err:  link.readErr,
			}
		}

		target := m.followLink(link, parentPath)
		return m.followLinks(target, followLastLink, count-1)
	}
	return path, nil
}

func (m *MockFs) followLink(link *mockLink, parentPath string) (result string) {
	return filepath.Clean(filepath.Join(parentPath, link.target))
}

func (m *MockFs) getFile(parentDir *mockDir, fileName string) (file *mockFile, err error) {
	file, isFile := parentDir.files[fileName]
	if !isFile {
		_, isDir := parentDir.subdirs[fileName]
		_, isLink := parentDir.symlinks[fileName]
		if isDir || isLink {
			return nil, &os.PathError{
				Op:   "open",
				Path: fileName,
				Err:  os.ErrInvalid,
			}
		}

		return nil, &os.PathError{
			Op:   "open",
			Path: fileName,
			Err:  os.ErrNotExist,
		}
	}
	if file.readErr != nil {
		return nil, &os.PathError{
			Op:   "open",
			Path: fileName,
			Err:  file.readErr,
		}
	}
	return file, nil
}

func (m *MockFs) getInode(parentDir *mockDir, name string) (inode *mockInode, err error) {
	file, isFile := parentDir.files[name]
	if isFile {
		return &file.mockInode, nil
	}
	link, isLink := parentDir.symlinks[name]
	if isLink {
		return &link.mockInode, nil
	}
	dir, isDir := parentDir.subdirs[name]
	if isDir {
		return &dir.mockInode, nil
	}
	return nil, &os.PathError{
		Op:   "stat",
		Path: name,
		Err:  os.ErrNotExist,
	}

}

func (m *MockFs) Open(path string) (io.ReadCloser, error) {
	path, err := m.resolve(path, true)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	parentPath, base := pathSplit(path)
	parentDir, err := m.getDir(parentPath, false)
	if err != nil {
		return nil, err
	}
	file, err := m.getFile(parentDir, base)
	if err != nil {
		return nil, err
	}
	return struct {
		io.Closer
		*bytes.Reader
	}{
		ioutil.NopCloser(nil),
		bytes.NewReader(file.bytes),
	}, nil

}

// a mockFileInfo is for exporting file stats in a way that satisfies the FileInfo interface
type mockFileInfo struct {
	path         string
	size         int64
	modTime      time.Time // time at which the inode's contents were modified
	permTime     time.Time // time at which the inode's permissions were modified
	isDir        bool
	inodeNumber  uint64
	deviceNumber uint64
}

func (m *mockFileInfo) Name() string {
	return m.path
}

func (m *mockFileInfo) Size() int64 {
	return m.size
}

func (m *mockFileInfo) Mode() os.FileMode {
	return 0
}

func (m *mockFileInfo) ModTime() time.Time {
	return m.modTime
}

func (m *mockFileInfo) IsDir() bool {
	return m.isDir
}

func (m *mockFileInfo) Sys() interface{} {
	return nil
}

func (m *MockFs) dirToFileInfo(d *mockDir, path string) (info *mockFileInfo) {
	return &mockFileInfo{
		path:         path,
		size:         1,
		modTime:      d.modTime,
		permTime:     d.permTime,
		isDir:        true,
		inodeNumber:  d.inodeNumber,
		deviceNumber: m.deviceNumber,
	}

}

func (m *MockFs) fileToFileInfo(f *mockFile, path string) (info *mockFileInfo) {
	return &mockFileInfo{
		path:         path,
		size:         1,
		modTime:      f.modTime,
		permTime:     f.permTime,
		isDir:        false,
		inodeNumber:  f.inodeNumber,
		deviceNumber: m.deviceNumber,
	}
}

func (m *MockFs) linkToFileInfo(l *mockLink, path string) (info *mockFileInfo) {
	return &mockFileInfo{
		path:         path,
		size:         1,
		modTime:      l.modTime,
		permTime:     l.permTime,
		isDir:        false,
		inodeNumber:  l.inodeNumber,
		deviceNumber: m.deviceNumber,
	}
}

func (m *MockFs) Lstat(path string) (stats os.FileInfo, err error) {
	// update aggregates
	m.aggregatesLock.Lock()
	m.StatCalls = append(m.StatCalls, path)
	m.aggregatesLock.Unlock()

	// resolve symlinks
	path, err = m.resolve(path, false)
	if err != nil {
		return nil, err
	}

	// special case for root dir
	if path == "/" {
		return m.dirToFileInfo(&m.root, "/"), nil
	}

	// determine type and handle appropriately
	parentPath, baseName := pathSplit(path)
	dir, err := m.getDir(parentPath, false)
	if err != nil {
		return nil, err
	}
	subdir, subdirExists := dir.subdirs[baseName]
	if subdirExists {
		return m.dirToFileInfo(subdir, path), nil
	}
	file, fileExists := dir.files[baseName]
	if fileExists {
		return m.fileToFileInfo(file, path), nil
	}
	link, linkExists := dir.symlinks[baseName]
	if linkExists {
		return m.linkToFileInfo(link, path), nil
	}
	// not found
	return nil, &os.PathError{
		Op:   "stat",
		Path: path,
		Err:  os.ErrNotExist,
	}
}

func (m *MockFs) InodeNumber(info os.FileInfo) (number uint64, err error) {
	mockInfo, ok := info.(*mockFileInfo)
	if ok {
		return mockInfo.inodeNumber, nil
	}
	return 0, fmt.Errorf("%v is not a mockFileInfo", info)
}
func (m *MockFs) DeviceNumber(info os.FileInfo) (number uint64, err error) {
	mockInfo, ok := info.(*mockFileInfo)
	if ok {
		return mockInfo.deviceNumber, nil
	}
	return 0, fmt.Errorf("%v is not a mockFileInfo", info)
}
func (m *MockFs) PermTime(info os.FileInfo) (when time.Time, err error) {
	mockInfo, ok := info.(*mockFileInfo)
	if ok {
		return mockInfo.permTime, nil
	}
	return time.Date(0, 0, 0, 0, 0, 0, 0, nil),
		fmt.Errorf("%v is not a mockFileInfo", info)
}

func (m *MockFs) ReadDir(path string) (contents []DirEntryInfo, err error) {
	// update aggregates
	m.aggregatesLock.Lock()
	m.ReadDirCalls = append(m.ReadDirCalls, path)
	m.aggregatesLock.Unlock()

	// locate directory
	path, err = m.resolve(path, true)
	if err != nil {
		return nil, err
	}
	results := []DirEntryInfo{}
	dir, err := m.getDir(path, false)
	if err != nil {
		return nil, err
	}
	if dir.readErr != nil {
		return nil, &os.PathError{
			Op:   "read",
			Path: path,
			Err:  dir.readErr,
		}
	}
	// describe its contents
	for name, subdir := range dir.subdirs {
		dirInfo := m.dirToFileInfo(subdir, name)
		results = append(results, dirInfo)
	}
	for name, file := range dir.files {
		info := m.fileToFileInfo(file, name)
		results = append(results, info)
	}
	for name, link := range dir.symlinks {
		info := m.linkToFileInfo(link, name)
		results = append(results, info)
	}
	return results, nil
}

func (m *MockFs) Rename(sourcePath string, destPath string) error {
	// validate source parent exists
	sourcePath, err := m.resolve(sourcePath, false)
	if err != nil {
		return err
	}
	sourceParentPath := filepath.Dir(sourcePath)
	sourceParentDir, err := m.getDir(sourceParentPath, false)
	if err != nil {
		return err
	}
	if sourceParentDir == nil {
		return &os.PathError{
			Op:   "move",
			Path: sourcePath,
			Err:  os.ErrNotExist,
		}
	}
	if sourceParentDir.readErr != nil {
		return &os.PathError{
			Op:   "move",
			Path: sourcePath,
			Err:  sourceParentDir.readErr,
		}
	}

	// validate dest parent exists
	destPath, err = m.resolve(destPath, false)
	destParentPath := filepath.Dir(destPath)
	destParentDir, err := m.getDir(destParentPath, false)
	if err != nil {
		return err
	}
	if destParentDir == nil {
		return &os.PathError{
			Op:   "move",
			Path: destParentPath,
			Err:  os.ErrNotExist,
		}
	}
	if destParentDir.readErr != nil {
		return &os.PathError{
			Op:   "move",
			Path: destParentPath,
			Err:  destParentDir.readErr,
		}
	}
	// check the source and dest themselves
	sourceBase := filepath.Base(sourcePath)
	destBase := filepath.Base(destPath)

	file, sourceIsFile := sourceParentDir.files[sourceBase]
	dir, sourceIsDir := sourceParentDir.subdirs[sourceBase]
	link, sourceIsLink := sourceParentDir.symlinks[sourceBase]

	// validate that the source exists
	if !sourceIsFile && !sourceIsDir && !sourceIsLink {
		return &os.PathError{
			Op:   "move",
			Path: sourcePath,
			Err:  os.ErrNotExist,
		}

	}

	// validate the destination doesn't already exist as an incompatible type
	_, destWasFile := destParentDir.files[destBase]
	_, destWasDir := destParentDir.subdirs[destBase]
	_, destWasLink := destParentDir.symlinks[destBase]

	if destWasDir {
		return &os.PathError{
			Op:   "move",
			Path: destPath,
			Err:  errors.New("destination exists as a directory"),
		}
	}

	if sourceIsDir && (destWasFile || destWasLink) {
		return &os.PathError{
			Op:   "move",
			Path: destPath,
			Err:  errors.New("destination exists as a file"),
		}
	}

	if destWasFile {
		delete(destParentDir.files, destBase)
	}
	if destWasDir {
		delete(destParentDir.subdirs, destBase)
	}
	if destWasLink {
		delete(destParentDir.symlinks, destBase)
	}

	if sourceIsFile {
		destParentDir.files[destBase] = file
		delete(sourceParentDir.files, sourceBase)
	}
	if sourceIsDir {
		destParentDir.subdirs[destBase] = dir
		delete(sourceParentDir.subdirs, sourceBase)
	}
	if sourceIsLink {
		destParentDir.symlinks[destBase] = link
		delete(destParentDir.symlinks, sourceBase)
	}

	destParentDir.modTime = m.Clock.Time()
	sourceParentDir.modTime = m.Clock.Time()
	return nil
}

func (m *MockFs) newInodeNumber() uint64 {
	result := m.nextInodeNumber
	m.nextInodeNumber++
	return result
}

func (m *MockFs) WriteFile(filePath string, data []byte, perm os.FileMode) error {
	filePath, err := m.resolve(filePath, true)
	if err != nil {
		return err
	}
	parentPath := filepath.Dir(filePath)
	parentDir, err := m.getDir(parentPath, false)
	if err != nil || parentDir == nil {
		return &os.PathError{
			Op:   "write",
			Path: parentPath,
			Err:  os.ErrNotExist,
		}
	}
	if parentDir.readErr != nil {
		return &os.PathError{
			Op:   "write",
			Path: parentPath,
			Err:  parentDir.readErr,
		}
	}

	baseName := filepath.Base(filePath)
	_, exists := parentDir.files[baseName]
	if !exists {
		parentDir.modTime = m.Clock.Time()
		parentDir.files[baseName] = m.newFile()
	} else {
		readErr := parentDir.files[baseName].readErr
		if readErr != nil {
			return &os.PathError{
				Op:   "write",
				Path: filePath,
				Err:  readErr,
			}
		}
	}
	file := parentDir.files[baseName]
	file.bytes = data
	file.modTime = m.Clock.Time()
	return nil
}

func (m *MockFs) newFile() *mockFile {
	newFile := &mockFile{}
	newFile.inodeNumber = m.newInodeNumber()
	newFile.modTime = m.Clock.Time()
	newFile.permTime = newFile.modTime
	return newFile
}

func (m *MockFs) newDir() *mockDir {
	newDir := &mockDir{
		subdirs:  make(map[string]*mockDir, 0),
		files:    make(map[string]*mockFile, 0),
		symlinks: make(map[string]*mockLink, 0),
	}
	newDir.inodeNumber = m.newInodeNumber()
	newDir.modTime = m.Clock.Time()
	newDir.permTime = newDir.modTime
	return newDir
}

func (m *MockFs) newLink(target string) *mockLink {
	newLink := &mockLink{
		target: target,
	}
	newLink.inodeNumber = m.newInodeNumber()
	newLink.modTime = m.Clock.Time()
	newLink.permTime = newLink.modTime

	return newLink
}
func (m *MockFs) MkDirs(path string) error {
	_, err := m.getDir(path, true)
	return err
}

// getDir doesn't support symlinks
func (m *MockFs) getDir(path string, createIfMissing bool) (dir *mockDir, err error) {
	cleanedPath := filepath.Clean(path)
	if cleanedPath == "/" {
		return &m.root, nil
	}

	parentPath, leaf := pathSplit(cleanedPath)
	if len(parentPath) >= len(path) {
		return &m.root, nil
	}
	parent, err := m.getDir(parentPath, createIfMissing)
	if err != nil {
		return nil, err
	}
	if parent.readErr != nil {
		return nil, &os.PathError{
			Op:   "stat",
			Path: path,
			Err:  parent.readErr,
		}
	}
	childDir, dirExists := parent.subdirs[leaf]
	if !dirExists {
		if createIfMissing {
			// confirm that a file with the same name doesn't already exist
			_, fileExists := parent.files[leaf]
			if fileExists {
				return nil, &os.PathError{
					Op:   "mkdir",
					Path: path,
					Err:  os.ErrExist,
				}
			}
			// create this directory
			childDir = m.newDir()
			parent.subdirs[leaf] = childDir
			parent.modTime = m.Clock.Time()
		} else {
			return nil, &os.PathError{
				Op:   "stat",
				Path: path,
				Err:  os.ErrNotExist,
			}
		}
	}
	return childDir, nil

}

func (m *MockFs) Remove(path string) (err error) {
	path, err = m.resolve(path, false)
	parentPath, leaf := pathSplit(path)
	if len(leaf) == 0 {
		return fmt.Errorf("Cannot remove %v\n", path)
	}
	parentDir, err := m.getDir(parentPath, false)
	if err != nil {
		return err
	}
	if parentDir == nil {
		return &os.PathError{
			Op:   "remove",
			Path: path,
			Err:  os.ErrNotExist,
		}
	}
	if parentDir.readErr != nil {
		return &os.PathError{
			Op:   "remove",
			Path: path,
			Err:  parentDir.readErr,
		}
	}
	_, isDir := parentDir.subdirs[leaf]
	if isDir {
		return &os.PathError{
			Op:   "remove",
			Path: path,
			Err:  os.ErrInvalid,
		}
	}
	_, isLink := parentDir.symlinks[leaf]
	if isLink {
		delete(parentDir.symlinks, leaf)
	} else {
		_, isFile := parentDir.files[leaf]
		if !isFile {
			return &os.PathError{
				Op:   "remove",
				Path: path,
				Err:  os.ErrNotExist,
			}
		}
		delete(parentDir.files, leaf)
	}
	parentDir.modTime = m.Clock.Time()
	return nil
}

func (m *MockFs) Symlink(oldPath string, newPath string) (err error) {
	newPath, err = m.resolve(newPath, false)
	if err != nil {
		return err
	}

	newParentPath, leaf := pathSplit(newPath)
	newParentDir, err := m.getDir(newParentPath, false)
	if newParentDir.readErr != nil {
		return &os.PathError{
			Op:   "link",
			Path: newPath,
			Err:  newParentDir.readErr,
		}
	}
	if err != nil {
		return err
	}
	newParentDir.symlinks[leaf] = m.newLink(oldPath)
	return nil
}

func (m *MockFs) RemoveAll(path string) (err error) {
	path, err = m.resolve(path, false)
	if err != nil {
		return err
	}
	parentPath, leaf := pathSplit(path)
	if len(leaf) == 0 {
		return fmt.Errorf("Cannot remove %v\n", path)
	}
	parentDir, err := m.getDir(parentPath, false)
	if err != nil {
		return err
	}
	if parentDir == nil {
		return &os.PathError{
			Op:   "removeAll",
			Path: path,
			Err:  os.ErrNotExist,
		}
	}
	if parentDir.readErr != nil {
		return &os.PathError{
			Op:   "removeAll",
			Path: path,
			Err:  parentDir.readErr,
		}

	}
	_, isFile := parentDir.files[leaf]
	_, isLink := parentDir.symlinks[leaf]
	if isFile || isLink {
		return m.Remove(path)
	}
	_, isDir := parentDir.subdirs[leaf]
	if !isDir {
		if !isDir {
			return &os.PathError{
				Op:   "removeAll",
				Path: path,
				Err:  os.ErrNotExist,
			}
		}
	}

	delete(parentDir.subdirs, leaf)
	parentDir.modTime = m.Clock.Time()
	return nil
}

func (m *MockFs) SetReadable(path string, readable bool) error {
	var readErr error
	if !readable {
		readErr = os.ErrPermission
	}
	return m.SetReadErr(path, readErr)
}

func (m *MockFs) SetReadErr(path string, readErr error) error {
	path, err := m.resolve(path, false)
	if err != nil {
		return err
	}
	parentPath, leaf := filepath.Split(path)
	parentDir, err := m.getDir(parentPath, false)
	if err != nil {
		return err
	}
	if parentDir.readErr != nil {
		return &os.PathError{
			Op:   "chmod",
			Path: parentPath,
			Err:  parentDir.readErr,
		}
	}

	inode, err := m.getInode(parentDir, leaf)
	if err != nil {
		return err
	}
	inode.readErr = readErr
	inode.permTime = m.Clock.Time()
	return nil
}

func (m *MockFs) ClearMetrics() {
	m.ReadDirCalls = []string{}
	m.StatCalls = []string{}
}

func (m *MockFs) ViewId() (id string) {
	return m.viewId
}

func (m *MockFs) SetViewId(id string) {
	m.viewId = id
}
func (m *MockFs) SetDeviceNumber(deviceNumber uint64) {
	m.deviceNumber = deviceNumber
}
