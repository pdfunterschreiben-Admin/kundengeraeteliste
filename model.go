package main

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Wert ist der Inhalt einer Zelle: leer, Text, Zahl oder Datum.
type Wert struct {
	T string `json:"t,omitempty"` // "" leer, "s" Text, "n" Zahl, "d" Datum (V = JJJJ-MM-TT)
	V string `json:"v,omitempty"`
}

func (w Wert) Leer() bool { return w.T == "" || strings.TrimSpace(w.V) == "" }

// Vergleichstext (entspricht CStr in den Makros: gleicher Wert = gleicher Text)
func (w Wert) Text() string {
	if w.T == "" {
		return ""
	}
	return w.V
}

type Zeile struct {
	ID      string            `json:"id"`
	Werte   []Wert            `json:"werte"`
	Notizen map[string]string `json:"notizen,omitempty"` // Spaltenname -> Kommentar
}

type Register struct {
	Name    string   `json:"name"`    // Registername (Blatt)
	Tabelle string   `json:"tabelle"` // Tabellenname, z. B. Geraete oder Geraete_Paderborn
	Spalten []string `json:"spalten"`
	Zeilen  []*Zeile `json:"zeilen"`
	Notiz   string   `json:"notiz,omitempty"` // Text unter der Tabelle
}

type ProtokollEintrag struct {
	Zeit   string `json:"zeit"`
	Von    string `json:"von"`
	Nummer string `json:"nummer"`
	SN     string `json:"sn"`
	Spalte string `json:"spalte"`
	Alt    string `json:"alt"`
	Neu    string `json:"neu"`
	Typ    string `json:"typ,omitempty"`
	Art    string `json:"art,omitempty"` // "lokal" = eigene Aenderung (nur fuer den Verlauf je Geraet)
}

const maxLokalProtokoll = 6000

// Lokal merkt eine eigene Aenderung fuer den Verlauf des Geraets vor.
func (l *Liste) Lokal(r *Register, z *Zeile, spalte, alt, neu string) {
	l.Protokoll = append(l.Protokoll, ProtokollEintrag{Zeit: time.Now().Format("02.01.2006 15:04"), Von: BenutzerName() + " (ich)",
		Nummer: r.Wert(z, spNummer).Text(), SN: r.Wert(z, spSN).Text(), Typ: r.Wert(z, "Typ").Text(), Spalte: spalte, Alt: alt, Neu: neu, Art: "lokal"})
	if len(l.Protokoll) > maxLokalProtokoll {
		// aelteste eigene Eintraege zuerst entfernen
		ueber := len(l.Protokoll) - maxLokalProtokoll
		var neuP []ProtokollEintrag
		for _, e := range l.Protokoll {
			if ueber > 0 && e.Art == "lokal" {
				ueber--
				continue
			}
			neuP = append(neuP, e)
		}
		l.Protokoll = neuP
	}
}

type Liste struct {
	ID            string             `json:"id"`
	Titel         string             `json:"titel"`
	Untertitel    string             `json:"untertitel,omitempty"`
	Empfaenger    string             `json:"empfaenger,omitempty"`
	Betreff       string             `json:"betreff,omitempty"` // leer = aus Titel
	Register      []*Register        `json:"register"`
	Zeit          map[string]float64 `json:"zeit"` // Zeitstempel je Feld (wie Blatt SyncDaten)
	Protokoll     []ProtokollEintrag `json:"protokoll,omitempty"`
	LetzterImport float64            `json:"letzterImport,omitempty"`
	SyncOffen     bool               `json:"syncOffen,omitempty"`
	ListenID      string             `json:"listenId,omitempty"`
	Herkunft      string             `json:"herkunft,omitempty"`
	Quelle        string             `json:"quelle,omitempty"`        // importierte Datei
	ReportOrdner  string             `json:"reportOrdner,omitempty"` // eigener Truck-Report-Ordner (leer = allgemeiner)
	Geaendert     string             `json:"geaendert,omitempty"`
	Angelegt      string             `json:"angelegt,omitempty"`
}

const (
	spNummer    = "Nummer"
	spSN        = "Seriennummer"
	spWartung   = "Wartung"
	spLetzteUVV = "Letzte UVV"
	spNaechste  = "Nächste UVV"
	spStunden   = "Betriebsstunden"
)

