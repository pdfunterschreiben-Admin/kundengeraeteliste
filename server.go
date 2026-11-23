package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webDateien embed.FS

type App struct {
	sp        *Speicher
	token     string
	letzterPing time.Time
	pingMu    sync.Mutex
	beenden   chan struct{}
	gemeldet  map[string]bool
	empfangen map[string]bool
	offen     map[string]*Offen // abgeholte Aenderungen, die auf Bestaetigung warten
	ende      sync.Once
}

func NeueApp(sp *Speicher) *App {
	b := make([]byte, 16)
	rand.Read(b)
	return &App{sp: sp, token: hex.EncodeToString(b), beenden: make(chan struct{}),
		gemeldet: map[string]bool{}, empfangen: map[string]bool{}, offen: map[string]*Offen{}}
}

type apiFehler struct {
	Code int
	Text string
}

func (e apiFehler) Error() string { return e.Text }

func fehler(code int, text string) error { return apiFehler{code, text} }

func antwort(w http.ResponseWriter, v interface{}, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		code := 400
		var af apiFehler
		if errors.As(err, &af) {
			code = af.Code
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"fehler": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(v)
}

func lesen(r *http.Request, v interface{}) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webDateien, "web")
	statisch := http.FileServer(http.FS(sub))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		statisch.ServeHTTP(w, r)
	}))
	api := func(pfad string, f func(r *http.Request) (interface{}, error)) {
		mux.HandleFunc(pfad, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Token") != a.token && r.URL.Query().Get("token") != a.token {
				http.Error(w, "nicht erlaubt", 403)
				return
			}
			v, err := f(r)
			antwort(w, v, err)
		})
	}
	api("/api/status", a.apiStatus)
	api("/api/ping", func(r *http.Request) (interface{}, error) {
		a.pingMu.Lock()
		a.letzterPing = time.Now()
		a.pingMu.Unlock()
		return map[string]bool{"ok": true}, nil
	})
	api("/api/schliessen", func(r *http.Request) (interface{}, error) {
		// nur Hinweis; beendet wird ueber das Schliessen des Programmfensters
		return map[string]bool{"ok": true}, nil
	})
	api("/api/listen", a.apiListen)
	api("/api/uebersicht", a.apiUebersicht)
	api("/api/liste", a.apiListe)
	api("/api/zelle", a.apiZelle)
	api("/api/notiz", a.apiNotiz)
	api("/api/geraet", a.apiGeraet)
	api("/api/register", a.apiRegister)
	api("/api/listeneinstellungen", a.apiListenEinst)
	api("/api/neu", a.apiNeu)
	api("/api/import", a.apiImport)
	api("/api/importordner", a.apiImportOrdner)
	api("/api/beenden", func(r *http.Request) (interface{}, error) {
		go func() {
			time.Sleep(500 * time.Millisecond)
			a.Beenden(false)
		}()
		return map[string]bool{"ok": true}, nil
	})
	api("/api/einlesen", a.apiEinlesen)
	api("/api/loeschen", a.apiLoeschen)
	api("/api/sync", a.apiSync)
	api("/api/empfangen", a.apiEmpfangen)
	api("/api/uebernehmen", a.apiUebernehmen)
	api("/api/protokoll", a.apiProtokoll)
	api("/api/verlauf", a.apiVerlauf)
	api("/api/typenschild", a.apiTypenschild)
	api("/api/pruefen", a.apiListePruefen)
	api("/api/sicherungsvorschau", a.apiSicherungsVorschau)
	api("/api/sicherungen", a.apiSicherungen)
	api("/api/wiederherstellen", a.apiWiederherstellen)
	api("/api/sicherungsordner", func(r *http.Request) (interface{}, error) {
		return nil, OrdnerZeigen(a.sp.SicherungsOrdner())
	})
	api("/api/datenordner", func(r *http.Request) (interface{}, error) {
		return nil, OrdnerZeigen(a.sp.Basis)
	})
	api("/api/einstellungen", a.apiEinstellungen)
	api("/api/reports", a.apiReports)
	api("/api/reportordner", a.apiReportOrdner)
	api("/api/reportaktion", a.apiReportAktion)
	api("/api/update", a.apiUpdate)
	mux.HandleFunc("/api/export", a.dlExport)
	return mux
}

// ---------- Status / Einstellungen ----------

func (a *App) apiStatus(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	return map[string]interface{}{
		"version": Version, "benutzer": BenutzerName(), "update": AktuellesUpdate(),
		"einstellungen": a.sp.Einst, "autoSync": a.sp.Einst.AutoSyncAn(), "vorschau": a.sp.Einst.VorschauAn(), "daten": a.sp.Basis,
	}, nil
}

