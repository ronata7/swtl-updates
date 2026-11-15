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
