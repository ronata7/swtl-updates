package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const zipPass = "fvf?GgD832d?1DSFes"

type ToolResult struct {
	Out  string
	Code int
}

func (r *Run) toolPath(rel ...string) string {
	return filepath.Join(append([]string{r.a.dir, "Tools"}, rel...)...)
}

// tool runs a third-party executable, logs its command line and output to the
// log file, and returns its exit code. err is only set when it could not start.
func (r *Run) tool(exe string, args []string, dir string, stdin io.Reader) (ToolResult, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	r.log.Raw("$ " + exe + " " + strings.Join(args, " "))
	err := cmd.Run()
	res := ToolResult{Out: buf.String()}
	if strings.TrimSpace(res.Out) != "" {
		r.log.Raw(res.Out)
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.Code = ee.ExitCode()
			r.log.Raw(fmt.Sprintf("(exit code %d)", res.Code))
			return res, nil
		}
		return res, err
	}
	return res, nil
}

// xor runs xor.exe. Its exit code was never checked by the old script, so it is
// not trusted here either: the output is verified by size instead.
func (r *Run) xor(in, out string) error {
	if _, err := r.tool(r.toolPath("xor.exe"), []string{in, out, "0x55"}, r.work, nil); err != nil {
		return err
	}
	a, err1 := fileSize(in)
	b, err2 := fileSize(out)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("xor produced no output for %s", filepath.Base(in))
	}
	if a != b || a == 0 {
		return fmt.Errorf("xor output size wrong for %s (%d -> %d bytes)", filepath.Base(in), a, b)
	}
	return nil
}

func (r *Run) sevenZ(args []string, dir string, stdin io.Reader) (ToolResult, error) {
	return r.tool(r.toolPath("7z.exe"), args, dir, stdin)
}

// isZipAfterXor checks that the first bytes of path, once XOR 0x55, are "PK\x03\x04".
func isXoredZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 4)
	if _, err := io.ReadFull(f, b); err != nil {
		return false
	}
	return b[0]^0x55 == 'P' && b[1]^0x55 == 'K' && b[2]^0x55 == 3 && b[3]^0x55 == 4
}
