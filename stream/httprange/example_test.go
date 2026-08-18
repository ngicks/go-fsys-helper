package httprange_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/ngicks/go-fsys-helper/stream/httprange"
)

// must is for the package-level values below, whose decoding cannot fail.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// exampleZipBytes is a small archive holding greeting.txt and data.bin, both
// stored uncompressed, embedded so the examples read known bytes.
var exampleZipBytes = must(hex.DecodeString(
	"504b0304140008000000000000000000000000000000000000000c0000006772" +
		"656574696e672e74787468656c6c6f2066726f6d207468652061726368697665" +
		"0a504b0708f2162e651700000017000000504b03041400080000000000000000" +
		"000000000000000000000008000000646174612e62696e303132333435363738" +
		"39504b0708c6c784a60a0000000a000000504b01021400140008000000000000" +
		"00f2162e6517000000170000000c000000000000000000000000000000000067" +
		"72656574696e672e747874504b0102140014000800000000000000c6c784a60a" +
		"0000000a000000080000000000000000000000000051000000646174612e6269" +
		"6e504b0506000000000200020070000000910000000000",
))

// ExampleNew_zipArchive reads one member out of a remote zip archive without
// downloading the rest of it. archive/zip starts at the central directory
// near the end of the file and jumps from there — access that is randomized
// from the very first read, which is what New is for: every ReadAt is a
// bounded range request of its own, fetching exactly the bytes asked for.
// The same member read from a local copy of the archive and through the
// reader prints identically. (A caller who will instead read one stretch
// front to back declares it with NewRange.)
func ExampleNew_zipArchive() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.ServeContent(
			w, req, "docs.zip",
			time.Date(2024, time.May, 6, 7, 8, 9, 0, time.UTC),
			bytes.NewReader(exampleZipBytes),
		)
	}))
	defer srv.Close()

	greeting := func(zr *zip.Reader) string {
		f, err := zr.Open("greeting.txt")
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			log.Fatal(err)
		}
		return string(b)
	}

	origin, err := zip.NewReader(
		bytes.NewReader(exampleZipBytes), int64(len(exampleZipBytes)),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	r, err := httprange.New(ctx, srv.URL+"/docs.zip", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	// zip.NewReader wants the archive's size, which construction did not
	// fetch: an explicit Probe settles it, and Metadata hands it back.
	if err := r.Probe(ctx); err != nil {
		log.Fatal(err)
	}
	m, _ := r.Metadata()
	remote, err := zip.NewReader(r, m.Size)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("origin:    %s", greeting(origin))
	fmt.Printf("httprange: %s", greeting(remote))
	// Output:
	// origin:    hello from the archive
	// httprange: hello from the archive
}

// randomBytes is 1 KiB nothing repeats over, embedded so the example
// downloads known bytes and its digests stay put.
var randomBytes = must(hex.DecodeString(
	"dc62c7be49d89e7d90ce7e0b08671d36dfc171361ca96edcff808d1e8ad2faf5" +
		"ba9555a2b11754da233857f59e08b61777fdf8dcb794170ad7a567c199eee3ac" +
		"cd674c82718003a78382fb030adad24f966607c83b5c8820d8a6106a89bcc024" +
		"64b9b236f471aa7b6ffbfa638a2aade88f3f1abde0ec51b1aea57db58feeaed6" +
		"50e53d7090fbab94d597200d1d5e8797d740d45e8c4ed5a81216ab1e4133eb1a" +
		"dbc2ee278498620b1bc14f4c2b53257407aa7c1cb03d52749413395380534f72" +
		"8063f38a9b1f37be7a9dc7a12dcbfc84ab5c4c5fa2a1b1cf3169e4ebb4e3390b" +
		"3d9c1cabb42a5dc912aa316f42f183373ba9153a8eb1dd46142fe5b9f12b49a5" +
		"f204beca951194f0abb8edb1e3f0aa455d61f30108b2c99b4fdfd2e4b6c76c60" +
		"b7e84843a1a3f88f29c4ac6d022c447dcc5a9376c0561bc42e5938807c6f24e1" +
		"918f4f3a76aab24e6d58b689831916584519c378de596582b0467b7e706613aa" +
		"0d81cdf769f7ca5907dcf784a4c57da474637c507490d3282c98850d66574aa5" +
		"db0548de30e19ef055f55788d8750786533ce445290a75d2dea928c17fed2aed" +
		"50eef4619ebe83642d987de27a8c7b52fb97dcc35d27e8afe30e3bd66a54fd15" +
		"b18c11889eda07407dd9870fc584416619824ae7523854ee707d8463bc13ca48" +
		"374db37b01d04c74864288b04bdc14c62b61e9dd4d2d89ba2afe1caaf61ae9d7" +
		"f75aa506fef3dc2dc750962870cd0ef4e18a8304e21cdc9aff9311388a138bf0" +
		"ae08c7ec7b8f863726ee2e77e56527bb8560034d46e711f2a39d61d6b40a4ada" +
		"600b8a8a31a66ed6e0d03236d0643f4c6b42c4db3b3356595c978f4f40a50662" +
		"368225deb81171f516b69944e4555a393ce643743f9cf6c448c4461608ef0d73" +
		"0e9d6506a59c5aa6b44da7525bc92640b58e6ea9102dc9cb1c3d5b9ec9713c28" +
		"e5c53739d34c48cf902f4e5addb4d3f8bcb6a4221ddcb0b60557d92767aca994" +
		"c502acb69663b0b5d246cab766a928a22a9da341be62cecb9ab9284206b2de66" +
		"661e5b23e54cbd11f1eaf12aceb15d0bfcd3a11a15367478eca77f36578bb20e" +
		"2e134e7b3e9b664bb6c68a76a75b8f804451337beaac7bd90e3b3d4134e1d2a0" +
		"14b01af3c47ee80c712fb3912faf6acf1bc15812adf090bea536e34839b1228e" +
		"87f9a3d167bb8b879f2add17969946bb389da9f5cc46356f88e0a05404c7c0b2" +
		"76746e533aba8905aa1a7ad484bd1f2bc58de9855b3d2c61b38546fb73ed9154" +
		"0e446dcc46284b7dda313cf098373526a9986cfbf4ce190c40aefa2cf74b1582" +
		"e91bd420e6aeee801907dedff389ad535f8303cb1764205599929e1f3cfcbaa8" +
		"a2b5e3726e5dcab2f5daf9c25677c25196772648547b56b6704762adffc2cd2a" +
		"92170cbe40051780f727f4fffbca21304443b6119cb0afe5d17e93e691389c10",
))

// ExampleNewRange_resume is the resume loop end to end. The first attempt
// streams the object front to back and is cut short; what it keeps is the
// bytes that landed and the reader's Metadata. The next attempt declares the
// remainder, hands the saved metadata back through Config, and probes first,
// so an object that changed in between fails right there — as
// ErrObjectChanged, before any byte moves — instead of splicing new bytes
// onto stale ones. The whole transfer costs three requests: the first
// attempt's stream, the probe, and the resumed stream.
func ExampleNewRange_resume() {
	content := randomBytes
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			requests.Add(1)
			w.Header().Set("ETag", `"v1"`)
			http.ServeContent(
				w, req, "object.bin",
				time.Date(2024, time.May, 6, 7, 8, 9, 0, time.UTC),
				bytes.NewReader(content),
			)
		},
	))
	defer srv.Close()
	ctx := context.Background()

	var local bytes.Buffer

	// The first attempt: one streaming request serves the in-order reads,
	// until the connection is lost — played here by simply stopping.
	first, err := httprange.NewRange(ctx, srv.URL+"/object.bin", 0, math.MaxInt64, nil)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := io.CopyN(
		&local,
		io.NewSectionReader(first, 0, math.MaxInt64),
		400,
	); err != nil {
		log.Fatal(err)
	}
	saved, _ := first.Metadata()
	first.Close()
	fmt.Printf(
		"interrupted at %d of %d bytes, %d request so far\n",
		local.Len(), len(content), requests.Load(),
	)

	// The next attempt: declare the remainder, hand the saved metadata back.
	rest, err := httprange.NewRange(
		ctx, srv.URL+"/object.bin", int64(local.Len()), math.MaxInt64,
		&httprange.Config{
			Size:         saved.Size,
			ETag:         saved.ETag,
			LastModified: saved.LastModified,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer rest.Close()
	if err := rest.Probe(ctx); err != nil {
		log.Fatal(err)
	}
	if _, err := io.Copy(&local, io.NewSectionReader(rest, 0, math.MaxInt64)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf(
		"resumed to %d of %d bytes, %d requests in all\n",
		local.Len(), len(content), requests.Load(),
	)
	fmt.Printf("origin:     sha256:%x\n", sha256.Sum256(content))
	fmt.Printf("downloaded: sha256:%x\n", sha256.Sum256(local.Bytes()))
	// Output:
	// interrupted at 400 of 1024 bytes, 1 request so far
	// resumed to 1024 of 1024 bytes, 3 requests in all
	// origin:     sha256:d72e110b9a3259b134791a609e2ee0520cbada1dbcf301f2b15f515554401725
	// downloaded: sha256:d72e110b9a3259b134791a609e2ee0520cbada1dbcf301f2b15f515554401725
}
