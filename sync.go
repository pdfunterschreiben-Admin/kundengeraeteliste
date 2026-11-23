package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var syncSperren sync.Map // Listen-ID -> *sync.Mutex

func syncSperre(id string) *sync.Mutex {
	m, _ := syncSperren.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// Adressen zerlegt eine Liste von E-Mail-Adressen (getrennt durch ; , oder Zeilenumbruch)
func Adressen(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == '\n' || r == '\r' }) {
		p = strings.TrimSpace(p)
		if p != "" && !seen[strings.ToLower(p)] {
			seen[strings.ToLower(p)] = true
			out = append(out, p)
		}
	}
	return out
}

// Empf: eine empfangene Liste (ein Anhang einer Mail)
type Empf struct {
	Herkunft string
	Liste    *Liste
}

// Offen: abgeholte, noch nicht uebernommene Aenderungen einer Liste
type Offen struct {
	Mails       []Empf
	Neuest      float64
	Fehler      []string
	AnzahlMails int
	Aenderungen []Aenderung
}

// Abrufen holt neue Mails aus Outlook und liest die Anhaenge; die Liste selbst bleibt unveraendert.
func (a *App) Abrufen(id string, warten bool) (*Offen, error) {
	a.sp.mu.Lock()
	l := a.sp.Holen(id)
	if l == nil {
		a.sp.mu.Unlock()
		return nil, errors.New("Liste nicht gefunden")
	}
	betreff, letzt := l.BetreffText(), l.LetzterImport
	a.sp.mu.Unlock()

	tmp, err := os.MkdirTemp("", "kgl-sync")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	mails, err := OutlookAbrufen(betreff, letzt, tmp, warten)
	if err != nil {
		return nil, err
	}
	o := &Offen{Neuest: letzt, AnzahlMails: len(mails)}
	for _, m := range mails {
		for _, an := range m.Anhaenge {
			b, err := os.ReadFile(an.Pfad)
			if err != nil {
				continue
			}
			mp, err := MappeLesen(b)
			if err != nil {
				o.Fehler = append(o.Fehler, an.Name+": "+err.Error())
				continue
			}
			ein, err := ListeAusMappe(mp)
			if err != nil {
				o.Fehler = append(o.Fehler, an.Name+": "+err.Error())
				continue
			}
			herkunft := fmt.Sprintf("%s (%s)", m.Absender, VonSerial(m.Empfangen).Format("02.01.2006 15:04"))
			o.Mails = append(o.Mails, Empf{Herkunft: herkunft, Liste: ein})
		}
		if m.Empfangen > o.Neuest {
			o.Neuest = m.Empfangen
		}
	}
	return o, nil
}

func kopieren(l *Liste) (*Liste, error) {
	b, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	k := new(Liste)
	return k, json.Unmarshal(b, k)
}

// Vorschau berechnet, was uebernommen wuerde (an einer Kopie)
func (a *App) Vorschau(id string, o *Offen) error {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l := a.sp.Holen(id)
	if l == nil {
		return errors.New("Liste nicht gefunden")
	}
	k, err := kopieren(l)
	if err != nil {
		return err
	}
	opt := &AbgleichOpt{}
	for i, m := range o.Mails {
		opt.Praefix = fmt.Sprintf("m%d|", i)
		Abgleichen(k, m.Liste, m.Herkunft, opt)
	}
	o.Aenderungen = opt.Liste
	return nil
}

