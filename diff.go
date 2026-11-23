package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Unterschied zwischen einer Sicherung und dem aktuellen Stand
type SicherungsVorschau struct {
	Zurueck  int      `json:"zurueck"`  // Geraete, die durch die Sicherung wieder auftauchen
	Weg      int      `json:"weg"`      // Geraete, die es nur im aktuellen Stand gibt (gehen verloren)
	Felder   int      `json:"felder"`   // Felder mit anderem Wert
	Beispiele []string `json:"beispiele"`
}

func (s *Speicher) SicherungVorschau(id, datei string) (*SicherungsVorschau, error) {
	if strings.ContainsAny(datei, `/\`) || !strings.HasPrefix(datei, id+"_") {
		return nil, errors.New("ungültige Sicherung")
	}
	b, err := os.ReadFile(filepath.Join(s.SicherungsOrdner(), datei))
	if err != nil {
		return nil, err
	}
	var sd sicherungsDatei
	if err := json.Unmarshal(b, &sd); err != nil || sd.Liste == nil {
		return nil, errors.New("Sicherung ist beschädigt")
	}
	akt := s.listen[id]
	if akt == nil {
		return nil, errors.New("Liste nicht gefunden")
	}
	return Unterschiede(sd.Liste, akt), nil
}

// Unterschiede zeigt, was sich beim Zuruecksetzen von akt auf alt aendern wuerde.
func Unterschiede(alt, akt *Liste) *SicherungsVorschau {
	v := &SicherungsVorschau{Beispiele: []string{}}
	for _, ra := range alt.Register {
		var rc *Register
		for _, r := range akt.Register {
			if r.Tabelle == ra.Tabelle || r.Name == ra.Name {
				rc = r
			}
		}
		if rc == nil {
			v.Zurueck += len(ra.Zeilen)
			continue
		}
		doppelt := ra.DoppelteSN()
		for k := range rc.DoppelteSN() {
			doppelt[k] = true
		}
		idx := map[string]*Zeile{}
		for _, z := range rc.Zeilen {
			idx[rc.GeraeteSchluessel(z, doppelt)] = z
		}
		gesehen := map[string]bool{}
		for _, za := range ra.Zeilen {
			key := ra.GeraeteSchluessel(za, doppelt)
			zc := idx[key]
			if zc == nil {
				v.Zurueck++
				continue
			}
			gesehen[key] = true
			for i, sp := range ra.Spalten {
				if IstFormelSpalte(sp) {
					continue
				}
				j := rc.Spalte(sp)
				if j < 0 {
					continue
				}
				wa, wc := ra.Wert(za, sp), rc.Wert(zc, sp)
				if wa.Text() != wc.Text() {
					v.Felder++
					if len(v.Beispiele) < 8 {
						v.Beispiele = append(v.Beispiele, fmt.Sprintf("%s (SN %s): %s  %s → %s", ra.Wert(za, spNummer).Text(), ra.Wert(za, spSN).Text(), sp,
							protokollWert(sp, wc), protokollWert(sp, wa)))
					}
				}
				_ = i
			}
		}
		for key := range idx {
			if !gesehen[key] {
				v.Weg++
			}
		}
	}
	return v
}
