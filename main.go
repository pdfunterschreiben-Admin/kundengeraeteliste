package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	portabel, warten := false, false
	for _, a := range os.Args[1:] {
		if a == "--portabel" || a == "--portable" {
			portabel = true
		}
		if a == "--nach-update" || a == "--warten" {
			warten = true
		}
	}
	AltesUpdateAufraeumen()
	daten := DatenOrdner()
	os.MkdirAll(daten, 0o755)
	urlDatei := filepath.Join(daten, "fenster.url")

	if runtime.GOOS == "windows" && !portabel && os.Getenv("KGL_DATEN") == "" {
		if installieren() {
			return
		}
		SymbolAktualisieren()
	}

	einzel := EinzelInstanz()
	for i := 0; !einzel && warten && i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		einzel = EinzelInstanz()
	}
	if !einzel {
		// Programm laeuft schon: nur das Fenster wieder oeffnen
		if b, err := os.ReadFile(urlDatei); err == nil {
			_, _ = FensterOeffnen(strings.TrimSpace(string(b)), daten)
		}
		return
	}

	sp, err := SpeicherOeffnen(daten)
	if err != nil {
		Meldung("Kundengeräteliste", "Der Datenordner kann nicht geöffnet werden:\n"+err.Error(), false)
		return
	}
	app := NeueApp(sp)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		Meldung("Kundengeräteliste", "Das Programm konnte nicht gestartet werden:\n"+err.Error(), false)
		return
	}
	url := fmt.Sprintf("http://%s/#t=%s", ln.Addr().String(), app.token)
	os.WriteFile(urlDatei, []byte(url), 0o600)
	srv := &http.Server{Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)

	go func() {
		sp.mu.Lock()
		test := sp.Einst.Testversionen
		sp.mu.Unlock()
		UpdatePruefen(test)
		for {
			time.Sleep(4 * time.Hour)
			UpdatePruefen(test)
		}
	}()

	var fensterZu <-chan struct{}
	if os.Getenv("KGL_KEIN_FENSTER") == "" {
		fensterZu, _ = FensterOeffnen(url, daten)
	} else {
		fmt.Println(url)
	}
	app.Ueberwachen(fensterZu)
}

// Das Programm endet, wenn das Programmfenster geschlossen wird.
// Ersatz (z. B. wenn das Fenster im normalen Browser laeuft): 5 Minuten ohne Lebenszeichen.
func (a *App) Ueberwachen(fensterZu <-chan struct{}) {
	frist := 5 * time.Minute
	if os.Getenv("KGL_KEIN_FENSTER") != "" {
		frist = 365 * 24 * time.Hour
	}
	start := time.Now()
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-fensterZu:
			if time.Since(start) > 15*time.Second {
				// Fenster wurde geschlossen: kurz warten, ob die Oberflaeche woanders noch offen ist
				time.Sleep(4 * time.Second)
				a.pingMu.Lock()
				letzter := a.letzterPing
				a.pingMu.Unlock()
				if time.Since(letzter) > 12*time.Second {
					a.Beenden(false)
					return
				}
			}
			fensterZu = nil // Edge hat an ein laufendes Fenster uebergeben: nur noch Lebenszeichen pruefen
		case <-tick.C:
			a.pingMu.Lock()
			letzter := a.letzterPing
			a.pingMu.Unlock()
			if letzter.IsZero() {
				if time.Since(start) > 3*time.Minute {
					a.Beenden(false)
					return
				}
				continue
			}
			if time.Since(letzter) > frist {
				a.Beenden(false)
				return
			}
		}
	}
}

// Beim Schliessen: geaenderte Listen automatisch an den Kollegen senden (wie in Excel)
func (a *App) Beenden(neustart bool) {
	a.ende.Do(func() {
		a.sp.mu.Lock()
		var offen []string
		if a.sp.Einst.AutoSyncAn() {
			for _, l := range a.sp.Alle() {
				if l.SyncOffen && strings.Contains(l.Empfaenger, "@") {
					offen = append(offen, l.ID)
				}
			}
		}
		a.sp.mu.Unlock()
		var fehlerText []string
		for _, id := range offen {
			txt, err := a.Synchronisieren(id)
			a.sp.mu.Lock()
			titel := ""
			if l := a.sp.Holen(id); l != nil {
				titel = l.Titel
			}
			a.sp.mu.Unlock()
			if err != nil {
				fehlerText = append(fehlerText, titel+": "+err.Error())
			} else if strings.Contains(txt, "übernommen:") {
				Meldung("Kundengeräteliste – "+titel, txt, false)
			}
		}
		if len(fehlerText) > 0 {
			Meldung("Kundengeräteliste", "Beim Senden der Änderungen an die Kollegen gab es ein Problem:\n\n"+strings.Join(fehlerText, "\n\n")+
				"\n\nDie Änderungen sind gespeichert und werden beim nächsten Synchronisieren gesendet.", false)
		}
		if neustart {
			if exe, err := os.Executable(); err == nil {
				NeuStarten(exe, "--nach-update")
			}
		}
		os.Exit(0)
	})
}

// Beim ersten Start: in den Benutzer-Programmordner kopieren und Verknuepfungen anlegen.
// true = installierte Version wurde gestartet, diese Instanz beendet sich.
func installieren() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, _ = filepath.EvalSymlinks(exe)
	ziel := filepath.Join(InstallOrdner(), "Kundengeraeteliste.exe")
	if strings.EqualFold(filepath.Clean(exe), filepath.Clean(ziel)) {
		return false
	}
	versionDatei := filepath.Join(InstallOrdner(), "version.txt")
	if _, err := os.Stat(ziel); err == nil {
		alt, _ := os.ReadFile(versionDatei)
		if !neuer(Version, strings.TrimSpace(string(alt))) {
			// installierte Version ist gleich oder neuer: die starten
			return NeuStarten(ziel) == nil
		}
		// diese Datei ist neuer: installierte Version ersetzen
		os.Remove(ziel + ".alt")
		if err := os.Rename(ziel, ziel+".alt"); err != nil {
			return NeuStarten(ziel) == nil
		}
		if err := dateiKopieren(exe, ziel); err != nil {
			os.Rename(ziel+".alt", ziel)
			return NeuStarten(ziel) == nil
		}
		os.WriteFile(versionDatei, []byte(Version), 0o644)
		_ = Verknuepfungen(ziel)
		return NeuStarten(ziel, "--warten") == nil
	}
	if !Meldung("Kundengeräteliste", "Kundengeräteliste jetzt installieren?\n\nDas Programm wird für deinen Benutzer eingerichtet, mit Verknüpfung auf dem Desktop und im Startmenü. Updates kommen danach automatisch.\n\n(Nein = nur dieses Mal so starten)", true) {
		return false
	}
	if err := os.MkdirAll(InstallOrdner(), 0o755); err != nil {
		Meldung("Kundengeräteliste", "Installation nicht möglich:\n"+err.Error(), false)
		return false
	}
	if err := dateiKopieren(exe, ziel); err != nil {
		Meldung("Kundengeräteliste", "Installation nicht möglich:\n"+err.Error(), false)
		return false
	}
	os.WriteFile(versionDatei, []byte(Version), 0o644)
	_ = Verknuepfungen(ziel)
	return NeuStarten(ziel, "--warten") == nil
}

func dateiKopieren(von, nach string) error {
	in, err := os.Open(von)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(nach)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
