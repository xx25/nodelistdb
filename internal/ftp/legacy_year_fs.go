package ftp

import (
	"errors"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

// legacyYearFs keeps the network-less FidoNet paths reachable after the archive
// grew a network level.
//
// FidoNet's year directories used to sit at the root of the nodelist archive,
// so the published FTP paths were /nodelists/2026/nodelist.191.gz — no network
// anywhere, while every other network had one. They now live under fidonet/
// like everyone else, which is what /nodelists lists. Those old paths have been
// handed out for as long as this archive has existed, though, so a miss on a
// top-level year retries inside fidonet/ before failing.
//
// The retry deliberately does not surface in directory listings: Readdir goes
// to the wrapped filesystem untouched, so /nodelists shows fidonet/ and not a
// second set of forty year directories. Legacy paths stay reachable without
// being advertised, which is the whole point.
//
// The rewrite happens inside the mount's already-chrooted filesystem and only
// ever prepends a fixed segment, so it cannot escape the mount.
type legacyYearFs struct {
	afero.Fs
}

// fidonetAlternatePath translates between the two spellings of a FidoNet year
// path, in whichever direction the given one is not:
//
//	/2026/nodelist.191.gz  <->  /fidonet/2026/nodelist.191.gz
//
// Both directions are needed, and not only for the archives that never migrate.
// Moving 41 year directories is 41 renames, so for the duration of a move some
// years answer to one spelling and some to the other; translating both ways
// means every year answers to both throughout. It reports false for anything
// that is not a year path, including /fidonet/<non-year>, which is why the
// pointlist tree (/fidonet/<source>/<year>) is left alone.
func fidonetAlternatePath(name string) (string, bool) {
	trimmed := strings.TrimPrefix(path.Clean("/"+name), "/")
	if trimmed == "" {
		return "", false
	}

	first, rest, _ := strings.Cut(trimmed, "/")

	// /fidonet/<year>/... -> /<year>/...
	if strings.EqualFold(first, fidonetDir) {
		year, tail, _ := strings.Cut(rest, "/")
		if !isYear(year) {
			return "", false
		}
		return "/" + path.Join(year, tail), true
	}

	// /<year>/... -> /fidonet/<year>/...
	if !isYear(first) {
		return "", false
	}
	return "/" + path.Join(fidonetDir, first, rest), true
}

// isYear reports whether a path segment is a 4-digit year directory.
func isYear(segment string) bool {
	if len(segment) != 4 {
		return false
	}
	_, err := strconv.Atoi(segment)
	return err == nil
}

// fidonetDir is the archive subdirectory FidoNet's year directories moved into.
const fidonetDir = "fidonet"

// retry reports whether err is worth a second lookup under fidonet/.
func retry(err error) bool {
	return err != nil && errors.Is(err, fs.ErrNotExist)
}

func (l legacyYearFs) Open(name string) (afero.File, error) {
	file, err := l.Fs.Open(name)
	if !retry(err) {
		return file, err
	}
	if alt, ok := fidonetAlternatePath(name); ok {
		if file, altErr := l.Fs.Open(alt); altErr == nil {
			return file, nil
		}
	}
	return nil, err
}

func (l legacyYearFs) OpenFile(name string, flag int, perm fs.FileMode) (afero.File, error) {
	file, err := l.Fs.OpenFile(name, flag, perm)
	if !retry(err) {
		return file, err
	}
	if alt, ok := fidonetAlternatePath(name); ok {
		if file, altErr := l.Fs.OpenFile(alt, flag, perm); altErr == nil {
			return file, nil
		}
	}
	return nil, err
}

func (l legacyYearFs) Stat(name string) (fs.FileInfo, error) {
	info, err := l.Fs.Stat(name)
	if !retry(err) {
		return info, err
	}
	if alt, ok := fidonetAlternatePath(name); ok {
		if info, altErr := l.Fs.Stat(alt); altErr == nil {
			return info, nil
		}
	}
	return nil, err
}
