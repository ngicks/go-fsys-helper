package stream

import (
	"fmt"
	"testing"
)

func BenchmarkMultiReadAtSeekCloser_ReadAt_lookup_worst(b *testing.B) {
	oldThreshold := searchThreshold
	for _, threshold := range []int{0, 1024} {
		for _, split := range []int{4, 8, 16, 32, 64, 128, 256} {
			// This bench is not allowed to be parallel, but anyway.
			threshold := threshold
			split := split
			searchThreshold = threshold
			b.Run(fmt.Sprintf("%d_readers,binary_search_threshold_is_%d", split, searchThreshold), func(b *testing.B) {
				r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes32KiB, []int{(32 * 1024) / split}, false))

				buf := make([]byte, 128)
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					_, _ = r.ReadAt(buf, int64(len(randomBytes32KiB))-200)
				}
			})
		}
	}
	searchThreshold = oldThreshold
}
