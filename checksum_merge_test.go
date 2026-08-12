package extsort

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"io"
	"slices"
	"testing"
)

type checksumFailureReader struct {
	data []byte
	err  error
}

func (r *checksumFailureReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

type checksumFailureTempReader struct {
	readers []*bufio.Reader
}

func (r *checksumFailureTempReader) Close() error {
	return nil
}

func (r *checksumFailureTempReader) Size() int {
	return len(r.readers)
}

func (r *checksumFailureTempReader) Read(i int) *bufio.Reader {
	return r.readers[i]
}

func TestChecksumFailureStopsMerge(t *testing.T) {
	for _, test := range []struct {
		name          string
		numWorkers    int
		bufferSize    int
		secondSection []byte
		want          []int
		exact         bool
	}{
		{
			name:          "single threaded",
			numWorkers:    2,
			bufferSize:    10,
			secondSection: []byte{1, 2, 1, 4},
			want:          []int{1, 2, 3},
			exact:         true,
		},
		{
			name:          "parallel",
			numWorkers:    1,
			secondSection: []byte{1, 2, 1, 4},
			want:          []int{1, 2, 3},
		},
		{name: "first block/single threaded", numWorkers: 2, bufferSize: 10, exact: true},
		{name: "first block/parallel", numWorkers: 1, exact: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			checksumErr := errors.New("temporary section 1 block 0 checksum mismatch")
			sorter := newSorter[int](
				nil,
				func(data []byte) (int, error) { return int(data[0]), nil },
				func(value int) ([]byte, error) { return []byte{byte(value)}, nil },
				cmp.Compare[int],
				&Config{ChunkSize: 2, NumWorkers: test.numWorkers, SortedChanBuffSize: test.bufferSize},
			)
			sorter.tempReader = &checksumFailureTempReader{readers: []*bufio.Reader{
				bufio.NewReader(&checksumFailureReader{data: []byte{1, 1, 1, 3, 1, 5}, err: io.EOF}),
				bufio.NewReader(&checksumFailureReader{data: test.secondSection, err: checksumErr}),
			}}

			go sorter.mergeNChunks(context.Background())

			var got []int
			for value := range sorter.mergeChunkChan {
				got = append(got, value)
			}
			if len(got) > len(test.want) || !slices.Equal(got, test.want[:len(got)]) {
				t.Fatalf("output before checksum failure = %v, want a prefix of %v", got, test.want)
			}
			if test.exact && !slices.Equal(got, test.want) {
				t.Fatalf("output before checksum failure = %v, want %v", got, test.want)
			}
			if err := <-sorter.mergeErrChan; !errors.Is(err, checksumErr) {
				t.Fatalf("merge error = %v, want %v", err, checksumErr)
			}
		})
	}
}
