package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const SicherungenBehalten = 30

type Einstellungen struct {
	ReportOrdner  string `json:"reportOrdner,omitempty"`
	Testversionen bool   `json:"testversionen,omitempty"`
	AutoSync      *bool  `json:"autoSync,omitempty"` // Abgleich beim Öffnen/Schließen (Standard an)
	Installiert   bool   `json:"installiert,omitempty"`
}

func (e Einstellungen) AutoSyncAn() bool { return e.AutoSync == nil || *e.AutoSync }

type Speicher struct {
	mu     sync.Mutex
	Basis  string
	listen map[string]*Liste
	Einst  Einstellungen
}

func DatenOrdner() string {
	if d := os.Getenv("KGL_DATEN"); d != "" {
		return d
	}
	if a := os.Getenv("APPDATA"); a != "" {
		return filepath.Join(a, "Kundengeraeteliste")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".kundengeraeteliste")
}

func SpeicherOeffnen(basis string) (*Speicher, error) {
	s := &Speicher{Basis: basis, listen: map[string]*Liste{}}
	for _, d := range []string{s.ListenOrdner(), s.SicherungsOrdner()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if b, err := os.ReadFile(filepath.Join(basis, "einstellungen.json")); err == nil {
		_ = json.Unmarshal(b, &s.Einst)
	}
	files, _ := filepath.Glob(filepath.Join(s.ListenOrdner(), "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var l Liste
		if json.Unmarshal(b, &l) == nil && l.ID != "" {
			s.listen[l.ID] = &l
		}
	}
	return s, nil
}

func (s *Speicher) ListenOrdner() string     { return filepath.Join(s.Basis, "Listen") }
func (s *Speicher) SicherungsOrdner() string { return filepath.Join(s.Basis, "Sicherungen") }

func (s *Speicher) EinstSpeichern() error {
	b, _ := json.MarshalIndent(s.Einst, "", "  ")
	return dateiSchreiben(filepath.Join(s.Basis, "einstellungen.json"), b)
}

// atomar schreiben (erst .tmp, dann umbenennen)
func dateiSchreiben(p string, b []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s *Speicher) Alle() []*Liste {
	var out []*Liste
	for _, l := range s.listen {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Titel) < strings.ToLower(out[j].Titel) })
	return out
}

func (s *Speicher) Holen(id string) *Liste { return s.listen[id] }

func (s *Speicher) Speichern(l *Liste) error {
	if l.ID == "" {
		l.ID = NeueID()
	}
	if l.Angelegt == "" {
		l.Angelegt = time.Now().Format(time.RFC3339)
	}
	l.Geaendert = time.Now().Format(time.RFC3339)
	s.listen[l.ID] = l
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return dateiSchreiben(filepath.Join(s.ListenOrdner(), l.ID+".json"), b)
}

func (s *Speicher) Loeschen(id string) error {
	l := s.listen[id]
	if l == nil {
		return errors.New("Liste nicht gefunden")
	}
	s.SicherungAnlegen(l, "vor dem Löschen")
	delete(s.listen, id)
	return os.Remove(filepath.Join(s.ListenOrdner(), id+".json"))
}

// ---------- Sicherungen ----------

type Sicherung struct {
	Datei  string `json:"datei"`
	Zeit   string `json:"zeit"`
	Grund  string `json:"grund"`
	Titel  string `json:"titel"`
	Geraete int   `json:"geraete"`
}

type sicherungsDatei struct {
	Grund string `json:"grund"`
	Zeit  string `json:"zeit"`
	Liste *Liste `json:"liste"`
}

func (s *Speicher) SicherungAnlegen(l *Liste, grund string) {
	b, err := json.Marshal(sicherungsDatei{Grund: grund, Zeit: time.Now().Format(time.RFC3339), Liste: l})
	if err != nil {
		return
	}
	name := l.ID + "_" + time.Now().Format("2006-01-02_15-04-05.000") + ".json"
	_ = os.WriteFile(filepath.Join(s.SicherungsOrdner(), name), b, 0o644)
	files, _ := filepath.Glob(filepath.Join(s.SicherungsOrdner(), l.ID+"_*.json"))
	sort.Strings(files)
	for len(files) > SicherungenBehalten {
		_ = os.Remove(files[0])
		files = files[1:]
	}
}

func (s *Speicher) Sicherungen(id string) []Sicherung {
	files, _ := filepath.Glob(filepath.Join(s.SicherungsOrdner(), id+"_*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	var out []Sicherung
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var sd sicherungsDatei
		if json.Unmarshal(b, &sd) != nil || sd.Liste == nil {
			continue
		}
		out = append(out, Sicherung{Datei: filepath.Base(f), Zeit: sd.Zeit, Grund: sd.Grund, Titel: sd.Liste.Titel, Geraete: anzahlGeraete(sd.Liste)})
	}
	return out
}

func (s *Speicher) Wiederherstellen(id, datei string) (*Liste, error) {
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
	if akt := s.listen[id]; akt != nil {
		s.SicherungAnlegen(akt, "vor dem Wiederherstellen")
	}
	sd.Liste.ID = id
	return sd.Liste, s.Speichern(sd.Liste)
}

// Liste, die per Sicherung geloescht wurde, wiederfinden
func (s *Speicher) GeloeschteListen() []Sicherung {
	files, _ := filepath.Glob(filepath.Join(s.SicherungsOrdner(), "*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	gesehen := map[string]bool{}
	var out []Sicherung
	for _, f := range files {
		base := filepath.Base(f)
		i := strings.LastIndex(base, "_")
		j := strings.LastIndex(base[:max(i, 0)], "_")
		if j <= 0 {
			continue
		}
		id := base[:j]
		if s.listen[id] != nil || gesehen[id] {
			continue
		}
		gesehen[id] = true
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var sd sicherungsDatei
		if json.Unmarshal(b, &sd) != nil || sd.Liste == nil {
			continue
		}
		out = append(out, Sicherung{Datei: base, Zeit: sd.Zeit, Grund: sd.Grund, Titel: sd.Liste.Titel, Geraete: anzahlGeraete(sd.Liste)})
	}
	return out
}

func anzahlGeraete(l *Liste) int {
	n := 0
	for _, r := range l.Register {
		for _, z := range r.Zeilen {
			if !r.Wert(z, spSN).Leer() {
				n++
			}
		}
	}
	return n
}
