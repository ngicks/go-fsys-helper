package sftpfs_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/sftpfs"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestFs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pkg/sftp server-side helpers expect POSIX semantics")
	}
	// both posixRename={false,true} passes same test case
	// while it should diverge.
	// It's just because sftp server we use delegates rename to
	// os.Rename,
	// which means if we run this test on posix machines,
	// Rename always works live posix.
	for _, posixRename := range []bool{false, true} {
		t.Run(fmt.Sprintf("posixRename=%t", posixRename), func(t *testing.T) {
			s := acceptancetest.Setup[vroot.File, *sftpfs.SftpFs]{
				Make: func(t *testing.T, lines []string) *sftpfs.SftpFs {
					return newSetupFs(t, posixRename, lines)
				},
				Option: acceptancetest.Option{
					Os:       acceptancetest.OsUnix,
					ChownUid: os.Getuid(),
					ChownGid: os.Getgid(),
				},
			}
			acceptancetest.RunFs(t, s)
		})
	}
}

// TestErrorConvention covers plan 01 V6: sftpfs must honor the vroot error
// convention — path ops return *fs.PathError, link ops return *os.LinkError —
// each wrapping a cause that errors.Is can match against the fs sentinels, and
// the surfaced Path must echo the caller's name rather than the absolute base.
func TestErrorConvention(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pkg/sftp server-side helpers expect POSIX semantics")
	}
	dir := t.TempDir()
	client := startSftpServer(t)
	fsys := sftpfs.New(client, false, dir)

	t.Run("MkdirAll is idempotent", func(t *testing.T) {
		if err := fsys.MkdirAll("a/b/c", 0o755); err != nil {
			t.Fatalf("first MkdirAll: %v", err)
		}
		// A second MkdirAll on the existing tree must succeed (os.MkdirAll
		// semantics). oci-image-copy relies on this.
		if err := fsys.MkdirAll("a/b/c", 0o755); err != nil {
			t.Fatalf("idempotent MkdirAll on existing dir: %v", err)
		}
	})

	t.Run("Mkdir over existing dir is ErrExist via errors.Is", func(t *testing.T) {
		if err := fsys.MkdirAll("exists", 0o755); err != nil {
			t.Fatalf("setup MkdirAll: %v", err)
		}
		err := fsys.Mkdir("exists", 0o755)
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("Mkdir over existing dir: want errors.Is(fs.ErrExist), got %v", err)
		}
		assertPathError(t, err, "exists")
	})

	t.Run("Stat missing is ErrNotExist *fs.PathError", func(t *testing.T) {
		_, err := fsys.Stat("nope/missing.txt")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat missing: want errors.Is(fs.ErrNotExist), got %v", err)
		}
		assertPathError(t, err, "nope/missing.txt")
	})

	t.Run("Open missing is ErrNotExist *fs.PathError", func(t *testing.T) {
		_, err := fsys.Open("nope/missing.txt")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open missing: want errors.Is(fs.ErrNotExist), got %v", err)
		}
		assertPathError(t, err, "nope/missing.txt")
	})

	t.Run("Rename missing source is *os.LinkError", func(t *testing.T) {
		err := fsys.Rename("missing-src", "dst")
		var le *os.LinkError
		if !errors.As(err, &le) {
			t.Fatalf("Rename: want *os.LinkError, got %T: %v", err, err)
		}
		if le.Old != "missing-src" || le.New != "dst" {
			t.Errorf("LinkError fields: got Old=%q New=%q, want missing-src/dst", le.Old, le.New)
		}
		assertNoAbsLeak(t, le.Old)
		assertNoAbsLeak(t, le.New)
	})

	t.Run("Link missing source is *os.LinkError", func(t *testing.T) {
		err := fsys.Link("missing-src", "dst")
		var le *os.LinkError
		if !errors.As(err, &le) {
			t.Fatalf("Link: want *os.LinkError, got %T: %v", err, err)
		}
	})
}