var SpaltenStandard = []string{"Nummer", "Typ", "Seriennummer", "Anbaugerät", "Wartung", "Letzte UVV", "Nächste UVV", "Betriebsstunden", "Info durchgeführte Arbeiten", "Vermerk"}
var SpaltenErweitert = []string{"Nummer", "Typ", "Seriennummer", "Hersteller", "Kunde", "Bereich", "Anbaugerät", "Wartung", "Letzte UVV", "Nächste UVV", "Betriebsstunden", "Info durchgeführte Arbeiten", "Vermerk"}

func gleich(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (r *Register) Spalte(name string) int {
	for i, s := range r.Spalten {
		if gleich(s, name) {
			return i
		}
	}
	return -1
}

// Spalten mit Datum (Anzeige MM/JJJJ)
func IstDatumSpalte(name string) bool {
	return gleich(name, spWartung) || gleich(name, spLetzteUVV) || gleich(name, spNaechste)
}

// Berechnete Spalte (in Excel Formel): wird nie gespeichert oder abgeglichen
func IstFormelSpalte(name string) bool { return gleich(name, spNaechste) }

func (r *Register) Wert(z *Zeile, name string) Wert {
	i := r.Spalte(name)
	if i < 0 || i >= len(z.Werte) {
		return Wert{}
	}
	return z.Werte[i]
}

// Naechste UVV = letzte UVV + 12 Monate (wie EDATE(F5;12))
func (r *Register) NaechsteUVV(z *Zeile) (time.Time, bool) {
	w := r.Wert(z, spLetzteUVV)
	if w.T != "d" {
		return time.Time{}, false
	}
	d, err := time.Parse("2006-01-02", w.V)
	if err != nil {
		return time.Time{}, false
	}
	return edate(d, 12), true
}

func edate(d time.Time, m int) time.Time {
	y, mo, day := d.Date()
	t := time.Date(y, mo+time.Month(m), 1, 0, 0, 0, 0, time.Local)
	last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	if day > last {
		day = last
	}
	return time.Date(t.Year(), t.Month(), day, 0, 0, 0, 0, time.Local)
}

// Schluessel wie in den Makros: "|" + Seriennummer gross, ohne Leerzeichen
func Schluessel(sn string) string {
	return "|" + strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(sn), " ", ""))
}

func typText(t string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(t), " ", ""))
}

// Seriennummern, die in diesem Register mehrfach vorkommen
func (r *Register) DoppelteSN() map[string]bool {
	n := map[string]int{}
	for _, z := range r.Zeilen {
		if k := Schluessel(r.Wert(z, spSN).Text()); k != "|" {
			n[k]++
		}
	}
	d := map[string]bool{}
	for k, c := range n {
		if c > 1 {
			d[k] = true
		}
	}
	return d
}

// Geraete-Schluessel: Seriennummer; kommt sie doppelt vor, zusaetzlich der Typ
// (z. B. zwei Geraete mit SN 10846, aber verschiedenem Typ).
func (r *Register) GeraeteSchluessel(z *Zeile, doppelt map[string]bool) string {
	k := Schluessel(r.Wert(z, spSN).Text())
	if k == "|" || !doppelt[k] {
		return k
	}
	return k + "~" + typText(r.Wert(z, "Typ").Text())
}

func (r *Register) Praefix() string {
	if r.Tabelle == "Geraete" || r.Tabelle == "" {
		return ""
	}
	return r.Tabelle + ":"
}

func (l *Liste) ZeitSetzen(r *Register, z *Zeile, spalte string, ts float64) {
	k := r.GeraeteSchluessel(z, r.DoppelteSN())
	if k == "|" {
		return
	}
	if l.Zeit == nil {
		l.Zeit = map[string]float64{}
	}
	l.Zeit[r.Praefix()+k+"|"+strings.ToUpper(spalte)] = ts
}

// ---------- Excel-Datumszahlen ----------

var excelNull = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// Ortszeit -> Excel-Seriennummer (wie CDbl(Now))
func ZuSerial(t time.Time) float64 {
	y, m, d := t.Date()
	hh, mm, ss := t.Clock()
	u := time.Date(y, m, d, hh, mm, ss, t.Nanosecond(), time.UTC)
	return u.Sub(excelNull).Hours() / 24
}

