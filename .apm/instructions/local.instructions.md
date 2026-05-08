---
description: "Basic instructions for the project"
applyTo: "*"
---

### General

A collection of Go modules that implement useful functions around \*os.Root, fs.FS, io.Reader/Writer, and etc

A mono-repo: the module top does not have anything.

### Structure overview

Each top level directory is separate Go module except for vroot-adapter.

```
.
├── aferofs             Deprecated; I'm moving away from afero.
├── fsutil              file system helper which can be used with vroot and others
│   ├── errdef          errdef defines syscall errors to use them where not defined
│   ├── internal
│   │   ├── bufpool     buffer pool
│   │   └── osfslite    os-fs for test
│   ├── pathutil        specialized strings.SplitSeq, etc
│   └── testhelper      test-helpers to create tidy acceptance tests in vroot
├── stream              helpers around
│   ├── internal
│   │   ├── serr        vendored github.com/ngicks/go-common/serr
│   │   └── testhelper
│   └── testdata
├── tarfs               Consumes tar archives as fs.FS
├── vroot               virtual-root: a file system abstraction lib built *os.Root in mind.
│   ├── acceptancetest  test for vroot.Fs and vroot.Root
│   ├── clock           mock timer; still prefer the timer-interface over sync-test
│   ├── internal
│   │   ├── openflag
│   │   └── paths
│   ├── memfs           in-mem fs. under the hood it is synthfs with default flags.
│   ├── osfs
│   ├── overlayfs       virtually overlaid fs
│   └── synthfs         synthetic fs; virtually mixes contents from other Fs impl
└── vroot-adapter
    └── sftpfs          sftpfs wrapped as vroot.Fs
```
