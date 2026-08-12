package tempfile

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestChecksummedTempFile(t *testing.T) {
	w, err := NewChecksummed(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte("checksum"), checksumBlockSize*2/len("checksum")+17)
	if _, err = w.WriteString(string(want)); err != nil {
		t.Fatal(err)
	}
	r, err := w.Save()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	got, err := io.ReadAll(r.Read(0))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("checksummed temporary file data mismatch")
	}
}

func TestChecksummedTempFileDetectsCorruption(t *testing.T) {
	w, err := NewChecksummed(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{0x5a}, checksumBlockSize*2)
	if _, err = w.Write(want); err != nil {
		t.Fatal(err)
	}
	tempReader, err := w.Save()
	if err != nil {
		t.Fatal(err)
	}
	r := tempReader.(*fileReader)
	defer r.Close()

	f := r.file
	if w.needsCleanup {
		f, err = os.OpenFile(r.filename, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
	}
	if _, err = f.WriteAt([]byte{0xff}, int64(checksumBlockSize+1)); err != nil {
		t.Fatal(err)
	}

	reader := r.Read(0)
	firstBlock := make([]byte, checksumBlockSize)
	if _, err = io.ReadFull(reader, firstBlock); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBlock, want[:checksumBlockSize]) {
		t.Fatal("valid block data mismatch")
	}
	buf := make([]byte, 1)
	if n, err := reader.Read(buf); err == nil || n != 0 {
		t.Fatalf("corrupted block read = %d, %v; want 0, error", n, err)
	}
}