func VonSerial(f float64) time.Time {
	ms := int64(math.Round(f * 86400000))
	u := excelNull.Add(time.Duration(ms) * time.Millisecond)
	return time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second(), 0, time.Local)
}

func Jetzt() float64 { return ZuSerial(time.Now()) }

func ZahlText(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ---------- Eingaben deuten ----------

var (
	reMonat   = regexp.MustCompile(`^(\d{1,2})\s*[/.\-]\s*(\d{4}|\d{2})$`)
	reTag     = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})\.(\d{4}|\d{2})$`)
	reISO     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reGanz    = regexp.MustCompile(`^[1-9]\d{0,14}$`)
	reDezimal = regexp.MustCompile(`^-?\d+([.,]\d+)?$`)
)

func jahr(s string) int {
	y, _ := strconv.Atoi(s)
	if y < 100 {
		y += 2000
	}
	return y
}

func datumWert(y, m, d int) (Wert, bool) {
	if m < 1 || m > 12 || d < 1 || d > 31 || y < 1950 || y > 2200 {
		return Wert{}, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d {
		return Wert{}, false
	}
	return Wert{T: "d", V: t.Format("2006-01-02")}, true
}

func DatumDeuten(s string) (Wert, bool) {
	s = strings.TrimSpace(s)
	if m := reISO.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return datumWert(y, mo, d)
	}
	if m := reTag.FindStringSubmatch(s); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		return datumWert(jahr(m[3]), mo, d)
	}
	if m := reMonat.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		return datumWert(jahr(m[2]), mo, 1)
	}
	return Wert{}, false
}

// Eingabe aus der Oberflaeche in einen Zellwert umwandeln
func EingabeDeuten(spalte, s string) Wert {
	s = strings.TrimSpace(s)
	if s == "" {
		return Wert{}
	}
	if IstDatumSpalte(spalte) {
		if w, ok := DatumDeuten(s); ok {
			return w
		}
		return Wert{T: "s", V: s}
	}
	if gleich(spalte, spSN) && reGanz.MatchString(s) {
		return Wert{T: "n", V: s}
	}
	if gleich(spalte, spStunden) {
		t := strings.ReplaceAll(s, ".", "")
		if reGanz.MatchString(t) {
			return Wert{T: "n", V: t}
		}
		if reDezimal.MatchString(s) {
			return Wert{T: "n", V: strings.ReplaceAll(s, ",", ".")}
		}
	}
	return Wert{T: "s", V: s}
}

// Anzeige wie in Excel (Datum MM/JJJJ, Stunden mit Tausenderpunkt)
func Anzeige(spalte string, w Wert) string {
	switch w.T {
	case "":
		return ""
	case "d":
		if t, err := time.Parse("2006-01-02", w.V); err == nil {
			if IstDatumSpalte(spalte) {
				return t.Format("01/2006")
			}
			return t.Format("02.01.2006")
		}
	case "n":
		if gleich(spalte, spStunden) {
			if f, err := strconv.ParseFloat(w.V, 64); err == nil && f == math.Trunc(f) {
				return tausender(int64(f))
			}
		}
		return strings.ReplaceAll(w.V, ".", ",")
	}
	return w.V
}

func tausender(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func OhneUmlaute(s string) string {
	r := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue", "ß", "ss", "'", "")
	return r.Replace(s)
}

func (l *Liste) BetreffText() string {
	if strings.TrimSpace(l.Betreff) != "" {
		return strings.TrimSpace(l.Betreff)
	}
	t := strings.TrimSpace(l.Titel)
	if t == "" {
		t = "Geräteliste"
	}
	return OhneUmlaute(t) + " - Synchronisation"
}

func (l *Liste) DateiName() string {
	s := OhneUmlaute(strings.TrimSpace(l.Titel))
	if s == "" {
		s = "Geraeteliste"
	}
	for _, z := range []string{"\\", "/", ":", "*", "?", "\"", "<", ">", "|", " "} {
		s = strings.ReplaceAll(s, z, "_")
	}
	return s
}

func NeueID() string {
	return time.Now().Format("20060102150405") + "-" + strconv.FormatInt(time.Now().UnixNano()%1000000007, 36)
}
