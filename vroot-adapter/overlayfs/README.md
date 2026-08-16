# overlayfs

[![Go Reference](https://pkg.go.dev/badge/github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs.svg)](https://pkg.go.dev/github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs)

A union mount over [`vroot`](https://github.com/ngicks/go-fsys-helper/tree/main/vroot)
backends: a writable **top** layer stacked over read-only **lower** layers,
exposed as a `vroot.Root[vroot.File, *overlayfs.Fs]` — the full `vroot.Fs`
suite plus `OpenRoot` and rooted confinement.

Reads resolve in mount order; writes land in the top after the lower entry is
**copied up**; deletions become **whiteouts** instead of touching the read-only
lowers. The design follows
[containers/fuse-overlayfs](https://github.com/containers/fuse-overlayfs) —
mount-order lowers, opaque-on-mkdir, `EXDEV` on the dir renames it refuses —
with one deliberate departure:

> **Masking is never a file.** Neither kernel overlayfs' character-device
> whiteouts nor fuse-overlayfs' `.wh.*` marker files appear inside any layer. A
> layer's whiteouts and opaque directories live in a sqlite database beside the
> layer's content, so a layer's tree contains only the files the caller put
> there.

Because layers are `vroot.Fs` values rather than directories, an overlay can
stack anything vroot implements — `osfs`, `memfs`, an sftp mount, a read-only
wrapper — and mix them freely in one stack. For layers on the real filesystem,
prefer the `os.Root`-backed `osfs.NewRoot` over `osfs.NewFs`: besides confining
traversal, its handles are opened with delete sharing on Windows, so files the
overlay still holds open stay renamable and removable there.

## Installation

```bash
go get github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs
```

## Quick start

```go
package main

import (
	"fmt"
	"log"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func main() {
	// The top's directory must already exist; "merged/", "work/" and
	// "meta.sqlite3" are created inside it if they are not there yet.
	topDir, err := osfs.NewRoot("/srv/overlay/top")
	if err != nil {
		log.Fatal(err)
	}
	top, err := overlayfs.NewDataSourceCanonical(topDir)
	if err != nil {
		log.Fatal(err)
	}

	// A plain data source is the directory as it stands: content at its root,
	// no masking state of its own. Lower-only.
	lowerDir, err := osfs.NewRoot("/srv/overlay/base")
	if err != nil {
		log.Fatal(err)
	}
	lower := overlayfs.NewDataSource(lowerDir)

	ovl, err := overlayfs.New(top, []*overlayfs.DataSource{lower}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ovl.Close() // closes every layer, and with it every layer's store

	// Read: top is asked first, then the lower. This one is only in the lower.
	b, err := vroot.ReadFile(ovl, "config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s", b)

	// Write: the lower's file is copied up into the top first and written
	// there. /srv/overlay/base/config.yaml is left untouched.
	err = vroot.WriteFile(ovl, "config.yaml", []byte("from top\n"), 0o666)
	if err != nil {
		log.Fatal(err)
	}

	// Remove: records a whiteout, because the lower still shows the name.
	// Nothing is deleted from /srv/overlay/base.
	if err := ovl.Remove("motd"); err != nil {
		log.Fatal(err)
	}
}
```

## Data sources

A `DataSource` is one layer: a `vroot.Fs` together with the masking state that
belongs to it. There are two kinds.

| kind | content root | masking state | usable as |
| --- | --- | --- | --- |
| **plain** — `NewDataSource` | the fs root | none | lower only |
| **canonical** — `NewDataSourceCanonical` | `merged/` | its own store | top or lower |
| **canonical** — `NewDataSourceCanonicalStore` | `merged/` | the store you pass | top or lower |

A plain data source is an ordinary directory taken as-is. It has nowhere to put
whiteouts, so `New` refuses one in the top slot with `ErrNotCanonical`.

A canonical data source carries the overlay's layout, all of it directly inside
the fs root:

```
/srv/overlay/top/
├── merged/                content root — what the overlay reads and writes
├── work/                  copy-up staging; overlay-private, swept at New
├── meta.sqlite3           this layer's whiteouts and opaque directories
└── meta.sqlite3-journal   sqlite's rollback journal (see "Things to know")
```

`New` also refuses a canonical top whose store could only be opened for reading
— `ErrTopReadOnly`, distinct from `ErrNotCanonical`, because the layout is
fine and only the writability is not.

A `DataSource` takes over what it is handed: `Close` closes the store **and**
the fs under it. A `*DataSource` belongs to one overlay and must not be shared
with a second: an overlay copies the layer's masking into memory as it is built
and never re-reads it, so two overlays over one `*DataSource` drift apart, and
whichever closes first closes the store and the fs out from under the other.

To stack one layer under two overlays, build a `DataSource` per overlay, each
over its own handle on the layer — a plain one, or a canonical one over a
`vroot.ReadOnlyFs`, whose store opens read-only and takes no lock. What does not
work is a second writable canonical data source over the same directory: it
blocks rather than fails for as long as the first holds the store's exclusive
lock.

### Restacking a stopped overlay as a lower

Because a canonical layer keeps its own masking, the top an overlay wrote is a
complete layer once that overlay is closed — hand it to the next `New` as a
lower and its copies, whiteouts and opaque directories go on masking whatever
is stacked below it, with nothing written back into it. This is exercised end
to end in `stacking_test.go`, including a restack over a `vroot.ReadOnlyFs`,
where the store falls back to a read-only open and masks identically.

## Mount order

`lowers` is given in mount order, the way `lowerdir=a:b:c` reads: `lowers[0]`
is the **highest-priority** lower. A name resolves

```
top  →  lowers[0]  →  lowers[1]  →  …
```

to the shallowest layer that shows it and is not masked for it. Reads are
served from wherever the name resolved, and a handle onto a lower is read-only
— writing through it would be the one route around the copy-up that every write
otherwise passes through.

A canonical lower's whiteouts mask only the layers **below that lower**, never
the top or a shallower lower. Masking keeps per-layer provenance rather than
collapsing into one flat set.

## Masking and the metadata store

`MetadataStore` is a public interface: `Load`, `SetWhiteout`, `ClearWhiteout`,
`SetOpaque`, `ClearOpaque`, `Close`. It is never on a lookup path — the overlay
calls `Load` once at `New` and answers every query from memory afterwards — but
**every setter is durable by the time it returns**, so there is no flush step
and no `Sync` on the overlay.

The default implementation, `MetadataStoreSQLite`, is a sqlite database reached
through a VFS built on the layer's own `vroot.Fs` rather than on OS paths, so
even a fully virtual layer (a `memfs`) keeps durable masking state. Schema v1
is a single `STRICT, WITHOUT ROWID` `node` table keyed by the full clean slash
path — the table is therefore a clustered B+Tree over paths, and a subtree is
one contiguous range scan — plus a `parent` adjacency index for direct-children
queries.

Swap it out with `NewDataSourceCanonicalStore(fsys, store)`, which builds a
canonical layer around a store you supply and never opens `meta.sqlite3`. The
contract a replacement must satisfy is spelled out on the interface and
executable as `acceptancetest.RunMetadataStore`.

Two masking marks exist:

- **whiteout** — the name, and everything the layers below show under it, is
  hidden. Recorded by `Remove`/`RemoveAll` when a lower still shows the name.
- **opaque** — the layers below stop contributing children to this directory.
  `Mkdir` and `MkdirAll` mark every directory they create opaque, following the
  reference's `ovl_mkdir(set_opaque=true)`. That is what makes `rm -rf dir &&
  mkdir dir` over lower content leave the old lower children hidden, with no
  separate rule for recreating over a whiteout. Directories a copy-up
  materializes in passing are *not* opaque — they were not created for their
  own sake, and hiding their lower siblings would be a surprise.

## Copy-up

Any open asking for more than a read, and every metadata write, goes through
the copy-up gate first: the file is brought into the top, or created there, and
the handle is onto the top's copy.

The default policy, `CopyUpPolicyWork`, stages a regular file as a temp under
the top's `work/`, fills it, applies mode and times, and publishes it by
renaming it onto its content path — atomic, since staging and content are two
paths in one fs. Directories and symlinks have no data to stage and are created
in the content tree directly; only the directory itself is copied, so its
children keep merging in from below and a copy-up never walks a whole subtree
to write one file in it.

Filling a regular file cascades:

1. **reflink** — on Linux, `FICLONE` via `unix.IoctlFileClone`, attempted only
   when both files are os-backed (`vroot.File.Fd()` returns something other
   than the `^uintptr(0)` sentinel). Non-Linux builds have no clone wired up;
   APFS `clonefile` and ReFS block cloning are reachable through no interface
   vroot exposes.
2. **`io.CopyBuffer`** — with a pooled 64 KiB buffer, for everything reflink
   did not take.

Every reflink failure is one answer, and none of them is remembered: whether
the ioctl said the filesystem has no reflinks, that the two files are on
different ones, or that this pair could not be cloned, the next pair is asked
again. The reference latches a global "no reflink here" flag on the first
refusal; one vroot overlay can span filesystems within a single top, so a latch
would answer for a pair it never tried.

`CopyUpPolicy` is a public extension point. A policy is handed a content-rooted
view of the source layer and the top data source, owns neither, and runs under
the overlay's write lock, so it never locks. The top is addressed through
`DataSource.Fsys`, `DataSource.ContentPath` and `DataSource.StagingPath` — the
three the default policy itself uses, so a policy written outside this package
reaches exactly as far.

## Things to know

- **A directory rename that a lower still contributes to fails with `EXDEV`.**
  The reference answers `EXDEV` for exactly this, because moving such a
  directory means either copying its whole lower subtree up or leaving a
  redirect behind for the old path, and it does neither. Nor does this: a
  rename that quietly deep-copied a lower tree would turn a metadata operation
  into an unbounded one, and a redirect is a second masking concept the store
  has no room for. A directory the top holds alone moves as the single rename
  it looks like. On plan 9, which numbers no `EXDEV`, a stand-in wrapping
  `syscall.EINVAL` carries the same message.

- **`Option.DisableOpenFileRemoval` refuses to remove a name the overlay still
  holds a handle on**, the way Windows refuses to unlink an open file.
  `RemoveAll` refuses when the handle is on the named path or anywhere under
  it, before anything is dropped. Setting it on every platform is what makes an
  overlay behave identically everywhere.
  The error is the real `ERROR_SHARING_VIOLATION` on Windows and a
  `syscall.EINVAL` wrapped in the same message elsewhere, so `errors.Is(err,
  syscall.EINVAL)` still matches off Windows.

- **`ErrTypeNotSupported` for special files.** A device, socket or named pipe
  can be read where it lies but never copied up: `vroot.Fs` has no method that
  creates one. The type is refused before anything is created, so a failed
  copy-up leaves the top exactly as it found it.

- **`meta.sqlite3-journal` sits beside the database permanently, its header
  zeroed.** The store runs `locking_mode=EXCLUSIVE` over a rollback journal —
  the overlay is the database's only user, and no journal mode needing shared
  memory can work over a VFS that has none. Under exclusive locking sqlite
  zeroes the journal header after a commit instead of deleting the file, and
  leaves whatever is past the header as it lies. A persistent
  `meta.sqlite3-journal` whose header is zeroed is normal and is not a hot
  journal.

- **No `Sync`.** Masking is durable by the time the call that changed it
  returns, so there is nothing to flush at a checkpoint.

- **`work/` is overlay-private** and emptied at `New` of whatever a previous run
  staged and never published.

- **`OpenRoot` returns a sub-overlay** that shares the layers, the masking state
  and the lock with the overlay it came from rather than stacking anything of
  its own. Its `Close` is a no-op — only the overlay `New` built owns the
  layers.

- **Concurrency** is one coarse `RWMutex` over the whole overlay state, held for
  each operation's entire run, so a lookup and whatever it decides land as one
  step. Data I/O on a handle the overlay already returned takes no lock.

## Testing

```bash
go test ./...
go test -race ./...
```

The suite runs `vroot/acceptancetest`'s `RunRoot` and `RunFs` over stacks built
on both `memfs` and `osfs`, plus overlay-specific tests for copy-up, whiteout,
opaque-on-mkdir, cross-layer symlinks, merged listings, canonical-lower
restacking, and `fstest.TestFS` through `vroot.ToIoFs`.

To check a `MetadataStore` of your own against the interface's contract:

```go
import "github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs/acceptancetest"

func TestMyStore(t *testing.T) {
	acceptancetest.RunMetadataStore(t, newBacking)
}
```
