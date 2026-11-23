package main

import (
	"fmt"
	"strings"
	"time"
)

type AbgleichErgebnis struct {
	Geraete int `json:"geraete"` // aktualisierte Geraete
	Felder  int `json:"felder"`
	Neu     int `json:"neu"`
	Tabellen int `json:"tabellen"`
}

func (e AbgleichErgebnis) Text() string {
	if e.Geraete == 0 && e.Neu == 0 {
		return "Empfangene Liste geprüft: keine Unterschiede zu deiner Liste."
	}
	return fmt.Sprintf("Empfangene Liste übernommen: %d Gerät(e) aktualisiert (%d Feld(er) geändert), %d neue(s) Gerät(e) hinzugefügt. Details im Sync-Protokoll.", e.Geraete, e.Felder, e.Neu)
}

func (e *AbgleichErgebnis) Add(o AbgleichErgebnis) {
	e.Geraete += o.Geraete
	e.Felder += o.Felder
	e.Neu += o.Neu
	e.Tabellen += o.Tabellen
}

// Eine einzelne Aenderung aus einer empfangenen Liste (fuer die Vorschau)
type Aenderung struct {
	ID       string `json:"id"`
	Von      string `json:"von"`
	Register string `json:"register,omitempty"`
	Nummer   string `json:"nummer"`
	Typ      string `json:"typ"`
	SN       string `json:"sn"`
	Spalte   string `json:"spalte"`
	Alt      string `json:"alt"`
	Neu      string `json:"neu"`
	NeuesGeraet bool `json:"neuesGeraet,omitempty"`
}

// Optionen fuer Vorschau und Auswahl beim Abgleich
type AbgleichOpt struct {
	Praefix   string          // macht die IDs je empfangener Liste eindeutig
	Abgelehnt map[string]bool // diese Aenderungen nicht uebernehmen
	Liste     []Aenderung     // Ausgabe: alle Aenderungen, die uebernommen wuerden
}

func (o *AbgleichOpt) id(tabelle, key, spalte string) string {
	return o.Praefix + tabelle + "|" + key + "|" + strings.ToUpper(spalte)
}

func protokollWert(spalte string, w Wert) string {
	if w.T == "d" {
		if t, err := time.Parse("2006-01-02", w.V); err == nil {
			return t.Format("02.01.2006")
		}
	}
	return Anzeige(spalte, w)
}

// Abgleich wie in den Excel-Makros:
//   - Geraete werden ueber die Seriennummer erkannt
//   - ein Feld wird nur uebernommen, wenn die empfangene Aenderung neuer ist;
//     ohne Zeitangabe wird nur ein leeres Feld gefuellt, nie etwas ueberschrieben
//   - neue Geraete werden unten angehaengt, geloeschte nicht uebertragen
func Abgleichen(l *Liste, ein *Liste, herkunft string, opts ...*AbgleichOpt) AbgleichErgebnis {
	var opt *AbgleichOpt
	if len(opts) > 0 {
		opt = opts[0]
	}
	var ges AbgleichErgebnis
	if l.Zeit == nil {
		l.Zeit = map[string]float64{}
	}
	paare := map[*Register]*Register{}
	for _, r := range l.Register {
		for _, q := range ein.Register {
			if q.Tabelle == r.Tabelle {
				paare[r] = q
			}
		}
	}
	if len(paare) == 0 && len(l.Register) > 0 && len(ein.Register) > 0 {
		paare[l.Register[0]] = ein.Register[0]
	}
	for _, r := range l.Register {
		q := paare[r]
		if q == nil {
			continue
		}
		ges.Add(tabelleAbgleichen(l, r, q, ein.Zeit, herkunft, opt))
	}
	return ges
}

