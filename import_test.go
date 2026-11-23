package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportExport(t *testing.T) {
	files, _ := filepath.Glob("/home/claude/in/*.xlsm")
	if len(files) == 0 {
		t.Skip("Beispieldateien fehlen")
	}
	os.MkdirAll("/tmp/kglout", 0o755)
	for _, f := range files {
		b, _ := os.ReadFile(f)
		m, err := MappeLesen(b)
		if err != nil { t.Fatal(f, err) }
		l, err := ListeAusMappe(m)
		if err != nil { t.Fatal(f, err) }
		t.Logf("%s: titel=%q betreff=%q empf=%q id=%q zeit=%d prot=%d letzt=%v", filepath.Base(f), l.Titel, l.BetreffText(), l.Empfaenger, l.ListenID, len(l.Zeit), len(l.Protokoll), l.LetzterImport)
		for _, r := range l.Register {
			t.Logf("   reg %q tab %q spalten=%d zeilen=%d notiz=%q", r.Name, r.Tabelle, len(r.Spalten), len(r.Zeilen), r.Notiz)
			for i, z := range r.Zeilen { if i < 1 || len(z.Notizen) > 0 { t.Logf("     %v %v", z.Werte, z.Notizen) } }
		}
		x, err := ListeAlsXlsx(l, true)
		if err != nil { t.Fatal(err) }
		out := "/tmp/kglout/" + filepath.Base(f)[9:]
		out = out[:len(out)-1] + "x"
		os.WriteFile(out, x, 0o644)
		// Rundreise
		m2, err := MappeLesen(x)
		if err != nil { t.Fatal(err) }
		l2, err := ListeAusMappe(m2)
		if err != nil { t.Fatal(err) }
		if anzahlGeraete(l2) != anzahlGeraete(l) || len(l2.Zeit) != len(l.Zeit) { t.Errorf("Rundreise %s: %d/%d %d/%d", f, anzahlGeraete(l2), anzahlGeraete(l), len(l2.Zeit), len(l.Zeit)) }
		for ri, r := range l.Register { for zi, z := range r.Zeilen { for k, w := range z.Werte {
			w2 := l2.Register[ri].Zeilen[zi].Werte[k]
			if w.Text() != w2.Text() || (w.T != w2.T && !(r.Spalten[k]=="Nummer")) { t.Errorf("%s %s r%d %s: %v != %v", f, r.Name, zi, r.Spalten[k], w, w2) }
		}}}
	}
}
