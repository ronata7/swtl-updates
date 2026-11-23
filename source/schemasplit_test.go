package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitLocaleSchema(t *testing.T) {
	p := filepath.Join(t.TempDir(), "schema.json")
	_ = os.WriteFile(p, []byte(`[{"file":"tb_A.res","fields":[{"name":"Unkown1","type":"uint32"}]},{"file":"tb_B.res","fields":[]}]`), 0o644)
	if err := splitLocaleSchema(p, []string{"tb_A_ENG.res", "tb_A_TWN.res", "tb_B.res", "tb_Z_ENG.res"}); err != nil {
		t.Fatal(err)
	}
	m, err := loadResSchema(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"tb_a.res", "tb_a_eng.res", "tb_a_twn.res", "tb_b.res"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if _, ok := m["tb_z_eng.res"]; ok || len(m) != 4 {
		t.Errorf("unexpected entries: %d", len(m))
	}
}
