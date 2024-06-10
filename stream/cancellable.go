package stream

import (
	"context"
	"io"
)

type cancellable struct {
	ctx context.Context
	r   io.Reader
	err error
}

// NewCancellable combines ctx and r so that a returned Reader reads from r and also respects context cancellation.
//
// The returned Reader stores a first error encountered,
// including EOF and context cancellation.
// If any error has occurred, any subsequent Read calls always return same error.
//
// The context cancellation prevents afterwards Read calls from actually reading the underlying r.
// However that does not mean that it would unblock already blocking Read calls (e.g. reading sockets, terminals, etc.)
// If r is possible to block long and you wish to unblock it in that case,
// r itself must be cancellable by its own mean.
// You can not use signals to make it return EINTR,
// since the Go runtime installs signal handler with SA_RESTART to all signals available to the platform.
// Instead, for files, you can combine os.Pipe and platform specific poll functions,
// for example, epoll for Linux, kqueue for Mac OS.
//
// The returned Reader is not goroutine safe.
// Calling Read multiple times simultaneously may cause undefined behaviors.
func NewCancellable(ctx context.Context, r io.Reader) io.Reader {
	return &cancellable{
		ctx: ctx,
		r:   r,
	}
}

func (c *cancellable) Read(p []byte) (n int, err error) {
	if c.err != nil {
		return 0, c.err
	}
	if c.ctx.Err() != nil {
		c.err = c.ctx.Err()
		return 0, c.err
	}
	n, err = c.r.Read(p)
	if err != nil {
		c.err = err
	}
	return n, err
}
