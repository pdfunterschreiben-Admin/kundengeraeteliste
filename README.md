# Kundengeräteliste

Windows-Programm für die Gerätelisten der Kunden (Wartung, UVV, Betriebsstunden) –
Nachfolger der Excel-Makro-Listen (`Geraeteliste_*.xlsm`), kompatibel zum Mail-Abgleich der Excel-Version.

## Aufbau
- Go (nur Standardbibliothek), Oberfläche als Web-Seite (`web/`) im Edge-App-Fenster
- `xlsxread.go` / `xlsxwrite.go` – Excel lesen/schreiben (Aufbau wie die Makro-Listen, Blatt `SyncDaten`)
- `abgleich.go` – Abgleich wie `TabelleAbgleichen` im VBA-Modul (Seriennummer, neuere Änderung gewinnt)
- `sync.go`, `plattform_windows.go`, `com_windows.go` – Outlook (classic) über COM
- `reports.go` – Truck-Reports per Seriennummer zuordnen
- `update.go` – Auto-Update über GitHub-Releases (`pdfunterschreiben-Admin/kundengeraeteliste`)
- Daten: `%APPDATA%\Kundengeraeteliste` (Listen, Sicherungen, Einstellungen)

## Bauen
    go test ./...
    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui -X main.Version=1.0.0" -o Kundengeraeteliste.exe .

`rsrc_windows_amd64.syso` enthält Programmsymbol und Manifest (neu erzeugen: `build/icon_render.py`, `build/syso.py`).

## Update veröffentlichen
Version taggen und pushen, z. B. `git tag v1.0.1 && git push --tags`. GitHub Actions baut die .exe
und legt das Release an. Tags mit Bindestrich (`v1.1.0-test1`) sind Testversionen.

## Entwickeln unter Linux
    KGL_DATEN=/tmp/kgl KGL_KEIN_FENSTER=1 go run .