func (a *App) apiEinstellungen(r *http.Request) (interface{}, error) {
	var in struct {
		ReportOrdner  *string `json:"reportOrdner"`
		AutoSync      *bool   `json:"autoSync"`
		Vorschau      *bool   `json:"vorschau"`
		Testversionen *bool   `json:"testversionen"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	if in.ReportOrdner != nil {
		a.sp.Einst.ReportOrdner = strings.TrimSpace(*in.ReportOrdner)
	}
	if in.AutoSync != nil {
		a.sp.Einst.AutoSync = in.AutoSync
	}
	if in.Vorschau != nil {
		a.sp.Einst.Vorschau = in.Vorschau
	}
	if in.Testversionen != nil {
		a.sp.Einst.Testversionen = *in.Testversionen
	}
	return a.sp.Einst, a.sp.EinstSpeichern()
}

// ---------- Listen ----------

type ListenInfo struct {
	ID           string   `json:"id"`
	Titel        string   `json:"titel"`
	Geraete      int      `json:"geraete"`
	Ueberfaellig int      `json:"ueberfaellig"`
	Faellig      int      `json:"faellig"`
	Fehlend      int      `json:"fehlend"`
	Register     []string `json:"register"`
	Empfaenger   string   `json:"empfaenger"`
	SyncOffen    bool     `json:"syncOffen"`
	Geaendert    string   `json:"geaendert"`
}

func uvvStatus(r *Register, z *Zeile, heute time.Time) string {
	if r.Wert(z, spSN).Leer() {
		return ""
	}
	n, ok := r.NaechsteUVV(z)
	if !ok {
		return "fehlt"
	}
	if n.Before(heute) {
		return "ueberfaellig"
	}
	if !n.After(heute.AddDate(0, 0, 30)) {
		return "faellig"
	}
	return "ok"
}

func heute() time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func info(l *Liste) ListenInfo {
	i := ListenInfo{ID: l.ID, Titel: l.Titel, Empfaenger: l.Empfaenger, SyncOffen: l.SyncOffen, Geaendert: l.Geaendert}
	h := heute()
	for _, r := range l.Register {
		i.Register = append(i.Register, r.Name)
		for _, z := range r.Zeilen {
			switch uvvStatus(r, z, h) {
			case "ueberfaellig":
				i.Ueberfaellig++
			case "faellig":
				i.Faellig++
			case "fehlt":
				i.Fehlend++
			}
			if !r.Wert(z, spSN).Leer() {
				i.Geraete++
			}
		}
	}
	return i
}

func (a *App) apiListen(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	out := []ListenInfo{}
	for _, l := range a.sp.Alle() {
		out = append(out, info(l))
	}
	return map[string]interface{}{"listen": out, "geloescht": a.sp.GeloeschteListen()}, nil
}

type Faelligkeit struct {
	ListeID  string `json:"listeId"`
	Liste    string `json:"liste"`
	Register string `json:"register"`
	RegIndex int    `json:"regIndex"`
	ZeileID  string `json:"zeileId"`
	Nummer   string `json:"nummer"`
	Typ      string `json:"typ"`
	SN       string `json:"sn"`
	Naechste string `json:"naechste"`
	Status   string `json:"status"`
	Tage     int    `json:"tage"`
}

func (a *App) apiUebersicht(r *http.Request) (interface{}, error) {
	tage := 60
	fmt.Sscan(r.URL.Query().Get("tage"), &tage)
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	h := heute()
	out := []Faelligkeit{}
	for _, l := range a.sp.Alle() {
		for ri, reg := range l.Register {
			for _, z := range reg.Zeilen {
				if reg.Wert(z, spSN).Leer() {
					continue
				}
				n, ok := reg.NaechsteUVV(z)
				if !ok || n.After(h.AddDate(0, 0, tage)) {
					continue
				}
				out = append(out, Faelligkeit{l.ID, l.Titel, reg.Name, ri, z.ID, reg.Wert(z, spNummer).Text(), reg.Wert(z, "Typ").Text(),
					reg.Wert(z, spSN).Text(), n.Format("2006-01-02"), uvvStatus(reg, z, h), int(n.Sub(h).Hours() / 24)})
			}
		}
	}
	return out, nil
}

func (a *App) liste(r *http.Request) (*Liste, error) {
	id := r.URL.Query().Get("id")
	l := a.sp.Holen(id)
	if l == nil {
		return nil, fehler(404, "Liste nicht gefunden")
	}
	return l, nil
}

func (a *App) apiListe(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	if !a.gemeldet[l.ID] {
		a.gemeldet[l.ID] = true
		if l.ListenID == "" {
			l.ListenID = time.Now().Format("20060102150405") + "-" + strings.ToUpper(hex.EncodeToString([]byte(l.ID))[:8])
			_ = a.sp.Speichern(l)
		}
		NutzungMelden(l, "liste_start")
	}
	kopie := *l
	kopie.Protokoll = nil
	kopie.Zeit = nil
	return map[string]interface{}{"liste": kopie, "info": info(l), "betreff": l.BetreffText(), "protokollAnzahl": len(l.Protokoll),
		"reportOrdner": a.reportOrdner(l)}, nil
}

func (a *App) register(l *Liste, i int) (*Register, error) {
	if i < 0 || i >= len(l.Register) {
		return nil, errors.New("Register nicht gefunden")
	}
	return l.Register[i], nil
}

// gibt es schon ein anderes Geraet mit gleicher Seriennummer UND gleichem Typ? (Name des Registers)
func gleichesGeraet(l *Liste, ich *Zeile, sn, typ string) string {
	k, t := Schluessel(sn), typText(typ)
	if k == "|" {
		return ""
	}
	for _, rr := range l.Register {
		for _, x := range rr.Zeilen {
			if x != ich && Schluessel(rr.Wert(x, spSN).Text()) == k && typText(rr.Wert(x, "Typ").Text()) == t {
				return rr.Name
			}
		}
	}
	return ""
}

func zeileSuchen(reg *Register, id string) (*Zeile, int) {
	for i, z := range reg.Zeilen {
		if z.ID == id {
			return z, i
		}
	}
	return nil, -1
}

func (a *App) apiZelle(r *http.Request) (interface{}, error) {
	var in struct {
		Register int    `json:"register"`
		Zeile    string `json:"zeile"`
		Spalte   int    `json:"spalte"`
		Text     string `json:"text"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	reg, err := a.register(l, in.Register)
	if err != nil {
		return nil, err
	}
	z, _ := zeileSuchen(reg, in.Zeile)
	if z == nil || in.Spalte < 0 || in.Spalte >= len(reg.Spalten) {
		return nil, errors.New("Zelle nicht gefunden")
	}
	sp := reg.Spalten[in.Spalte]
	if IstFormelSpalte(sp) {
		return nil, errors.New("„Nächste UVV“ wird automatisch berechnet (letzte UVV + 12 Monate).")
	}
	for len(z.Werte) < len(reg.Spalten) {
		z.Werte = append(z.Werte, Wert{})
	}
	neu := EingabeDeuten(sp, in.Text)
	if gleich(sp, spSN) && neu.Leer() {
		return nil, errors.New("Die Seriennummer darf nicht leer sein – daran wird das Gerät erkannt.")
	}
	if (gleich(sp, spSN) && !neu.Leer()) || gleich(sp, "Typ") {
		sn, typ := reg.Wert(z, spSN).Text(), reg.Wert(z, "Typ").Text()
		if gleich(sp, spSN) {
			sn = neu.Text()
		} else {
			typ = neu.Text()
		}
		if x := gleichesGeraet(l, z, sn, typ); x != "" {
			return nil, fmt.Errorf("Ein Gerät mit Seriennummer %s und Typ %s gibt es in dieser Liste schon (Register %s). Bei gleicher Seriennummer muss sich der Typ unterscheiden.", sn, typ, x)
		}
	}
	if z.Werte[in.Spalte].Text() != neu.Text() || z.Werte[in.Spalte].T != neu.T {
		alt := protokollWert(sp, z.Werte[in.Spalte])
		z.Werte[in.Spalte] = neu
		if alt != protokollWert(sp, neu) {
			l.Lokal(reg, z, sp, alt, protokollWert(sp, neu))
		}
		ts := Jetzt()
		if gleich(sp, spSN) {
			// neue Seriennummer = fuer den Kollegen ein neues Geraet: alle Felder mit Zeit versehen
			for _, s := range reg.Spalten {
				if !IstFormelSpalte(s) {
					l.ZeitSetzen(reg, z, s, ts)
				}
			}
		} else {
			l.ZeitSetzen(reg, z, sp, ts)
		}
		l.SyncOffen = true
		if err := a.sp.Speichern(l); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"zeile": z, "info": info(l)}, nil
}

func (a *App) apiNotiz(r *http.Request) (interface{}, error) {
	var in struct {
		Register int    `json:"register"`
		Zeile    string `json:"zeile"`
		Spalte   string `json:"spalte"`
		Text     string `json:"text"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	reg, err := a.register(l, in.Register)
	if err != nil {
		return nil, err
	}
	z, _ := zeileSuchen(reg, in.Zeile)
	if z == nil {
		return nil, errors.New("Gerät nicht gefunden")
	}
	t := strings.TrimSpace(in.Text)
	if t == "" {
		delete(z.Notizen, in.Spalte)
	} else {
		if z.Notizen == nil {
			z.Notizen = map[string]string{}
		}
		z.Notizen[in.Spalte] = t
	}
	return map[string]interface{}{"zeile": z}, a.sp.Speichern(l)
}

func (a *App) apiGeraet(r *http.Request) (interface{}, error) {
	var in struct {
		Aktion   string            `json:"aktion"` // neu | loeschen | verschieben
		Register int               `json:"register"`
		Zeile    string            `json:"zeile"`
		Werte    map[string]string `json:"werte"`
		Ziel     int               `json:"ziel"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	reg, err := a.register(l, in.Register)
	if err != nil {
		return nil, err
	}
	switch in.Aktion {
	case "neu":
		z := &Zeile{ID: NeueID(), Werte: make([]Wert, len(reg.Spalten))}
		for k, v := range in.Werte {
			if i := reg.Spalte(k); i >= 0 && !IstFormelSpalte(reg.Spalten[i]) {
				z.Werte[i] = EingabeDeuten(reg.Spalten[i], v)
			}
		}
		sn := reg.Wert(z, spSN)
		if sn.Leer() {
			return nil, errors.New("Bitte eine Seriennummer eintragen – daran wird das Gerät erkannt.")
		}
		if x := gleichesGeraet(l, z, sn.Text(), reg.Wert(z, "Typ").Text()); x != "" {
			return nil, fmt.Errorf("Ein Gerät mit Seriennummer %s und gleichem Typ gibt es in dieser Liste schon (Register %s). Bei gleicher Seriennummer muss sich der Typ unterscheiden.", sn.V, x)
		}
		reg.Zeilen = append(reg.Zeilen, z)
		l.Lokal(reg, z, "(Gerät hinzugefügt)", "", "")
		ts := Jetzt()
		for i, s := range reg.Spalten {
			if !IstFormelSpalte(s) && !z.Werte[i].Leer() {
				l.ZeitSetzen(reg, z, s, ts)
			}
		}
		l.SyncOffen = true
		return map[string]interface{}{"zeile": z, "info": info(l)}, a.sp.Speichern(l)
	case "loeschen":
		z0, i := zeileSuchen(reg, in.Zeile)
		if i < 0 {
			return nil, errors.New("Gerät nicht gefunden")
		}
		a.sp.SicherungAnlegen(l, "vor dem Löschen eines Geräts")
		l.Lokal(reg, z0, "(Gerät gelöscht)", "", "")
		reg.Zeilen = append(reg.Zeilen[:i], reg.Zeilen[i+1:]...)
		return map[string]interface{}{"info": info(l)}, a.sp.Speichern(l)
	case "verschieben":
		ziel, err := a.register(l, in.Ziel)
		if err != nil {
			return nil, err
		}
		z, i := zeileSuchen(reg, in.Zeile)
		if i < 0 || ziel == reg {
			return nil, errors.New("Gerät nicht gefunden")
		}
		nz := &Zeile{ID: z.ID, Werte: make([]Wert, len(ziel.Spalten)), Notizen: z.Notizen}
		ts := Jetzt()
		for k, s := range reg.Spalten {
			if j := ziel.Spalte(s); j >= 0 && k < len(z.Werte) {
				nz.Werte[j] = z.Werte[k]
			}
		}
		reg.Zeilen = append(reg.Zeilen[:i], reg.Zeilen[i+1:]...)
		ziel.Zeilen = append(ziel.Zeilen, nz)
		for j, s := range ziel.Spalten {
			if !IstFormelSpalte(s) && !nz.Werte[j].Leer() {
				l.ZeitSetzen(ziel, nz, s, ts)
			}
		}
		l.SyncOffen = true
		return map[string]interface{}{"info": info(l)}, a.sp.Speichern(l)
	}
	return nil, errors.New("unbekannte Aktion")
}

func (a *App) apiRegister(r *http.Request) (interface{}, error) {
	var in struct {
		Aktion string `json:"aktion"` // neu | umbenennen | loeschen | notiz
		Index  int    `json:"index"`
		Name   string `json:"name"`
		Notiz  string `json:"notiz"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	switch in.Aktion {
	case "neu":
		if name == "" {
			return nil, errors.New("Bitte einen Namen für das Register (z. B. Standort) eingeben.")
		}
		sp := append([]string{}, SpaltenStandard...)
		if len(l.Register) > 0 {
			sp = append([]string{}, l.Register[0].Spalten...)
		}
		l.Register = append(l.Register, &Register{Name: name, Spalten: sp})
		TabellenPruefen(l)
	case "umbenennen":
		reg, err := a.register(l, in.Index)
		if err != nil {
			return nil, err
		}
		if name == "" {
			return nil, errors.New("Name fehlt")
		}
		reg.Name = name // Tabellenname bleibt (wichtig fuer den Abgleich)
	case "notiz":
		reg, err := a.register(l, in.Index)
		if err != nil {
			return nil, err
		}
		reg.Notiz = strings.TrimSpace(in.Notiz)
	case "loeschen":
		reg, err := a.register(l, in.Index)
		if err != nil {
			return nil, err
		}
		if len(l.Register) == 1 {
			return nil, errors.New("Das letzte Register kann nicht gelöscht werden.")
		}
		a.sp.SicherungAnlegen(l, "vor dem Löschen des Registers "+reg.Name)
		l.Register = append(l.Register[:in.Index], l.Register[in.Index+1:]...)
		TabellenPruefen(l)
	default:
		return nil, errors.New("unbekannte Aktion")
	}
	return map[string]interface{}{"info": info(l)}, a.sp.Speichern(l)
}

func (a *App) apiListenEinst(r *http.Request) (interface{}, error) {
	var in struct {
		Titel        *string `json:"titel"`
		Untertitel   *string `json:"untertitel"`
		Empfaenger   *string `json:"empfaenger"`
		Betreff      *string `json:"betreff"`
		ReportOrdner *string `json:"reportOrdner"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	if in.Titel != nil {
		t := strings.TrimSpace(*in.Titel)
		if t == "" {
			return nil, errors.New("Der Titel darf nicht leer sein.")
		}
		// Betreff festhalten, damit der Abgleich mit dem Kollegen weiter funktioniert
		if l.Betreff == "" && t != l.Titel {
			l.Betreff = l.BetreffText()
		}
		l.Titel = t
	}
	if in.Untertitel != nil {
		l.Untertitel = strings.TrimSpace(*in.Untertitel)
	}
	if in.Empfaenger != nil {
		adr := Adressen(*in.Empfaenger)
		for _, x := range adr {
			if !strings.Contains(x, "@") || strings.ContainsAny(x, " <>") {
				return nil, errors.New("Das ist keine gültige E-Mail-Adresse: " + x)
			}
		}
		l.Empfaenger = strings.Join(adr, "; ")
	}
	if in.Betreff != nil {
		l.Betreff = strings.TrimSpace(*in.Betreff)
		if l.Betreff != "" && !strings.HasSuffix(l.Betreff, "- Synchronisation") {
			l.Betreff = strings.TrimSpace(l.Betreff) + " - Synchronisation"
		}
	}
	if in.ReportOrdner != nil {
		l.ReportOrdner = strings.TrimSpace(*in.ReportOrdner)
	}
	return map[string]interface{}{"info": info(l), "betreff": l.BetreffText()}, a.sp.Speichern(l)
}

func (a *App) apiNeu(r *http.Request) (interface{}, error) {
	var in struct {
		Titel      string   `json:"titel"`
		Aufbau     string   `json:"aufbau"` // standard | erweitert
		Register   []string `json:"register"`
		Empfaenger string   `json:"empfaenger"`
		Spalten    []string `json:"spalten"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	t := strings.TrimSpace(in.Titel)
	if t == "" {
		return nil, errors.New("Bitte den Kundennamen eingeben.")
	}
	if !strings.HasPrefix(strings.ToLower(t), "geräteliste") {
		t = "Geräteliste " + t
	}
	sp := SpaltenStandard
	if in.Aufbau == "erweitert" {
		sp = SpaltenErweitert
	}
	if len(in.Spalten) > 0 {
		sp = in.Spalten
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	for _, l := range a.sp.Alle() {
		if strings.EqualFold(l.Titel, t) {
			return nil, fmt.Errorf("Es gibt schon eine Liste „%s“.", t)
		}
	}
	l := &Liste{Titel: t, Herkunft: "Vorlage", Zeit: map[string]float64{}, Empfaenger: strings.Join(Adressen(in.Empfaenger), "; ")}
	namen := in.Register
	if len(namen) == 0 {
		namen = []string{"Geräteliste"}
	}
	for _, n := range namen {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		l.Register = append(l.Register, &Register{Name: n, Spalten: append([]string{}, sp...)})
	}
	if len(l.Register) == 0 {
		l.Register = []*Register{{Name: "Geräteliste", Spalten: append([]string{}, sp...)}}
	}
	TabellenPruefen(l)
	l.ListenID = time.Now().Format("20060102150405") + "-" + strings.ToUpper(hex.EncodeToString([]byte(t))[:6])
	if err := a.sp.Speichern(l); err != nil {
		return nil, err
	}
	a.gemeldet[l.ID] = true
	NutzungMelden(l, "liste_vorlage")
	NutzungMelden(l, "liste_start")
	return map[string]string{"id": l.ID}, nil
}

// ---------- Import / Einlesen ----------

type ImportErgebnis struct {
	Datei   string `json:"datei"`
	ID      string `json:"id,omitempty"`
	Titel   string `json:"titel,omitempty"`
	Geraete int    `json:"geraete"`
	Status  string `json:"status"` // neu | ersetzt | abgeglichen | vorhanden | fehler
	Text    string `json:"text"`
	Vorhanden string `json:"vorhanden,omitempty"`
	Pfad      string `json:"pfad,omitempty"`
}

func dateienLesen(r *http.Request) (map[string][]byte, []string, error) {
	if err := r.ParseMultipartForm(200 << 20); err != nil {
		return nil, nil, errors.New("Dateien konnten nicht übertragen werden")
	}
	out := map[string][]byte{}
	var namen []string
	for _, fh := range r.MultipartForm.File["dateien"] {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(f)
		f.Close()
		out[fh.Filename] = b
		namen = append(namen, fh.Filename)
	}
	if len(namen) == 0 {
		return nil, nil, errors.New("keine Datei ausgewählt")
	}
	return out, namen, nil
}

func (a *App) apiImport(r *http.Request) (interface{}, error) {
	dateien, namen, err := dateienLesen(r)
	if err != nil {
		return nil, err
	}
	return a.importieren(dateien, namen, nil, r.FormValue("modus"), r.FormValue("ziel"))
}

// Import aus einem Ordner (Ordner-Dialog) oder einer einzelnen Datei per Pfad
func (a *App) apiImportOrdner(r *http.Request) (interface{}, error) {
	var in struct {
		Waehlen bool   `json:"waehlen"`
		Pfad    string `json:"pfad"`  // Ordner
		Datei   string `json:"datei"` // einzelne Datei (zweiter Schritt bei "vorhanden")
		Modus   string `json:"modus"`
		Ziel    string `json:"ziel"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	var pfade []string
	if in.Datei != "" {
		pfade = []string{in.Datei}
	} else {
		ordner := strings.TrimSpace(in.Pfad)
		if in.Waehlen {
			var err error
			ordner, err = OrdnerWaehlen("Ordner mit den Excel-Gerätelisten auswählen")
			if err != nil {
				return nil, err
			}
			if ordner == "" {
				return []ImportErgebnis{}, nil
			}
		}
		for _, muster := range []string{"*.xlsm", "*.xlsx"} {
			f, _ := filepath.Glob(filepath.Join(ordner, muster))
			pfade = append(pfade, f...)
		}
		if len(pfade) == 0 {
			return nil, errors.New("In diesem Ordner liegen keine Excel-Dateien (.xlsm/.xlsx): " + ordner)
		}
	}
	dateien := map[string][]byte{}
	var namen []string
	quelle := map[string]string{}
	for _, p := range pfade {
		if strings.HasPrefix(filepath.Base(p), "~$") {
			continue // Excel-Sperrdatei
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		n := filepath.Base(p)
		dateien[n] = b
		namen = append(namen, n)
		quelle[n] = p
	}
	return a.importieren(dateien, namen, quelle, in.Modus, in.Ziel)
}

func (a *App) importieren(dateien map[string][]byte, namen []string, quelle map[string]string, modus, ziel string) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	out := []ImportErgebnis{}
	for _, n := range namen {
		e := ImportErgebnis{Datei: n, Pfad: quelle[n]}
		m, err := MappeLesen(dateien[n])
		var l *Liste
		if err == nil {
			l, err = ListeAusMappe(m)
		}
		if err != nil {
			e.Status, e.Text = "fehler", err.Error()
			out = append(out, e)
			continue
		}
		l.Quelle = n
		if anzahlGeraete(l) == 0 && strings.Contains(strings.ToLower(l.Titel), "kundenname") {
			e.Status, e.Text = "fehler", "Das ist die leere Vorlage – neue Listen legst du im Programm über „+ Neu“ an."
			out = append(out, e)
			continue
		}
		TabellenPruefen(l)
		e.Titel, e.Geraete = l.Titel, anzahlGeraete(l)
		var vorhanden *Liste
		for _, x := range a.sp.Alle() {
			if (l.ListenID != "" && x.ListenID == l.ListenID) || strings.EqualFold(x.Titel, l.Titel) {
				vorhanden = x
			}
		}
		if ziel != "" {
			vorhanden = a.sp.Holen(ziel)
		}
		if vorhanden != nil && modus == "" {
			e.Status, e.Vorhanden = "vorhanden", vorhanden.ID
			e.Text = "Diese Liste gibt es schon im Programm."
			out = append(out, e)
			continue
		}
		switch {
		case vorhanden != nil && modus == "ersetzen":
			a.sp.SicherungAnlegen(vorhanden, "vor dem Ersetzen durch "+n)
			l.ID = vorhanden.ID
			l.ReportOrdner = vorhanden.ReportOrdner
			e.Status, e.Text = "ersetzt", "Liste wurde durch die Datei ersetzt."
		case vorhanden != nil && modus == "abgleichen":
			a.sp.SicherungAnlegen(vorhanden, "vor dem Einlesen von "+n)
			erg := Abgleichen(vorhanden, l, "Datei: "+n)
			l = vorhanden
			e.Status, e.Text = "abgeglichen", erg.Text()
		default:
			if vorhanden != nil && l.ListenID == vorhanden.ListenID {
				l.ListenID = ""
			}
			e.Status, e.Text = "neu", "Liste wurde übernommen."
		}
		if err := a.sp.Speichern(l); err != nil {
			return nil, err
		}
		e.ID = l.ID
		e.Geraete = anzahlGeraete(l)
		out = append(out, e)
	}
	return out, nil
}

// "Datei einlesen": empfangene Liste von Hand abgleichen
func (a *App) apiEinlesen(r *http.Request) (interface{}, error) {
	dateien, namen, err := dateienLesen(r)
	if err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	a.sp.SicherungAnlegen(l, "vor dem Einlesen einer Datei")
	var ges AbgleichErgebnis
	for _, n := range namen {
		m, err := MappeLesen(dateien[n])
		if err != nil {
			return nil, fmt.Errorf("%s: %v", n, err)
		}
		ein, err := ListeAusMappe(m)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", n, err)
		}
		ges.Add(Abgleichen(l, ein, "Datei: "+n))
	}
	return map[string]interface{}{"text": ges.Text(), "ergebnis": ges, "info": info(l)}, a.sp.Speichern(l)
}

func (a *App) apiLoeschen(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	return nil, a.sp.Loeschen(r.URL.Query().Get("id"))
}

func (a *App) dlExport(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") != a.token {
		http.Error(w, "nicht erlaubt", 403)
		return
	}
	a.sp.mu.Lock()
	l := a.sp.Holen(r.URL.Query().Get("id"))
	var b []byte
	var err error
	name := ""
	if l != nil {
		b, err = ListeAlsXlsx(l, true)
		name = l.DateiName() + "_" + time.Now().Format("2006-01-02") + ".xlsx"
	}
	a.sp.mu.Unlock()
	if l == nil || err != nil {
		http.Error(w, "Export fehlgeschlagen", 500)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+OhneUmlaute(name)+"\"; filename*=UTF-8''"+url.PathEscape(name))
	w.Write(b)
}

// ---------- Abgleich ----------

func (a *App) apiSync(r *http.Request) (interface{}, error) {
	id := r.URL.Query().Get("id")
	var in struct {
		An        string `json:"an"`
		NurSenden bool   `json:"nurSenden"`
	}
	_ = lesen(r, &in)
	var txt string
	var aend []Aenderung
	var err error
	if in.NurSenden {
		txt, err = a.Senden(id, in.An)
	} else {
		txt, aend, err = a.Synchronisieren(id, in.An, a.vorschauAn())
	}
	if err != nil {
		if txt != "" {
			return nil, errors.New(txt + "\n\n" + err.Error())
		}
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l := a.sp.Holen(id)
	if l == nil {
		return nil, fehler(404, "Liste nicht gefunden")
	}
	return map[string]interface{}{"text": txt, "info": info(l), "aenderungen": aend}, nil
}

// zurueckgehaltene Aenderungen uebernehmen (abgelehnte ausgenommen)
func (a *App) apiUebernehmen(r *http.Request) (interface{}, error) {
	id := r.URL.Query().Get("id")
	var in struct {
		Abgelehnt []string `json:"abgelehnt"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	sp := syncSperre(id)
	sp.Lock()
	defer sp.Unlock()
	erg, err := a.Uebernehmen(id, in.Abgelehnt)
	if err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l := a.sp.Holen(id)
	if l == nil {
		return nil, fehler(404, "Liste nicht gefunden")
	}
	return map[string]interface{}{"text": erg.Text(), "info": info(l)}, nil
}

// beim Oeffnen einer Liste: neue Daten vom Kollegen holen (nur empfangen)
func (a *App) apiEmpfangen(r *http.Request) (interface{}, error) {
	id := r.URL.Query().Get("id")
	a.sp.mu.Lock()
	l := a.sp.Holen(id)
	auto := a.sp.Einst.AutoSyncAn()
	schon := a.empfangen[id]
	a.empfangen[id] = true
	a.sp.mu.Unlock()
	if l == nil {
		return nil, fehler(404, "Liste nicht gefunden")
	}
	if !auto || schon || !strings.Contains(l.Empfaenger, "@") {
		return map[string]interface{}{"mails": 0}, nil
	}
	sp := syncSperre(id)
	if !sp.TryLock() {
		return map[string]interface{}{"mails": 0}, nil
	}
	defer sp.Unlock()
	if a.vorschauAn() {
		aend, erg, n, err := a.Pruefen(id)
		if err != nil {
			return map[string]interface{}{"mails": 0, "hinweis": err.Error()}, nil
		}
		if len(aend) > 0 {
			return map[string]interface{}{"mails": n, "aenderungen": aend}, nil
		}
		txt := ""
		if erg.Geraete > 0 || erg.Neu > 0 {
			txt = erg.Text()
		}
		return map[string]interface{}{"mails": n, "text": txt, "ergebnis": erg}, nil
	}
	erg, n, err := a.Empfangen(id, true)
	if err != nil {
		return map[string]interface{}{"mails": 0, "hinweis": err.Error()}, nil
	}
	txt := ""
	if erg.Geraete > 0 || erg.Neu > 0 {
		txt = erg.Text()
	}
	return map[string]interface{}{"mails": n, "text": txt, "ergebnis": erg}, nil
}

func (a *App) apiProtokoll(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	out := []ProtokollEintrag{}
	for i := len(l.Protokoll) - 1; i >= 0; i-- {
		if l.Protokoll[i].Art != "lokal" {
			out = append(out, l.Protokoll[i])
		}
	}
	return out, nil
}

// Verlauf eines Geraets: eigene und uebernommene Aenderungen, neueste zuerst
func (a *App) apiVerlauf(r *http.Request) (interface{}, error) {
	var in struct {
		Register int    `json:"register"`
		Zeile    string `json:"zeile"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	reg, err := a.register(l, in.Register)
	if err != nil {
		return nil, err
	}
	z, _ := zeileSuchen(reg, in.Zeile)
	if z == nil {
		return nil, errors.New("Gerät nicht gefunden")
	}
	sn, typ := reg.Wert(z, spSN).Text(), reg.Wert(z, "Typ").Text()
	out := []ProtokollEintrag{}
	for i := len(l.Protokoll) - 1; i >= 0 && len(out) < 300; i-- {
		e := l.Protokoll[i]
		if e.SN == sn && sn != "" && (e.Typ == "" || typ == "" || e.Typ == typ) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (a *App) apiSicherungen(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	s := a.sp.Sicherungen(r.URL.Query().Get("id"))
	if s == nil {
		s = []Sicherung{}
	}
	return s, nil
}

func (a *App) apiWiederherstellen(r *http.Request) (interface{}, error) {
	var in struct {
		ID    string `json:"id"`
		Datei string `json:"datei"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.sp.Wiederherstellen(in.ID, in.Datei)
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": l.ID}, nil
}

// ---------- Truck-Reports ----------

func (a *App) reportOrdner(l *Liste) string {
	if l != nil && l.ReportOrdner != "" {
		return l.ReportOrdner
	}
	return a.sp.Einst.ReportOrdner
}

func (a *App) apiReports(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	l, err := a.liste(r)
	if err != nil {
		a.sp.mu.Unlock()
		return nil, err
	}
	ordner := a.reportOrdner(l)
	kopie := &Liste{Register: l.Register}
	data, _ := json.Marshal(kopie)
	var k Liste
	json.Unmarshal(data, &k)
	a.sp.mu.Unlock()
	if ordner == "" {
		return map[string]interface{}{"ordner": "", "reports": []Report{}}, nil
	}
	rep, err := ReportsSuchen(&k, ordner)
	if err != nil {
		return map[string]interface{}{"ordner": ordner, "reports": []Report{}, "fehler": "Ordner nicht erreichbar: " + ordner}, nil
	}
	if rep == nil {
		rep = []Report{}
	}
	return map[string]interface{}{"ordner": ordner, "reports": rep}, nil
}

func (a *App) apiReportOrdner(r *http.Request) (interface{}, error) {
	var in struct {
		Pfad    string `json:"pfad"`
		Waehlen bool   `json:"waehlen"`
		Liste   string `json:"liste"` // leer = allgemein
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	p := strings.TrimSpace(in.Pfad)
	if in.Waehlen {
		var err error
		p, err = OrdnerWaehlen("Ordner mit den Truck-Reports auswählen")
		if err != nil {
			return nil, err
		}
		if p == "" {
			return map[string]string{"pfad": ""}, nil
		}
	}
	if p != "" {
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return nil, errors.New("Der Ordner wurde nicht gefunden: " + p)
		}
	}
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	if in.Liste != "" {
		l := a.sp.Holen(in.Liste)
		if l == nil {
			return nil, fehler(404, "Liste nicht gefunden")
		}
		l.ReportOrdner = p
		return map[string]string{"pfad": p}, a.sp.Speichern(l)
	}
	a.sp.Einst.ReportOrdner = p
	return map[string]string{"pfad": p}, a.sp.EinstSpeichern()
}

func (a *App) apiReportAktion(r *http.Request) (interface{}, error) {
	var in struct {
		Liste   string   `json:"liste"`
		Aktion  string   `json:"aktion"` // oeffnen | ordner | mail | whatsapp | kopieren
		Dateien []string `json:"dateien"`
		Text    string   `json:"text"`
	}
	if err := lesen(r, &in); err != nil {
		return nil, err
	}
	a.sp.mu.Lock()
	l := a.sp.Holen(in.Liste)
	ordner := a.reportOrdner(l)
	titel := ""
	if l != nil {
		titel = l.Titel
	}
	a.sp.mu.Unlock()
	if len(in.Dateien) == 0 {
		return nil, errors.New("Bitte mindestens einen Report auswählen.")
	}
	for _, d := range in.Dateien {
		if !imOrdner(d, ordner) {
			return nil, errors.New("Datei liegt nicht im Report-Ordner")
		}
		if _, err := os.Stat(d); err != nil {
			return nil, errors.New("Datei nicht gefunden: " + filepath.Base(d))
		}
	}
	switch in.Aktion {
	case "oeffnen":
		return nil, Oeffnen(in.Dateien[0])
	case "ordner":
		return nil, OrdnerZeigen(in.Dateien[0])
	case "mail":
		kunde := strings.TrimSpace(strings.TrimPrefix(titel, "Geräteliste"))
		betreff := "Truck-Reports " + kunde
		text := "Hallo,\r\n\r\nanbei die Truck-Reports"
		if kunde != "" {
			text += " für " + kunde
		}
		text += ".\r\n\r\nMit freundlichen Grüßen"
		return map[string]string{"text": "Die E-Mail ist in Outlook geöffnet – Empfänger eintragen und senden."}, OutlookEntwurf("", betreff, text, in.Dateien)
	case "whatsapp", "kopieren":
		if err := DateienInZwischenablage(in.Dateien); err != nil {
			return nil, err
		}
		if in.Aktion == "whatsapp" {
			if err := WhatsAppOeffnen(); err != nil {
				return map[string]string{"text": "Die Dateien sind kopiert. WhatsApp konnte nicht geöffnet werden – bitte WhatsApp öffnen, Chat wählen und mit Strg+V einfügen."}, nil
			}
			return map[string]string{"text": "WhatsApp wird geöffnet. Chat auswählen und mit Strg+V einfügen – die Reports sind schon kopiert."}, nil
		}
		return map[string]string{"text": "Die Reports sind kopiert und können mit Strg+V eingefügt werden."}, nil
	}
	return nil, errors.New("unbekannte Aktion")
}

// ---------- Update ----------

func (a *App) apiUpdate(r *http.Request) (interface{}, error) {
	switch r.URL.Query().Get("aktion") {
	case "pruefen":
		a.sp.mu.Lock()
		test := a.sp.Einst.Testversionen
		a.sp.mu.Unlock()
		return UpdatePruefen(test), nil
	case "installieren":
		if err := UpdateInstallieren(); err != nil {
			return nil, err
		}
		go func() {
			time.Sleep(700 * time.Millisecond)
			a.Beenden(true)
		}()
		return map[string]string{"text": "Update installiert – das Programm startet neu."}, nil
	}
	return AktuellesUpdate(), nil
}

func (a *App) apiSicherungsVorschau(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	return a.sp.SicherungVorschau(r.URL.Query().Get("id"), r.URL.Query().Get("datei"))
}

func (a *App) apiListePruefen(r *http.Request) (interface{}, error) {
	a.sp.mu.Lock()
	defer a.sp.mu.Unlock()
	l, err := a.liste(r)
	if err != nil {
		return nil, err
	}
	b := ListePruefen(l, time.Now())
	if b == nil {
		b = []Befund{}
	}
	return b, nil
}

// Foto eines Typenschilds: Text erkennen und Typ/Seriennummer herausziehen
func (a *App) apiTypenschild(r *http.Request) (interface{}, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var in struct {
			Text string `json:"text"`
		}
		if err := lesen(r, &in); err != nil {
			return nil, err
		}
		return TypenschildAuswerten(in.Text), nil
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, errors.New("Das Foto konnte nicht gelesen werden.")
	}
	f, h, err := r.FormFile("bild")
	if err != nil {
		return nil, errors.New("Kein Foto übergeben.")
	}
	defer f.Close()
	tmp, err := os.MkdirTemp("", "kgl-ocr")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	ext := strings.ToLower(filepath.Ext(h.Filename))
	if ext == "" || len(ext) > 6 {
		ext = ".jpg"
	}
	pfad := filepath.Join(tmp, "foto"+ext)
	out, err := os.Create(pfad)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(out, io.LimitReader(f, 30<<20)); err != nil {
		out.Close()
		return nil, err
	}
	out.Close()
	text, err := TextErkennen(pfad)
	if err != nil {
		return nil, err
	}
	quelle := "Windows"
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("Im Foto wurde kein Text erkannt. Bitte näher, gerade und bei gutem Licht fotografieren.")
	}
	t := TypenschildAuswerten(text)
	t.Quelle = quelle
	return t, nil
}
