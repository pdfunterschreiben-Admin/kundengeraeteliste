package main

import (
	"bytes"
	"compress/zlib"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type ReportGeraet struct {
	Register string `json:"register"`
	Nummer   string `json:"nummer"`
	SN       string `json:"sn"`
	Typ      string `json:"typ"`
}

type Report struct {
	Pfad    string         `json:"pfad"`
	Name    string         `json:"name"`
	Ordner  string         `json:"ordner"`
	Groesse int64          `json:"groesse"`
	Datum   string         `json:"datum"`
	Geraete []ReportGeraet `json:"geraete"`
	Treffer string         `json:"treffer"` // "Dateiname" oder "Inhalt"
}

type inhaltCache struct {
	mod  time.Time
	size int64
	text string
}

var (
	cacheMu sync.Mutex
	cache   = map[string]inhaltCache{}
)

var inhaltEndungen = map[string]bool{".txt": true, ".pdf": true, ".htm": true, ".html": true, ".csv": true, ".xml": true, ".log": true, ".json": true}

func dateiInhalt(p string, info fs.FileInfo) string {
	ext := strings.ToLower(filepath.Ext(p))
	if !inhaltEndungen[ext] || info.Size() > 15<<20 {
		return ""
	}
	cacheMu.Lock()
	c, ok := cache[p]
	cacheMu.Unlock()
	if ok && c.mod.Equal(info.ModTime()) && c.size == info.Size() {
		return c.text
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var text string
	if ext == ".pdf" {
		text = pdfText(b)
	} else {
		text = string(b)
		// UTF-16-Dateien (BOM) vereinfachen
		if len(b) > 2 && ((b[0] == 0xFF && b[1] == 0xFE) || (b[0] == 0xFE && b[1] == 0xFF)) {
			text = strings.ReplaceAll(text, "\x00", "")
		}
	}
	text = strings.ToUpper(text)
	cacheMu.Lock()
	cache[p] = inhaltCache{info.ModTime(), info.Size(), text}
	cacheMu.Unlock()
	return text
}

var reStream = regexp.MustCompile(`(?s)stream\r?\n`)
var rePdfStr = regexp.MustCompile(`\(((?:\\.|[^\\)])*)\)`)

// grobe Textsuche in PDFs (entpackte Inhalte + Textstuecke)
func pdfText(b []byte) string {
	var out bytes.Buffer
	out.Write(b)
	locs := reStream.FindAllIndex(b, 500)
	for _, l := range locs {
		start := l[1]
		end := bytes.Index(b[start:], []byte("endstream"))
		if end < 0 {
			continue
		}
		r, err := zlib.NewReader(bytes.NewReader(b[start : start+end]))
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(r, 8<<20))
		r.Close()
		out.WriteByte('\n')
		out.Write(data)
		// Textstuecke "(...)" zusammenhaengen (Zahlen sind oft zerstueckelt: [(68)-3(16)] TJ)
		var sb strings.Builder
		for _, m := range rePdfStr.FindAllSubmatch(data, -1) {
			sb.Write(m[1])
		}
		out.WriteByte('\n')
		out.WriteString(sb.String())
	}
	return out.String()
}

func istZeichen(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func istZiffer(c byte) bool { return c >= '0' && c <= '9' }

// SN im Text finden, ohne dass direkt davor/dahinter weitere Ziffern (bzw. Buchstaben) stehen
func enthaeltSN(text, sn string, streng bool) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], sn)
		if j < 0 {
			return false
		}
		a := i + j
		e := a + len(sn)
		vorn := a == 0 || !(istZiffer(text[a-1]) || (streng && istZeichen(text[a-1])))
		hinten := e >= len(text) || !(istZiffer(text[e]) || (streng && istZeichen(text[e])))
		if vorn && hinten {
			return true
		}
		i = a + 1
	}
}

type snInfo struct {
	sn string
	g  ReportGeraet
}

func ReportsSuchen(l *Liste, ordner string) ([]Report, error) {
	if ordner == "" {
		return nil, nil
	}
	if _, err := os.Stat(ordner); err != nil {
		return nil, err
	}
	var sns []snInfo
	for _, r := range l.Register {
		for _, z := range r.Zeilen {
			sn := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(r.Wert(z, spSN).Text()), " ", ""))
			if len(sn) < 4 {
				continue
			}
			sns = append(sns, snInfo{sn, ReportGeraet{r.Name, r.Wert(z, spNummer).Text(), r.Wert(z, spSN).Text(), r.Wert(z, "Typ").Text()}})
		}
	}
	var out []Report
	anzahl := 0
	err := filepath.WalkDir(ordner, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != ordner {
				return filepath.SkipDir
			}
			return nil
		}
		anzahl++
		if anzahl > 50000 {
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		name := strings.ToUpper(d.Name())
		rel, _ := filepath.Rel(ordner, filepath.Dir(p))
		pfadText := strings.ToUpper(rel) + "/" + name
		var treffer []ReportGeraet
		art := ""
		for _, s := range sns {
			if enthaeltSN(pfadText, s.sn, false) {
				treffer = append(treffer, s.g)
				art = "Dateiname"
			}
		}
		if len(treffer) == 0 {
			if txt := dateiInhalt(p, info); txt != "" {
				for _, s := range sns {
					if len(s.sn) >= 5 && enthaeltSN(txt, s.sn, true) {
						treffer = append(treffer, s.g)
						art = "Inhalt"
					}
				}
				// Inhaltstreffer mit sehr vielen Geraeten sind meist Sammellisten -> ignorieren
				if len(treffer) > 3 {
					treffer = nil
				}
			}
		}
		if len(treffer) > 0 {
			if rel == "." {
				rel = ""
			}
			out = append(out, Report{Pfad: p, Name: d.Name(), Ordner: rel, Groesse: info.Size(),
				Datum: info.ModTime().Format("2006-01-02 15:04"), Geraete: treffer, Treffer: art})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Datum > out[j].Datum })
	return out, err
}

// Pfad liegt im Report-Ordner? (Schutz: nur dort Dateien oeffnen/teilen)
func imOrdner(pfad, ordner string) bool {
	if ordner == "" {
		return false
	}
	a, err1 := filepath.Abs(pfad)
	b, err2 := filepath.Abs(ordner)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(b, a)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
