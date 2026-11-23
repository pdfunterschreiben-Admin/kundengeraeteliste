package main

import (
	"fmt"
	"strings"
	"time"
)

// Befund: ein Hinweis aus der Listenpruefung
type Befund struct {
	Schwere  string `json:"schwere"` // fehler | hinweis
	Register int    `json:"register"`
	Zeile    string `json:"zeile"`
	Nummer   string `json:"nummer"`
	Typ      string `json:"typ"`
	SN       string `json:"sn"`
	Text     string `json:"text"`
}

// ListePruefen sucht typische Fehler: doppelte Geraete, fehlende Angaben, unplausible Daten.
func ListePruefen(l *Liste, heute time.Time) []Befund {
	var out []Befund
	heuteStr := heute.Format("2006-01-02")
	type ref struct {
		ri int
		z  *Zeile
	}
	paare := map[string][]ref{}
	for ri, r := range l.Register {
		add := func(z *Zeile, schwere, text string) {
			out = append(out, Befund{Schwere: schwere, Register: ri, Zeile: z.ID, Nummer: r.Wert(z, spNummer).Text(),
				Typ: r.Wert(z, "Typ").Text(), SN: r.Wert(z, spSN).Text(), Text: text})
		}
		doppeltSN := r.DoppelteSN()
		nummern := map[string][]*Zeile{}
		for _, z := range r.Zeilen {
			sn := strings.TrimSpace(r.Wert(z, spSN).Text())
			typ := strings.TrimSpace(r.Wert(z, "Typ").Text())
			nr := strings.TrimSpace(r.Wert(z, spNummer).Text())
			leer := true
			for i, w := range z.Werte {
				if i < len(r.Spalten) && !IstFormelSpalte(r.Spalten[i]) && !w.Leer() {
					leer = false
				}
			}
			if leer {
				continue
			}
			if sn == "" {
				add(z, "fehler", "Keine Seriennummer – daran wird das Gerät beim Abgleich erkannt.")
				continue
			}
			if typ == "" {
				add(z, "hinweis", "Kein Typ eingetragen.")
			}
			pk := Schluessel(sn) + "~" + strings.ToUpper(typ)
			paare[pk] = append(paare[pk], ref{ri, z})
			if nr != "" {
				nummern[strings.ToUpper(nr)] = append(nummern[strings.ToUpper(nr)], z)
			}
			if strings.TrimSpace(sn) != r.Wert(z, spSN).Text() {
				add(z, "hinweis", "Die Seriennummer hat Leerzeichen am Anfang oder Ende.")
			}
			if w := r.Wert(z, spLetzteUVV); w.T == "d" && w.V > heuteStr {
				add(z, "fehler", "Letzte UVV liegt in der Zukunft ("+protokollWert(spLetzteUVV, w)+").")
			} else if w.T == "d" && w.V < heute.AddDate(-6, 0, 0).Format("2006-01-02") {
				add(z, "hinweis", "Letzte UVV ist über 6 Jahre alt ("+protokollWert(spLetzteUVV, w)+") – Datum richtig?")
			}
			if w := r.Wert(z, spLetzteUVV); !w.Leer() && w.T != "d" {
				add(z, "fehler", "Letzte UVV ist kein gültiges Datum: „"+w.Text()+"“.")
			}
		}
		for k := range doppeltSN {
			if len(strings.Split(k, "~")) > 0 {
				// gleiche SN mit unterschiedlichem Typ: nur zur Information
				var zs []*Zeile
				for _, z := range r.Zeilen {
					if Schluessel(r.Wert(z, spSN).Text()) == k {
						zs = append(zs, z)
					}
				}
				typen := map[string]bool{}
				for _, z := range zs {
					typen[strings.ToUpper(strings.TrimSpace(r.Wert(z, "Typ").Text()))] = true
				}
				if len(typen) > 1 {
					for _, z := range zs {
						add(z, "hinweis", "Seriennummer gibt es mehrfach, aber mit unterschiedlichem Typ – wird beim Abgleich an SN und Typ erkannt (die Excel-Makros erkennen nur die SN).")
					}
				}
			}
		}
		for nr, zs := range nummern {
			if len(zs) > 1 {
				for _, z := range zs {
					add(z, "hinweis", fmt.Sprintf("Die Nummer %s steht %d-mal in diesem Register.", nr, len(zs)))
				}
			}
		}
	}
	for _, rs := range paare {
		if len(rs) > 1 {
			for _, x := range rs {
				r := l.Register[x.ri]
				ort := ""
				if len(l.Register) > 1 {
					ort = " (Register " + r.Name + ")"
				}
				out = append(out, Befund{Schwere: "fehler", Register: x.ri, Zeile: x.z.ID, Nummer: r.Wert(x.z, spNummer).Text(),
					Typ: r.Wert(x.z, "Typ").Text(), SN: r.Wert(x.z, spSN).Text(),
					Text: fmt.Sprintf("Seriennummer und Typ kommen %d-mal in der Liste vor%s – bitte zusammenführen oder den Typ korrigieren.", len(rs), ort)})
			}
		}
	}
	return out
}
