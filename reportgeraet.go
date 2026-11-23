package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Angaben fuer ein neues Geraet aus einem Truck-Report (Dateiname + Inhalt)
type ReportAngaben struct {
	Nummer     string `json:"nummer"`
	Typ        string `json:"typ"`
	SN         string `json:"sn"`
	Hersteller string `json:"hersteller"`
	Stunden    string `json:"stunden"`
	Quelle     string `json:"quelle"` // woher die Angaben stammen
}

var (
	reRTyp     = regexp.MustCompile(`(?im)^\s*(?:truck\s*type|truck\s*model|fahrzeugtyp|modell|model|typ|type)\b\s*[:=.]?\s*['"]?([A-Z0-9][A-Z0-9 \-/]{1,20}?)['"]?\s*$`)
	reRSN      = regexp.MustCompile(`(?im)^\s*(?:serial\s*(?:no|nr|number)?|serien\s*-?\s*(?:nr|nummer)|seriennummer|serie|s/n|fabriknummer)\b\.?\s*[:=.]?\s*['"]?([A-Z0-9][A-Z0-9\-/]{2,24})['"]?`)
	reRStd     = regexp.MustCompile(`(?im)^\s*(?:hours?|hour\s*meter|operating\s*hours|betriebsstunden|stunden(?:z[aä]hler)?|h-?z[aä]hler|run\s*time)\b\.?\s*[:=.]?\s*([0-9][0-9.,]*)`)
	reTypToken = regexp.MustCompile(`^[0-9]?[A-Z]{2,}[A-Z0-9]*[0-9][A-Z0-9]*$`)
	reNrToken  = regexp.MustCompile(`(?i)^nr\.?(\d+[A-Z]?)$`)
	reDatum    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$|^\d{2}\.\d{2}\.\d{4}$|^\d{8}$`)
	// Toyota CB TruckCom: Seriennummer "9FBE16T-10301" = Typ + Nummer
	reTypMitSN = regexp.MustCompile(`^([0-9]?[A-Z]{2,}[A-Z0-9]*[0-9][A-Z0-9]*)-(\d{3,})$`)
)

func nurZiffern(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n++
		}
	}
	return n
}

// ReportAuswerten liest Typ, Seriennummer, Nummer und Betriebsstunden aus Dateiname und Inhalt.
func ReportAuswerten(pfad string, inhalt []byte) ReportAngaben {
	var a ReportAngaben
	text := string(inhalt)
	if strings.EqualFold(filepath.Ext(pfad), ".pdf") {
		text = pdfText(inhalt)
	}
	if len(inhalt) > 2 && inhalt[0] == 0xFF && inhalt[1] == 0xFE {
		text = strings.ReplaceAll(text, "\x00", "")
	}
	text = strings.ReplaceAll(text, "\r", "")
	quellen := []string{}
	if m := reRSN.FindStringSubmatch(text); m != nil {
		a.SN = strings.ToUpper(strings.TrimSpace(m[1]))
	}
	if m := reRTyp.FindStringSubmatch(text); m != nil {
		a.Typ = strings.ToUpper(strings.TrimSpace(m[1]))
	}
	if m := reRStd.FindStringSubmatch(text); m != nil {
		a.Stunden = strings.TrimRight(m[1], ".,")
	}
	if a.SN != "" || a.Typ != "" || a.Stunden != "" {
		quellen = append(quellen, "Inhalt")
	}
	// Dateiname: z. B. B_S_HF_Nr404_RRE140H___7080018.json, TruckReport_14215_9FBM35T.txt
	name := strings.TrimSuffix(filepath.Base(pfad), filepath.Ext(pfad))
	ausName := false
	var snKand string
	for _, t := range strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool { return r == '_' || r == ' ' || r == '+' || r == '(' || r == ')' }) {
		if reDatum.MatchString(t) {
			continue
		}
		if m := reNrToken.FindStringSubmatch(t); m != nil {
			if a.Nummer == "" {
				a.Nummer, ausName = m[1], true
			}
			continue
		}
		if m := reTypMitSN.FindStringSubmatch(t); m != nil {
			if a.Typ == "" {
				a.Typ, ausName = m[1], true
			}
			snKand = m[2]
			continue
		}
		if reTypToken.MatchString(t) && nurZiffern(t) < len(t)-1 && a.Typ == "" {
			a.Typ, ausName = t, true
			continue
		}
		if nurZiffern(t) >= 5 && nurZiffern(t) >= len(t)-2 {
			snKand = t
		}
	}
	if a.SN == "" && snKand != "" {
		a.SN, ausName = snKand, true
	}
	// "9FBE16T-10301" als Seriennummer aus dem Inhalt aufteilen
	if m := reTypMitSN.FindStringSubmatch(a.SN); m != nil {
		if a.Typ == "" {
			a.Typ = m[1]
		}
		a.SN = m[2]
	}
	if ausName {
		quellen = append(quellen, "Dateiname")
	}
	up := strings.ToUpper(text + " " + name + " ")
	for _, h := range hersteller {
		if strings.Contains(up, h.such) {
			a.Hersteller = h.name
			break
		}
	}
	if a.Hersteller == "" && (strings.HasPrefix(a.Typ, "RRE") || strings.HasPrefix(a.Typ, "SPE") || strings.HasPrefix(a.Typ, "LPE") || strings.HasPrefix(a.Typ, "SWE") || strings.HasPrefix(a.Typ, "OSE") || strings.HasPrefix(a.Typ, "LWE")) {
		a.Hersteller = "Toyota"
	}
	if len(a.Typ) > 3 && a.Hersteller == "" && regexp.MustCompile(`^\d?FB|^\d?FD|^\d?FG`).MatchString(a.Typ) {
		a.Hersteller = "Toyota"
	}
	a.Quelle = strings.Join(quellen, " und ")
	return a
}

func reportDateiLesen(p string) ([]byte, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.Size() > 15<<20 {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(p)
}
