package main

import (
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

// Empfangen: neue Staende aus dem Posteingang uebernehmen
func (a *App) Empfangen(id string, warten bool) (AbgleichErgebnis, int, error) {
	var ges AbgleichErgebnis
	a.sp.mu.Lock()
	l := a.sp.Holen(id)
	if l == nil {
		a.sp.mu.Unlock()
		return ges, 0, errors.New("Liste nicht gefunden")
	}
	betreff, letzt := l.BetreffText(), l.LetzterImport
	a.sp.mu.Unlock()

	tmp, err := os.MkdirTemp("", "kgl-sync")
	if err != nil {
		return ges, 0, err
	}
	defer os.RemoveAll(tmp)
	mails, err := OutlookAbrufen(betreff, letzt, tmp, warten)
	if err != nil {
		return ges, 0, err
	}
	if len(mails) == 0 {
		return ges, 0, nil
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l = a.sp.Holen(id)
	if l == nil {
		return ges, 0, errors.New("Liste nicht gefunden")
	}
	a.sp.SicherungAnlegen(l, "vor dem Übernehmen vom Kollegen")
	neuest := l.LetzterImport
	var fehler []string
	for _, m := range mails {
		for _, an := range m.Anhaenge {
			b, err := os.ReadFile(an.Pfad)
			if err != nil {
				continue
			}
			mp, err := MappeLesen(b)
			if err != nil {
				fehler = append(fehler, an.Name+": "+err.Error())
				continue
			}
			ein, err := ListeAusMappe(mp)
			if err != nil {
				fehler = append(fehler, an.Name+": "+err.Error())
				continue
			}
			herkunft := fmt.Sprintf("%s (%s)", m.Absender, VonSerial(m.Empfangen).Format("02.01.2006 15:04"))
			ges.Add(Abgleichen(l, ein, herkunft))
		}
		if m.Empfangen > neuest {
			neuest = m.Empfangen
		}
	}
	l.LetzterImport = neuest
	if err := a.sp.Speichern(l); err != nil {
		return ges, len(mails), err
	}
	if len(fehler) > 0 {
		return ges, len(mails), errors.New("Einige Anhänge konnten nicht gelesen werden: " + strings.Join(fehler, "; "))
	}
	return ges, len(mails), nil
}

// Synchronisieren: erst empfangen, dann eigene Liste an den Kollegen senden
func (a *App) Synchronisieren(id string) (string, error) {
	sp := syncSperre(id)
	if !sp.TryLock() {
		return "", errors.New("Der Abgleich dieser Liste läuft bereits.")
	}
	defer sp.Unlock()

	a.sp.mu.Lock()
	l := a.sp.Holen(id)
	if l == nil {
		a.sp.mu.Unlock()
		return "", errors.New("Liste nicht gefunden")
	}
	empf := strings.TrimSpace(l.Empfaenger)
	a.sp.mu.Unlock()
	if !strings.Contains(empf, "@") {
		return "", errors.New("Bitte zuerst die E-Mail-Adresse des Kollegen in den Listen-Einstellungen eintragen.")
	}
	erg, mails, err := a.Empfangen(id, true)
	if err != nil {
		return "", err
	}
	bericht := "Keine neuen Daten vom Kollegen im Posteingang."
	if mails > 0 {
		bericht = erg.Text()
	}

	a.sp.mu.Lock()
	l = a.sp.Holen(id)
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
		return bericht, err
	}
	a.sp.mu.Lock()
	if l = a.sp.Holen(id); l != nil {
		l.SyncOffen = false
		_ = a.sp.Speichern(l)
		NutzungMelden(l, "liste_sync")
	}
	a.sp.mu.Unlock()
	return bericht + "\nDeine Liste wurde an " + empf + " gesendet.", nil
}
