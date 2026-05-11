package acceptancetest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
	"golang.org/x/sync/errgroup"
)

// TestRaceMethods exercises many different Fs methods concurrently against the same Fs.
//
// The point is to surface data races and lock contention bugs under -race. Each
// goroutine targets a distinct file or distinct directory subtree so that "lost update"
// style failures do not cause spurious errors. The only assertion is that no method
// returns an unexpected error type (errors that happen because a sibling goroutine
// deleted the target are tolerated).
func TestRaceMethods[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	// Each worker gets its own directory.
	const workers = 8
	for i := range workers {
		c.MkdirAll(fmt.Sprintf("w%d", i), 0o755)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	var ops atomic.Int64
	g, ctx := errgroup.WithContext(ctx)

	for i := range workers {
		dir := fmt.Sprintf("w%d", i)
		g.Go(func() error {
			counter := 0
			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}

				name := fmt.Sprintf("%s/f%d", dir, counter)
				counter++

				if err := vroot.WriteFile(fsys, name, []byte("body"), 0o644); err != nil {
					return fmt.Errorf("WriteFile %s: %w", name, err)
				}
				if _, err := fsys.Stat(name); err != nil {
					return fmt.Errorf("Stat %s: %w", name, err)
				}
				if _, err := fsys.Lstat(name); err != nil {
					return fmt.Errorf("Lstat %s: %w", name, err)
				}
				if err := fsys.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("Remove %s: %w", name, err)
				}
				ops.Add(1)
			}
		})
	}

	if err := g.Wait(); err != nil {
		t.Fatalf("race methods: %v", err)
	}
	t.Logf("race methods completed: ops=%d", ops.Load())
}
