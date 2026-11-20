package main

import (
	"os"
	"path/filepath"
	"sort"
)

// keepNewest deletes all but the newest `keep` files matching pattern in dir (by modification time).
// keep <= 0 keeps everything. It returns how many files it removed.
func keepNewest(dir, pattern string, keep int) int {
	if keep <= 0 {
		return 0
	}
	files, _ := filepath.Glob(filepath.Join(dir, pattern))
	type fi struct {
		p string
		t int64
	}
	var l []fi
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			l = append(l, fi{f, st.ModTime().UnixNano()})
		}
	}
	if len(l) <= keep {
		return 0
	}
	sort.Slice(l, func(i, j int) bool { return l[i].t > l[j].t })
	n := 0
	for _, x := range l[keep:] {
		if os.Remove(x.p) == nil {
			n++
		}
	}
	return n
}

// cleanOld trims old logs, reports and cached schemas. Quiet; never touches anything but our own files.
func (a *App) cleanOld() {
	k := a.cfg.LogsKeep
	if k <= 0 {
		return
	}
	logs := a.logsDir()
	keepNewest(logs, "update_*.log", k)
	keepNewest(logs, "export_*.log", k)
	for _, p := range []string{"1_*.log", "2_*.log", "3_*.log", "4_*.log", "5_*.log", "6_*.log"} {
		keepNewest(logs, p, k)
	}
	rk := k / 6
	if rk < 3 {
		rk = 3
	}
	rep := filepath.Join(a.dir, "reports")
	keepNewest(rep, "report_*.zip", rk)
	keepNewest(rep, "report_*.zip.0*", rk*4)
	sk := k / 10
	if sk < 3 {
		sk = 3
	}
	keepNewest(filepath.Join(a.dir, "cache"), "schema_*.json", sk)
}
