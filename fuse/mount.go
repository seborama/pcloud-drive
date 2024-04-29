package fuse

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"github.com/samber/lo"
	"github.com/seborama/pcloud-drive/v1/logger"
	"github.com/seborama/pcloud-sdk/sdk"
)

type Drive struct {
	fs   fs.FS
	conn *fuse.Conn // TODO: define an interface
}

const mb = 1_048_576

func NewDrive(mountpoint string, readWrite bool, pcClient *sdk.Client) (*Drive, error) {
	mountOpts := []fuse.MountOption{
		fuse.FSName("pcloud"),
		fuse.Subtype("seborama"),
		fuse.MaxReadahead(10 * mb),
		fuse.AsyncRead(),
		fuse.WritebackCache(),
	}
	if !readWrite {
		mountOpts = append(mountOpts, fuse.ReadOnly())
	}

	conn, err := fuse.Mount(mountpoint, mountOpts...)
	if err != nil {
		return nil, err
	}

	logger.Infof("fuse connection", "features", conn.Features().String())

	user, err := user.Current()
	if err != nil {
		return nil, err
	}
	uid, err := strconv.ParseUint(user.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(user.Gid, 10, 32)
	if err != nil {
		return nil, err
	}

	return &Drive{
		fs: &FS{
			conn:      conn,
			pcClient:  pcClient,
			uid:       uint32(uid),
			gid:       uint32(gid),
			dirPerms:  0o750,
			filePerms: 0o640,
			dirValid:  2 * time.Second,
			fileValid: time.Second,
		},
		conn: conn,
	}, nil
}

func (d *Drive) Unmount() error {
	return d.conn.Close()
}

func (d *Drive) Mount() error {
	return fs.Serve(d.conn, d.fs)
}

// FS implements the pCloud file system.
type FS struct {
	conn      *fuse.Conn
	pcClient  *sdk.Client // TODO: define an interface
	uid       uint32
	gid       uint32
	dirPerms  os.FileMode
	filePerms os.FileMode
	dirValid  time.Duration
	fileValid time.Duration
}

// ensure interfaces conpliance
var (
	_ fs.FS = (*FS)(nil)
)

func (fs *FS) Root() (fs.Node, error) {
	logger.Infof("entering")

	rootDir := &Dir{
		Type: fuse.DT_Dir,
		fs:   fs,
	}

	err := rootDir.materialiseFolder(context.Background())
	if err != nil {
		return nil, err
	}

	return rootDir, nil
}

// Dir implements both Node and Handle for the root directory.
type Dir struct {
	Type       fuse.DirentType
	Attributes fuse.Attr

	Entries map[string]fs.Node

	fs             *FS
	parentFolderID uint64
	folderID       uint64
}

// ensure interfaces conpliance
var (
	_ fs.Node               = (*Dir)(nil)
	_ fs.NodeStringLookuper = (*Dir)(nil)
	_ fs.HandleReadDirAller = (*Dir)(nil)
	_ fs.NodeCreater        = (*Dir)(nil)
	_ fs.NodeRemover        = (*Dir)(nil)
)

func (d *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	logger.Infof("entering", slog.Uint64("folderID", d.folderID))
	*a = d.Attributes
	return nil
}

func (d *Dir) materialiseFolder(ctx context.Context) error {
	logger.Infof("entering", slog.Uint64("folderID", d.folderID))

	fsList, err := d.fs.pcClient.ListFolder(ctx, sdk.T1FolderByID(d.folderID), false, false, false, false)
	if err != nil {
		logger.Errorf("ListFolder failed", "folderID", d.folderID, "error", err)
		return err
	}

	// TODO: is this necessary? perhaps only for the root folder?
	d.Attributes = fuse.Attr{
		Valid: d.fs.dirValid,
		Inode: d.folderID,
		Atime: fsList.Metadata.Modified.Time,
		Mtime: fsList.Metadata.Modified.Time,
		Ctime: fsList.Metadata.Modified.Time,
		Mode:  os.ModeDir | d.fs.dirPerms,
		Nlink: 1, // TODO: is that right? How else can we find this value?
		Uid:   d.fs.uid,
		Gid:   d.fs.gid,
	}
	d.parentFolderID = fsList.Metadata.ParentFolderID
	d.folderID = fsList.Metadata.FolderID

	entries := lo.SliceToMap(fsList.Metadata.Contents, func(item *sdk.Metadata) (string, fs.Node) {
		if item.IsFolder {
			return item.Name, &Dir{
				Type: fuse.DT_Dir,
				Attributes: fuse.Attr{
					Valid: d.fs.dirValid,
					Inode: item.FolderID,
					Atime: item.Modified.Time,
					Mtime: item.Modified.Time,
					Ctime: item.Modified.Time,
					Mode:  os.ModeDir | d.fs.dirPerms,
					Nlink: 1, // the official pCloud client can show other values that 1 - dunno how
					Uid:   d.fs.uid,
					Gid:   d.fs.gid,
				},
				Entries:        nil, // will be populated upon access by Dir.Lookup or Dir.ReadDirAll
				fs:             d.fs,
				parentFolderID: item.ParentFolderID,
				folderID:       item.FolderID,
			}
		}

		return item.Name, &File{
			Type: fuse.DT_File,
			Attributes: fuse.Attr{
				Valid:     d.fs.fileValid,
				Inode:     item.FileID,
				Size:      item.Size,
				Blocks:    item.Size / 512, // TODO: or / BlockSize??
				Atime:     item.Modified.Time,
				Mtime:     item.Modified.Time,
				Ctime:     item.Modified.Time,
				Mode:      d.fs.filePerms,
				Nlink:     1, // TODO: is that right? How else can we find this value?
				Uid:       d.fs.uid,
				Gid:       d.fs.gid,
				BlockSize: 1_048_576,
			},
			fs:     d.fs,
			fileID: item.FileID,
			file:   nil,
		}
	})

	d.Entries = entries

	return nil
}

// Lookup looks up a specific entry in the receiver,
// which must be a directory.  Lookup should return a Node
// corresponding to the entry.  If the name does not exist in
// the directory, Lookup should return ENOENT.
//
// Lookup need not to handle the names "." and "..".
func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	logger.Infof("entering", slog.Uint64("folderID", d.folderID), slog.Group("args", slog.String("name", name)), slog.Int("entries_count", len(d.Entries)))

	if node, ok := d.Entries[name]; ok {
		return node.(fs.Node), nil
	}

	// materialise the folder and try again
	if err := d.materialiseFolder(ctx); err != nil {
		logger.Errorf("materialiseFolder failed", "folderID", d.folderID, "name", name, "error", err)
		return nil, err
	}
	logger.Infof("content refreshed", slog.Uint64("folderID", d.folderID))

	if node, ok := d.Entries[name]; ok {
		return node.(fs.Node), nil
	}

	return nil, syscall.ENOENT
}

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	logger.Infof("entering", slog.Uint64("folderID", d.folderID), slog.Uint64("parentFolderID", d.parentFolderID))

	if err := d.materialiseFolder(ctx); err != nil {
		logger.Errorf("materialiseFolder failed", "folderID", d.folderID, "error", err)
		return nil, err
	}

	dirEntries := lo.MapToSlice(d.Entries, func(key string, value fs.Node) fuse.Dirent {
		switch castEntry := value.(type) {
		case *File:
			return fuse.Dirent{
				Inode: castEntry.Attributes.Inode,
				Type:  castEntry.Type,
				Name:  key,
			}

		case *Dir:
			return fuse.Dirent{
				Inode: castEntry.Attributes.Inode,
				Type:  castEntry.Type,
				Name:  key,
			}

		default:
			logger.Infof("unknown directory entry type", slog.Uint64("folderID", d.folderID), slog.String("type", slog.AnyValue(castEntry).Kind().String()))
			return fuse.Dirent{
				Inode: 9_505_505_505_505_505_505, // 9 followed by SOS
				Type:  fuse.DT_Unknown,
				Name:  key,
			}
		}
	})

	return dirEntries, nil
}

