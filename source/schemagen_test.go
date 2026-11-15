package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDumperToSchema(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "tb_Speech_PC_B.cs"), []byte(`namespace Common.Resources.Tables.Definitions
{
    public class tb_Speech_PC_B 
    {
        public uint Unk0 { get; set; }
        public byte Unk1 { get; set; }
        public string Unk2 { get; set; }
        public ushort Unk3 { get; set; }
        public short Unk4 { get; set; }
        public int Unk5 { get; set; }
        public float Unk6 { get; set; }
    }
}`), 0o644)
	es, err := dumperToSchema(d)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"uint32", "byte", "string", "uint16", "int16", "int32", "single"}
	if len(es) != 1 || es[0].File != "tb_Speech_PC_B.res" || len(es[0].Fields) != len(want) {
		t.Fatalf("%+v", es)
	}
	for i, f := range es[0].Fields {
		if f.Type != want[i] || f.Name != "Unkown"+string(rune('1'+i)) {
			t.Errorf("field %d: %+v", i, f)
		}
	}
	out := filepath.Join(d, "schema.json")
	if err := writeSchema(out, es); err != nil {
		t.Fatal(err)
	}
	m, err := loadResSchema(out)
	if err != nil || len(m["tb_speech_pc_b.res"]) != 7 {
		t.Fatalf("round trip failed: %v", err)
	}
}

func TestDumperUnknownType(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "a.cs"), []byte("public class a { public weird Unk0 { get; set; } }"), 0o644)
	if _, err := dumperToSchema(d); err == nil {
		t.Fatal("expected error")
	}
}

// Optional: compare with a real dump and the schema SWAdmin made from it.
// SWTL_TEST_DUMP=<dumper folder> SWTL_TEST_SCHEMA=<schema.json> go test -run RealDump
func TestRealDump(t *testing.T) {
	dump, sch := os.Getenv("SWTL_TEST_DUMP"), os.Getenv("SWTL_TEST_SCHEMA")
	if dump == "" || sch == "" {
		t.Skip("no real dump given")
	}
	es, err := dumperToSchema(dump)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "s.json")
	if err := writeSchema(out, es); err != nil {
		t.Fatal(err)
	}
	if ok, why := sameSchema(out, sch); !ok {
		t.Fatal(why)
	}
	t.Logf("%d tables identical to SWAdmin's schema", len(es))
}
