package main

import (
	"os"
	"os/user"
	"sort"
	"strings"
)

func sortStrings(s []string) { sort.Strings(s) }

// Name fuer Protokoll und Meldungen
func BenutzerName() string {
	if u, err := user.Current(); err == nil {
		n := u.Username
		if i := strings.LastIndex(n, `\`); i >= 0 {
			n = n[i+1:]
		}
		if n != "" {
			return n
		}
	}
	if n := os.Getenv("USERNAME"); n != "" {
		return n
	}
	return "Benutzer"
}

func RechnerName() string {
	if n := os.Getenv("COMPUTERNAME"); n != "" {
		return n
	}
	h, _ := os.Hostname()
	return h
}

// Tabellennamen gueltig und eindeutig halten (wichtig fuer den Abgleich mit Excel)
func TabellenPruefen(l *Liste) {
	vergeben := map[string]bool{}
	for i, r := range l.Register {
		t := r.Tabelle
		ok := t != "" && istGeraeteTabelle(t) && !vergeben[strings.ToLower(t)]
		if ok {
			for _, c := range t {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
					ok = false
				}
			}
		}
		if !ok {
			if i == 0 && !vergeben["geraete"] {
				t = "Geraete"
			} else {
				t = tabellenName(r.Name, vergeben)
			}
		}
		r.Tabelle = t
		vergeben[strings.ToLower(t)] = true
		for _, z := range r.Zeilen {
			for len(z.Werte) < len(r.Spalten) {
				z.Werte = append(z.Werte, Wert{})
			}
		}
	}
}
