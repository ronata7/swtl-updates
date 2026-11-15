package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

// Built-in replacement for the external xor.exe (by Luigi Auriemma, aluigi.org) that the original
// script used. data12.v is a zip archive whose every byte is XORed with 0x55; XOR is its own
// inverse, so the same function decodes and encodes.

const dataKey = 0x55

// xorFile writes in XOR key to out. The output is created next to the final name and only renamed
// into place when it is complete and has the right size, so a failure never leaves a half file.
func xorFile(in, out string, key byte) error {
	src, err := os.Open(in)
	if err != nil {
		return err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		return fmt.Errorf("%s is empty", in)
	}
	tmp := out + ".xortmp"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := xorStream(dst, bufio.NewReaderSize(src, 1<<20), key)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != st.Size() {
		err = fmt.Errorf("copied %d of %d bytes", n, st.Size())
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("xor of %s failed: %v", in, err)
	}
	_ = os.Remove(out)
	return os.Rename(tmp, out)
}

func xorStream(w io.Writer, r io.Reader, key byte) (int64, error) {
	buf := make([]byte, 1<<20)
	var total int64
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			b := buf[:n]
			for i := range b {
				b[i] ^= key
			}
			if _, err := w.Write(b); err != nil {
				return total, err
			}
			total += int64(n)
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}
