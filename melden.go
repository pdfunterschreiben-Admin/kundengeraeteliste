package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Nutzungsmeldung wie in den Excel-Listen (Listenname, Benutzer, Rechner – keine Inhalte)
const (
	meldeURL = "https://script.google.com/macros/s/AKfycbwj7l1K2RcGtPUu4wdRFjI7NLayzAtqjx0_XorzQmMNEqh-JyuEaOrUlOEN1BT9HVSC/exec"
	meldeKey = "geraeteliste-v1"
)

func NutzungMelden(l *Liste, ereignis string) {
	titel := strings.TrimSpace(l.Titel)
	if titel == "" || strings.Contains(strings.ToLower(titel), "kundenname") {
		return
	}
	body, _ := json.Marshal(map[string]interface{}{
		"key": meldeKey, "event": ereignis, "listId": l.ListenID, "liste": titel,
		"datei": "Kundengeraeteliste (Programm)", "vorlage": l.Herkunft == "Vorlage",
		"user": BenutzerName(), "name": BenutzerName(), "pc": RechnerName(),
		"version": "Programm " + Version, "app": "Kundengeraeteliste",
	})
	go func() {
		cl := &http.Client{Timeout: 10 * time.Second}
		resp, err := cl.Post(meldeURL, "text/plain;charset=utf-8", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
		}
	}()
}