// TODO: should check FileMode (including but not only, ModeDir!)
func (d *Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	logger.Infof("entering", slog.Uint64("folderID", d.folderID), slog.Group("args", "req", req))

	openFlags := fuseToPcloudFlags(req.Flags)

	pcFile, err := d.fs.pcClient.FileOpen(ctx, openFlags, sdk.T4FileByFolderIDName(d.folderID, req.Name))
	if err != nil {
		logger.Errorf("FileOpen failed", "folderID", d.folderID, "req.Name", req.Name, "error", err)
		return nil, nil, err
	}
	if err = d.fs.pcClient.FileClose(ctx, pcFile.FD); err != nil {
		// FileCreate returns an FD so we close it to avoid a leak.
		// TODO: instead, we could store this in the File structure to re-use it, if that's safe...
		logger.Warnf("FileClose failed", "FD", pcFile.FD, "FileID", pcFile.FileID, "error", err)
	}

	now := time.Now()

	file := &File{
		Type: fuse.DT_File,
		Attributes: fuse.Attr{
			Valid:     d.fs.fileValid,
			Inode:     pcFile.FileID,
			Size:      0,   // file was just created
			Blocks:    0,   // file was just created
			Atime:     now, // TODO: we should call pcClient.Stat() to get the file details from pCloud
			Mtime:     now, // TODO: we should call pcClient.Stat() to get the file details from pCloud
			Ctime:     now, // TODO: we should call pcClient.Stat() to get the file details from pCloud
			Mode:      d.fs.filePerms,
			Nlink:     1, // TODO: is that right? How else can we find this value?
			Uid:       d.fs.uid,
			Gid:       d.fs.gid,
			BlockSize: 1_048_576,
		},
		fs:     d.fs,
		fileID: pcFile.FileID,
		file:   nil,
	}

	return file, file, nil
}

