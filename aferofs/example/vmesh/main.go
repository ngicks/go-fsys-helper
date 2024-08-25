package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"maps"
	"os"
	"strconv"

	"github.com/ngicks/go-fsys-helper/aferofs"
	"github.com/ngicks/go-fsys-helper/aferofs/clock"
	"github.com/ngicks/go-fsys-helper/aferofs/vmesh"
)

//go:embed data
var dataFsys embed.FS

var (
	// taken by split and sha256sum.
	// run `split -b 1024 archive` to split files.
	knownSha256Sum = map[string]string{
		"total":      "959a684d6e104edacb67511844dbba2b80673c7542d7b194d310db4dbb9b0621",
		"foo/arch_0": "ca0dbca1bee04a53a0f64c4b878fdd1cdc4ef496c813dbd63260701267357b0b",
		"foo/arch_1": "f5b57f729c4c20caa5978babd8b02aa020d9829ee5a43033b9d887b4b4d8b09b",
		"foo/arch_2": "5fd8a55d55d0581f81129d9d277f9aefe32d9b1f9a4dd2d941c36b5fa268ee72",
	}
)

func main() {
	clock := clock.RealWallClock()
	vmeshFs := vmesh.New(0, vmesh.NewMemFileDataAllocator(clock), vmesh.WithWallClock(clock))

	for i := range 3 {
		view, err := vmesh.NewFsLinkFileRangedView(dataFsys, "data/archive", int64(i*1024), 1024)
		if err != nil {
			panic(err)
		}
		err = vmeshFs.AddFile("foo/arch_"+strconv.FormatInt(int64(i), 10), view)
		if err != nil {
			panic(err)
		}
	}

	hashesTaken := hashFsys(&aferofs.IoFs{Fs: vmeshFs}, []string{"foo/arch_0", "foo/arch_1", "foo/arch_2"})

	f, err := vmeshFs.Create("foo/hashes.json")
	if err != nil {
		panic(err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "    ")
	err = enc.Encode(hashesTaken)
	_ = f.Close()
	if err != nil {
		panic(err)
	}

	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}
	fmt.Printf("tmp dir: %s\n", tmpDir)
	defer os.RemoveAll(tmpDir)

	err = os.CopyFS(tmpDir, &aferofs.IoFs{Fs: vmeshFs})
	if err != nil {
		panic(err)
	}

	dirFs := os.DirFS(tmpDir)

	var seen []string
	err = fs.WalkDir(dirFs, ".", func(path string, d fs.DirEntry, err error) error {
		if path == "." || err != nil {
			return err
		}
		seen = append(seen, path)
		return nil
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s: %#v\n", tmpDir, seen)

	hashesJson := map[string]string{}

	fsFile, err := dirFs.Open("foo/hashes.json")
	if err != nil {
		panic(err)
	}
	err = json.NewDecoder(fsFile).Decode(&hashesJson)
	_ = fsFile.Close()
	if err != nil {
		panic(err)
	}

	hashesTakenCopied := hashFsys(dirFs, []string{"foo/arch_0", "foo/arch_1", "foo/arch_2"})

	fmt.Printf("sum map: %#v\n", hashesJson)
	fmt.Printf("known hashes == sum taken form copied fsys: %t\n", maps.Equal(knownSha256Sum, hashesTakenCopied))
	fmt.Printf("known hashes == copied hashes.json: %t\n", maps.Equal(knownSha256Sum, hashesJson))
}

func hashFsys(fsys fs.FS, paths []string) map[string]string {
	hashes := map[string]string{}
	totalCopied := sha256.New()
	for _, s := range paths {
		hashes[s] = sha256Sum(fsys, s, totalCopied)
	}
	hashes["total"] = hex.EncodeToString(totalCopied.Sum(nil))
	return hashes
}

func sha256Sum(fsys fs.FS, path string, total hash.Hash) string {
	f, err := fsys.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	h := sha256.New()
	w := io.MultiWriter(h, total)
	_, err = io.Copy(w, f)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
