// Package fsio reads and writes files inside one work directory and nowhere
// else.
//
// A spawned MCP server is confined by no runtime (organization ADR-021 §7), and
// the work directory is writable from inside the caller's sandbox, so anything
// in it — a symlink, a FIFO, a planted file at the output path — is input.
// Every open therefore goes through os.Root, which refuses to leave the
// directory by "..", by an absolute link or by a link chain, at the kernel's
// pace rather than after a check on the spelling (a check-then-open is a
// race). On top of that, each file actually opened is named back by the kernel
// (fcntl F_GETPATH) and judged by pathguard: the floor of places never to
// touch, which is not containment and so never replaces os.Root (organization
// ADR-022).
//
// Only regular files are read or written. A FIFO is opened without blocking
// and refused, a directory is refused, and an output is created under a fresh
// temporary name with O_EXCL and renamed into place, so a link planted at the
// output path is replaced rather than followed.
package fsio

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/nlink-jp/report-studio-mcp/internal/toolerr"
)

// Judge is pathguard's file verdict, as the workdir adapter exposes it. Both
// return a details.reason and a sentence, or two empty strings.
type Judge interface {
	// LocalPath judges a file read or written on this machine.
	LocalPath(raw, resolved string) (reason, why string)
	// OutboundPath judges a file whose content leaves the machine.
	OutboundPath(raw, resolved string) (reason, why string)
}

// Dir is one work directory opened for a call.
type Dir struct {
	root    *os.Root
	judge   Judge
	maxOut  int64
	written int64
}

// Open opens dir, which the caller has already validated as a work directory.
// maxOutput bounds the bytes all WriteFile calls on this Dir may write.
func Open(dir string, judge Judge, maxOutput int64) (*Dir, error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot open the work directory: %v", err)
	}
	return &Dir{root: r, judge: judge, maxOut: maxOutput}, nil
}

// Close releases the directory.
func (d *Dir) Close() error { return d.root.Close() }

// ReadFile reads the regular file at rel, a path relative to the work
// directory, refusing it when it is larger than max bytes. The content is
// judged as leaving the machine: whatever is read here ends up in a document
// handed to someone.
func (d *Dir) ReadFile(rel string, max int64) ([]byte, error) {
	rel, err := local(rel)
	if err != nil {
		return nil, err
	}
	// O_NONBLOCK: a FIFO opened plainly waits for a writer for ever. It has no
	// effect on a regular file.
	f, err := d.root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, openErr(rel, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot stat %s: %v", rel, err)
	}
	if !st.Mode().IsRegular() {
		return nil, refused(rel, "not_regular_file", "it is not a regular file")
	}
	if err := d.judgeOpen(f, rel, d.judge.OutboundPath); err != nil {
		return nil, err
	}
	// Read one byte past the limit: the size is judged on what is read, not on
	// the stat, which a file growing after it would outrun.
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot read %s: %v", rel, err)
	}
	if int64(len(b)) > max {
		return nil, tooLarge(rel, int64(len(b)), max)
	}
	return b, nil
}

// WriteFile places data at rel, a path relative to the work directory,
// creating its parent directories. It returns the path the kernel names for
// the written file.
func (d *Dir) WriteFile(rel string, data []byte) (string, error) {
	rel, err := local(rel)
	if err != nil {
		return "", err
	}
	if d.written+int64(len(data)) > d.maxOut {
		return "", toolerr.Newf(toolerr.CodeLimitExceeded, "the output would exceed %d bytes", d.maxOut).
			WithDetails(map[string]any{"limit": "output_bytes", "max": d.maxOut, "size": d.written + int64(len(data))})
	}
	dir := filepath.Dir(rel)
	if err := d.root.MkdirAll(dir, 0o755); err != nil {
		return "", openErr(dir, err)
	}
	tmp := filepath.Join(dir, "."+filepath.Base(rel)+"."+nonce()+".tmp")
	f, err := d.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", openErr(tmp, err)
	}
	placed := false
	defer func() {
		if !placed {
			_ = d.root.Remove(tmp)
		}
	}()
	kpath, err := descriptorPath(f)
	if err != nil {
		_ = f.Close()
		return "", toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot name %s: %v", rel, err)
	}
	// Judge the final name in the directory the kernel actually holds.
	kpath = filepath.Join(filepath.Dir(kpath), filepath.Base(rel))
	if reason, why := d.judge.LocalPath(rel, kpath); reason != "" {
		_ = f.Close()
		return "", refused(rel, reason, why)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot write %s: %v", rel, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot write %s: %v", rel, err)
	}
	if err := f.Close(); err != nil {
		return "", toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot write %s: %v", rel, err)
	}
	if err := d.root.Rename(tmp, rel); err != nil {
		return "", toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot place %s: %v", rel, err)
	}
	placed = true
	d.written += int64(len(data))
	return kpath, nil
}

// judgeOpen names the open file by the kernel's path and applies verdict.
func (d *Dir) judgeOpen(f *os.File, rel string, verdict func(raw, resolved string) (string, string)) error {
	kpath, err := descriptorPath(f)
	if err != nil {
		return toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot name %s: %v", rel, err)
	}
	if reason, why := verdict(rel, kpath); reason != "" {
		return refused(rel, reason, why)
	}
	return nil
}

// local cleans rel and refuses absolute paths and paths that climb out by
// their spelling. ".." is folded on the spelling, as a relative link in a
// document is resolved (a/../b is b even where a does not exist); what the
// kernel then resolves — links — is os.Root's to confine. Refusing a climb
// here, where os.Root would too, gives the caller the reason.
func local(rel string) (string, error) {
	if !filepath.IsLocal(rel) {
		return rel, refused(rel, "outside_work_dir", "it is not a path inside the work directory")
	}
	return filepath.Clean(rel), nil
}

// openErr maps an os.Root failure. A path that resolves outside the root (a
// link pointing out) is reported as such; os.Root's error for it is not
// exported, so it is recognised by its text, and a test pins that text.
func openErr(rel string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return toolerr.Newf(toolerr.CodeFileNotFound, "%s does not exist in the work directory", rel)
	case strings.Contains(err.Error(), "escapes from parent"):
		return refused(rel, "outside_work_dir", "it resolves to a place outside the work directory")
	default:
		return toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot open %s: %v", rel, err)
	}
}

func refused(rel, reason, why string) error {
	return toolerr.Newf(toolerr.CodePathNotAllowed, "%s is refused: %s", rel, why).
		WithDetails(map[string]any{"reason": reason, "path": rel})
}

func tooLarge(rel string, size, max int64) error {
	return toolerr.Newf(toolerr.CodeLimitExceeded, "%s is %d bytes, over the limit of %d", rel, size, max).
		WithDetails(map[string]any{"limit": "file_bytes", "max": max, "size": size, "path": rel})
}

// nonce names a temporary output file. A variable so a test can predict the
// name and plant something there.
var nonce = func() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// fGetPath is fcntl's F_GETPATH on Darwin (<sys/fcntl.h>).
const fGetPath = 50

// descriptorPath returns the path the kernel holds for an open file: links
// resolved, the volume's case.
func descriptorPath(f *os.File) (string, error) {
	var buf [1024]byte // MAXPATHLEN
	if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), fGetPath, uintptr(unsafe.Pointer(&buf[0]))); e != 0 {
		return "", fmt.Errorf("F_GETPATH: %w", e)
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n]), nil
}
