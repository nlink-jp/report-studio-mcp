package fsio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nlink-jp/report-studio-mcp/internal/toolerr"
)

// stubJudge refuses any path whose resolved form contains "secret".
type stubJudge struct{ calls []string }

func (j *stubJudge) verdict(kind, raw, resolved string) (string, string) {
	j.calls = append(j.calls, kind+":"+resolved)
	if strings.Contains(resolved, "secret") {
		return "sensitive_path", "it is a secret"
	}
	return "", ""
}
func (j *stubJudge) LocalPath(raw, resolved string) (string, string) {
	return j.verdict("local", raw, resolved)
}
func (j *stubJudge) OutboundPath(raw, resolved string) (string, string) {
	return j.verdict("outbound", raw, resolved)
}

// realDir returns a fresh directory by its resolved path (/var → /private/var).
func realDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func open(t *testing.T, dir string, j Judge) *Dir {
	t.Helper()
	d, err := Open(dir, j, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantCode(t *testing.T, err error, code, reason string) {
	t.Helper()
	var te *toolerr.Error
	if !errors.As(err, &te) {
		t.Fatalf("error %v is not a toolerr", err)
	}
	if te.Code != code {
		t.Fatalf("code = %q, want %q (%v)", te.Code, code, err)
	}
	if reason != "" && te.Details["reason"] != reason {
		t.Fatalf("reason = %v, want %q (%v)", te.Details["reason"], reason, err)
	}
}

func TestReadInsideIncludingDotDotWithin(t *testing.T) {
	dir := realDir(t)
	write(t, filepath.Join(dir, "img", "a.png"), "png")
	j := &stubJudge{}
	d := open(t, dir, j)
	for _, rel := range []string{"img/a.png", "doc/../img/a.png"} {
		b, err := d.ReadFile(rel, 100)
		if err != nil || string(b) != "png" {
			t.Errorf("ReadFile(%q) = %q, %v", rel, b, err)
		}
	}
	// The verdict is asked with the kernel's path and the outbound policy.
	if len(j.calls) == 0 || j.calls[0] != "outbound:"+filepath.Join(dir, "img", "a.png") {
		t.Errorf("judge calls = %v", j.calls)
	}
}

func TestReadRefusesSpellingsOutside(t *testing.T) {
	d := open(t, realDir(t), &stubJudge{})
	for _, rel := range []string{"/etc/hosts", "../x", "a/../../x", ""} {
		_, err := d.ReadFile(rel, 100)
		wantCode(t, err, toolerr.CodePathNotAllowed, "outside_work_dir")
	}
}

// A link that points out of the work directory is refused by os.Root; the
// error text it is recognised by is pinned here.
func TestReadRefusesLinkOut(t *testing.T) {
	outside := realDir(t)
	write(t, filepath.Join(outside, "hosts"), "outside")
	dir := realDir(t)
	if err := os.Symlink(filepath.Join(outside, "hosts"), filepath.Join(dir, "abs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../"+filepath.Base(outside)+"/hosts", filepath.Join(dir, "rel")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	d := open(t, dir, &stubJudge{})
	for _, rel := range []string{"abs", "rel", "sub/hosts"} {
		_, err := d.ReadFile(rel, 100)
		wantCode(t, err, toolerr.CodePathNotAllowed, "outside_work_dir")
	}
}

func TestReadLinkWithinIsFollowed(t *testing.T) {
	dir := realDir(t)
	write(t, filepath.Join(dir, "real.png"), "ok")
	if err := os.Symlink("real.png", filepath.Join(dir, "alias.png")); err != nil {
		t.Fatal(err)
	}
	b, err := open(t, dir, &stubJudge{}).ReadFile("alias.png", 100)
	if err != nil || string(b) != "ok" {
		t.Fatalf("ReadFile(alias) = %q, %v", b, err)
	}
}

func TestReadRefusesFIFOWithoutBlocking(t *testing.T) {
	dir := realDir(t)
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := open(t, dir, &stubJudge{})
	done := make(chan error, 1)
	go func() { _, err := d.ReadFile("pipe", 100); done <- err }()
	select {
	case err := <-done:
		wantCode(t, err, toolerr.CodePathNotAllowed, "not_regular_file")
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFile blocked on a FIFO")
	}
}

func TestReadRefusesDirectoryMissingAndLarge(t *testing.T) {
	dir := realDir(t)
	write(t, filepath.Join(dir, "d", "x"), "x")
	write(t, filepath.Join(dir, "big"), strings.Repeat("a", 101))
	d := open(t, dir, &stubJudge{})
	_, err := d.ReadFile("d", 100)
	wantCode(t, err, toolerr.CodePathNotAllowed, "not_regular_file")
	_, err = d.ReadFile("nope", 100)
	wantCode(t, err, toolerr.CodeFileNotFound, "")
	_, err = d.ReadFile("big", 100)
	wantCode(t, err, toolerr.CodeLimitExceeded, "")
}

func TestReadRefusesWhatTheFloorRefuses(t *testing.T) {
	dir := realDir(t)
	write(t, filepath.Join(dir, "secret", "key.png"), "k")
	if err := os.Symlink("secret/key.png", filepath.Join(dir, "innocent.png")); err != nil {
		t.Fatal(err)
	}
	d := open(t, dir, &stubJudge{})
	// The innocent spelling is judged by where the kernel says it is.
	_, err := d.ReadFile("innocent.png", 100)
	wantCode(t, err, toolerr.CodePathNotAllowed, "sensitive_path")
}

func TestWritePlacesAndReplaces(t *testing.T) {
	dir := realDir(t)
	d := open(t, dir, &stubJudge{})
	for _, body := range []string{"one", "two"} {
		kp, err := d.WriteFile("ws/out/a/report.pdf", []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(dir, "ws", "out", "a", "report.pdf"); kp != want {
			t.Errorf("kernel path = %q, want %q", kp, want)
		}
		got, _ := os.ReadFile(filepath.Join(dir, "ws", "out", "a", "report.pdf"))
		if string(got) != body {
			t.Errorf("content = %q, want %q", got, body)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "ws", "out", "a"))
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

// A link planted at the output path is replaced, not followed: the file it
// points to is untouched.
func TestWriteReplacesPlantedLink(t *testing.T) {
	outside := realDir(t)
	victim := filepath.Join(outside, "victim")
	write(t, victim, "keep")
	dir := realDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, "out", "r.pdf")); err != nil {
		t.Fatal(err)
	}
	if _, err := open(t, dir, &stubJudge{}).WriteFile("out/r.pdf", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("victim was written through the link: %q", b)
	}
	st, err := os.Lstat(filepath.Join(dir, "out", "r.pdf"))
	if err != nil || !st.Mode().IsRegular() {
		t.Errorf("output is not a regular file: %v %v", st, err)
	}
}

// An output directory that is a link pointing out is refused.
func TestWriteRefusesLinkedDirectoryOut(t *testing.T) {
	outside := realDir(t)
	dir := realDir(t)
	if err := os.Symlink(outside, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	_, err := open(t, dir, &stubJudge{}).WriteFile("out/r.pdf", []byte("x"))
	wantCode(t, err, toolerr.CodePathNotAllowed, "outside_work_dir")
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("something was written outside: %v", entries)
	}
}

func TestWriteRefusedByTheFloorLeavesNothing(t *testing.T) {
	dir := realDir(t)
	d := open(t, dir, &stubJudge{})
	_, err := d.WriteFile("secret/r.pdf", []byte("x"))
	wantCode(t, err, toolerr.CodePathNotAllowed, "sensitive_path")
	if entries, _ := os.ReadDir(filepath.Join(dir, "secret")); len(entries) != 0 {
		t.Errorf("temporary file left behind: %v", entries)
	}
}

func TestWriteHonoursTheOutputLimit(t *testing.T) {
	dir := realDir(t)
	d, err := Open(dir, &stubJudge{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.WriteFile("a", []byte("123456")); err != nil {
		t.Fatal(err)
	}
	_, err = d.WriteFile("b", []byte("123456"))
	wantCode(t, err, toolerr.CodeLimitExceeded, "")
	if _, err := os.Stat(filepath.Join(dir, "b")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("b was written: %v", err)
	}
}

// The temporary name is created with O_EXCL: something planted under it — here
// a link to the manuscript — makes the write fail instead of writing through.
func TestWriteNeverOpensAPlantedTemporaryName(t *testing.T) {
	dir := realDir(t)
	write(t, filepath.Join(dir, "report.md"), "manuscript")
	old := nonce
	nonce = func() string { return "fixed" }
	defer func() { nonce = old }()
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../report.md", filepath.Join(dir, "out", ".r.pdf.fixed.tmp")); err != nil {
		t.Fatal(err)
	}
	_, err := open(t, dir, &stubJudge{}).WriteFile("out/r.pdf", []byte("PDF"))
	if err == nil {
		t.Fatal("WriteFile succeeded through a planted temporary name")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "report.md")); string(b) != "manuscript" {
		t.Errorf("manuscript was overwritten: %q", b)
	}
}
