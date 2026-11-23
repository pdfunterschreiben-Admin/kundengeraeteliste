//go:build !windows

package main

// Ersatz fuer Linux/Mac: nur zum Entwickeln und Testen der Oberflaeche.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Anhang struct {
	Name string
	Pfad string
}

type Mail struct {
	Absender  string
	Empfangen float64
	Anhaenge  []Anhang
}

var errNurWindows = errors.New("Outlook gibt es nur unter Windows")

func Meldung(titel, text string, frage bool) bool { fmt.Println(titel+":", text); return false }
func EinzelInstanz() bool                         { return true }
func FensterOeffnen(url, daten string) (<-chan struct{}, error) {
	fmt.Println("Oberfläche:", url)
	return nil, nil
}
func Oeffnen(p string) error      { return exec.Command("xdg-open", p).Start() }
func OrdnerZeigen(p string) error { return exec.Command("xdg-open", filepath.Dir(p)).Start() }
func OrdnerWaehlen(t string) (string, error) {
	if d := os.Getenv("KGL_TEST_ORDNER"); d != "" {
		return d, nil
	}
	return "", errors.New("Ordnerauswahl gibt es nur unter Windows – bitte Pfad eintippen")
}
func DateienInZwischenablage(p []string) error { return errNurWindows }
func WhatsAppOeffnen() error                   { return errNurWindows }
func OutlookAbrufen(betreff string, nach float64, tmp string, warten bool) ([]Mail, error) {
	return nil, errNurWindows
}
func OutlookSenden(an, betreff, text string, anh []string) error  { return errNurWindows }
func OutlookEntwurf(an, betreff, text string, anh []string) error { return errNurWindows }
func InstallOrdner() string                                       { h, _ := os.UserHomeDir(); return filepath.Join(h, ".local", "kgl") }
func Verknuepfungen(exe string) error                             { return nil }
func NeuStarten(exe string, args ...string) error                 { return exec.Command(exe, args...).Start() }
