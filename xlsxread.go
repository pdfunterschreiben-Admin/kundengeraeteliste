package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// ---------- einfache Lesestruktur einer Excel-Datei ----------

type xZelle struct {
	Wert    Wert
	Formel  bool
	Kommentar string
}

type xTabelle struct {
	Name           string
	R1, C1, R2, C2 int // 1-basiert
	Spalten        []string
}

type xBlatt struct {
	Name     string
	Zellen   map[[2]int]*xZelle // [zeile, spalte]
	Tabellen []xTabelle
	MaxZeile int
	Versteckt bool
}

type xMappe struct {
	Blaetter []*xBlatt
	Namen    map[string]string // definierte Namen (ohne Blattbezug)
	VBA      []byte
}

func zipLesen(zr *zip.Reader, name string) ([]byte, error) {
	name = strings.TrimPrefix(name, "/")
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, name) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s fehlt", name)
}

type rels struct {
	Rel []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
		Type   string `xml:"Type,attr"`
	} `xml:"Relationship"`
}

func relsLesen(zr *zip.Reader, teil string) map[string]string {
	dir, file := path.Split(teil)
	b, err := zipLesen(zr, dir+"_rels/"+file+".rels")
	m := map[string]string{}
	if err != nil {
		return m
	}
	var r rels
	if xml.Unmarshal(b, &r) == nil {
		for _, x := range r.Rel {
			t := x.Target
			if strings.HasPrefix(t, "/") {
				t = strings.TrimPrefix(t, "/")
			} else {
				t = path.Clean(dir + t)
			}
			m[x.ID] = t
		}
	}
	return m
}

var reZelle = regexp.MustCompile(`^([A-Z]+)(\d+)$`)

func zellAdresse(s string) (int, int) {
	m := reZelle.FindStringSubmatch(strings.ReplaceAll(strings.ToUpper(s), "$", ""))
	if m == nil {
		return 0, 0
	}
	c := 0
	for _, ch := range m[1] {
		c = c*26 + int(ch-'A'+1)
	}
	r, _ := strconv.Atoi(m[2])
	return r, c
}

func spaltenName(c int) string {
	s := ""
	for c > 0 {
		c--
		s = string(rune('A'+c%26)) + s
		c /= 26
	}
	return s
}

