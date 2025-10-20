package ftp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// Mount represents a filesystem mount point
type Mount struct {
	VirtualPath string   // Virtual path (e.g., /fidonet/nodelists)
	Fs          afero.Fs // Filesystem to serve at this path
}

// MountFs implements a multi-mount virtual filesystem
// It routes requests to different filesystems based on virtual path prefixes
type MountFs struct {
	mounts []Mount
}

// NewMountFs creates a new mount-based filesystem
func NewMountFs(mounts []Mount) *MountFs {
	// Sort mounts by path length (longest first) for proper prefix matching
	sorted := make([]Mount, len(mounts))
	copy(sorted, mounts)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].VirtualPath) > len(sorted[j].VirtualPath)
	})

	return &MountFs{mounts: sorted}
}

// findMount finds the mount for a given virtual path
// Returns the mount, the relative path within that mount, and whether found
func (m *MountFs) findMount(virtualPath string) (*Mount, string, bool) {
	virtualPath = filepath.Clean("/" + virtualPath)

	for i := range m.mounts {
		mount := &m.mounts[i]
		mountPath := filepath.Clean("/" + mount.VirtualPath)

		// Exact match or under this mount
		if virtualPath == mountPath {
			return mount, "/", true
		}
		if strings.HasPrefix(virtualPath, mountPath+"/") {
			relPath := strings.TrimPrefix(virtualPath, mountPath)
			return mount, relPath, true
		}
	}

	return nil, "", false
}

// listMountsInDir lists all mount points that are direct children of the given directory
func (m *MountFs) listMountsInDir(virtualPath string) []string {
	virtualPath = filepath.Clean("/" + virtualPath)
	var children []string
	seen := make(map[string]bool)

	for i := range m.mounts {
		mountPath := filepath.Clean("/" + m.mounts[i].VirtualPath)

		// Skip mounts that aren't under this directory
		if virtualPath != "/" && !strings.HasPrefix(mountPath, virtualPath+"/") {
			continue
		}

		// Get the relative path from virtualPath to the mount
		var relPath string
		if virtualPath == "/" {
			relPath = strings.TrimPrefix(mountPath, "/")
		} else {
			relPath = strings.TrimPrefix(mountPath, virtualPath+"/")
		}

		// Get just the first component
		parts := strings.Split(relPath, "/")
		if len(parts) > 0 && parts[0] != "" && !seen[parts[0]] {
			children = append(children, parts[0])
			seen[parts[0]] = true
		}
	}

	return children
}

// Open opens a file
func (m *MountFs) Open(name string) (afero.File, error) {
	name = filepath.Clean("/" + name)

	// Check if this is a mount point or under one
	mount, relPath, found := m.findMount(name)
	if found {
		return mount.Fs.Open(relPath)
	}

	// This is a virtual directory containing mount points
	children := m.listMountsInDir(name)
	if len(children) > 0 {
		return &virtualDir{
			path:     name,
			children: children,
		}, nil
	}

	return nil, os.ErrNotExist
}

// Stat returns file info
func (m *MountFs) Stat(name string) (os.FileInfo, error) {
	name = filepath.Clean("/" + name)

	// Check if this is a mount point or under one
	mount, relPath, found := m.findMount(name)
	if found {
		return mount.Fs.Stat(relPath)
	}

	// This might be a virtual directory containing mount points
	children := m.listMountsInDir(name)
	if len(children) > 0 {
		return &virtualDirInfo{name: filepath.Base(name), modTime: time.Now()}, nil
	}

	return nil, os.ErrNotExist
}

// Read-only operations that always fail
func (m *MountFs) Create(name string) (afero.File, error)                         { return nil, os.ErrPermission }
func (m *MountFs) Mkdir(name string, perm os.FileMode) error                      { return os.ErrPermission }
func (m *MountFs) MkdirAll(path string, perm os.FileMode) error                   { return os.ErrPermission }
func (m *MountFs) Remove(name string) error                                        { return os.ErrPermission }
func (m *MountFs) RemoveAll(path string) error                                     { return os.ErrPermission }
func (m *MountFs) Rename(oldname, newname string) error                            { return os.ErrPermission }
func (m *MountFs) Chmod(name string, mode os.FileMode) error                       { return os.ErrPermission }
func (m *MountFs) Chown(name string, uid, gid int) error                           { return os.ErrPermission }
func (m *MountFs) Chtimes(name string, atime time.Time, mtime time.Time) error { return os.ErrPermission }
func (m *MountFs) Name() string                                                     { return "MountFs" }

func (m *MountFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	// Only allow read-only access
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, os.ErrPermission
	}
	return m.Open(name)
}

// virtualDir represents a virtual directory that contains mount points
type virtualDir struct {
	path     string
	children []string
	index    int
}

func (v *virtualDir) Close() error               { return nil }
func (v *virtualDir) Read(p []byte) (n int, err error) { return 0, os.ErrInvalid }
func (v *virtualDir) ReadAt(p []byte, off int64) (n int, err error) { return 0, os.ErrInvalid }
func (v *virtualDir) Seek(offset int64, whence int) (int64, error) { return 0, os.ErrInvalid }
func (v *virtualDir) Write(p []byte) (n int, err error) { return 0, os.ErrPermission }
func (v *virtualDir) WriteAt(p []byte, off int64) (n int, err error) { return 0, os.ErrPermission }
func (v *virtualDir) WriteString(s string) (n int, err error) { return 0, os.ErrPermission }
func (v *virtualDir) Name() string { return v.path }
func (v *virtualDir) Sync() error  { return nil }
func (v *virtualDir) Truncate(size int64) error { return os.ErrPermission }

func (v *virtualDir) Readdir(count int) ([]os.FileInfo, error) {
	if count <= 0 {
		count = len(v.children)
	}

	if v.index >= len(v.children) {
		return nil, nil
	}

	end := v.index + count
	if end > len(v.children) {
		end = len(v.children)
	}

	infos := make([]os.FileInfo, end-v.index)
	for i := v.index; i < end; i++ {
		infos[i-v.index] = &virtualDirInfo{
			name:    v.children[i],
			modTime: time.Now(),
		}
	}

	v.index = end
	return infos, nil
}

func (v *virtualDir) Readdirnames(n int) ([]string, error) {
	infos, err := v.Readdir(n)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name()
	}
	return names, nil
}

func (v *virtualDir) Stat() (os.FileInfo, error) {
	return &virtualDirInfo{name: filepath.Base(v.path), modTime: time.Now()}, nil
}

// virtualDirInfo represents info for a virtual directory
type virtualDirInfo struct {
	name    string
	modTime time.Time
}

func (v *virtualDirInfo) Name() string       { return v.name }
func (v *virtualDirInfo) Size() int64        { return 0 }
func (v *virtualDirInfo) Mode() os.FileMode  { return os.ModeDir | 0755 }
func (v *virtualDirInfo) ModTime() time.Time { return v.modTime }
func (v *virtualDirInfo) IsDir() bool        { return true }
func (v *virtualDirInfo) Sys() interface{}   { return nil }