func (d *Dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	logger.Infof("entering", slog.Uint64("folderID", d.folderID), slog.Group("args", "req", req))

	node, ok := d.Entries[req.Name]
	if !ok {
		// materialise the folder and try again
		if err := d.materialiseFolder(ctx); err != nil {
			logger.Errorf("materialiseFolder failed", "folderID", d.folderID, "Name", req.Name, "error", err)
			return err
		}
		logger.Infof("content refreshed", slog.Uint64("folderID", d.folderID))

		if node, ok = d.Entries[req.Name]; !ok {
			logger.Errorf("Remove failed", "req.ID", req.ID, "error", syscall.ENOENT)
			return syscall.ENOENT
		}
	}

	// return node.(fs.Node), nil
	if req.Dir {
		if _, err := d.fs.pcClient.DeleteFolder(ctx, sdk.T1FolderByID(node.(*Dir).folderID)); err != nil {
			logger.Errorf("DeleteFolder failed", "folderID", d.folderID, "Name", req.Name, "error", err)
		}
	} else {
		if _, err := d.fs.pcClient.DeleteFile(ctx, sdk.T3FileByID(node.(*File).fileID)); err != nil {
			logger.Errorf("DeleteFile failed", "fileID", node.(*File).fileID, "Name", req.Name, "error", err)
		}
	}

	delete(d.Entries, req.Name)
	return nil
}

func (d *Dir) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	logger.Infof("entering", "req.ID", req.ID, slog.Group("args", "req", req), "valid", req.Valid.String())

	if req.Valid.Atime() {
		d.Attributes.Atime = req.Atime
	}
	if req.Valid.Mtime() {
		d.Attributes.Mtime = req.Mtime
	}
	if req.Valid.Size() {
		d.Attributes.Size = req.Size
	}
	if req.Valid.Gid() {
		d.Attributes.Gid = req.Gid
	}
	if req.Valid.Uid() {
		d.Attributes.Uid = req.Uid
	}
	if req.Valid.Mode() {
		d.Attributes.Mode = req.Mode
	}

	resp.Attr = d.Attributes
	logger.Infof("response", "resp", resp)

	return nil
}

func (d *Dir) Getattr(ctx context.Context, req *fuse.GetattrRequest, resp *fuse.GetattrResponse) error {
	logger.Infof("entering", "req.ID", req.ID, slog.Group("args", "req", req))
	resp.Attr = d.Attributes
	return nil
}

// File implements both Node and Handle for the hello file.
type File struct {
	Type       fuse.DirentType
	Attributes fuse.Attr
	fs         *FS
	fileID     uint64
	file       *sdk.File
}

// ensure interfaces conpliance
var (
	_ = (fs.Node)((*File)(nil))
	_ = (fs.NodeOpener)((*File)(nil))
	_ = (fs.NodeSetattrer)((*File)(nil))
	_ = (fs.NodeGetattrer)((*File)(nil))
	_ = (fs.HandleWriter)((*File)(nil))
	_ = (fs.HandleReader)((*File)(nil))
	_ = (fs.HandleFlusher)((*File)(nil))
	_ = (fs.HandleReleaser)((*File)(nil))
	// _ = (fs.HandleReadAller)((*File)(nil)) // NOTE: it's best avoiding to implement this method to avoid costly memory operations with large files.
)

func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	logger.Infof("entering", slog.Uint64("fileID", f.fileID))
	*a = f.Attributes
	return nil
}

// TODO: we could return fileHandle from Open and implement all the goodies against it.
// type fileHandle struct {
// 	file *sdk.File
// }

