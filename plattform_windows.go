//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procMessageBoxW      = user32.NewProc("MessageBoxW")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procCreateMutexW     = kernel32.NewProc("CreateMutexW")
)

func Meldung(titel, text string, frage bool) bool {
	t, _ := syscall.UTF16PtrFromString(titel)
	m, _ := syscall.UTF16PtrFromString(text)
	flags := uintptr(0x40 | 0x10000 | 0x40000) // Info, Vordergrund, oben
	if frage {
		flags = 0x4 | 0x20 | 0x10000 | 0x40000 // Ja/Nein, Frage, oben
	}
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return r == 6 || (!frage && r == 1)
}

// Nur eine Instanz je Benutzer
func EinzelInstanz() bool {
	n, _ := syscall.UTF16PtrFromString("Local\\Kundengeraeteliste-" + BenutzerName())
	h, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(n)))
	if err == syscall.Errno(183) { // ERROR_ALREADY_EXISTS
		syscall.CloseHandle(syscall.Handle(h))
		return false
	}
	return true
}

func versteckt(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func edgePfad() string {
	for _, p := range []string{
		os.Getenv("ProgramFiles(x86)") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("ProgramFiles") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("LOCALAPPDATA") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("ProgramFiles") + `\Google\Chrome\Application\chrome.exe`,
		os.Getenv("ProgramFiles(x86)") + `\Google\Chrome\Application\chrome.exe`,
		os.Getenv("LOCALAPPDATA") + `\Google\Chrome\Application\chrome.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Oberflaeche als eigenes App-Fenster (Edge im App-Modus).
// Der Kanal wird geschlossen, wenn das Fenster (der Edge-Prozess) zu ist.
func FensterOeffnen(url string, datenOrdner string) (<-chan struct{}, error) {
	if e := edgePfad(); e != "" {
		profil := filepath.Join(datenOrdner, "Fenster")
		cmd := exec.Command(e, "--app="+url, "--user-data-dir="+profil, "--no-first-run",
			"--no-default-browser-check", "--disable-features=Translate", "--start-maximized",
			// Fenster im Hintergrund nicht ausbremsen (sonst gilt das Programm als geschlossen)
			"--disable-background-timer-throttling", "--disable-renderer-backgrounding",
			"--disable-backgrounding-occluded-windows", "--disable-background-mode")
		if err := cmd.Start(); err == nil {
			zu := make(chan struct{})
			go func() {
				cmd.Wait()
				close(zu)
			}()
			return zu, nil
		}
	}
	return nil, exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func Oeffnen(pfad string) error {
	return versteckt(exec.Command("rundll32", "url.dll,FileProtocolHandler", pfad)).Start()
}

func OrdnerZeigen(pfad string) error {
	if st, err := os.Stat(pfad); err == nil && !st.IsDir() {
		return exec.Command("explorer.exe", "/select,", pfad).Start()
	}
	return exec.Command("explorer.exe", pfad).Start()
}

func OrdnerWaehlen(titel string) (string, error) {
	var pfad string
	err := ComAusfuehren(func() error {
		sh, err := ComErstellen("Shell.Application")
		if err != nil {
			return err
		}
		defer sh.Release()
		f, err := sh.Obj("BrowseForFolder", 0, titel, 0x0041|0x0010, 0) // neuer Dialog, Eingabefeld
		if err != nil {
			return nil // abgebrochen
		}
		defer f.Release()
		self, err := f.Obj("Self")
		if err != nil {
			return err
		}
		defer self.Release()
		pfad, err = self.Str("Path")
		return err
	})
	return pfad, err
}

// Dateien wie im Explorer kopieren (zum Einfuegen in WhatsApp mit Strg+V)
func DateienInZwischenablage(pfade []string) error {
	var u []uint16
	for _, p := range pfade {
		w, _ := syscall.UTF16FromString(p)
		u = append(u, w...) // inkl. 0
	}
	u = append(u, 0)
	const kopf = 20 // DROPFILES
	groesse := kopf + len(u)*2
	h, _, _ := procGlobalAlloc.Call(0x0042, uintptr(groesse)) // GHND
	if h == 0 {
		return errors.New("Speicher für die Zwischenablage fehlt")
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return errors.New("Zwischenablage gesperrt")
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), groesse)
	*(*uint32)(unsafe.Pointer(&b[0])) = kopf // pFiles
	*(*uint32)(unsafe.Pointer(&b[16])) = 1   // fWide
	for i, c := range u {
		*(*uint16)(unsafe.Pointer(&b[kopf+i*2])) = c
	}
	procGlobalUnlock.Call(h)
	var ok uintptr
	for i := 0; i < 10; i++ {
		ok, _, _ = procOpenClipboard.Call(0)
		if ok != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if ok == 0 {
		return errors.New("Zwischenablage ist gerade belegt")
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	if r, _, _ := procSetClipboardData.Call(15, h); r == 0 { // CF_HDROP
		return errors.New("Dateien konnten nicht in die Zwischenablage gelegt werden")
	}
	return nil
}

func WhatsAppOeffnen() error {
	return Oeffnen("whatsapp://")
}

// ---------- Outlook ----------

type Anhang struct {
	Name string
	Pfad string
}

type Mail struct {
	Absender  string
	Empfangen float64
	Anhaenge  []Anhang
}

func outlookHolen() (*Disp, bool, error) {
	if ol := ComAktiv("Outlook.Application"); ol != nil {
		return ol, false, nil
	}
	ol, err := ComErstellen("Outlook.Application")
	if err != nil {
		return nil, false, errors.New("Outlook konnte nicht geöffnet werden. Der Abgleich braucht das klassische Outlook (Desktop-Programm) – mit dem „neuen Outlook“ oder Webmail funktioniert er nicht.")
	}
	return ol, true, nil
}

func sendenEmpfangen(ol *Disp) {
	if s, err := ol.Obj("Session"); err == nil {
		_ = s.Call("SendAndReceive", false)
		s.Release()
	}
}

// Holt alle Sync-Mails mit passendem Betreff, die nach "nach" eingegangen sind (aelteste zuerst),
// speichert die .xlsx-Anhaenge in tmpDir.
func OutlookAbrufen(betreff string, nach float64, tmpDir string, warten bool) ([]Mail, error) {
	var mails []Mail
	err := ComAusfuehren(func() error {
		ol, neu, err := outlookHolen()
		if err != nil {
			return err
		}
		defer ol.Release()
		sendenEmpfangen(ol)
		if neu && warten {
			time.Sleep(8 * time.Second)
		}
		ns, err := ol.Obj("GetNamespace", "MAPI")
		if err != nil {
			return err
		}
		defer ns.Release()
		inbox, err := ns.Obj("GetDefaultFolder", 6)
		if err != nil {
			return errors.New("Posteingang konnte nicht gelesen werden.")
		}
		defer inbox.Release()
		items, err := inbox.Obj("Items")
		if err != nil {
			return err
		}
		defer items.Release()
		filter := `@SQL="urn:schemas:httpmail:subject" LIKE '` + strings.ReplaceAll(betreff, "'", "''") + `%'`
		its, err := items.Obj("Restrict", filter)
		if err != nil {
			its = items
		} else {
			defer its.Release()
		}
		_ = its.Call("Sort", "[ReceivedTime]", false)
		n, _ := its.Num("Count")
		if n > 3000 {
			n = 3000
		}
		for i := 1; i <= int(n); i++ {
			it, err := its.Obj("Item", i)
			if err != nil {
				continue
			}
			func() {
				defer it.Release()
				subj, _ := it.Str("Subject")
				if !strings.HasPrefix(subj, betreff) {
					return
				}
				rt, err := it.Num("ReceivedTime")
				if err != nil || rt <= nach {
					return
				}
				m := Mail{Empfangen: rt}
				m.Absender, _ = it.Str("SenderName")
				atts, err := it.Obj("Attachments")
				if err == nil {
					cnt, _ := atts.Num("Count")
					for a := 1; a <= int(cnt); a++ {
						at, err := atts.Obj("Item", a)
						if err != nil {
							continue
						}
						fn, _ := at.Str("FileName")
						if strings.HasSuffix(strings.ToLower(fn), ".xlsx") {
							p := filepath.Join(tmpDir, fmt.Sprintf("sync_%d_%d_%s", i, a, filepath.Base(fn)))
							if at.Call("SaveAsFile", p) == nil {
								m.Anhaenge = append(m.Anhaenge, Anhang{fn, p})
							}
						}
						at.Release()
					}
					atts.Release()
				}
				_ = it.Put("UnRead", false)
				mails = append(mails, m)
			}()
		}
		return nil
	})
	return mails, err
}