func tabelleAbgleichen(l *Liste, lo *Register, src *Register, zIn map[string]float64, herkunft string, opt *AbgleichOpt) AbgleichErgebnis {
	var e AbgleichErgebnis
	e.Tabellen = 1
	ziel := make([]int, len(src.Spalten))
	for j, s := range src.Spalten {
		ziel[j] = lo.Spalte(s)
	}
	if src.Spalte(spSN) < 0 || lo.Spalte(spSN) < 0 {
		return e
	}
	// doppelte Seriennummern (eigene oder empfangene Liste): dann zusaetzlich ueber den Typ erkennen
	doppelt := lo.DoppelteSN()
	for k := range src.DoppelteSN() {
		doppelt[k] = true
	}
	idx := map[string]*Zeile{}
	for _, z := range lo.Zeilen {
		k := lo.GeraeteSchluessel(z, doppelt)
		if k != "|" && idx[k] == nil {
			idx[k] = z
		}
	}
	pf := lo.Praefix()
	spText := func(s string) string {
		if lo.Tabelle == "Geraete" || lo.Tabelle == "" {
			return s
		}
		return lo.Name + ": " + s
	}
	jetzt := time.Now().Format("02.01.2006 15:04")
	for _, sz := range src.Zeilen {
		key := src.GeraeteSchluessel(sz, doppelt)
		if key == "|" {
			continue
		}
		if z := idx[key]; z != nil {
			geaendert := false
			for j, k := range ziel {
				if k < 0 || IstFormelSpalte(lo.Spalten[k]) || j >= len(sz.Werte) {
					continue
				}
				for len(z.Werte) < len(lo.Spalten) {
					z.Werte = append(z.Werte, Wert{})
				}
				v := sz.Werte[j]
				alt := z.Werte[k]
				kc := pf + key + "|" + strings.ToUpper(lo.Spalten[k])
				tLoc := l.Zeit[kc]
				tIn := zIn[kc]
				if alt.Text() != v.Text() {
					if (tIn > 0 && tIn > tLoc) || (tIn == 0 && strings.TrimSpace(alt.Text()) == "" && strings.TrimSpace(v.Text()) != "") {
						if opt != nil {
							id := opt.id(lo.Tabelle, key, lo.Spalten[k])
							if opt.Abgelehnt[id] {
								// abgelehnt: eigener Wert bleibt und gilt ab jetzt als der neuere
								if tIn > 0 {
									l.Zeit[kc] = tIn + 1.0/1440
								}
								continue
							}
							opt.Liste = append(opt.Liste, Aenderung{ID: id, Von: herkunft, Register: lo.Name,
								Nummer: lo.Wert(z, spNummer).Text(), Typ: lo.Wert(z, "Typ").Text(), SN: lo.Wert(z, spSN).Text(),
								Spalte: lo.Spalten[k], Alt: protokollWert(lo.Spalten[k], alt), Neu: protokollWert(lo.Spalten[k], v)})
						}
						z.Werte[k] = v
						if tIn > 0 {
							l.Zeit[kc] = tIn
						}
						l.Protokoll = append(l.Protokoll, ProtokollEintrag{Zeit: jetzt, Von: herkunft,
							Nummer: lo.Wert(z, spNummer).Text(), SN: lo.Wert(z, spSN).Text(), Typ: lo.Wert(z, "Typ").Text(), Spalte: spText(lo.Spalten[k]),
							Alt: protokollWert(lo.Spalten[k], alt), Neu: protokollWert(lo.Spalten[k], v)})
						e.Felder++
						geaendert = true
					}
				} else if tIn > tLoc {
					l.Zeit[kc] = tIn
				}
			}
			// Kommentare vom Kollegen ergaenzen (eigene bleiben)
			for sp, txt := range sz.Notizen {
				if lo.Spalte(sp) >= 0 && (z.Notizen == nil || z.Notizen[sp] == "") {
					if z.Notizen == nil {
						z.Notizen = map[string]string{}
					}
					z.Notizen[sp] = txt
				}
			}
			if geaendert {
				e.Geraete++
			}
		} else {
			var nid string
			if opt != nil {
				nid = opt.id(lo.Tabelle, key, "*")
				if opt.Abgelehnt[nid] {
					continue
				}
			}
			z := &Zeile{ID: NeueID() + key, Werte: make([]Wert, len(lo.Spalten))}
			for j, k := range ziel {
				if k < 0 || IstFormelSpalte(lo.Spalten[k]) || j >= len(sz.Werte) {
					continue
				}
				z.Werte[k] = sz.Werte[j]
				kc := pf + key + "|" + strings.ToUpper(lo.Spalten[k])
				if t, ok := zIn[kc]; ok {
					l.Zeit[kc] = t
				}
			}
			for sp, txt := range sz.Notizen {
				if lo.Spalte(sp) >= 0 {
					if z.Notizen == nil {
						z.Notizen = map[string]string{}
					}
					z.Notizen[sp] = txt
				}
			}
			lo.Zeilen = append(lo.Zeilen, z)
			idx[key] = z
			if opt != nil {
				opt.Liste = append(opt.Liste, Aenderung{ID: nid, Von: herkunft, Register: lo.Name, NeuesGeraet: true,
					Nummer: lo.Wert(z, spNummer).Text(), Typ: lo.Wert(z, "Typ").Text(), SN: lo.Wert(z, spSN).Text(), Spalte: "(neues Gerät)"})
			}
			l.Protokoll = append(l.Protokoll, ProtokollEintrag{Zeit: jetzt, Von: herkunft,
				Nummer: lo.Wert(z, spNummer).Text(), SN: lo.Wert(z, spSN).Text(), Typ: lo.Wert(z, "Typ").Text(), Spalte: spText("(neues Gerät)")})
			e.Neu++
		}
	}
	return e
}