// assertPathError checks err is a *fs.PathError whose Path echoes the caller's
// (relative) name and does not leak the absolute base.
func assertPathError(t *testing.T, err error, wantPath string) {
	t.Helper()
	var pe *fs.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("want *fs.PathError, got %T: %v", err, err)
	}
	if pe.Path != wantPath {
		t.Errorf("PathError.Path: got %q, want %q (must echo caller name, not abs base)", pe.Path, wantPath)
	}
	assertNoAbsLeak(t, pe.Path)
}

// assertNoAbsLeak fails when p is an absolute path (a leaked host-side base).
func assertNoAbsLeak(t *testing.T, p string) {
	t.Helper()
	if len(p) > 0 && p[0] == '/' {
		t.Errorf("error path %q is absolute; must not leak the sftp base", p)
	}
}

// newSetupFs returns a fresh SftpFs rooted at a per-test temp directory served
// by an in-process SFTP server. Both the setup helper writes and the test
// assertions go through the same SFTP boundary, so the acceptance suite
// exercises the wire protocol end-to-end.
func newSetupFs(t *testing.T, posixRename bool, lines []string) *sftpfs.SftpFs {
	t.Helper()
	dir := t.TempDir()
	client := startSftpServer(t)
	fsys := sftpfs.New(client, posixRename, dir)
	testhelper.New(t, fsys).SetupLines(lines...)
	return fsys
}

// startSftpServer starts an in-process SSH server that accepts an SFTP
// subsystem on a loopback TCP listener and returns a connected sftp.Client.
//
// Auth is disabled (NoClientAuth) and an ephemeral Ed25519 host key is
// generated per call; the listener, SSH server, and client are torn down via
// t.Cleanup. The server uses pkg/sftp's stock os-backed handler, so its file
// ops act on the real local filesystem.
func startSftpServer(t *testing.T) *sftp.Client {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("ssh.NewSignerFromKey: %v", err)
	}

	serverCfg := &ssh.ServerConfig{NoClientAuth: true}
	serverCfg.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	var (
		wg     sync.WaitGroup
		errsMu sync.Mutex
		errs   []error
	)
	record := func(err error) {
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return
		}
		errsMu.Lock()
		errs = append(errs, err)
		errsMu.Unlock()
	}

	wg.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				record(err)
				return
			}
			wg.Go(func() {
				serveSsh(conn, serverCfg, record)
			})
		}
	})

	clientCfg := &ssh.ClientConfig{
		User:            "anyone",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	sshClient, err := ssh.Dial("tcp", listener.Addr().String(), clientCfg)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("ssh.Dial: %v", err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		_ = listener.Close()
		t.Fatalf("sftp.NewClient: %v", err)
	}

	t.Cleanup(func() {
		_ = sftpClient.Close()
		_ = sshClient.Close()
		_ = listener.Close()
		wg.Wait()
		errsMu.Lock()
		defer errsMu.Unlock()
		for _, err := range errs {
			t.Logf("sftp server: %v", err)
		}
	})

	return sftpClient
}

// serveSsh runs the SSH server handshake on conn, then dispatches sftp
// subsystem requests to a per-session pkg/sftp server.
func serveSsh(conn net.Conn, cfg *ssh.ServerConfig, record func(error)) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		record(err)
		_ = conn.Close()
		return
	}
	defer func() { _ = sshConn.Close() }()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, chReqs, err := newChan.Accept()
		if err != nil {
			record(err)
			continue
		}
		go handleSftpSession(channel, chReqs, record)
	}
}

// handleSftpSession services one SSH session: it replies "ok" to
// subsystem=sftp, then runs an sftp.Server on the channel.
func handleSftpSession(channel ssh.Channel, reqs <-chan *ssh.Request, record func(error)) {
	go func() {
		for req := range reqs {
			ok := req.Type == "subsystem" &&
				len(req.Payload) >= 4 &&
				string(req.Payload[4:]) == "sftp"
			_ = req.Reply(ok, nil)
		}
	}()

	server, err := sftp.NewServer(channel)
	if err != nil {
		record(err)
		_ = channel.Close()
		return
	}
	if err := server.Serve(); err != nil {
		record(err)
	}
	_ = server.Close()
}