// Anwenden uebernimmt die abgeholten Aenderungen (ausser den abgelehnten) in die Liste
func (a *App) Anwenden(id string, o *Offen, abgelehnt map[string]bool) (AbgleichErgebnis, error) {
	var ges AbgleichErgebnis
	if o == nil {
		return ges, nil
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l := a.sp.Holen(id)
	if l == nil {
		return ges, errors.New("Liste nicht gefunden")
	}
	a.sp.SicherungAnlegen(l, "vor dem Übernehmen vom Kollegen")
	opt := &AbgleichOpt{Abgelehnt: abgelehnt}
	for i, m := range o.Mails {
		opt.Praefix = fmt.Sprintf("m%d|", i)
		ges.Add(Abgleichen(l, m.Liste, m.Herkunft, opt))
	}
	if o.Neuest > l.LetzterImport {
		l.LetzterImport = o.Neuest
	}
	if err := a.sp.Speichern(l); err != nil {
		return ges, err
	}
	if len(o.Fehler) > 0 {
		return ges, errors.New("Einige Anhänge konnten nicht gelesen werden: " + strings.Join(o.Fehler, "; "))
	}
	return ges, nil
}

// Empfangen: neue Staende aus dem Posteingang direkt uebernehmen
func (a *App) Empfangen(id string, warten bool) (AbgleichErgebnis, int, error) {
	o, err := a.Abrufen(id, warten)
	if err != nil {
		return AbgleichErgebnis{}, 0, err
	}
	if o.AnzahlMails == 0 {
		return AbgleichErgebnis{}, 0, nil
	}
	erg, err := a.Anwenden(id, o, nil)
	return erg, o.AnzahlMails, err
}

// Pruefen: neue Staende abholen und anzeigen, was sich aendern wuerde.
// Gibt es Aenderungen, werden sie zur Bestaetigung zurueckgehalten (a.offen);
// sonst wird sofort uebernommen.
func (a *App) Pruefen(id string) (aend []Aenderung, erg AbgleichErgebnis, mails int, err error) {
	o, err := a.Abrufen(id, true)
	if err != nil {
		return nil, erg, 0, err
	}
	if o.AnzahlMails == 0 {
		return nil, erg, 0, nil
	}
	if err = a.Vorschau(id, o); err != nil {
		return nil, erg, 0, err
	}
	if len(o.Aenderungen) > 0 {
		a.sp.mu.Lock()
		a.offen[id] = o
		a.sp.mu.Unlock()
		return o.Aenderungen, erg, o.AnzahlMails, nil
	}
	erg, err = a.Anwenden(id, o, nil)
	return nil, erg, o.AnzahlMails, err
}

// Uebernehmen: zurueckgehaltene Aenderungen uebernehmen (abgelehnte ausgenommen)
func (a *App) Uebernehmen(id string, abgelehnt []string) (AbgleichErgebnis, error) {
	a.sp.mu.Lock()
	o := a.offen[id]
	delete(a.offen, id)
	a.sp.mu.Unlock()
	if o == nil {
		return AbgleichErgebnis{}, errors.New("Es liegen keine offenen Änderungen vor. Bitte noch einmal synchronisieren.")
	}
	m := map[string]bool{}
	for _, x := range abgelehnt {
		m[x] = true
	}
	return a.Anwenden(id, o, m)
}

func (a *App) vorschauAn() bool {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	return a.sp.Einst.VorschauAn()
}

// Empfaenger fuer diesen Abgleich bestimmen: Auswahl, sonst alle gespeicherten
func (a *App) empfaengerFuer(id, an string) (string, error) {
	a.sp.mu.Lock()
	l := a.sp.Holen(id)
	if l == nil {
		a.sp.mu.Unlock()
		return "", errors.New("Liste nicht gefunden")
	}
	gespeichert := l.Empfaenger
	a.sp.mu.Unlock()
	liste := Adressen(an)
	if len(liste) == 0 {
		liste = Adressen(gespeichert)
	}
	for _, x := range liste {
		if !strings.Contains(x, "@") {
			return "", errors.New("Ungültige E-Mail-Adresse: " + x)
		}
	}
	if len(liste) == 0 {
		return "", errors.New("Bitte zuerst die E-Mail-Adresse des Kollegen in den Listen-Einstellungen eintragen.")
	}
	return strings.Join(liste, "; "), nil
}

// Synchronisieren: erst empfangen, dann eigene Liste an die Kollegen senden.
// Mit vorschau=true werden Aenderungen zuerst zur Pruefung zurueckgegeben (dann wird noch nichts gesendet).
func (a *App) Synchronisieren(id, an string, vorschau bool) (string, []Aenderung, error) {
	sp := syncSperre(id)
	if !sp.TryLock() {
		return "", nil, errors.New("Der Abgleich dieser Liste läuft bereits.")
	}
	defer sp.Unlock()
	empf, err := a.empfaengerFuer(id, an)
	if err != nil {
		return "", nil, err
	}
	var erg AbgleichErgebnis
	var mails int
	if vorschau {
		var aend []Aenderung
		aend, erg, mails, err = a.Pruefen(id)
		if err != nil {
			return "", nil, err
		}
		if len(aend) > 0 {
			return "", aend, nil
		}
	} else {
		erg, mails, err = a.Empfangen(id, true)
		if err != nil {
			return "", nil, err
		}
	}
	bericht := "Keine neuen Daten von den Kollegen im Posteingang."
	if mails > 0 {
		bericht = erg.Text()
	}
	txt, err := a.senden(id, empf)
	if err != nil {
		return bericht, nil, err
	}
	return bericht + "\n" + txt, nil, nil
}

// Senden: nur die eigene Liste senden (ohne vorher zu empfangen)
func (a *App) Senden(id, an string) (string, error) {
	sp := syncSperre(id)
	if !sp.TryLock() {
		return "", errors.New("Der Abgleich dieser Liste läuft bereits.")
	}
	defer sp.Unlock()
	empf, err := a.empfaengerFuer(id, an)
	if err != nil {
		return "", err
	}
	return a.senden(id, empf)
}

func (a *App) senden(id, empf string) (string, error) {
	a.sp.mu.Lock()
	l := a.sp.Holen(id)
	if l == nil {
		a.sp.mu.Unlock()
		return "", errors.New("Liste nicht gefunden")
	}
	daten, err := ListeAlsXlsx(l, true)
	titel, betreff, dateiname := l.Titel, l.BetreffText(), l.DateiName()
	a.sp.mu.Unlock()
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "kgl-senden")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	pfad := filepath.Join(tmp, dateiname+"_"+time.Now().Format("2006-01-02_1504")+".xlsx")
	if err := os.WriteFile(pfad, daten, 0o644); err != nil {
		return "", err
	}
	text := "Hallo,\r\n\r\nanbei der aktuelle Stand der " + titel + ".\r\n" +
		"Bitte in der eigenen Geräteliste auf 'Synchronisieren' klicken, dann wird dieser Stand automatisch übernommen.\r\n\r\n" +
		"(Automatisch gesendet aus der Kundengeräteliste)"
	if err := OutlookSenden(empf, betreff+" "+time.Now().Format("02.01.2006 15:04"), text, []string{pfad}); err != nil {
		return "", err
	}
	a.sp.mu.Lock()
	if l = a.sp.Holen(id); l != nil {
		l.SyncOffen = false
		_ = a.sp.Speichern(l)
		NutzungMelden(l, "liste_sync")
	}
	a.sp.mu.Unlock()
	return "Deine Liste wurde an " + empf + " gesendet.", nil
}
