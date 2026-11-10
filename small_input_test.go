package extsort

import (
	"cmp"
	"context"
	"encoding/binary"
	"runtime"
	"slices"
	"testing"
	"time"
)

// TestSmallInputWithLargeChunkSize checks that the cost of sorting a few records
// does not depend on ChunkSize: no temporary file is created for a single chunk,
// the chunk buffer is not preallocated to ChunkSize, and building the chunk
// stops as soon as the input is closed.
func TestSmallInputWithLargeChunkSize(t *testing.T) {
	const chunkSize = 1 << 24
	var want []uint64
	input := make(chan uint64, 20)
	for i := uint64(20); i > 0; i-- {
		input <- i
		want = append(want, i)
	}
	close(input)
	slices.Sort(want)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()

	s, out, errc := Generic(input,
		func(b []byte) (uint64, error) { return binary.BigEndian.Uint64(b), nil },
		func(v uint64) ([]byte, error) { return binary.BigEndian.AppendUint64(nil, v), nil },
		cmp.Compare[uint64],
		&Config{ChunkSize: chunkSize, NumWorkers: 2, TempFilesDir: t.TempDir()})
	if s == nil {
		t.Fatalf("create sorter: %v", <-errc)
	}
	s.Sort(context.Background())
	var got []uint64
	for v := range out {
		got = append(got, v)
	}
	if err := <-errc; err != nil {
		t.Fatalf("sort: %v", err)
	}

	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s.tempWriter != nil {
		t.Fatalf("a temporary file is created for a single chunk")
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 1<<20 {
		t.Fatalf("sorting %d records allocated %d bytes", len(want), alloc)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("sorting %d records took %s", len(want), elapsed)
	}
}
