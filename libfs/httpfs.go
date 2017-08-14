package libfs

import (
	"errors"
	"net/http"
	"os"

	"github.com/keybase/kbfs/libkbfs"
)

type dir struct {
	fs      *FS
	dirname string
	node    libkbfs.Node
}

func (d *dir) Readdir(count int) (fis []os.FileInfo, err error) {
	d.fs.log.CDebugf(d.fs.ctx, "ReadDir %s", count)
	defer func() {
		d.fs.deferLog.CDebugf(d.fs.ctx, "ReadDir done: %+v", err)
	}()

	children, err := d.fs.config.KBFSOps().GetDirChildren(d.fs.ctx, d.node)
	if err != nil {
		return nil, err
	}

	fis = make([]os.FileInfo, 0, len(children))
	for name, ei := range children {
		fis = append(fis, &FileInfo{
			fs:   d.fs,
			ei:   ei,
			name: name,
		})
	}
	return fis, nil
}

// FileOrDir is a wrapper around billy FS types that satisfies http.File, which
// is either a file or a dir.
type FileOrDir struct {
	file *File
	dir  *dir
	ei   libkbfs.EntryInfo
}

var _ http.File = FileOrDir{}

// FileOrDir implements the http.File interface.
func (fod FileOrDir) Read(p []byte) (n int, err error) {
	if fod.file == nil {
		return 0, libkbfs.NotFileError{}
	}
	return fod.file.Read(p)
}

// Close implements the http.File interface.
func (fod FileOrDir) Close() (err error) {
	if fod.file != nil {
		err = fod.file.Close()
	}
	if fod.dir != nil {
		fod.dir.node = nil
	}
	fod.file = nil
	fod.dir = nil
	return err
}

// Seek implements the http.File interface.
func (fod FileOrDir) Seek(offset int64, whence int) (n int64, err error) {
	if fod.file == nil {
		return 0, libkbfs.NotFileError{}
	}
	return fod.file.Seek(offset, whence)
}

// Readdir implements the http.File interface.
func (fod FileOrDir) Readdir(count int) ([]os.FileInfo, error) {
	if fod.dir == nil {
		return nil, libkbfs.NotDirError{}
	}
	return fod.dir.Readdir(count)
}

// Stat implements the http.File interface.
func (fod FileOrDir) Stat() (os.FileInfo, error) {
	if fod.file != nil {
		return &FileInfo{
			fs:   fod.file.fs,
			ei:   fod.ei,
			name: fod.file.filename,
		}, nil
	} else if fod.dir != nil {
		return &FileInfo{
			fs:   fod.dir.fs,
			ei:   fod.ei,
			name: fod.dir.dirname,
		}, nil
	}
	return nil, errors.New("invalid fod")
}

// HTTPFileSystem is a simple wrapper around *FS that satisfies http.FileSystem
// interface.
type HTTPFileSystem struct {
	fs *FS
}

var _ http.FileSystem = HTTPFileSystem{}

// Open impelements the http.FileSystem interface.
func (hfs HTTPFileSystem) Open(filename string) (entry http.File, err error) {
	hfs.fs.log.CDebugf(
		hfs.fs.ctx, "hfs.Open %s", filename)
	defer func() {
		hfs.fs.deferLog.CDebugf(hfs.fs.ctx, "hfs.Open done: %+v", err)
	}()

	n, ei, err := hfs.fs.lookupOrCreateEntry(filename, os.O_RDONLY, 0600)
	if err != nil {
		return FileOrDir{}, err
	}

	if ei.Type.IsFile() {
		return FileOrDir{
			file: &File{
				fs:       hfs.fs,
				filename: filename,
				node:     n,
				readOnly: true,
				offset:   0,
			},
		}, nil
	}
	return FileOrDir{
		dir: &dir{
			fs:      hfs.fs,
			dirname: filename,
			node:    n,
		},
	}, nil
}
