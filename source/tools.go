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
	// the archive password is never written to the log (logs get sent to other people)
	mask := func(s string) string {
		if pw := r.password(); pw != "" {
			s = strings.ReplaceAll(s, pw, "***")
		}
		return s
	}
	r.log.Raw("$ " + mask(exe+" "+strings.Join(args, " ")))
	err := cmd.Run()
	res := ToolResult{Out: buf.String()}
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
	}
	if strings.TrimSpace(res.Out) != "" {
		r.log.Raw(shortenToolOutput(mask(res.Out), code))
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.Code = code
			r.log.Raw(fmt.Sprintf("(exit code %d)", res.Code))
			return res, nil
		}
		return res, err
	}
	return res, nil
}

// shortenToolOutput keeps long successful tool output (for example the list of 360 files that 7z prints)
// out of the log: only the first and last lines are kept. Failures are always logged in full.
func shortenToolOutput(s string, code int) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	if code != 0 || len(lines) <= 60 {
		return s
	}
	head := strings.Join(lines[:25], "\n")
	tail := strings.Join(lines[len(lines)-12:], "\n")
	return fmt.Sprintf("%s\n   ... (%d more lines left out of the log) ...\n%s", head, len(lines)-37, tail)
}

// xor decodes/encodes data12 with the built-in XOR (see xor.go). The result is checked by size.
// Set SWTL_XOR_CROSSCHECK=1 and keep Tools\\xor.exe to also compare the result with the original tool.
func (r *Run) xor(in, out string) error {
	if err := xorFile(in, out, dataKey); err != nil {
		return err
	}
	a, err1 := fileSize(in)
	b, err2 := fileSize(out)
	if err1 != nil || err2 != nil || a != b || a == 0 {
		return fmt.Errorf("xor output size wrong for %s (%d -> %d bytes)", filepath.Base(in), a, b)
	}
	if os.Getenv("SWTL_XOR_CROSSCHECK") == "1" && exists(r.toolPath("xor.exe")) {
		legacy := out + ".legacy"
		_, _ = r.tool(r.toolPath("xor.exe"), []string{in, legacy, "0x55"}, r.work, nil)
		h1, e1 := hashFile(out)
		h2, e2 := hashFile(legacy)
		_ = os.Remove(legacy)
		if e1 != nil || e2 != nil || h1 != h2 {
			return fmt.Errorf("built-in xor and xor.exe give DIFFERENT results for %s - please report this", filepath.Base(in))
		}
		fmt.Println("    (cross-check: identical to xor.exe)")
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
