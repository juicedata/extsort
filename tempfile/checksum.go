package tempfile

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"
)

const checksumBlockSize = 1 << 16

var checksumTable = crc32.MakeTable(crc32.Castagnoli)

type checksumWriter struct {
	blockSize     int
	blockChecksum uint32
	checksums     []uint32
	buf           []byte
}

func (w *checksumWriter) write(dst *bufio.Writer, p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		blockRemaining := checksumBlockSize - w.blockSize
		if blockRemaining > len(p) {
			blockRemaining = len(p)
		}
		chunk := p[:blockRemaining]
		n, err := dst.Write(chunk)
		if err != nil {
			return 0, err
		}
		if n != len(chunk) {
			return 0, io.ErrShortWrite
		}
		w.blockChecksum = crc32.Update(w.blockChecksum, checksumTable, chunk)
		w.blockSize += n
		written += n
		if w.blockSize == checksumBlockSize {
			w.finishBlock()
		}
		p = p[n:]
	}
	return written, nil
}

func (w *checksumWriter) writeString(dst *bufio.Writer, s string) (int, error) {
	bufSize := min(len(s), checksumBlockSize)
	if cap(w.buf) < bufSize {
		w.buf = make([]byte, bufSize)
	} else {
		w.buf = w.buf[:bufSize]
	}
	written := 0
	for len(s) > 0 {
		n := copy(w.buf, s)
		m, err := w.write(dst, w.buf[:n])
		if err != nil {
			return 0, err
		}
		written += m
		s = s[m:]
	}
	return written, nil
}

func (w *checksumWriter) finishBlock() {
	if w.blockSize == 0 {
		return
	}
	w.checksums = append(w.checksums, w.blockChecksum)
	w.blockSize = 0
	w.blockChecksum = 0
}

func (w *checksumWriter) finishSection() []uint32 {
	w.finishBlock()
	checksums := w.checksums
	w.checksums = nil
	return checksums
}

func newChecksummedReader(reader io.Reader, section int, checksums []uint32, size int64) *bufio.Reader {
	return bufio.NewReaderSize(&checksummedReader{
		reader:    reader,
		section:   section,
		checksums: checksums,
		remaining: size,
	}, 16)
}

type checksummedReader struct {
	reader    io.Reader
	section   int
	checksums []uint32
	nextBlock int
	remaining int64
	data      []byte
	offset    int
}

func (r *checksummedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.offset == len(r.data) {
		if r.nextBlock == len(r.checksums) {
			return 0, io.EOF
		}
		blockSize := checksumBlockSize
		if int64(blockSize) > r.remaining {
			blockSize = int(r.remaining)
		}
		if cap(r.data) < blockSize {
			r.data = make([]byte, blockSize)
		} else {
			r.data = r.data[:blockSize]
		}
		if _, err := io.ReadFull(r.reader, r.data); err != nil {
			return 0, fmt.Errorf("read temporary section %d block %d: %w", r.section, r.nextBlock, err)
		}
		if crc32.Checksum(r.data, checksumTable) != r.checksums[r.nextBlock] {
			return 0, fmt.Errorf("temporary section %d block %d checksum mismatch", r.section, r.nextBlock)
		}
		r.nextBlock++
		r.remaining -= int64(blockSize)
		r.offset = 0
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