func mailErstellen(ol *Disp, an, betreff, text string, anhaenge []string) (*Disp, error) {
	m, err := ol.Obj("CreateItem", 0)
	if err != nil {
		return nil, err
	}
	if an != "" {
		if err := m.Put("To", an); err != nil {
			m.Release()
			return nil, err
		}
	}
	_ = m.Put("Subject", betreff)
	_ = m.Put("Body", text)
	atts, err := m.Obj("Attachments")
	if err == nil {
		for _, p := range anhaenge {
			if err := atts.Call("Add", p); err != nil {
				atts.Release()
				m.Release()
				return nil, fmt.Errorf("Anhang %s: %v", filepath.Base(p), err)
			}
		}
		atts.Release()
	}
	return m, nil
}

func OutlookSenden(an, betreff, text string, anhaenge []string) error {
	return ComAusfuehren(func() error {
		ol, _, err := outlookHolen()
		if err != nil {
			return err
		}
		defer ol.Release()
		m, err := mailErstellen(ol, an, betreff, text, anhaenge)
		if err != nil {
			return err
		}
		defer m.Release()
		if err := m.Call("Send"); err != nil {
			return fmt.Errorf("Die E-Mail konnte nicht gesendet werden: %v", err)
		}
		sendenEmpfangen(ol)
		// kurz warten, bis der Postausgang leer ist (max. 20 Sek.)
		if s, err := ol.Obj("Session"); err == nil {
			if out, err := s.Obj("GetDefaultFolder", 4); err == nil {
				for i := 0; i < 20; i++ {
					its, err := out.Obj("Items")
					if err != nil {
						break
					}
					n, _ := its.Num("Count")
					its.Release()
					if n == 0 {
						break
					}
					time.Sleep(time.Second)
				}
				out.Release()
			}
			s.Release()
		}
		return nil
	})
}

