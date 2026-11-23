package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version wird beim Bauen gesetzt: -ldflags "-X main.Version=1.2.3"
var Version = "1.0.0"

const (
	UpdateRepo  = "pdfunterschreiben-Admin/kundengeraeteliste"
	UpdateDatei = "Kundengeraeteliste.exe"
)

type UpdateInfo struct {
	Verfuegbar bool   `json:"verfuegbar"`
	Version    string `json:"version"`
	Hinweise   string `json:"hinweise"`
	Test       bool   `json:"test"`
	url        string
	groesse    int64
	Fehler     string `json:"fehler,omitempty"`
	Geprueft   string `json:"geprueft,omitempty"`
}

var (
	updMu   sync.Mutex
	updInfo UpdateInfo
)

func versionTeile(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	for len(out) < 3 {
		out = append(out, 0)
	}
	return out
}

func neuer(a, b string) bool {
	x, y := versionTeile(a), versionTeile(b)
	for i := 0; i < 3; i++ {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

func UpdatePruefen(test bool) UpdateInfo {
	info := UpdateInfo{Geprueft: time.Now().Format("02.01.2006 15:04")}
	cl := &http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+UpdateRepo+"/releases?per_page=15", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Kundengeraeteliste/"+Version)
	resp, err := cl.Do(req)
	if err != nil {
		info.Fehler = "Keine Verbindung zum Update-Server"
		return merken(info)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		info.Fehler = fmt.Sprintf("Update-Server antwortet mit %d", resp.StatusCode)
		return merken(info)
	}
	var rel []struct {
		Tag    string `json:"tag_name"`
		Body   string `json:"body"`
		Draft  bool   `json:"draft"`
		Pre    bool   `json:"prerelease"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		info.Fehler = "Antwort des Update-Servers unlesbar"
		return merken(info)
	}
	for _, r := range rel {
		if r.Draft || (r.Pre && !test) {
			continue
		}
		for _, a := range r.Assets {
			if strings.EqualFold(a.Name, UpdateDatei) {
				if neuer(r.Tag, Version) && (!info.Verfuegbar || neuer(r.Tag, info.Version)) {
					info.Verfuegbar, info.Version, info.Hinweise, info.Test = true, strings.TrimPrefix(r.Tag, "v"), r.Body, r.Pre
					info.url, info.groesse = a.URL, a.Size
				}
			}
		}
	}
	return merken(info)
}

func merken(i UpdateInfo) UpdateInfo {
	updMu.Lock()
	updInfo = i
	updMu.Unlock()
	return i
}

func AktuellesUpdate() UpdateInfo {
	updMu.Lock()
	defer updMu.Unlock()
	return updInfo
}

// Laedt die neue Version, tauscht die .exe aus und startet neu.
func UpdateInstallieren() error {
	info := AktuellesUpdate()
	if !info.Verfuegbar || info.url == "" {
		return errors.New("kein Update verfügbar")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	cl := &http.Client{Timeout: 10 * time.Minute}
	req, _ := http.NewRequest("GET", info.url, nil)
	req.Header.Set("User-Agent", "Kundengeraeteliste/"+Version)
	resp, err := cl.Do(req)
	if err != nil {
		return errors.New("Download fehlgeschlagen")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Download fehlgeschlagen (%d)", resp.StatusCode)
	}
	neu := exe + ".neu"
	f, err := os.Create(neu)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil || (info.groesse > 0 && n != info.groesse) || n < 1<<20 {
		os.Remove(neu)
		return errors.New("Download unvollständig – bitte später erneut versuchen")
	}
	alt := exe + ".alt"
	os.Remove(alt)
	if err := os.Rename(exe, alt); err != nil {
		os.Remove(neu)
		return fmt.Errorf("Programmdatei kann nicht ersetzt werden: %v", err)
	}
	if err := os.Rename(neu, exe); err != nil {
		os.Rename(alt, exe)
		return fmt.Errorf("Programmdatei kann nicht ersetzt werden: %v", err)
	}
	os.WriteFile(filepath.Join(filepath.Dir(exe), "version.txt"), []byte(info.Version), 0o644)
	return nil
}

func AltesUpdateAufraeumen() {
	if exe, err := os.Executable(); err == nil {
		for i := 0; i < 10; i++ {
			if err := os.Remove(exe + ".alt"); err == nil || os.IsNotExist(err) {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