// Text aus <si> bzw. <is>: alle <t> zusammen (ohne phonetische Angaben)
type richText struct {
	T  string `xml:"t"`
	R  []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (r richText) String() string {
	if len(r.R) == 0 {
		return r.T
	}
	var b strings.Builder
	b.WriteString(r.T)
	for _, x := range r.R {
		b.WriteString(x.T)
	}
	return b.String()
}

func istDatumFormat(id int, code string) bool {
	if (id >= 14 && id <= 22) || (id >= 45 && id <= 47) || (id >= 27 && id <= 36) || (id >= 50 && id <= 58) {
		return true
	}
	if code == "" {
		return false
	}
	// Teile in Anfuehrungszeichen und [..] entfernen
	c := regexp.MustCompile(`"[^"]*"|\[[^\]]*\]|\\.`).ReplaceAllString(strings.ToLower(code), "")
	return strings.ContainsAny(c, "ymd") && !strings.Contains(c, "#")
}

func MappeLesen(daten []byte) (*xMappe, error) {
	zr, err := zip.NewReader(bytes.NewReader(daten), int64(len(daten)))
	if err != nil {
		return nil, errors.New("keine gültige Excel-Datei (.xlsx/.xlsm)")
	}
	mappe := &xMappe{Namen: map[string]string{}}

	// Arbeitsmappe
	wbB, err := zipLesen(zr, "xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	var wb struct {
		Sheets []struct {
			Name  string `xml:"name,attr"`
			State string `xml:"state,attr"`
			RID   string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
		Names []struct {
			Name  string `xml:"name,attr"`
			Local string `xml:"localSheetId,attr"`
			Wert  string `xml:",chardata"`
		} `xml:"definedNames>definedName"`
	}
	if err := xml.Unmarshal(wbB, &wb); err != nil {
		return nil, err
	}
	for _, n := range wb.Names {
		if n.Local == "" {
			mappe.Namen[n.Name] = n.Wert
		}
	}
	wbRels := relsLesen(zr, "xl/workbook.xml")
	for _, t := range wbRels {
		if strings.HasSuffix(strings.ToLower(t), "vbaproject.bin") {
			mappe.VBA, _ = zipLesen(zr, t)
		}
	}

	// gemeinsame Texte
	var sst []string
	if b, err := zipLesen(zr, "xl/sharedStrings.xml"); err == nil {
		var s struct {
			SI []richText `xml:"si"`
		}
		if xml.Unmarshal(b, &s) == nil {
			for _, x := range s.SI {
				sst = append(sst, x.String())
			}
		}
	}

	// Formate: welche Stile sind Datumsangaben?
	datumStil := map[int]bool{}
	if b, err := zipLesen(zr, "xl/styles.xml"); err == nil {
		var st struct {
			NumFmts []struct {
				ID   int    `xml:"numFmtId,attr"`
				Code string `xml:"formatCode,attr"`
			} `xml:"numFmts>numFmt"`
			Xfs []struct {
				NumFmt int `xml:"numFmtId,attr"`
			} `xml:"cellXfs>xf"`
		}
		if xml.Unmarshal(b, &st) == nil {
			codes := map[int]string{}
			for _, n := range st.NumFmts {
				codes[n.ID] = n.Code
			}
			for i, x := range st.Xfs {
				if istDatumFormat(x.NumFmt, codes[x.NumFmt]) {
					datumStil[i] = true
				}
			}
		}
	}

	for _, s := range wb.Sheets {
		teil := wbRels[s.RID]
		if teil == "" {
			continue
		}
		b, err := zipLesen(zr, teil)
		if err != nil {
			continue
		}
		bl := &xBlatt{Name: s.Name, Zellen: map[[2]int]*xZelle{}, Versteckt: s.State != "" && s.State != "visible"}
		var ws struct {
			Rows []struct {
				C []struct {
					R  string    `xml:"r,attr"`
					T  string    `xml:"t,attr"`
					S  int       `xml:"s,attr"`
					F  *struct{} `xml:"f"`
					V  string    `xml:"v"`
					IS richText  `xml:"is"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
			TableParts []struct {
				RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
			} `xml:"tableParts>tablePart"`
		}
		if err := xml.Unmarshal(b, &ws); err != nil {
			return nil, fmt.Errorf("Blatt %s: %v", s.Name, err)
		}
		for _, row := range ws.Rows {
			for _, c := range row.C {
				r, k := zellAdresse(c.R)
				if r == 0 {
					continue
				}
				z := &xZelle{Formel: c.F != nil}
				switch c.T {
				case "s":
					i, _ := strconv.Atoi(c.V)
					if i >= 0 && i < len(sst) {
						z.Wert = Wert{T: "s", V: sst[i]}
					}
				case "inlineStr":
					z.Wert = Wert{T: "s", V: c.IS.String()}
				case "str":
					z.Wert = Wert{T: "s", V: c.V}
				case "b":
					z.Wert = Wert{T: "s", V: map[string]string{"1": "WAHR", "0": "FALSCH"}[c.V]}
				case "e":
					z.Wert = Wert{}
				default:
					if c.V != "" {
						f, err := strconv.ParseFloat(c.V, 64)
						if err == nil {
							if datumStil[c.S] {
								z.Wert = Wert{T: "d", V: VonSerial(f).Format("2006-01-02")}
							} else {
								z.Wert = Wert{T: "n", V: ZahlText(f)}
							}
						}
					}
				}
				if z.Wert.T == "s" && z.Wert.V == "" {
					z.Wert = Wert{}
				}
				bl.Zellen[[2]int{r, k}] = z
				if r > bl.MaxZeile && (!z.Wert.Leer() || z.Formel) {
					bl.MaxZeile = r
				}
			}
		}
		shRels := relsLesen(zr, teil)
		for _, tp := range ws.TableParts {
			tb, err := zipLesen(zr, shRels[tp.RID])
			if err != nil {
				continue
			}
			var t struct {
				Name    string `xml:"name,attr"`
				Display string `xml:"displayName,attr"`
				Ref     string `xml:"ref,attr"`
				Header  string `xml:"headerRowCount,attr"`
				Cols    []struct {
					Name string `xml:"name,attr"`
				} `xml:"tableColumns>tableColumn"`
			}
			if xml.Unmarshal(tb, &t) != nil {
				continue
			}
			teile := strings.Split(t.Ref, ":")
			if len(teile) != 2 || t.Header == "0" {
				continue
			}
			xt := xTabelle{Name: t.Display}
			if xt.Name == "" {
				xt.Name = t.Name
			}
			xt.R1, xt.C1 = zellAdresse(teile[0])
			xt.R2, xt.C2 = zellAdresse(teile[1])
			for _, c := range t.Cols {
				xt.Spalten = append(xt.Spalten, strings.ReplaceAll(c.Name, "_x000a_", " "))
			}
			bl.Tabellen = append(bl.Tabellen, xt)
		}
		// Kommentare
		for _, t := range shRels {
			if strings.Contains(strings.ToLower(path.Base(t)), "comments") && strings.HasSuffix(t, ".xml") {
				cb, err := zipLesen(zr, t)
				if err != nil {
					continue
				}
				var cm struct {
					Authors []string `xml:"authors>author"`
					List    []struct {
						Ref  string   `xml:"ref,attr"`
						Text richText `xml:"text"`
					} `xml:"commentList>comment"`
				}
				if xml.Unmarshal(cb, &cm) != nil {
					continue
				}
				for _, c := range cm.List {
					r, k := zellAdresse(c.Ref)
					txt := kommentarBereinigen(c.Text.String())
					if r == 0 || txt == "" {
						continue
					}
					z := bl.Zellen[[2]int{r, k}]
					if z == nil {
						z = &xZelle{}
						bl.Zellen[[2]int{r, k}] = z
					}
					z.Kommentar = txt
				}
			}
		}
		mappe.Blaetter = append(mappe.Blaetter, bl)
	}
	return mappe, nil
}

func kommentarBereinigen(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	if i := strings.Index(s, "Kommentar:"); strings.HasPrefix(s, "[Kommentarthread]") && i >= 0 {
		s = s[i+len("Kommentar:"):]
		// Antworten ("Antworten:") bleiben mit drin
	}
	// "Name:" am Anfang (Autor der alten Notiz) weglassen
	if i := strings.Index(s, ":\n"); i > 0 && i < 40 && !strings.Contains(s[:i], "\n") {
		s = s[i+2:]
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (b *xBlatt) Zelle(r, c int) *xZelle {
	if z := b.Zellen[[2]int{r, c}]; z != nil {
		return z
	}
	return &xZelle{}
}

// ---------- Import einer Geraeteliste ----------

func istGeraeteTabelle(name string) bool {
	return name == "Geraete" || strings.HasPrefix(name, "Geraete_")
}

func nameWert(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "=") {
		s = s[1:]
	}
	return strings.Trim(s, "\"")
}

// Liest eine Geraeteliste (Makro-Datei oder empfangene Kopie) in das Programmformat.
func ListeAusMappe(m *xMappe) (*Liste, error) {
	l := &Liste{Zeit: map[string]float64{}}
	var erstes *xBlatt
	for _, bl := range m.Blaetter {
		for _, t := range bl.Tabellen {
			if !istGeraeteTabelle(t.Name) && len(m.Blaetter) > 0 && hatGeraeteTabelle(m) {
				continue
			}
			reg := &Register{Name: bl.Name, Tabelle: t.Name}
			for _, s := range t.Spalten {
				reg.Spalten = append(reg.Spalten, strings.TrimSpace(s))
			}
			if reg.Spalte(spSN) < 0 {
				continue
			}
			if erstes == nil {
				erstes = bl
			}
			for r := t.R1 + 1; r <= t.R2; r++ {
				z := &Zeile{ID: NeueID() + strconv.Itoa(r), Werte: make([]Wert, len(reg.Spalten))}
				hat := false
				for k := range reg.Spalten {
					c := bl.Zelle(r, t.C1+k)
					if IstFormelSpalte(reg.Spalten[k]) {
						continue
					}
					z.Werte[k] = c.Wert
					if !c.Wert.Leer() {
						hat = true
					}
					if c.Kommentar != "" {
						if z.Notizen == nil {
							z.Notizen = map[string]string{}
						}
						z.Notizen[reg.Spalten[k]] = c.Kommentar
						hat = true
					}
				}
				if hat {
					reg.Zeilen = append(reg.Zeilen, z)
				}
			}
			// Notizen unter der Tabelle (ohne "Anzahl Geraete")
			var notiz []string
			for r := t.R2 + 1; r <= bl.MaxZeile && r <= t.R2+30; r++ {
				for c := 1; c <= t.C2+3; c++ {
					z := bl.Zelle(r, c)
					if z.Formel || z.Wert.Leer() {
						continue
					}
					v := strings.TrimSpace(z.Wert.V)
					if strings.HasPrefix(strings.ToLower(v), "anzahl ger") {
						continue
					}
					notiz = append(notiz, v)
				}
			}
			reg.Notiz = strings.Join(notiz, "\n")
			l.Register = append(l.Register, reg)
		}
	}
	if len(l.Register) == 0 {
		return nil, errors.New("in der Datei wurde keine Gerätetabelle mit der Spalte „Seriennummer“ gefunden")
	}
	// Titel, Untertitel, Empfaenger aus dem ersten Register
	if erstes != nil {
		l.Titel = strings.TrimSpace(erstes.Zelle(1, 1).Wert.V)
		l.Untertitel = strings.TrimSpace(erstes.Zelle(2, 1).Wert.V)
		for c := 1; c <= 20; c++ {
			v := strings.TrimSpace(erstes.Zelle(3, c).Wert.V)
			if strings.Contains(v, "@") {
				l.Empfaenger = v
			}
		}
	}
	if l.Titel == "" {
		l.Titel = "Geräteliste"
	}
	// Zeitstempel (Blatt SyncDaten)
	for _, bl := range m.Blaetter {
		if bl.Name != "SyncDaten" {
			continue
		}
		for r := 1; r <= bl.MaxZeile+1; r++ {
			k := bl.Zelle(r, 1).Wert
			v := bl.Zelle(r, 2).Wert
			if k.Leer() {
				continue
			}
			if f, err := strconv.ParseFloat(v.V, 64); err == nil && v.T == "n" {
				l.Zeit[k.V] = f
			}
		}
	}
	// Protokoll (Blatt Sync-Protokoll)
	for _, bl := range m.Blaetter {
		if bl.Name != "Sync-Protokoll" {
			continue
		}
		for r := 2; r <= bl.MaxZeile; r++ {
			t := func(c int) string {
				w := bl.Zelle(r, c).Wert
				if w.T == "d" {
					return w.V
				}
				return w.V
			}
			if t(1) == "" && t(4) == "" {
				continue
			}
			l.Protokoll = append(l.Protokoll, ProtokollEintrag{Zeit: t(1), Von: t(2), Nummer: t(3), SN: t(4), Spalte: t(5), Alt: t(6), Neu: t(7)})
		}
	}
	l.ListenID = nameWert(m.Namen["Listen_ID"])
	l.Herkunft = nameWert(m.Namen["Listen_Herkunft"])
	if f, err := strconv.ParseFloat(nameWert(m.Namen["Sync_LetzterImport"]), 64); err == nil {
		l.LetzterImport = f
	}
	l.SyncOffen = nameWert(m.Namen["Sync_Offen"]) == "1"
	if len(m.VBA) > 0 {
		if b := BetreffAusVBA(m.VBA); b != "" {
			l.Betreff = b
		}
	}
	return l, nil
}

func hatGeraeteTabelle(m *xMappe) bool {
	for _, bl := range m.Blaetter {
		for _, t := range bl.Tabellen {
			if istGeraeteTabelle(t.Name) {
				return true
			}
		}
	}
	return false
}

// Feste Betreffzeile aus dem Makro (z. B. Const BETREFF = "Geraeteliste Claas - Synchronisation")
var reBetreff = regexp.MustCompile(`(?i)Const\s+BETREFF\s+As\s+String\s*=\s*"([^"]+)"`)

func BetreffAusVBA(vba []byte) string {
	mods, err := VBAModule(vba)
	if err != nil {
		return ""
	}
	for _, code := range mods {
		if m := reBetreff.FindStringSubmatch(code); m != nil {
			return m[1]
		}
	}
	return ""
}