// Mail mit Anhaengen oeffnen (nicht senden) – Empfaenger traegt der Benutzer ein
func OutlookEntwurf(an, betreff, text string, anhaenge []string) error {
	return ComAusfuehren(func() error {
		ol, neu, err := outlookHolen()
		if err != nil {
			return err
		}
		defer ol.Release()
		if neu {
			// Outlook lief noch nicht: Hauptfenster zeigen, sonst bleibt die Mail unsichtbar
			if ns, err := ol.Obj("GetNamespace", "MAPI"); err == nil {
				if inbox, err := ns.Obj("GetDefaultFolder", 6); err == nil {
					_ = inbox.Call("Display")
					inbox.Release()
				}
				ns.Release()
			}
			time.Sleep(3 * time.Second)
		}
		m, err := mailErstellen(ol, an, betreff, text, anhaenge)
		if err != nil {
			return err
		}
		defer m.Release()
		return m.Call("Display", false)
	})
}

// ---------- Installation ----------

func InstallOrdner() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Kundengeraeteliste")
}

func Verknuepfungen(exe string) error {
	return ComAusfuehren(func() error {
		sh, err := ComErstellen("WScript.Shell")
		if err != nil {
			return err
		}
		defer sh.Release()
		ziele := []string{}
		if d, err := sh.Obj("SpecialFolders"); err == nil {
			for _, n := range []string{"Desktop", "Programs"} {
				if p, err := d.Str("Item", n); err == nil && p != "" {
					ziele = append(ziele, p)
				}
			}
			d.Release()
		}
		for _, ord := range ziele {
			lnk, err := sh.Obj("CreateShortcut", filepath.Join(ord, "Kundengeräteliste.lnk"))
			if err != nil {
				continue
			}
			_ = lnk.Put("TargetPath", exe)
			_ = lnk.Put("WorkingDirectory", filepath.Dir(exe))
			_ = lnk.Put("IconLocation", exe+",0")
			_ = lnk.Put("Description", "Kundengeräteliste – Gerätelisten und UVV-Termine")
			_ = lnk.Call("Save")
			lnk.Release()
		}
		return nil
	})
}

func NeuStarten(exe string, args ...string) error {
	return exec.Command(exe, args...).Start()
}
