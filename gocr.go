package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Anmeldung der Service-Mappe auf diesem Rechner (id + E-Mail), damit der Server die
// Google-Texterkennung freigibt. Die Kundengeräteliste hat keine eigene Anmeldung.
func serviceMappeNutzer() (id, mail string) {
	var s struct {
		User *struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	b, err := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "PDF Unterschreiben", "einstellungen.json"))
	if err != nil || json.Unmarshal(b, &s) != nil || s.User == nil {
		return "", ""
	}
	return s.User.ID, s.User.Email
}

// GoogleTextErkennen schickt das Foto an die Texterkennung von Google (über den Server der Service-Mappe).
func GoogleTextErkennen(img []byte) (string, error) {
	id, mail := serviceMappeNutzer()
	if id == "" || mail == "" {
		return "", errors.New("Service-Mappe ist auf diesem Rechner nicht angemeldet")
	}
	if len(img) < 100 || len(img) > 10<<20 {
		return "", errors.New("Foto zu groß (max. 10 MB)")
	}
	payload, _ := json.Marshal(map[string]any{
		"key": meldeKey, "event": "liste_ocr", "id": id, "email": mail, "version": "Programm " + Version,
		"b64": base64.StdEncoding.EncodeToString(img), "mime": http.DetectContentType(img),
	})
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Post(meldeURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", errors.New("keine Verbindung zu Google")
	}
	defer resp.Body.Close()
	var res struct {
		OK    bool   `json:"ok"`
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&res); err != nil {
		return "", errors.New("keine gültige Antwort vom Server")
	}
	if !res.OK {
		if res.Error == "" {
			res.Error = "vom Server abgelehnt"
		}
		return "", errors.New(res.Error)
	}
	t := strings.ReplaceAll(res.Text, "\r", "")
	t = strings.TrimLeft(strings.TrimPrefix(strings.TrimSpace(t), "\uFEFF"), "_\n ")
	return t, nil
}
