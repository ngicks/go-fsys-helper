# IDEA — overlayfs rewrite #2 (fuse-overlayfs-style DataSource design)

> The idea phase was **deliberately skipped at the user's direction** ("Skip IDEA
> phase since this is clearly library and use case is very clear to me") — see
> DECISION.md D2. This file records only the user-stated intent so PLAN.md has a
> grounded source; it is not an elaborated idea document.

## What it should be (user's statement, paraphrased)

Replace `vroot/overlayfs` wholesale with a re-implementation modeled on
`github.com/containers/fuse-overlayfs`:

- Layers are called **DataSource**. Two constructors:
  1. from a plain `vroot.Fs` — the fs root itself is the content root;
  2. from a `vroot.Fs` with the **canonical dir structure** — `work/` and
     `merged/` sub-directories.
- The **top layer always has `work/` and `merged/`**. The default copy-up
  mechanism stages into `work/` and renames into `merged/`.
- The copy-up strategy stays **extensible** (pluggable policy).
- Copy-up **tries reflink first**; where not possible, falls back to plain
  `io.CopyBuffer`.
- The **metadata store uses a CGO-free sqlite3** implementation, with an
  **efficient trie format** for paths.
- There are **no whiteout files in any data source** — masking state lives only
  in the metadata store, never as files in a layer.
- (Follow-up, same day) `vroot` gains a go-billy-style **`Locker` extension
  interface** on files — `Lock(level)`/`Unlock()` with shared/exclusive
  levels, since POSIX and Windows both natively have them — documented with
  a warning that Lock may set the file to non-blocking mode; the sqlite VFS
  uses it.

## Primary use case (as understood)

A Go program embeds a union filesystem over arbitrary `vroot.Fs` backends
(osfs, memfs, synthfs, sftpfs, …): read-only lower layers, one writable top,
copy-on-write into the top's `merged/` tree, deletions recorded side-band in
sqlite. A stopped overlay's top directory (`work/` + `merged/` + its metadata
DB) is itself a canonical layer and can later be stacked as a lower of another
overlay.
