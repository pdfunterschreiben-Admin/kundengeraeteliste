package main

import (
	"os"
	"testing"
	"time"
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

func TestAbgleichAuswahl(t *testing.T) {
	ich := claas(t)
	kollege := claas(t)
	r := kollege.Register[0]
	z := r.Zeilen[0]
	r.Zeilen[0].Werte[r.Spalte(spStunden)] = Wert{T: "s", V: "300h"}
	kollege.ZeitSetzen(r, z, spStunden, Jetzt())
	r.Zeilen = append(r.Zeilen, &Zeile{ID: "x", Werte: []Wert{{T: "s", V: "NEU-1"}, {T: "s", V: "8FBE15T"}, {T: "n", V: "99999"}, {}, {}, {T: "d", V: "2026-01-01"}, {}, {}, {}, {}}})
	x, _ := ListeAlsXlsx(kollege, true)
	m, _ := MappeLesen(x)
	ein, _ := ListeAusMappe(m)

	vorher := len(ich.Register[0].Zeilen)
	k, err := kopieren(ich)
	if err != nil { t.Fatal(err) }
	op := &AbgleichOpt{Praefix: "m0|"}
	Abgleichen(k, ein, "Kollege", op)
	if len(op.Liste) != 2 { t.Fatalf("Vorschau: %d Aenderungen", len(op.Liste)) }
	if len(ich.Register[0].Zeilen) != vorher { t.Fatalf("Vorschau hat die Liste veraendert") }

	// Stunden ablehnen, neues Geraet uebernehmen
	abg := map[string]bool{}
	for _, a := range op.Liste { if !a.NeuesGeraet { abg[a.ID] = true } }
	Abgleichen(ich, ein, "Kollege", &AbgleichOpt{Praefix: "m0|", Abgelehnt: abg})
	g := ich.Register[0]
	if len(g.Zeilen) != vorher+1 { t.Errorf("neues Geraet fehlt") }
	if v := g.Wert(g.Zeilen[0], spStunden).V; v == "300h" { t.Errorf("abgelehnte Aenderung uebernommen") }
	// beim naechsten Mal wird die abgelehnte Aenderung nicht erneut angeboten
	op2 := &AbgleichOpt{Praefix: "m0|"}
	Abgleichen(ich, ein, "Kollege", op2)
	if len(op2.Liste) != 0 { t.Errorf("abgelehnte Aenderung wieder angeboten: %+v", op2.Liste) }
}

func TestAdressen(t *testing.T) {
	a := Adressen("a@x.de; b@x.de,\nA@x.de ")
	if len(a) != 2 || a[1] != "b@x.de" { t.Errorf("%v", a) }
}

func TestPruefungUndVorschau(t *testing.T) {
	l := claas(t)
	b := ListePruefen(l, time.Now())
	for _, x := range b { t.Logf("%s %s %s: %s", x.Schwere, x.SN, x.Typ, x.Text) }
	if len(b) == 0 { t.Errorf("Claas-Liste hat doppelte SN, erwartet Hinweise") }
	k, _ := kopieren(l)
	r := k.Register[0]
	r.Zeilen = r.Zeilen[1:]
	r.Zeilen[0].Werte[r.Spalte(spStunden)] = Wert{T: "s", V: "1h"}
	v := Unterschiede(l, k)
	if v.Zurueck != 1 || v.Felder < 1 { t.Errorf("Vorschau %+v", v) }
}

func TestTypenschild(t *testing.T) {
	x := TypenschildAuswerten("TOYOTA Material Handling\nTyp / Type: 8FBMT30\nSerien-Nr. / Serial No.: 6905790\nBaujahr / Year: 2019\nNennlast 3000 kg")
	if x.Typ != "8FBMT30" || x.SN != "6905790" || x.Hersteller != "Toyota" || x.Baujahr != "2019" { t.Errorf("%+v", x) }
	y := TypenschildAuswerten("Linde\nType H25D-01\nSerial no. H2X393K01234\nYear 2015")
	if y.SN != "H2X393K01234" || y.Hersteller != "Linde" { t.Errorf("%+v", y) }
	t.Logf("%+v", y)
}

func TestReportJSONDateiname(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(d+"/B_S_HF_Nr404_RRE140H___7080018.json", []byte(`[{"Id":"x","Value":"15000"}]`), 0o644)
	l := &Liste{Register: []*Register{{Name: "Herford", Spalten: SpaltenStandard, Zeilen: []*Zeile{
		{ID: "a", Werte: []Wert{{T: "s", V: "404"}, {T: "s", V: "RRE140H"}, {T: "s", V: "7080018"}}},
		{ID: "b", Werte: []Wert{{T: "s", V: "1"}, {T: "s", V: "X"}, {T: "s", V: "15000"}}},
	}}}}
	r, err := ReportsSuchen(l, d)
	if err != nil || len(r) != 1 || len(r[0].Geraete) != 1 || r[0].Geraete[0].SN != "7080018" { t.Fatalf("%v %+v", err, r) }
}

func TestReportAuswerten(t *testing.T) {
	a := ReportAuswerten("B_S_HF_Nr404_RRE140H___7080018.json", []byte(`[{"Id":"x","Value":"15000"}]`))
	if a.Nummer != "404" || a.Typ != "RRE140H" || a.SN != "7080018" || a.Hersteller != "Toyota" { t.Errorf("%+v", a) }
	b := ReportAuswerten("Report_RRE160H2_2026-11-03.txt", []byte("TruckCom Report\nSerial No.: 6874482\nHours: 581\n"))
	if b.Typ != "RRE160H2" || b.SN != "6874482" || b.Stunden != "581" { t.Errorf("%+v", b) }
	c := ReportAuswerten("x.txt", []byte("Serie='9FBE16T-10301' Modell='X265'"))
	if c.SN != "10301" || c.Typ != "9FBE16T" { t.Errorf("%+v", c) }
	d := ReportAuswerten("TruckReport_14215_9FBM35T.txt", nil)
	if d.SN != "14215" || d.Typ != "9FBM35T" { t.Errorf("%+v", d) }
}
