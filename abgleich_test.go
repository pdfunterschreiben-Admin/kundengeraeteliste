package main

import (
	"os"
	"testing"
)

func claas(t *testing.T) *Liste {
	b, err := os.ReadFile("/home/claude/in/2d2212c7-Geraeteliste_Claas.xlsm")
	if err != nil {
		t.Skip("Beispieldatei fehlt")
	}
	m, err := MappeLesen(b)
	if err != nil { t.Fatal(err) }
	l, err := ListeAusMappe(m)
	if err != nil { t.Fatal(err) }
	TabellenPruefen(l)
	return l
}

func TestAbgleich(t *testing.T) {
	ich := claas(t)
	kollege := claas(t)
	r := kollege.Register[0]
	z := r.Zeilen[0] // SN 14215
	// Kollege aendert Betriebsstunden (neuer) und Vermerk (aelter als meine Aenderung)
	r.Zeilen[0].Werte[r.Spalte(spStunden)] = Wert{T: "s", V: "300h"}
	kollege.ZeitSetzen(r, z, spStunden, Jetzt())
	r.Zeilen[0].Werte[r.Spalte("Vermerk")] = Wert{T: "s", V: "alt vom Kollegen"}
	kollege.ZeitSetzen(r, z, "Vermerk", 1)
	ich.Register[0].Zeilen[0].Werte[r.Spalte("Vermerk")] = Wert{T: "s", V: "meins"}
	ich.ZeitSetzen(ich.Register[0], ich.Register[0].Zeilen[0], "Vermerk", Jetzt())
	// Kollege fuellt leeres Feld ohne Zeitstempel (Wartung) und legt neues Geraet an
	r.Zeilen[1].Werte[r.Spalte(spWartung)] = Wert{T: "d", V: "2026-12-01"}
	r.Zeilen = append(r.Zeilen, &Zeile{ID: "x", Werte: []Wert{{T: "s", V: "NEU-1"}, {T: "s", V: "8FBE15T"}, {T: "n", V: "99999"}, {}, {}, {T: "d", V: "2026-01-01"}, {}, {}, {}, {}}})
	// Rundreise ueber die Excel-Datei wie beim Mailversand
	x, err := ListeAlsXlsx(kollege, true)
	if err != nil { t.Fatal(err) }
	m, _ := MappeLesen(x)
	ein, err := ListeAusMappe(m)
	if err != nil { t.Fatal(err) }
	e := Abgleichen(ich, ein, "Test")
	t.Logf("%+v %s", e, e.Text())
	g := ich.Register[0]
	if v := g.Wert(g.Zeilen[0], spStunden).V; v != "300h" { t.Errorf("Stunden %q", v) }
	if v := g.Wert(g.Zeilen[0], "Vermerk").V; v != "meins" { t.Errorf("Vermerk %q", v) }
	if v := g.Wert(g.Zeilen[1], spWartung).V; v != "2026-12-01" { t.Errorf("Wartung %q", v) }
	if e.Neu != 1 || len(g.Zeilen) != 28 { t.Errorf("neu %d %d", e.Neu, len(g.Zeilen)) }
	// SN 10846 gibt es zweimal (9FBE20T und 9FB20T): die Daten duerfen nicht vermischt werden
	if e.Geraete != 2 || e.Felder != 2 { t.Errorf("doppelte SN vermischt: %+v", e) }
	for _, z := range g.Zeilen {
		if g.Wert(z, spSN).V == "10846" && g.Wert(z, "Typ").V == "9FBE20T" && !g.Wert(z, spStunden).Leer() { t.Errorf("Stunden des anderen Geraets uebernommen") }
	}
	// zweites Mal: keine Aenderungen
	e2 := Abgleichen(ich, ein, "Test")
	if e2.Felder != 0 || e2.Neu != 0 { t.Errorf("zweiter Abgleich %+v", e2) }
	for _, p := range ich.Protokoll { t.Logf("%+v", p) }
}

func TestEingabe(t *testing.T) {
	for _, c := range [][3]string{{"Letzte UVV", "06/2026", "d2026-06-01"}, {"Letzte UVV", "2.6.26", "d2026-06-02"}, {"Wartung", "fällig 1708h", "sfällig 1708h"}, {"Seriennummer", "14215", "n14215"}, {"Seriennummer", "0815", "s0815"}, {"Betriebsstunden", "1.319", "n1319"}, {"Betriebsstunden", "285h", "s285h"}, {"Nummer", "501", "s501"}} {
		w := EingabeDeuten(c[0], c[1])
		if w.T+w.V != c[2] { t.Errorf("%v -> %v", c, w) }
	}
}
