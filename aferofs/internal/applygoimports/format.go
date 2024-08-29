package applygoimports

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

func CheckGoimports() error {
	_, err := exec.LookPath("goimports")
	return err
}

type cmdPipeReader struct {
	cmd      *exec.Cmd
	pipe     io.Reader
	stderr   *bytes.Buffer
	waitOnce sync.Once
	err      error
}

func newCmdPipeReader(cmd *exec.Cmd, r io.Reader) (*cmdPipeReader, error) {
	stderr := new(bytes.Buffer)

	cmd.Stdin = r
	cmd.Stderr = stderr

	p, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return &cmdPipeReader{cmd: cmd, pipe: p, stderr: stderr}, nil
}

func (r *cmdPipeReader) Read(p []byte) (n int, err error) {
	return r.pipe.Read(p)
}

func (r *cmdPipeReader) Close() error {
	r.waitOnce.Do(func() {
		err := r.cmd.Wait()
		if err != nil {
			err = fmt.Errorf("%s failed: err = %w, msg = %s", r.cmd.Path, err, r.stderr.Bytes())
		}
		r.err = err
	})
	return r.err
}

func ApplyGoimportsPiped(ctx context.Context, r io.Reader) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, "goimports")
	return newCmdPipeReader(cmd, r)
}

func ApplyGoimports(ctx context.Context, r io.Reader) (*bytes.Buffer, error) {
	p, err := ApplyGoimportsPiped(ctx, r)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, p)
	cErr := p.Close()

	switch {
	case err != nil && cErr != nil:
		err = fmt.Errorf("copy err: %w, wait err: %w", err, cErr)
	case err != nil:
	case cErr != nil:
		err = cErr
	}

	return &buf, err
}
