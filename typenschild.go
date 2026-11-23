package main

import (
	"regexp"
	"strings"
)

// Typenschild: aus dem erkannten Text gefundene Angaben
type Typenschild struct {
	Typ        string `json:"typ"`
	SN         string `json:"sn"`
	Hersteller string `json:"hersteller"`
	Baujahr    string `json:"baujahr"`
	Text       string `json:"text"`
	Quelle     string `json:"quelle,omitempty"`
	Hinweis    string `json:"hinweis,omitempty"`
}

var hersteller = []struct{ such, name string }{
	{"TOYOTA", "Toyota"}, {"BT ", "BT"}, {"LINDE", "Linde"}, {"STILL", "Still"}, {"JUNGHEINRICH", "Jungheinrich"},
	{"HYSTER", "Hyster"}, {"YALE", "Yale"}, {"CROWN", "Crown"}, {"MITSUBISHI", "Mitsubishi"}, {"NISSAN", "Nissan"},
	{"CATERPILLAR", "Caterpillar"}, {"CLARK", "Clark"}, {"HUBTEX", "Hubtex"}, {"MANITOU", "Manitou"}, {"KOMATSU", "Komatsu"},
	{"DOOSAN", "Doosan"}, {"HELI", "Heli"}, {"HANGCHA", "Hangcha"}, {"UNICARRIERS", "UniCarriers"}, {"RAYMOND", "Raymond"},
}

var (
	reSNLabel  = regexp.MustCompile(`(?i)(?:serien[\s-]*(?:nr|nummer|no)|serial[\s-]*(?:no|nr|number)?|ser\.?\s*-?\s*nr|fabr(?:ik)?\.?[\s-]*(?:nr|nummer)|s/?n)\b\.?\s*[:.]?\s*([A-Z0-9][A-Z0-9\-/]{3,})`)
	reTypLabel = regexp.MustCompile(`(?i)(?:typ|type|modell|model)\b\.?\s*[:.]?\s*([A-Z0-9][A-Z0-9\-/ ]{2,18})`)
	reJahr     = regexp.MustCompile(`(?i)(?:baujahr|year|bj\.?|jahr)\b\.?\s*[:.]?\s*((?:19|20)\d\d)`)
)

// TypenschildAuswerten sucht Typ, Seriennummer, Hersteller und Baujahr im Text.
func TypenschildAuswerten(text string) Typenschild {
	t := Typenschild{Text: text}
	flach := strings.ReplaceAll(text, "\r", "")
	if m := reSNLabel.FindStringSubmatch(flach); m != nil {
		t.SN = strings.TrimRight(m[1], "-/")
	}
	if m := reTypLabel.FindStringSubmatch(flach); m != nil {
		typ := strings.TrimSpace(strings.SplitN(m[1], "\n", 2)[0])
		// Typ endet vor dem naechsten Beschriftungswort
		for _, stop := range []string{" SERIEN", " SERIAL", " BAUJAHR", " YEAR"} {
			if i := strings.Index(strings.ToUpper(typ), stop); i > 0 {
				typ = typ[:i]
			}
		}
		t.Typ = strings.TrimSpace(typ)
	}
	if m := reJahr.FindStringSubmatch(flach); m != nil {
		t.Baujahr = m[1]
	}
	up := strings.ToUpper(flach) + " "
	for _, h := range hersteller {
		if strings.Contains(up, h.such) {
			t.Hersteller = h.name
			break
		}
	}
	return t
}
