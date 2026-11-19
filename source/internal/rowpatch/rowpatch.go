// Package rowpatch makes and applies small patches between two versions of a tab-separated table file.
// Rows are matched by the text before the first tab (the ID), so adding, removing or changing a few rows
// gives a patch of a few KB. Apply always rebuilds the exact bytes the patch was made for; callers must
// still compare the result with the signed SHA-256 before trusting it.
//
// Patch text format (UTF-8, one operation per line, the whole text is usually gzip-compressed on disk):
//
//	SWTLROWPATCH1
//	=N      copy the next N lines of the old file
//	-N      skip (delete) the next N lines of the old file
//	+text   insert one line with this text
package rowpatch

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strconv"
)

const magic = "SWTLROWPATCH1"

func key(line []byte) string {
	if i := bytes.IndexByte(line, '\t'); i >= 0 {
		return string(line[:i])
	}
	return string(line)
}

// Make returns the patch text that turns old into new.
func Make(old, new []byte) []byte {
	oldL := bytes.Split(old, []byte("\n"))
	newL := bytes.Split(new, []byte("\n"))
	idx := make(map[string]int, len(oldL))
	for i, l := range oldL {
		k := key(l)
		if _, dup := idx[k]; dup {
			idx[k] = -1 // ambiguous ID: never match on it
		} else {
			idx[k] = i
		}
	}
	var out bytes.Buffer
	out.WriteString(magic + "\n")
	copyRun := 0
	flush := func() {
		if copyRun > 0 {
			fmt.Fprintf(&out, "=%d\n", copyRun)
			copyRun = 0
		}
	}
	cur := 0
	for _, nl := range newL {
		j, ok := idx[key(nl)]
		if ok && j >= cur {
			if j > cur {
				flush()
				fmt.Fprintf(&out, "-%d\n", j-cur)
			}
			if bytes.Equal(oldL[j], nl) {
				copyRun++
			} else {
				flush()
				out.WriteString("-1\n+")
				out.Write(nl)
				out.WriteByte('\n')
			}
			cur = j + 1
			continue
		}
		flush()
		out.WriteByte('+')
		out.Write(nl)
		out.WriteByte('\n')
	}
	flush()
	return out.Bytes()
}

// Apply rebuilds the new file from the old file and a patch made by Make.
func Apply(old, patch []byte) ([]byte, error) {
	if !bytes.HasPrefix(patch, []byte(magic+"\n")) {
		return nil, errors.New("not a row patch")
	}
	patch = patch[len(magic)+1:]
	oldL := bytes.Split(old, []byte("\n"))
	var out bytes.Buffer
	out.Grow(len(old) + len(patch))
	first := true
	emit := func(line []byte) {
		if !first {
			out.WriteByte('\n')
		}
		first = false
		out.Write(line)
	}
	cur := 0
	for len(patch) > 0 {
		end := bytes.IndexByte(patch, '\n')
		if end < 0 {
			return nil, errors.New("patch is cut off")
		}
		op := patch[:end]
		patch = patch[end+1:]
		if len(op) == 0 {
			return nil, errors.New("empty operation in patch")
		}
		switch op[0] {
		case '+':
			emit(op[1:])
		case '=', '-':
			n, err := strconv.Atoi(string(op[1:]))
			if err != nil || n < 0 || cur+n > len(oldL) {
				return nil, errors.New("patch does not fit the file")
			}
			if op[0] == '=' {
				for _, l := range oldL[cur : cur+n] {
					emit(l)
				}
			}
			cur += n
		default:
			return nil, fmt.Errorf("unknown operation %q", op[0])
		}
	}
	return out.Bytes(), nil
}

// Gzip and Gunzip wrap the patch text for storage; Gunzip refuses to expand beyond max bytes.
func Gzip(b []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Write(b)
	zw.Close()
	return buf.Bytes()
}

func Gunzip(b []byte, max int64) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	out, err := io.ReadAll(io.LimitReader(zr, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > max {
		return nil, errors.New("patch expands too much")
	}
	return out, nil
}