func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	logger.Infof("entering", "req.ID", req.ID, slog.Group("args", "req", req, "f.fileID", f.fileID))

	openFlags := fuseToPcloudFlags(req.Flags)

	file, err := f.fs.pcClient.FileOpen(ctx, openFlags, sdk.T4FileByID(f.fileID))
	if err != nil {
		logger.Errorf("FileOpen", "req.ID", req.ID, "file", file, "error", err)
		return nil, err
	}
	logger.Infof("file opened", "req.ID", req.ID, "file.FD", file.FD)

	f.file = file
	resp.Flags |= fuse.OpenKeepCache

	// TODO: this may be safer but causes delay for the file size to be reflected after a write.
	// return &File{
	// 	Type:       f.Type,
	// 	Attributes: f.Attributes,
	// 	fs:         f.fs,
	// 	fileID:     f.fileID,
	// 	file:       file,
	// }, nil
	// TODO: is this thread safe? Should we add a lock?
	return f, nil
}

// TODO: translate req.LockOwner >> pCloud::sdk.FileLock() (not yet implemented by the SDK)?
func (f *File) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	logger.Infof("entering", "req.ID", req.ID, slog.Group("args", "req", req))

	if req.FileFlags.IsWriteOnly() {
		logger.Errorf("IsWriteOnly", "req.ID", req.ID)
		return fuse.Errno(syscall.EACCES)
	}

	if f.file == nil {
		logger.Infof("opening file", "req.ID", req.ID)
		openFlags := fuseToPcloudFlags(req.FileFlags)
		file, err := f.fs.pcClient.FileOpen(ctx, openFlags, sdk.T4FileByID(f.fileID))
		if err != nil {
			logger.Errorf("FileOpen failed", "req.ID", req.ID, "error", err)
			return err
		}
		f.file = file
	}
	logger.Infof("file handle", "req.ID", req.ID, "file", f.file)

	// TODO: is the offset always relative to the beginning of the file??
	data, err := f.fs.pcClient.FilePRead(ctx, f.file.FD, uint64(req.Size), uint64(req.Offset))
	if err != nil {
		logger.Errorf("FilePRead failed", "req.ID", req.ID, "error", err)
		return err
	}
	resp.Data = data

	return nil
}

func fuseToPcloudFlags(openFlags fuse.OpenFlags) uint64 {
	var pcFlags uint64 = 0 // read-only

	if openFlags.IsWriteOnly() || openFlags.IsReadWrite() {
		pcFlags = sdk.O_WRITE
	}

	if openFlags&fuse.OpenAppend != 0 {
		logger.Infof(">>>>>> fuse.OpenAppend")
		pcFlags |= sdk.O_APPEND
	}
	if openFlags&fuse.OpenCreate != 0 {
		logger.Infof(">>>>>> fuse.OpenCreate")
		pcFlags |= sdk.O_CREAT
	}
	if openFlags&fuse.OpenExclusive != 0 {
		logger.Infof(">>>>>> fuse.OpenExclusive")
		pcFlags |= sdk.O_EXCL
	}
	if openFlags&fuse.OpenTruncate != 0 {
		logger.Infof(">>>>>> fuse.OpenTruncate")
		pcFlags |= sdk.O_TRUNC
	}

	logger.Infof("fuseToPcloudFlags", "pcFlags", pcFlags, slog.Uint64("openFlags", uint64(openFlags)), "openFlags.String", openFlags.String())

	return pcFlags
}

// TODO: process the req.WriteFlags
// TODO: translate req.LockOwner >> pCloud::sdk.FileLock() (not yet implemented by the SDK)?
func (f *File) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	logger.Infof("entering", "req.ID", req.ID, slog.Group("args", "req.Header", req.Header, "req.FileFlags", req.FileFlags.String(), "req.Flags", req.Flags.String(), "req.Offset", req.Offset, "req.Pid", req.Pid, "req", req.String()))

	// TODO: this gets set at unexpected times :\ Need more understanding
	// TODO: It may have something to do with the flags passed to File.Open
	// if req.FileFlags.IsReadOnly() {
	// 	logger.Errorf("write precluded: ReadOnly", "req.ID", req.ID, "error", syscall.EACCES)
	// 	return fuse.Errno(syscall.EACCES)
	// }

	openFlags := fuseToPcloudFlags(req.FileFlags)
	logger.Infof("fuseToPcloudFlags", "req.ID", req.ID, "openFlags", openFlags)

	if f.file == nil {
		logger.Infof("opening file for writing", "req.ID", req.ID)
		file, err := f.fs.pcClient.FileOpen(ctx, sdk.O_WRITE|openFlags, sdk.T4FileByID(f.fileID))
		if err != nil {
			logger.Errorf("FileOpen failed", "req.ID", req.ID, "error", err)
			return err
		}
		f.file = file
	}
	logger.Infof("file handle", "req.ID", req.ID, "file", f.file)

	if req.Offset != 0 {
		// TODO: is the offset always relative to the beginning of the file??
		_, err := f.fs.pcClient.FileSeek(ctx, f.file.FD, uint64(req.Offset), 0)
		if err != nil {
			logger.Errorf("FileSeek failed", "req.ID", req.ID, "error", err)
			return err
		}
	}

	fdt, err := f.fs.pcClient.FileWrite(ctx, f.file.FD, req.Data)
	if err != nil {
		logger.Errorf("FileWrite failed", "req.ID", req.ID, "error", err)
		return err
	}

	if req.Offset == 0 {
		f.Attributes.Size = fdt.Bytes
		logger.Infof("file size set", "size", f.Attributes.Size)
	} else {
		// the safest size evaluation is to ask pCloud because the Seek point could be
		// in the middle of the file and the bytes written may or may not be in excess of
		// the file end
		// TODO/NOTE: pCloud's SDK also has a FileSize() operation.
		fr, err := f.fs.pcClient.Stat(ctx, sdk.T3FileByID(f.fileID))
		if err != nil {
			logger.Errorf("Stat failed", "req.ID", req.ID, "error", err)
			// NOTE: this is tricky - the file was successfully written to, but we don't know
			// its size anymore. Without an error, we may report the wrong file size, with an
			// error we may cause the caller to retry and not achieve the correct outcome...
			logger.Warnf("file handle", "req.ID", req.ID, "file", f.file)
			if err = f.fs.conn.InvalidateNode(req.Node, 0, 0); err != nil {
				// Well, we tried our best...
				logger.Errorf("InvalidateNode failed - abandoning all efforts", "req.ID", req.ID, "file", f.file)
			}
		} else {
			f.Attributes.Size = fr.Metadata.Size
			logger.Infof("file size cloud refreshed", "size", f.Attributes.Size)
		}
	}

	resp.Size = int(f.Attributes.Size)

	return nil
}

func (f *File) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	logger.Infof("entering", "req.ID", req.ID, slog.Group("args", "req", req), "valid", req.Valid.String())

	if req.Valid.Atime() {
		f.Attributes.Atime = req.Atime
	}
	if req.Valid.Mtime() {
		f.Attributes.Mtime = req.Mtime
	}
	if req.Valid.Size() {
		f.Attributes.Size = req.Size
	}
	if req.Valid.Gid() {
		f.Attributes.Gid = req.Gid
	}
	if req.Valid.Uid() {
		f.Attributes.Uid = req.Uid
	}
	if req.Valid.Mode() {
		f.Attributes.Mode = req.Mode
	}

	resp.Attr = f.Attributes
	logger.Infof("response", "resp", resp)

	return nil
}

func (f *File) Getattr(ctx context.Context, req *fuse.GetattrRequest, resp *fuse.GetattrResponse) error {
	logger.Infof("entering", "req.ID", req.ID, slog.Group("args", "req", req))
	resp.Attr = f.Attributes
	return nil
}

// Flush is called each time the file or directory is closed.
// Because there can be multiple file descriptors referring to a
// single opened file, Flush can be called multiple times.
// TODO: consider req.LockOwner??
func (f *File) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	log.Printf("File.Flush - (%s) - called with req: %+#v", req.ID, req)
	if f.file == nil {
		log.Printf("File.Flush - (%s) - FD: nil", req.ID)
	}

	if f.file != nil {
		log.Printf("File.Flush - (%s) - FD: %d", req.ID, f.file.FD)
		err := f.fs.pcClient.FileClose(ctx, f.file.FD)
		if err != nil {
			logger.Errorf("FileClose failed", "req.ID", req.ID, "error", err)
		}
		f.file = nil
		return err
	}

	return nil
}

// A ReleaseRequest asks to release (close) an open file handle.
// TODO: consider req.LockOwner??
// TODO: consider req.ReleaseFlags??
// TODO: consider req.OpenFlags??
func (f *File) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	log.Printf("File.Release - (%s) - called with req: %+#v", req.ID, req)
	if f.file == nil {
		log.Printf("File.Release - (%s) - FD: nil", req.ID)
	}

	if f.file != nil {
		log.Printf("File.Release - (%s) - FD: %d", req.ID, f.file.FD)
		err := f.fs.pcClient.FileClose(ctx, f.file.FD)
		if err != nil {
			logger.Errorf("FileClose failed", "req.ID", req.ID, "error", err)
		}
		f.file = nil
		return err
	}

	return nil
}
