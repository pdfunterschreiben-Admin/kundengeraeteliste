"use strict";
// Kundengeräteliste – Oberfläche

const TOKEN = (location.hash.match(/t=([0-9a-f]+)/) || [])[1] || sessionStorage.getItem("kgl-token") || "";
if (TOKEN) sessionStorage.setItem("kgl-token", TOKEN);
history.replaceState(null, "", location.pathname);

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

async function api(pfad, daten, optionen = {}) {
  const init = { method: daten === undefined ? "GET" : "POST", headers: { "X-Token": TOKEN } };
  if (daten instanceof FormData) init.body = daten;
  else if (daten !== undefined) { init.body = JSON.stringify(daten); init.headers["Content-Type"] = "application/json"; }
  let r;
  for (let versuch = 0; ; versuch++) {
    try { r = await fetch(pfad, init); break; }
    catch {
      if (versuch >= 3) throw new Error("Keine Verbindung zum Programm. Bitte Kundengeräteliste neu starten.");
      await new Promise((ok) => setTimeout(ok, 1500));
    }
  }
  const text = await r.text();
  let j = null;
  try { j = text ? JSON.parse(text) : null; } catch { j = null; }
  if (!r.ok) throw new Error((j && j.fehler) || "Unerwarteter Fehler (" + r.status + ")");
  return j;
}

// ---------- Zustand ----------
const Z = {
  ansicht: "start",
  listen: [], geloescht: [],
  liste: null, info: null, betreff: "", reportOrdner: "",
  reg: 0, filter: "alle", suche: "", sort: null,
  status: null, empfangen: false, markieren: null,
  reports: null, reportAuswahl: new Set(), reportFilter: "",
  updateSpaeter: false,
};

// ---------- Hilfen ----------
function toast(text, art = "") {
  const t = document.createElement("div");
  t.className = "toast " + art;
  t.textContent = text;
  $("#toasts").append(t);
  setTimeout(() => t.remove(), art === "fehler" ? 9000 : 5200);
}
const fehlerToast = (e) => toast(e.message || String(e), "fehler");

const gleich = (a, b) => String(a).trim().toLowerCase() === String(b).trim().toLowerCase();
const istDatumSpalte = (s) => ["wartung", "letzte uvv", "nächste uvv"].includes(String(s).trim().toLowerCase());
const istFormel = (s) => gleich(s, "Nächste UVV");

function datum(iso) { const [y, m, d] = iso.split("-").map(Number); return new Date(y, m - 1, d); }
function monat(iso) { const [y, m] = iso.split("-"); return m + "/" + y; }
function tagText(iso) { const [y, m, d] = iso.split("-"); return d + "." + m + "." + y; }
function tausender(n) { return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, "."); }

function anzeige(spalte, w) {
  if (!w || !w.t) return "";
  if (w.t === "d") return istDatumSpalte(spalte) ? monat(w.v) : tagText(w.v);
  if (w.t === "n") {
    if (gleich(spalte, "Betriebsstunden") && /^\d+$/.test(w.v)) return tausender(w.v);
    return w.v.replace(".", ",");
  }
  return w.v;
}
function eingabeText(spalte, w) {
  if (!w || !w.t) return "";
  if (w.t === "d") {
    const [y, m, d] = w.v.split("-");
    return d === "01" && istDatumSpalte(spalte) ? m + "/" + y : d + "." + m + "." + y;
  }
  if (w.t === "n") return w.v.replace(".", ",");
  return w.v;
}

function heute() { const d = new Date(); return new Date(d.getFullYear(), d.getMonth(), d.getDate()); }
function edate(d, m) {
  const t = new Date(d.getFullYear(), d.getMonth() + m, 1);
  const last = new Date(t.getFullYear(), t.getMonth() + 1, 0).getDate();
  return new Date(t.getFullYear(), t.getMonth(), Math.min(d.getDate(), last));
}
function spalteIdx(reg, name) { return reg.spalten.findIndex((s) => gleich(s, name)); }
function wert(reg, z, name) { const i = spalteIdx(reg, name); return i >= 0 ? z.werte[i] || {} : {}; }
function naechsteUVV(reg, z) {
  const w = wert(reg, z, "Letzte UVV");
  return w.t === "d" ? edate(datum(w.v), 12) : null;
}
function uvvStatus(reg, z) {
  if (!wert(reg, z, "Seriennummer").t) return "";
  const n = naechsteUVV(reg, z);
  if (!n) return "fehlt";
  const h = heute();
  if (n < h) return "ueberfaellig";
  if (n <= new Date(h.getTime() + 30 * 864e5)) return "faellig";
  return "ok";
}
function plakette(d, status) {
  if (!d) return "";
  const m = String(d.getMonth() + 1).padStart(2, "0") + "/" + d.getFullYear();
  const titel = status === "ueberfaellig" ? "UVV überfällig" : status === "faellig" ? "UVV in den nächsten 30 Tagen fällig" : "UVV gültig";
  return `<span class="plakette ${status}" title="${titel}">${m}</span>`;
}

function knopfLaedt(btn, an, text) {
  if (!btn) return;
  if (an) { btn.dataset.alt = btn.innerHTML; btn.disabled = true; btn.innerHTML = `<span class="dreh"></span>${esc(text || "Bitte warten …")}`; }
  else if (btn.dataset.alt) { btn.innerHTML = btn.dataset.alt; btn.disabled = false; }
}

// ---------- Dialoge ----------
function dialog({ titel, html = "", knoepfe = [{ text: "OK", wert: true, art: "primaer" }], breit = false, beimOeffnen }) {
  return new Promise((fertig) => {
    const s = document.createElement("div");
    s.className = "schleier";
    s.innerHTML = `<div class="dialog ${breit ? "breit" : ""}" role="dialog" aria-modal="true">
      <header><h2>${esc(titel)}</h2></header><div class="koerper">${html}</div>
      <footer>${knoepfe.map((k, i) => `<button class="knopf ${k.art || ""}" data-i="${i}">${esc(k.text)}</button>`).join("")}</footer></div>`;
    const zu = (w) => { s.remove(); document.removeEventListener("keydown", taste); fertig(w); };
    const taste = (e) => {
      if (e.key === "Escape") zu(null);
      if (e.key === "Enter" && e.target.tagName !== "TEXTAREA" && e.target.tagName !== "BUTTON") {
        const p = knoepfe.findIndex((k) => k.art && k.art.includes("primaer"));
        if (p >= 0) { e.preventDefault(); klick(p); }
      }
    };
    const klick = (i) => {
      const k = knoepfe[i];
      const w = typeof k.wert === "function" ? k.wert(s) : k.wert;
      if (w === undefined) return; // Prüfung fehlgeschlagen, offen lassen
      zu(w);
    };
    s.addEventListener("click", (e) => { const b = e.target.closest("footer button"); if (b) klick(+b.dataset.i); });
    s.addEventListener("mousedown", (e) => { if (e.target === s) s.dataset.weg = "1"; });
    s.addEventListener("mouseup", (e) => { if (e.target === s && s.dataset.weg) zu(null); delete s.dataset.weg; });
    document.addEventListener("keydown", taste);
    document.body.append(s);
    const f = s.querySelector("input:not([type=radio]):not([type=checkbox]), textarea, select") || s.querySelector("footer .primaer");
    if (f) setTimeout(() => f.focus(), 30);
    if (beimOeffnen) beimOeffnen(s);
  });
}
const frage = (titel, text, ja = "Ja", art = "primaer") =>
  dialog({ titel, html: `<p class="vorschau-text">${esc(text)}</p>`, knoepfe: [{ text: "Abbrechen", wert: false }, { text: ja, wert: true, art }] });
const hinweis = (titel, text) => dialog({ titel, html: `<p class="vorschau-text">${esc(text)}</p>` });

function menue(anker, eintraege) {
  $$(".menue").forEach((m) => m.remove());
  const m = document.createElement("div");
  m.className = "menue";
  m.innerHTML = eintraege.map((e, i) => (e === "-" ? "<hr>" : `<button data-i="${i}" class="${e.gefahr ? "gefahr" : ""}">${esc(e.text)}</button>`)).join("");
  document.body.append(m);
  const r = anker.getBoundingClientRect();
  const mw = m.offsetWidth, mh = m.offsetHeight;
  m.style.left = Math.max(8, Math.min(r.right - mw, innerWidth - mw - 8)) + "px";
  m.style.top = (r.bottom + mh + 6 > innerHeight ? Math.max(8, r.top - mh - 4) : r.bottom + 4) + "px";
  const weg = (e) => { if (!m.contains(e.target)) { m.remove(); document.removeEventListener("mousedown", weg, true); } };
  setTimeout(() => document.addEventListener("mousedown", weg, true));
  m.addEventListener("click", (e) => {
    const b = e.target.closest("button"); if (!b) return;
    m.remove(); document.removeEventListener("mousedown", weg, true);
    eintraege[+b.dataset.i].tun();
  });
  m.querySelector("button")?.focus();
}

// ---------- Laden ----------
async function listenLaden() {
  const j = await api("/api/listen");
  Z.listen = j.listen; Z.geloescht = j.geloescht || [];
  seitenleiste();
}

function seitenleiste() {
  const q = $("#listen-suche").value.trim().toLowerCase();
  const el = $("#listen");
  const liste = Z.listen.filter((l) => !q || l.titel.toLowerCase().includes(q));
  if (!Z.listen.length) {
    el.innerHTML = `<div class="leer-hinweis">Noch keine Gerätelisten. Lege eine neue an oder importiere deine Excel-Listen.</div>`;
  } else {
    el.innerHTML = liste.map((l) => `
      <button class="liste-eintrag ${Z.ansicht === "liste" && Z.liste && Z.liste.id === l.id ? "aktiv" : ""}" data-id="${l.id}">
        <div class="t"><span>${esc(l.titel.replace(/^Geräteliste\s+/i, ""))}</span>${l.syncOffen && l.empfaenger ? '<span class="punkt" title="Änderungen noch nicht an den Kollegen gesendet"></span>' : ""}
          <span class="marken">${l.ueberfaellig ? `<span class="zahl rot" title="UVV überfällig">${l.ueberfaellig}</span>` : ""}${l.faellig ? `<span class="zahl orange" title="UVV in 30 Tagen fällig">${l.faellig}</span>` : ""}</span></div>
        <div class="u"><span>${l.geraete} Geräte</span>${l.register.length > 1 ? `<span>${l.register.length} Standorte</span>` : ""}</div>
      </button>`).join("") || `<div class="leer-hinweis">Kein Kunde gefunden.</div>`;
  }
  const summe = Z.listen.reduce((a, l) => a + l.ueberfaellig + l.faellig, 0);
  $("#nav-faellig").textContent = summe || "";
  $$(".nav[data-ansicht]").forEach((b) => b.classList.toggle("aktiv", b.dataset.ansicht === Z.ansicht));
}

async function listeOeffnen(id, reg = 0, markieren = null) {
  try {
    const j = await api("/api/liste?id=" + encodeURIComponent(id));
    const neu = !Z.liste || Z.liste.id !== id;
    Z.liste = j.liste; Z.info = j.info; Z.betreff = j.betreff; Z.reportOrdner = j.reportOrdner;
    Z.ansicht = "liste";
    if (neu) { Z.reg = 0; Z.filter = "alle"; Z.suche = ""; Z.sort = null; Z.reports = null; Z.reportsJ = null; Z.reportAuswahl.clear(); Z.reportFilter = ""; }
    Z.reg = reg === "reports" ? "reports" : Math.min(reg, Z.liste.register.length - 1);
    Z.markieren = markieren;
    zeichnen();
    seitenleiste();
    if (neu) automatischEmpfangen(id);
  } catch (e) { fehlerToast(e); }
}

async function automatischEmpfangen(id) {
  const l = Z.liste;
  if (!l.empfaenger || !Z.status || !Z.status.autoSync) return;
  Z.empfangen = true; statusZeile();
  try {
    const j = await api("/api/empfangen?id=" + encodeURIComponent(id), {});
    if (j.text) {
      toast(j.text, "gut");
      if (Z.liste && Z.liste.id === id) await listeOeffnen(id, Z.reg);
      await listenLaden();
    }
    if (j.hinweis && !Z.hinweisGezeigt) { Z.hinweisGezeigt = true; toast("Automatischer Abgleich nicht möglich: " + j.hinweis, "fehler"); }
  } catch (e) { /* still */ }
  Z.empfangen = false; statusZeile();
}

// ---------- Zeichnen ----------
function zeichnen() {
  const h = $("#haupt");
  if (Z.ansicht === "liste" && Z.liste) return listeZeichnen(h);
  if (Z.ansicht === "einstellungen") return einstellungenZeichnen(h);
  if (Z.ansicht === "hilfe") return hilfeZeichnen(h);
  if (Z.ansicht === "start" && Z.listen.length) { h.innerHTML = updateBand(); updateBandVerbinden(h); return; }
  return uebersichtZeichnen(h);
}

function updateBand() {
  const u = Z.status && Z.status.update;
  if (!u || !u.verfuegbar) return "";
  return `<div class="band"><strong>Neue Version ${esc(u.version)}${u.test ? " (Testversion)" : ""} ist da.</strong>
    <span>${Z.updateSpaeter ? "" : "Sie wird gleich automatisch installiert."}</span>
    <button class="knopf primaer" id="upd-jetzt">Jetzt aktualisieren</button>${Z.updateSpaeter ? "" : '<button class="knopf" id="upd-spaeter">Später</button>'}</div>`;
}
function updateBandVerbinden(h) {
  $("#upd-jetzt", h)?.addEventListener("click", (e) => updateInstallieren(e.currentTarget));
  $("#upd-spaeter", h)?.addEventListener("click", () => { Z.updateSpaeter = true; zeichnen(); });
}

// ----- Übersicht -----
async function uebersichtZeichnen(h) {
  if (!Z.listen.length) {
    h.innerHTML = updateBand() + `<div class="leerseite"><img src="icon.svg" alt="">
      <h1>Willkommen in der Kundengeräteliste</h1>
      <p>Hier verwaltest du die Geräte deiner Kunden mit Wartung, UVV-Terminen und Betriebsstunden. Übernimm deine bisherigen Excel-Gerätelisten oder lege eine neue an.</p>
      <div class="reihe"><button class="knopf primaer" id="l-import">Excel-Listen importieren</button><button class="knopf" id="l-neu">Neue Geräteliste</button></div>
      <p style="margin-top:18px;font-size:13px">Tipp: Du kannst die .xlsm-Dateien auch einfach ins Fenster ziehen.</p></div>`;
    $("#l-import").onclick = importWahl;
    $("#l-neu").onclick = neueListe;
    updateBandVerbinden(h);
    return;
  }
  h.innerHTML = updateBand() + `<div class="kopf"><div class="kopf-zeile"><div><h1>Fällige UVV</h1>
    <div class="unterzeile"><span>Alle Kunden · überfällig und in den nächsten Wochen fällig</span></div></div>
    <div class="aktionen"><div class="filter" id="tage">${[30, 60, 90].map((t) => `<button data-t="${t}" class="${(Z.tage || 60) === t ? "aktiv" : ""}">${t} Tage</button>`).join("")}</div></div></div>
    <div style="height:14px"></div></div>
    <div class="inhalt"><div class="kacheln" id="kacheln"></div><h2>Geräte mit fälliger UVV</h2><div id="faellig"><div class="status-punkt"><span class="dreh"></span>Wird geladen …</div></div></div>`;
  updateBandVerbinden(h);
  $("#tage").onclick = (e) => { const b = e.target.closest("button"); if (b) { Z.tage = +b.dataset.t; zeichnen(); } };
  $("#kacheln").innerHTML = Z.listen.map((l) => {
    const g = Math.max(l.geraete, 1);
    return `<button class="kachel" data-id="${l.id}"><div class="t">${esc(l.titel.replace(/^Geräteliste\s+/i, ""))}</div>
      <div class="u">${l.geraete} Geräte${l.register.length > 1 ? " · " + l.register.length + " Standorte" : ""}</div>
      <div class="balken"><i style="width:${(l.ueberfaellig / g) * 100}%;background:#D64541"></i><i style="width:${(l.faellig / g) * 100}%;background:var(--gelb)"></i><i style="width:${(l.fehlend / g) * 100}%;background:#C3CAD2"></i></div>
      <div class="werte">${l.ueberfaellig ? `<span class="r">${l.ueberfaellig} überfällig</span>` : ""}${l.faellig ? `<span class="o">${l.faellig} in 30 Tagen</span>` : ""}${l.fehlend ? `<span>${l.fehlend} ohne UVV-Datum</span>` : ""}${!l.ueberfaellig && !l.faellig && !l.fehlend ? "<span>alles im grünen Bereich</span>" : ""}</div></button>`;
  }).join("");
  $("#kacheln").onclick = (e) => { const k = e.target.closest(".kachel"); if (k) listeOeffnen(k.dataset.id); };
  try {
    const f = await api("/api/uebersicht?tage=" + (Z.tage || 60));
    if (Z.ansicht !== "uebersicht") return;
    const el = $("#faellig");
    if (!f.length) { el.innerHTML = `<p style="color:var(--leise)">In diesem Zeitraum ist keine UVV fällig.</p>`; return; }
    f.sort((a, b) => a.naechste.localeCompare(b.naechste));
    el.innerHTML = `<table class="einfach"><thead><tr><th>Nächste UVV</th><th>Kunde</th><th>Standort</th><th>Nummer</th><th>Typ</th><th>Seriennummer</th><th>Wann</th></tr></thead><tbody>
      ${f.map((x, i) => `<tr class="klick" data-i="${i}"><td>${plakette(datum(x.naechste), x.status)}</td><td>${esc(x.liste.replace(/^Geräteliste\s+/i, ""))}</td><td>${esc(x.register)}</td><td>${esc(x.nummer)}</td><td>${esc(x.typ)}</td><td>${esc(x.sn)}</td>
      <td>${x.tage < 0 ? `seit ${-x.tage} Tagen` : x.tage === 0 ? "heute" : `in ${x.tage} Tagen`}</td></tr>`).join("")}</tbody></table>`;
    el.onclick = (e) => { const r = e.target.closest("tr.klick"); if (r) { const x = f[+r.dataset.i]; listeOeffnen(x.listeId, x.regIndex, x.zeileId); } };
  } catch (e) { fehlerToast(e); }
}

// ----- Liste -----
function statusZeile() {
  const el = $("#statuszeile");
  if (!el || !Z.liste) return;
  const l = Z.liste, i = Z.info;
  const teile = [`<span>${i.geraete} Geräte</span>`];
  if (l.empfaenger) teile.push(`<span>Abgleich mit ${esc(l.empfaenger)}</span>`);
  else teile.push(`<span>Kein Abgleich eingerichtet</span>`);
  if (l.syncOffen && l.empfaenger) teile.push(`<span class="offen">Änderungen noch nicht gesendet</span>`);
  if (Z.empfangen) teile.push(`<span class="status-punkt"><span class="dreh"></span>Suche neue Daten vom Kollegen …</span>`);
  el.innerHTML = teile.join("");
}

function listeZeichnen(h) {
  const l = Z.liste;
  const regs = l.register;
  h.innerHTML = updateBand() + `
  <div class="kopf">
    <div class="kopf-zeile">
      <div><h1><button id="titel" title="Listen-Einstellungen">${esc(l.titel)}</button></h1><div class="unterzeile" id="statuszeile"></div></div>
      <div class="aktionen">
        <button class="knopf" id="b-geraet"><svg viewBox="0 0 24 24"><path d="M11 5h2v6h6v2h-6v6h-2v-6H5v-2h6z"/></svg>Gerät hinzufügen</button>
        <button class="knopf primaer" id="b-sync" title="Neue Daten vom Kollegen holen und eigene Liste senden"><svg viewBox="0 0 24 24"><path d="M12 4V1L8 5l4 4V6a6 6 0 0 1 5.7 7.9l1.5 1.5A8 8 0 0 0 12 4Zm0 14a6 6 0 0 1-5.7-7.9L4.8 8.6A8 8 0 0 0 12 20v3l4-4-4-4v3Z"/></svg>Synchronisieren</button>
        <button class="knopf" id="b-mehr" aria-label="Weitere Aktionen"><svg viewBox="0 0 24 24"><path d="M6 10a2 2 0 1 0 0 4 2 2 0 0 0 0-4Zm6 0a2 2 0 1 0 0 4 2 2 0 0 0 0-4Zm6 0a2 2 0 1 0 0 4 2 2 0 0 0 0-4Z"/></svg>Mehr</button>
      </div>
    </div>
    <div class="reiter" id="reiter">
      ${regs.map((r, i) => `<button data-r="${i}" class="${Z.reg === i ? "aktiv" : ""}">${esc(r.name)}<small>${r.zeilen.filter((z) => wert(r, z, "Seriennummer").t).length}</small></button>`).join("")}
      <button class="plus" id="r-neu" title="Weiteres Register (z. B. Standort) anlegen">+ Register</button>
      <button data-r="reports" class="rechts ${Z.reg === "reports" ? "aktiv" : ""}">Truck-Reports</button>
    </div>
  </div>
  <div id="bereich" style="display:flex;flex-direction:column;flex:1"></div>`;
  updateBandVerbinden(h);
  statusZeile();
  $("#titel").onclick = listenEinstellungen;
  $("#b-sync").onclick = (e) => synchronisieren(e.currentTarget);
  $("#b-geraet").onclick = geraetHinzufuegen;
  $("#b-mehr").onclick = (e) => mehrMenue(e.currentTarget);
  $("#r-neu").onclick = registerNeu;
  $("#reiter").addEventListener("click", (e) => {
    const b = e.target.closest("button[data-r]"); if (!b) return;
    Z.reg = b.dataset.r === "reports" ? "reports" : +b.dataset.r;
    $$("#reiter button").forEach((x) => x.classList.toggle("aktiv", x === b));
    bereichZeichnen();
  });
  $("#reiter").addEventListener("contextmenu", (e) => {
    const b = e.target.closest("button[data-r]"); if (!b || b.dataset.r === "reports") return;
    e.preventDefault(); registerMenue(b, +b.dataset.r);
  });
  $("#reiter").addEventListener("dblclick", (e) => {
    const b = e.target.closest("button[data-r]"); if (!b || b.dataset.r === "reports") return;
    registerMenue(b, +b.dataset.r);
  });
  bereichZeichnen();
}

function bereichZeichnen() {
  $("#b-geraet").hidden = Z.reg === "reports";
  if (Z.reg === "reports") return reportsZeichnen();
  tabelleZeichnen();
}

function zeilenAuswahl(reg) {
  const q = Z.suche.trim().toLowerCase();
  let zeilen = reg.zeilen.filter((z) => {
    const st = uvvStatus(reg, z);
    if (Z.filter === "ueberfaellig" && st !== "ueberfaellig") return false;
    if (Z.filter === "faellig" && st !== "faellig") return false;
    if (Z.filter === "fehlt") {
      const fehlt = st === "fehlt" || (spalteIdx(reg, "Betriebsstunden") >= 0 && !wert(reg, z, "Betriebsstunden").t);
      if (!fehlt) return false;
    }
    if (q) {
      const txt = reg.spalten.map((s, i) => anzeige(s, z.werte[i])).join(" ").toLowerCase() + " " + Object.values(z.notizen || {}).join(" ").toLowerCase();
      if (!q.split(/\s+/).every((t) => txt.includes(t))) return false;
    }
    return true;
  });
  if (Z.sort) {
    const { i, ab } = Z.sort;
    const sp = reg.spalten[i];
    const key = (z) => {
      if (istFormel(sp)) { const d = naechsteUVV(reg, z); return d ? d.getTime() : Infinity; }
      const w = z.werte[i] || {};
      if (!w.t) return null;
      if (w.t === "d") return datum(w.v).getTime();
      if (w.t === "n") return parseFloat(w.v);
      const n = w.v.match(/^\d+/); return n && gleich(sp, "Nummer") ? parseInt(n[0], 10) + w.v.length / 1000 : w.v.toLowerCase();
    };
    zeilen = zeilen.slice().sort((a, b) => {
      const x = key(a), y = key(b);
      if (x === y) return 0; if (x === null) return 1; if (y === null) return -1;
      if (typeof x === typeof y) return (x < y ? -1 : 1) * (ab ? -1 : 1);
      return (typeof x === "number" ? -1 : 1) * (ab ? -1 : 1);
    });
  }
  return zeilen;
}

function zellKlasse(sp) {
  if (gleich(sp, "Nummer")) return "nr";
  if (gleich(sp, "Seriennummer")) return "sn";
  if (istDatumSpalte(sp)) return "datum";
  if (gleich(sp, "Info durchgeführte Arbeiten") || gleich(sp, "Vermerk")) return "breit";
  if (gleich(sp, "Anbaugerät") || gleich(sp, "Typ")) return "mittel";
  return "";
}

// Geräte werden an der Seriennummer erkannt, bei doppelter Seriennummer zusätzlich am Typ.
// Gewarnt wird nur, wenn Seriennummer UND Typ gleich sind.
const normSN = (v) => String(v || "").replace(/\s/g, "").toUpperCase();
function geraetKey(r, z) { return normSN(wert(r, z, "Seriennummer").v) + "~" + normSN(wert(r, z, "Typ").v); }
function doppelteSN() {
  const n = new Map();
  (Z.liste ? Z.liste.register : []).forEach((r) => r.zeilen.forEach((z) => {
    if (!normSN(wert(r, z, "Seriennummer").v)) return;
    const k = geraetKey(r, z);
    n.set(k, (n.get(k) || 0) + 1);
  }));
  return new Set([...n].filter(([, c]) => c > 1).map(([k]) => k));
}
let DOPPELT = new Set();

function zelleHTML(reg, z, i) {
  const sp = reg.spalten[i];
  if (gleich(sp, "Seriennummer") && normSN((z.werte[i] || {}).v) && DOPPELT.has(geraetKey(reg, z))) {
    return `<td class="sn doppelt" data-s="${i}" title="Seriennummer und Typ stehen mehrfach in der Liste. Beim Abgleich werden Geräte an Seriennummer und Typ erkannt – bitte korrigieren."><div class="z">${esc(anzeige(sp, z.werte[i]))} ⚠</div></td>`;
  }
  const notiz = z.notizen && z.notizen[sp];
  let kl = zellKlasse(sp);
  let inhalt;
  if (istFormel(sp)) {
    kl += " formel";
    const d = naechsteUVV(reg, z);
    inhalt = plakette(d, uvvStatus(reg, z));
  } else {
    inhalt = esc(anzeige(sp, z.werte[i]));
    const hatSN = !!wert(reg, z, "Seriennummer").t;
    if (hatSN && !(z.werte[i] || {}).t && (gleich(sp, "Letzte UVV") || gleich(sp, "Betriebsstunden"))) kl += " fehlt";
  }
  if (notiz) kl += " notiz";
  return `<td class="${kl}" data-s="${i}"${notiz ? ` title="${esc(notiz)}"` : ""}><div class="z">${inhalt}</div></td>`;
}

function tabelleZeichnen() {
  const l = Z.liste, reg = l.register[Z.reg];
  const alle = reg.zeilen;
  const zaehl = { alle: alle.length, ueberfaellig: 0, faellig: 0, fehlt: 0 };
  alle.forEach((z) => {
    const st = uvvStatus(reg, z);
    if (st === "ueberfaellig") zaehl.ueberfaellig++;
    if (st === "faellig") zaehl.faellig++;
    if (st === "fehlt" || (spalteIdx(reg, "Betriebsstunden") >= 0 && wert(reg, z, "Seriennummer").t && !wert(reg, z, "Betriebsstunden").t)) zaehl.fehlt++;
  });
  const zeilen = zeilenAuswahl(reg);
  DOPPELT = doppelteSN();
  const b = $("#bereich");
  b.innerHTML = `
  <div class="werkzeug">
    <input class="feld suche" id="suche" placeholder="Suchen: Nummer, Typ, Seriennummer, Vermerk …" value="${esc(Z.suche)}" autocomplete="off">
    <div class="filter" id="filter">
      <button data-f="alle">Alle<b>${zaehl.alle}</b></button>
      <button data-f="ueberfaellig">Überfällig<b>${zaehl.ueberfaellig}</b></button>
      <button data-f="faellig">In 30 Tagen<b>${zaehl.faellig}</b></button>
      <button data-f="fehlt">Angaben fehlen<b>${zaehl.fehlt}</b></button>
    </div>
    <div class="legende"><span><i style="background:var(--rot-flaeche)"></i>überfällig</span><span><i style="background:var(--orange-flaeche)"></i>in 30 Tagen</span><span><i style="background:var(--grau-flaeche)"></i>fehlt noch</span></div>
  </div>
  <div class="nur-druck" style="margin:0 0 4px">${esc(reg.name)} · Stand ${new Date().toLocaleDateString("de-DE")}</div>
  <div class="tabelle-rahmen"><table class="geraete"><thead><tr>
    ${reg.spalten.map((s, i) => `<th data-i="${i}" title="Sortieren">${esc(s)}${Z.sort && Z.sort.i === i ? `<span class="pfeil">${Z.sort.ab ? "▼" : "▲"}</span>` : ""}</th>`).join("")}<th style="width:36px"></th>
  </tr></thead><tbody>
    ${zeilen.map((z) => `<tr data-z="${z.id}">${reg.spalten.map((_, i) => zelleHTML(reg, z, i)).join("")}<td class="menu"><button aria-label="Aktionen für dieses Gerät" title="Aktionen">⋯</button></td></tr>`).join("")
      || `<tr class="leer-zeile"><td colspan="${reg.spalten.length + 1}">${alle.length ? "Keine Geräte für diese Auswahl." : "Noch keine Geräte in diesem Register. Lege das erste über „Gerät hinzufügen“ an."}</td></tr>`}
  </tbody></table></div>
  <div class="fuss-info">
    <div><strong>Anzahl Geräte: ${alle.filter((z) => wert(reg, z, "Seriennummer").t).length}</strong>${zeilen.length !== alle.length ? ` · angezeigt ${zeilen.length}` : ""}</div>
    <div class="register-notiz"><textarea class="feld klein" id="reg-notiz" placeholder="Notiz zu diesem Register (steht in Excel unter der Tabelle) …" rows="1">${esc(reg.notiz || "")}</textarea></div>
  </div>`;
  $$("#filter button").forEach((x) => x.classList.toggle("aktiv", x.dataset.f === Z.filter));
  $("#filter").onclick = (e) => { const x = e.target.closest("button"); if (x) { Z.filter = x.dataset.f; tabelleZeichnen(); } };
  const s = $("#suche");
  s.oninput = () => { Z.suche = s.value; const pos = s.selectionStart; tabelleZeichnen(); const n = $("#suche"); n.focus(); n.setSelectionRange(pos, pos); };
  $("thead", b).onclick = (e) => {
    const th = e.target.closest("th[data-i]"); if (!th) return;
    const i = +th.dataset.i;
    Z.sort = Z.sort && Z.sort.i === i ? (Z.sort.ab ? null : { i, ab: true }) : { i, ab: false };
    tabelleZeichnen();
  };
  const tb = $("tbody", b);
  tb.addEventListener("click", (e) => {
    const m = e.target.closest("td.menu button");
    if (m) return geraetMenue(m, m.closest("tr").dataset.z);
    const td = e.target.closest("td[data-s]");
    if (td && !td.querySelector(".edit")) bearbeiten(td);
  });
  tb.addEventListener("contextmenu", (e) => {
    const tr = e.target.closest("tr[data-z]"); if (!tr) return;
    e.preventDefault();
    const td = e.target.closest("td[data-s]");
    geraetMenue(td || tr, tr.dataset.z, td ? +td.dataset.s : null);
  });
  const rn = $("#reg-notiz");
  rn.onchange = async () => {
    try { await api(`/api/register?id=${l.id}`, { aktion: "notiz", index: Z.reg, notiz: rn.value }); reg.notiz = rn.value.trim(); toast("Notiz gespeichert."); }
    catch (e) { fehlerToast(e); }
  };
  if (Z.markieren) {
    const tr = $(`tr[data-z="${CSS.escape(Z.markieren)}"]`, b);
    if (tr) { tr.classList.add("markiert"); tr.scrollIntoView({ block: "center" }); }
    Z.markieren = null;
  }
}

function zeileFinden(reg, id) { return reg.zeilen.find((z) => z.id === id); }

function bearbeiten(td) {
  const l = Z.liste, reg = l.register[Z.reg];
  const tr = td.closest("tr"), z = zeileFinden(reg, tr.dataset.z);
  const i = +td.dataset.s, sp = reg.spalten[i];
  if (istFormel(sp)) { toast("„Nächste UVV“ wird automatisch berechnet: letzte UVV + 12 Monate."); return; }
  const alt = eingabeText(sp, z.werte[i]);
  const lang = zellKlasse(sp) === "breit";
  const inp = document.createElement(lang ? "textarea" : "input");
  inp.className = "edit";
  inp.value = alt;
  if (istDatumSpalte(sp)) inp.placeholder = "MM/JJJJ";
  if (lang) inp.rows = Math.max(2, Math.ceil(alt.length / 40));
  const z0 = td.querySelector(".z");
  z0.hidden = true;
  td.append(inp);
  inp.focus();
  if (!lang) inp.select();
  let fertig = false;
  const ende = async (speichern, weiter) => {
    if (fertig) return; fertig = true;
    const neu = inp.value;
    if (!speichern || neu.trim() === alt.trim()) { inp.remove(); z0.hidden = false; if (weiter) springen(tr, i, weiter); return; }
    if (gleich(sp, "Seriennummer") && alt) {
      const ok = await frage("Seriennummer ändern?", "Daran wird das Gerät beim Abgleich erkannt. Beim Kollegen wird das Gerät mit der neuen Seriennummer als neues Gerät angelegt, das alte bleibt dort stehen.", "Ändern");
      if (!ok) { inp.remove(); z0.hidden = false; return; }
    }
    try {
      const j = await api(`/api/zelle?id=${l.id}`, { register: Z.reg, zeile: z.id, spalte: i, text: neu });
      Object.assign(z, j.zeile); Z.info = j.info; l.syncOffen = true;
      const nTd = document.createElement("tbody");
      nTd.innerHTML = `<tr>${zelleHTML(reg, z, i)}</tr>`;
      const ersatz = nTd.querySelector("td");
      td.replaceWith(ersatz);
      // abhängige Zellen (Nächste UVV, Graufärbung) neu zeichnen
      reg.spalten.forEach((s, k) => {
        if (k !== i && (istFormel(s) || gleich(s, "Betriebsstunden") || gleich(s, "Letzte UVV"))) {
          const c = tr.querySelector(`td[data-s="${k}"]`);
          if (c) { const t = document.createElement("tbody"); t.innerHTML = `<tr>${zelleHTML(reg, z, k)}</tr>`; c.replaceWith(t.querySelector("td")); }
        }
      });
      statusZeile(); aktualisiereSeite();
      if (weiter) springen(tr, i, weiter);
    } catch (e) {
      fehlerToast(e); inp.remove(); z0.hidden = false;
    }
  };
  inp.addEventListener("keydown", (e) => {
    if (e.key === "Escape") { e.preventDefault(); ende(false); }
    else if (e.key === "Enter" && !(lang && e.shiftKey)) { e.preventDefault(); ende(true, "runter"); }
    else if (e.key === "Tab") { e.preventDefault(); ende(true, e.shiftKey ? "links" : "rechts"); }
  });
  inp.addEventListener("blur", () => ende(true));
}

function springen(tr, i, richtung) {
  let ziel = null;
  const reg = Z.liste.register[Z.reg];
  if (richtung === "runter") { const n = tr.nextElementSibling; ziel = n && n.querySelector(`td[data-s="${i}"]`); }
  else {
    let k = i;
    do { k += richtung === "rechts" ? 1 : -1; } while (k >= 0 && k < reg.spalten.length && istFormel(reg.spalten[k]));
    ziel = tr.querySelector(`td[data-s="${k}"]`);
  }
  if (ziel) bearbeiten(ziel);
}

let seiteTimer;
function aktualisiereSeite() { clearTimeout(seiteTimer); seiteTimer = setTimeout(() => listenLaden().catch(() => {}), 400); }

function geraetMenue(anker, zid, spalte) {
  const l = Z.liste, reg = l.register[Z.reg], z = zeileFinden(reg, zid);
  const sn = wert(reg, z, "Seriennummer").v || "";
  const e = [];
  e.push({ text: spalte != null ? `Kommentar zu „${reg.spalten[spalte]}“ …` : "Kommentar …", tun: () => notizBearbeiten(z, spalte != null ? reg.spalten[spalte] : reg.spalten[0]) });
  e.push({ text: "Truck-Reports zu diesem Gerät", tun: () => { Z.reg = "reports"; Z.reportFilter = sn; listeZeichnen($("#haupt")); } });
  if (l.register.length > 1) e.push({ text: "In anderes Register verschieben …", tun: () => geraetVerschieben(z) });
  e.push("-");
  e.push({ text: "Gerät löschen …", gefahr: true, tun: () => geraetLoeschen(z) });
  menue(anker, e);
}

async function notizBearbeiten(z, spalte) {
  const l = Z.liste, reg = l.register[Z.reg];
  const opts = reg.spalten.filter((s) => !istFormel(s)).map((s) => `<option ${s === spalte ? "selected" : ""}>${esc(s)}</option>`).join("");
  const r = await dialog({
    titel: "Kommentar", html: `<p>Kommentare werden in der Zelle mit einer roten Ecke markiert (wie in Excel).</p>
      <label>Spalte</label><select class="feld" id="n-sp">${opts}</select>
      <label>Kommentar</label><textarea class="feld" id="n-text" rows="4">${esc((z.notizen || {})[spalte] || "")}</textarea>`,
    knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Speichern", art: "primaer", wert: (s) => ({ sp: $("#n-sp", s).value, text: $("#n-text", s).value }) }],
    beimOeffnen: (s) => { $("#n-sp", s).onchange = (e) => { $("#n-text", s).value = (z.notizen || {})[e.target.value] || ""; }; },
  });
  if (!r) return;
  try {
    const j = await api(`/api/notiz?id=${l.id}`, { register: Z.reg, zeile: z.id, spalte: r.sp, text: r.text });
    Object.assign(z, j.zeile); if (!j.zeile.notizen) z.notizen = {};
    tabelleZeichnen();
  } catch (e) { fehlerToast(e); }
}

async function geraetLoeschen(z) {
  const l = Z.liste, reg = l.register[Z.reg];
  const name = [wert(reg, z, "Nummer").v, wert(reg, z, "Typ").v, "SN " + (wert(reg, z, "Seriennummer").v || "–")].filter(Boolean).join(" · ");
  const ok = await frage("Gerät löschen?", name + "\n\nGelöschte Geräte werden nicht an den Kollegen übertragen – dort bleibt das Gerät stehen. Vorher wird eine Sicherung angelegt.", "Löschen", "gefahr voll");
  if (!ok) return;
  try {
    const j = await api(`/api/geraet?id=${l.id}`, { aktion: "loeschen", register: Z.reg, zeile: z.id });
    reg.zeilen = reg.zeilen.filter((x) => x !== z); Z.info = j.info;
    listeZeichnen($("#haupt")); aktualisiereSeite(); toast("Gerät gelöscht.");
  } catch (e) { fehlerToast(e); }
}

async function geraetVerschieben(z) {
  const l = Z.liste;
  const opts = l.register.map((r, i) => (i === Z.reg ? "" : `<option value="${i}">${esc(r.name)}</option>`)).join("");
  const ziel = await dialog({ titel: "Gerät verschieben", html: `<label>In welches Register?</label><select class="feld" id="v-ziel">${opts}</select>`,
    knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Verschieben", art: "primaer", wert: (s) => +$("#v-ziel", s).value }] });
  if (ziel == null) return;
  try { await api(`/api/geraet?id=${l.id}`, { aktion: "verschieben", register: Z.reg, zeile: z.id, ziel }); await listeOeffnen(l.id, ziel, z.id); }
  catch (e) { fehlerToast(e); }
}

async function geraetHinzufuegen() {
  const l = Z.liste, reg = l.register[Z.reg];
  const felder = reg.spalten.filter((s) => !istFormel(s));
  const html = `<p>Pflicht ist nur die Seriennummer – daran wird das Gerät beim Abgleich erkannt. Datumsfelder als MM/JJJJ eingeben.</p>
    <div class="zeile2">${felder.map((s) => `<div ${zellKlasse(s) === "breit" ? 'style="grid-column:1/-1"' : ""}><label>${esc(s)}${gleich(s, "Seriennummer") ? " *" : ""}</label>
      ${zellKlasse(s) === "breit" ? `<textarea class="feld" data-s="${esc(s)}" rows="2"></textarea>` : `<input class="feld" data-s="${esc(s)}" ${istDatumSpalte(s) ? 'placeholder="MM/JJJJ"' : ""}>`}</div>`).join("")}</div>`;
  while (true) {
    const werte = await dialog({ titel: "Gerät hinzufügen – " + reg.name, html, breit: true,
      knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Gerät anlegen", art: "primaer", wert: (s) => {
        const w = {}; $$("[data-s]", s).forEach((x) => { if (x.value.trim()) w[x.dataset.s] = x.value; });
        if (!w["Seriennummer"] && !Object.keys(w).some((k) => gleich(k, "Seriennummer"))) { toast("Bitte die Seriennummer eintragen.", "fehler"); return undefined; }
        return w;
      } }] });
    if (!werte) return;
    try {
      const j = await api(`/api/geraet?id=${l.id}`, { aktion: "neu", register: Z.reg, werte });
      reg.zeilen.push(j.zeile); Z.info = j.info; l.syncOffen = true;
      Z.markieren = j.zeile.id; Z.filter = "alle"; Z.suche = "";
      listeZeichnen($("#haupt")); aktualisiereSeite(); toast("Gerät angelegt.", "gut");
      return;
    } catch (e) { fehlerToast(e); }
  }
}

function mehrMenue(anker) {
  const l = Z.liste;
  menue(anker, [
    { text: "Listen-Einstellungen (Titel, Abgleich) …", tun: listenEinstellungen },
    { text: "Datei einlesen (empfangene Liste) …", tun: () => $("#datei-einlesen").click() },
    "-",
    { text: "Als Excel-Datei speichern", tun: () => { location.href = `/api/export?id=${encodeURIComponent(l.id)}&token=${TOKEN}`; toast("Die Excel-Datei wird in „Downloads“ gespeichert."); } },
    { text: "Drucken / als PDF speichern", tun: () => window.print() },
    "-",
    { text: "Sync-Protokoll", tun: protokollZeigen },
    { text: "Sicherungen …", tun: sicherungenZeigen },
    "-",
    { text: "Liste löschen …", gefahr: true, tun: listeLoeschen },
  ]);
}

function registerMenue(anker, i) {
  const l = Z.liste, r = l.register[i];
  menue(anker, [
    { text: "Register umbenennen …", tun: async () => {
      const n = await dialog({ titel: "Register umbenennen", html: `<label>Name</label><input class="feld" id="rn" value="${esc(r.name)}">`,
        knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Umbenennen", art: "primaer", wert: (s) => $("#rn", s).value }] });
      if (!n) return;
      try { await api(`/api/register?id=${l.id}`, { aktion: "umbenennen", index: i, name: n }); await listeOeffnen(l.id, i); await listenLaden(); } catch (e) { fehlerToast(e); }
    } },
    { text: "Register löschen …", gefahr: true, tun: async () => {
      if (!(await frage("Register löschen?", `„${r.name}“ mit ${r.zeilen.length} Geräten wird gelöscht. Vorher wird eine Sicherung angelegt.`, "Löschen", "gefahr voll"))) return;
      try { await api(`/api/register?id=${l.id}`, { aktion: "loeschen", index: i }); await listeOeffnen(l.id, 0); await listenLaden(); } catch (e) { fehlerToast(e); }
    } },
  ]);
}

async function registerNeu() {
  const l = Z.liste;
  const n = await dialog({ titel: "Neues Register", html: `<p>Ein Register ist ein eigenes Blatt in der Liste, z. B. für einen weiteren Standort des Kunden. Es bekommt dieselben Spalten wie das erste Register.</p><label>Name</label><input class="feld" id="rn" placeholder="z. B. Paderborn">`,
    knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Register anlegen", art: "primaer", wert: (s) => $("#rn", s).value }] });
  if (!n) return;
  try { await api(`/api/register?id=${l.id}`, { aktion: "neu", name: n }); await listeOeffnen(l.id, l.register.length); await listenLaden(); } catch (e) { fehlerToast(e); }
}

async function listenEinstellungen() {
  const l = Z.liste;
  const r = await dialog({ titel: "Listen-Einstellungen", html: `
    <label>Titel</label><input class="feld" id="e-titel" value="${esc(l.titel)}">
    <label>E-Mail des Kollegen für den Abgleich <small>(wie Zelle I3 in Excel)</small></label><input class="feld" id="e-empf" value="${esc(l.empfaenger || "")}" placeholder="v.nachname@schlueter-gabelstapler.de">
    <label>Betreff der Abgleich-Mails <small>(muss beim Kollegen gleich sein)</small></label><input class="feld" id="e-betreff" value="${esc(Z.betreff)}">
    <p style="font-size:12.5px;margin-top:8px">Der Betreff kommt aus der importierten Excel-Liste. Nur ändern, wenn der Kollege ihn auch ändert – sonst finden sich die Mails nicht mehr.</p>`,
    knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Speichern", art: "primaer", wert: (s) => ({ titel: $("#e-titel", s).value, empfaenger: $("#e-empf", s).value, betreff: $("#e-betreff", s).value }) }] });
  if (!r) return;
  try {
    const daten = { titel: r.titel, empfaenger: r.empfaenger };
    if (r.betreff.trim() !== Z.betreff) daten.betreff = r.betreff;
    await api(`/api/listeneinstellungen?id=${l.id}`, daten);
    await listeOeffnen(l.id, Z.reg); await listenLaden(); toast("Gespeichert.");
  } catch (e) { fehlerToast(e); }
}

async function synchronisieren(btn) {
  const l = Z.liste;
  if (!l.empfaenger) {
    const e = await dialog({ titel: "Abgleich einrichten", html: `<p>An welche E-Mail-Adresse soll die Liste gesendet werden? Die Adresse kannst du später in den Listen-Einstellungen ändern.</p><label>E-Mail des Kollegen</label><input class="feld" id="se" placeholder="v.nachname@schlueter-gabelstapler.de">`,
      knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Speichern und synchronisieren", art: "primaer", wert: (s) => $("#se", s).value }] });
    if (!e) return;
    try { await api(`/api/listeneinstellungen?id=${l.id}`, { empfaenger: e }); l.empfaenger = e.trim(); } catch (er) { return fehlerToast(er); }
  }
  knopfLaedt(btn, true, "Synchronisiere …");
  try {
    const j = await api(`/api/sync?id=${l.id}`, {});
    l.syncOffen = false; Z.info = j.info;
    await hinweis("Synchronisieren", j.text);
    await listeOeffnen(l.id, Z.reg); await listenLaden();
  } catch (e) { await hinweis("Synchronisieren", e.message); }
  knopfLaedt(btn, false);
}

async function protokollZeigen() {
  const l = Z.liste;
  try {
    const p = await api(`/api/protokoll?id=${l.id}`);
    await dialog({ titel: "Sync-Protokoll – was vom Kollegen übernommen wurde", breit: true,
      html: p.length ? `<table class="einfach liste-proto"><thead><tr><th>Zeitpunkt</th><th>Von</th><th>Nummer</th><th>Seriennummer</th><th>Spalte</th><th>Alter Wert</th><th>Neuer Wert</th></tr></thead><tbody>
        ${p.slice(0, 1500).map((x) => `<tr><td>${esc(x.zeit)}</td><td>${esc(x.von)}</td><td>${esc(x.nummer)}</td><td>${esc(x.sn)}</td><td>${esc(x.spalte)}</td><td>${esc(x.alt)}</td><td>${esc(x.neu)}</td></tr>`).join("")}</tbody></table>`
        : `<p>Noch keine Änderungen vom Kollegen übernommen.</p>` });
  } catch (e) { fehlerToast(e); }
}

async function sicherungenZeigen() {
  const l = Z.liste;
  try {
    const s = await api(`/api/sicherungen?id=${l.id}`);
    const w = await dialog({ titel: "Sicherungen", breit: true, html: `<p>Vor jeder Übernahme vom Kollegen, jedem Einlesen und jedem Löschen wird automatisch eine Sicherung angelegt. Die letzten 30 bleiben erhalten.</p>
      ${s.length ? `<table class="einfach"><thead><tr><th>Zeitpunkt</th><th>Anlass</th><th>Geräte</th><th></th></tr></thead><tbody>
      ${s.map((x, i) => `<tr><td>${new Date(x.zeit).toLocaleString("de-DE")}</td><td>${esc(x.grund)}</td><td>${x.geraete}</td><td style="text-align:right"><button class="knopf" data-i="${i}">Wiederherstellen</button></td></tr>`).join("")}</tbody></table>` : "<p>Noch keine Sicherungen vorhanden.</p>"}`,
      knoepfe: [{ text: "Ordner öffnen", wert: "ordner" }, { text: "Schließen", wert: null, art: "primaer" }],
      beimOeffnen: (d) => d.addEventListener("click", async (e) => {
        const b = e.target.closest("button[data-i]"); if (!b) return;
        const x = s[+b.dataset.i];
        if (!(await frage("Sicherung wiederherstellen?", `Die Liste wird auf den Stand vom ${new Date(x.zeit).toLocaleString("de-DE")} zurückgesetzt. Der aktuelle Stand wird vorher selbst gesichert.`, "Wiederherstellen"))) return;
        try { await api("/api/wiederherstellen", { id: l.id, datei: x.datei }); d.remove(); await listeOeffnen(l.id, 0); await listenLaden(); toast("Sicherung wiederhergestellt.", "gut"); }
        catch (er) { fehlerToast(er); }
      }) });
    if (w === "ordner") api("/api/sicherungsordner", {}).catch(fehlerToast);
  } catch (e) { fehlerToast(e); }
}

async function listeLoeschen() {
  const l = Z.liste;
  if (!(await frage("Liste löschen?", `„${l.titel}“ wird aus dem Programm entfernt. Eine Sicherung bleibt erhalten und kann unter Einstellungen wiederhergestellt werden.`, "Löschen", "gefahr voll"))) return;
  try { await api(`/api/loeschen?id=${l.id}`, {}); Z.liste = null; Z.ansicht = "uebersicht"; await listenLaden(); zeichnen(); toast("Liste gelöscht."); }
  catch (e) { fehlerToast(e); }
}

// ----- Neue Liste -----
async function neueListe() {
  const r = await dialog({ titel: "Neue Geräteliste", html: `
    <label>Kunde</label><input class="feld" id="n-titel" placeholder="z. B. Mustermann GmbH">
    <label>Aufbau</label>
    <div class="wahl">
      <label><input type="radio" name="aufbau" value="standard" checked><b>Standard</b><span>Nummer, Typ, Seriennummer, Anbaugerät, Wartung, UVV, Betriebsstunden, Info, Vermerk</span></label>
      <label><input type="radio" name="aufbau" value="erweitert"><b>Mit Hersteller, Kunde, Bereich</b><span>Für große Kunden mit mehreren Bereichen (wie die B+S-Liste)</span></label>
    </div>
    <label>Standorte / Register <small>(optional, einer pro Zeile)</small></label>
    <textarea class="feld" id="n-reg" rows="3" placeholder="Leer lassen für ein Register &#10;oder z. B.&#10;Borgholzhausen&#10;Paderborn"></textarea>
    <label>E-Mail des Kollegen für den Abgleich <small>(optional)</small></label><input class="feld" id="n-empf" placeholder="v.nachname@schlueter-gabelstapler.de">`,
    knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Liste anlegen", art: "primaer", wert: (s) => {
      const t = $("#n-titel", s).value.trim();
      if (!t) { toast("Bitte den Kunden eintragen.", "fehler"); return undefined; }
      return { titel: t, aufbau: $("input[name=aufbau]:checked", s).value, register: $("#n-reg", s).value.split("\n").map((x) => x.trim()).filter(Boolean), empfaenger: $("#n-empf", s).value };
    } }] });
  if (!r) return;
  try { const j = await api("/api/neu", r); await listenLaden(); await listeOeffnen(j.id); toast("Liste angelegt.", "gut"); }
  catch (e) { fehlerToast(e); }
}

// ----- Import -----
async function importieren(auswahl) {
  const dateien = [...auswahl]; // Kopie: das Datei-Feld wird gleich geleert
  const fd = new FormData();
  [...dateien].forEach((f) => fd.append("dateien", f));
  try {
    toast("Dateien werden eingelesen …");
    const erg = await api("/api/import", fd);
    await importErgebnisse(erg, dateien);
  } catch (e) { fehlerToast(e); }
}

async function importWahl() {
  const w = await dialog({ titel: "Excel-Listen importieren", html: `<p>Übernimm deine bisherigen Gerätelisten (.xlsm oder .xlsx) mit allen Registern, Kommentaren und den Abgleich-Einstellungen. Die Excel-Dateien selbst bleiben unverändert.</p>
    <div class="wahl"><label><input type="radio" name="iw" value="dateien" checked><b>Dateien auswählen</b><span>Eine oder mehrere Listen auswählen</span></label>
    <label><input type="radio" name="iw" value="ordner"><b>Ganzen Ordner einlesen</b><span>Alle Excel-Listen eines Ordners, z. B. „Arbeitsprogramme“</span></label></div>`,
    knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Weiter", art: "primaer", wert: (s) => $("input[name=iw]:checked", s).value }] });
  if (w === "dateien") $("#datei-import").click();
  if (w === "ordner") importOrdner();
}

async function importOrdner() {
  try {
    toast("Ordner auswählen …");
    const erg = await api("/api/importordner", { waehlen: true });
    await importErgebnisse(erg, null);
  } catch (e) { fehlerToast(e); }
}

async function importErgebnisse(erg, dateien) {
  let letzte = null;
  for (const e of erg) {
    if (e.status === "vorhanden") {
      const w = await dialog({ titel: "Liste schon vorhanden", html: `<p><strong>${esc(e.titel)}</strong> (${esc(e.datei)}, ${e.geraete} Geräte) gibt es schon im Programm. Was soll passieren?</p>
        <p style="font-size:12.5px">Abgleichen = wie „Datei einlesen“: neuere Änderungen und neue Geräte werden übernommen. Ersetzen = die Liste im Programm wird durch die Datei ersetzt (vorher Sicherung).</p>`,
        knoepfe: [{ text: "Überspringen", wert: null }, { text: "Als neue Liste", wert: "neu" }, { text: "Ersetzen", wert: "ersetzen" }, { text: "Abgleichen", wert: "abgleichen", art: "primaer" }] });
      if (!w) continue;
      let r2;
      if (e.pfad) r2 = (await api("/api/importordner", { datei: e.pfad, modus: w, ziel: w !== "neu" ? e.vorhanden : "" }))[0];
      else {
        const f2 = new FormData();
        f2.append("dateien", dateien.find((f) => f.name === e.datei));
        f2.append("modus", w);
        if (w !== "neu") f2.append("ziel", e.vorhanden);
        r2 = (await api("/api/import", f2))[0];
      }
      toast(`${r2.titel}: ${r2.text}`, "gut"); letzte = r2.id;
    } else if (e.status === "fehler") toast(`${e.datei}: ${e.text}`, "fehler");
    else { toast(`${e.titel}: ${e.geraete} Geräte übernommen.`, "gut"); letzte = e.id; }
  }
  await listenLaden();
  if (letzte) await listeOeffnen(letzte); else zeichnen();
}

async function einlesen(datei) {
  const l = Z.liste;
  const fd = new FormData(); fd.append("dateien", datei);
  try {
    const j = await api(`/api/einlesen?id=${l.id}`, fd);
    await hinweis("Datei einlesen", j.text);
    await listeOeffnen(l.id, Z.reg); await listenLaden();
  } catch (e) { fehlerToast(e); }
}

// ----- Truck-Reports -----
async function reportsZeichnen(nurFiltern) {
  const b = $("#bereich"), l = Z.liste;
  let j = nurFiltern === true ? Z.reportsJ : null;
  if (!j) {
    b.innerHTML = `<div class="reports-kopf"><span class="status-punkt"><span class="dreh"></span>Reports werden gesucht …</span></div>`;
    try { j = await api(`/api/reports?id=${l.id}`); } catch (e) { b.innerHTML = ""; return fehlerToast(e); }
    if (Z.reg !== "reports" || !Z.liste || Z.liste.id !== l.id) return;
    Z.reportsJ = j;
  }
  Z.reports = j.reports;
  const vorhanden = new Set(j.reports.map((r) => r.pfad));
  [...Z.reportAuswahl].forEach((p) => { if (!vorhanden.has(p)) Z.reportAuswahl.delete(p); });
  if (!j.ordner) {
    b.innerHTML = `<div class="leerseite" style="margin-top:6vh"><h1>Truck-Reports</h1><p>Wähle den Ordner, in dem deine Truck-Reports liegen. Das Programm zeigt dann zu jeder Geräteliste alle Reports, deren Dateiname oder Inhalt die Seriennummer eines Geräts enthält – zum Weiterschicken per E-Mail oder WhatsApp.</p>
      <div class="reihe"><button class="knopf primaer" id="ro-waehlen">Ordner auswählen</button></div></div>`;
    $("#ro-waehlen").onclick = () => reportOrdnerWaehlen(false);
    return;
  }
  const q = Z.reportFilter.trim().toUpperCase();
  const gruppen = new Map();
  j.reports.forEach((r) => r.geraete.forEach((g) => {
    const k = g.register + "|" + g.sn;
    if (q && !(g.sn + " " + g.nummer + " " + g.typ + " " + r.name).toUpperCase().includes(q)) return;
    if (!gruppen.has(k)) gruppen.set(k, { g, reports: [] });
    gruppen.get(k).reports.push(r);
  }));
  const liste = [...gruppen.values()].sort((a, b) => (a.g.nummer || a.g.sn).localeCompare(b.g.nummer || b.g.sn, "de", { numeric: true }));
  b.innerHTML = `
    <div class="reports-kopf">
      <input class="feld suche" id="rf" placeholder="Gerät suchen (Nummer, Seriennummer, Typ) …" value="${esc(Z.reportFilter)}">
      <span class="pfad" title="${esc(j.ordner)}">${esc(j.ordner)}</span>
      <button class="knopf" id="ro-waehlen">Ordner ändern</button>
      <button class="knopf" id="ro-neu" title="Ordner neu durchsuchen">Aktualisieren</button>
      <span style="margin-left:auto;color:var(--leise);font-size:13px">${j.reports.length} Reports zu ${new Set(j.reports.flatMap((r) => r.geraete.map((g) => g.sn))).size} Geräten</span>
    </div>
    ${j.fehler ? `<p style="margin:0 28px 10px;color:var(--rot)">${esc(j.fehler)}</p>` : ""}
    ${liste.length ? liste.map(({ g, reports }) => `
      <div class="gruppe"><div class="gruppe-kopf"><input type="checkbox" data-gruppe="${esc(g.sn)}" title="Alle Reports dieses Geräts auswählen"><strong>${esc(g.nummer || "–")}</strong><span>${esc(g.typ)}</span><span>SN ${esc(g.sn)}</span>${l.register.length > 1 ? `<span>${esc(g.register)}</span>` : ""}<span style="margin-left:auto">${reports.length} Report${reports.length === 1 ? "" : "s"}</span></div>
      ${reports.map((r) => {
        const ext = (r.name.split(".").pop() || "").toLowerCase();
        return `<div class="report"><input type="checkbox" data-pfad="${esc(r.pfad)}" ${Z.reportAuswahl.has(r.pfad) ? "checked" : ""}>
        <span class="typ ${ext === "pdf" ? "pdf" : ""}">${esc(ext.toUpperCase().slice(0, 4))}</span>
        <div class="name"><b title="${esc(r.pfad)}">${esc(r.name)}</b><span>${esc(r.datum.split(" ")[0].split("-").reverse().join("."))} ${esc(r.datum.split(" ")[1] || "")} · ${(r.groesse / 1024).toFixed(0)} KB${r.ordner ? " · " + esc(r.ordner) : ""}${r.treffer === "Inhalt" ? " · Seriennummer im Inhalt gefunden" : ""}</span></div>
        <button class="link" data-oeffnen="${esc(r.pfad)}">Öffnen</button><button class="link" data-zeigen="${esc(r.pfad)}">Im Ordner</button></div>`;
      }).join("")}</div>`).join("")
    : `<p style="margin:10px 28px;color:var(--leise)">${q ? "Keine Reports für diese Suche." : "Im Ordner wurden keine Reports zu den Seriennummern dieser Liste gefunden."}</p>`}
    <div class="teilen" id="teilen"></div>`;
  teilenLeiste();
  $("#ro-waehlen").onclick = () => reportOrdnerWaehlen(true);
  $("#ro-neu").onclick = () => reportsZeichnen();
  const rf = $("#rf");
  rf.oninput = () => { Z.reportFilter = rf.value; clearTimeout(rf.t); rf.t = setTimeout(() => { reportsNurZeichnen(); }, 150); };
  b.onchange = (e) => {
    const c = e.target;
    if (c.dataset.pfad) { c.checked ? Z.reportAuswahl.add(c.dataset.pfad) : Z.reportAuswahl.delete(c.dataset.pfad); }
    if (c.dataset.gruppe !== undefined && c.dataset.gruppe !== "") {
      $$(".report input", c.closest(".gruppe")).forEach((x) => { x.checked = c.checked; c.checked ? Z.reportAuswahl.add(x.dataset.pfad) : Z.reportAuswahl.delete(x.dataset.pfad); });
    }
    teilenLeiste();
  };
  b.onclick = (e) => {
    const o = e.target.closest("[data-oeffnen]"); if (o) reportAktion("oeffnen", [o.dataset.oeffnen]);
    const z = e.target.closest("[data-zeigen]"); if (z) reportAktion("ordner", [z.dataset.zeigen]);
  };
}
function reportsNurZeichnen() {
  const pos = $("#rf").selectionStart;
  // ohne neue Suche im Ordner: vorhandene Liste neu filtern
  reportsZeichnen(true).then(() => { const n = $("#rf"); if (n) { n.focus(); n.setSelectionRange(pos, pos); } });
}

function teilenLeiste() {
  const el = $("#teilen"); if (!el) return;
  const n = Z.reportAuswahl.size;
  el.innerHTML = n ? `<strong>${n} Report${n === 1 ? "" : "s"} ausgewählt</strong>
    <button class="knopf" id="t-mail">Per E-Mail senden</button>
    <button class="knopf wa" id="t-wa">Per WhatsApp senden</button>
    <button class="knopf" id="t-kopie">Dateien kopieren</button>
    <button class="knopf" id="t-weg" style="margin-left:auto">Auswahl aufheben</button>`
    : `<span style="opacity:.8">Reports anhaken, um sie per E-Mail oder WhatsApp weiterzuschicken.</span>`;
  if (!n) return;
  const sel = () => [...Z.reportAuswahl];
  $("#t-mail").onclick = () => reportAktion("mail", sel());
  $("#t-wa").onclick = () => reportAktion("whatsapp", sel());
  $("#t-kopie").onclick = () => reportAktion("kopieren", sel());
  $("#t-weg").onclick = () => { Z.reportAuswahl.clear(); $$(".gruppe input").forEach((x) => (x.checked = false)); teilenLeiste(); };
}

async function reportAktion(aktion, dateien) {
  try {
    const j = await api("/api/reportaktion", { liste: Z.liste.id, aktion, dateien });
    if (j && j.text) toast(j.text, "gut");
  } catch (e) { fehlerToast(e); }
}

async function reportOrdnerWaehlen(mitEingabe) {
  try {
    const j = await api("/api/reportordner", { waehlen: true });
    if (j.pfad) { toast("Report-Ordner gespeichert."); if (Z.liste) { Z.reportsJ = null; reportsZeichnen(); } if (Z.ansicht === "einstellungen") zeichnen(); return; }
  } catch (e) {
    if (!mitEingabe) toast(e.message, "fehler");
  }
  // Ersatz: Pfad eintippen
  const p = await dialog({ titel: "Report-Ordner", html: `<label>Pfad zum Ordner mit den Truck-Reports</label><input class="feld" id="rp" value="${esc((Z.status && Z.status.einstellungen.reportOrdner) || "")}" placeholder="C:\\Users\\...\\Truck Reports">`,
    knoepfe: [{ text: "Abbrechen", wert: null }, { text: "Speichern", art: "primaer", wert: (s) => $("#rp", s).value }] });
  if (p == null) return;
  try { await api("/api/reportordner", { pfad: p }); await statusLaden(); if (Z.liste && Z.ansicht === "liste") reportsZeichnen(); else zeichnen(); }
  catch (e) { fehlerToast(e); }
}

// ----- Einstellungen -----
function einstellungenZeichnen(h) {
  const s = Z.status || { einstellungen: {}, update: {} };
  const u = s.update || {};
  h.innerHTML = updateBand() + `<div class="kopf"><div class="kopf-zeile"><div><h1>Einstellungen</h1><div class="unterzeile"><span>Version ${esc(s.version)} · angemeldet als ${esc(s.benutzer)}</span></div></div></div><div style="height:14px"></div></div>
  <div class="inhalt">
    <div class="karte"><h3>Truck-Reports</h3><p>Ordner, in dem die Truck-Reports liegen (mit Unterordnern). Pro Geräteliste werden die Reports über die Seriennummer zugeordnet.</p>
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap"><span class="pfad">${esc(s.einstellungen.reportOrdner || "noch kein Ordner gewählt")}</span><button class="knopf" id="e-ro">Ordner auswählen</button></div></div>
    <div class="karte"><h3>Abgleich mit Kollegen</h3>
      <label class="schalter"><input type="checkbox" id="e-auto" ${s.autoSync ? "checked" : ""}><span><b>Automatisch abgleichen</b><br><span style="color:var(--leise)">Beim Öffnen einer Liste werden neue Daten vom Kollegen übernommen, beim Schließen des Programms werden geänderte Listen gesendet – wie bei der Excel-Liste. Braucht das klassische Outlook.</span></span></label></div>
    <div class="karte"><h3>Updates</h3><p>${u.verfuegbar ? `Version ${esc(u.version)} ist verfügbar.` : u.fehler ? esc(u.fehler) : "Das Programm ist aktuell."}${u.geprueft ? ` Zuletzt geprüft: ${esc(u.geprueft)}.` : ""}</p>
      <div style="display:flex;gap:8px;flex-wrap:wrap">${u.verfuegbar ? '<button class="knopf primaer" id="e-upd">Jetzt aktualisieren</button>' : ""}<button class="knopf" id="e-pruef">Nach Updates suchen</button></div>
      <label class="schalter" style="margin-top:12px"><input type="checkbox" id="e-test" ${s.einstellungen.testversionen ? "checked" : ""}><span>Testversionen erhalten <span style="color:var(--leise)">(nur zum Ausprobieren neuer Funktionen)</span></span></label></div>
    <div class="karte"><h3>Daten</h3><p>Die Listen werden für deinen Windows-Benutzer gespeichert. Sicherungen liegen im Unterordner „Sicherungen“.</p>
      <div style="display:flex;gap:8px;flex-wrap:wrap"><button class="knopf" id="e-daten">Datenordner öffnen</button><button class="knopf" id="e-ende">Programm beenden</button></div>
      ${Z.geloescht.length ? `<h3 style="margin-top:16px">Gelöschte Listen</h3><table class="einfach"><tbody>${Z.geloescht.map((g, i) => `<tr><td>${esc(g.titel)}</td><td>${g.geraete} Geräte</td><td>${new Date(g.zeit).toLocaleString("de-DE")}</td><td style="text-align:right"><button class="knopf" data-g="${i}">Wiederherstellen</button></td></tr>`).join("")}</tbody></table>` : ""}
    </div>
  </div>`;
  updateBandVerbinden(h);
  $("#e-ro").onclick = () => reportOrdnerWaehlen(true);
  $("#e-auto").onchange = async (e) => { try { await api("/api/einstellungen", { autoSync: e.target.checked }); await statusLaden(); } catch (er) { fehlerToast(er); } };
  $("#e-test").onchange = async (e) => { try { await api("/api/einstellungen", { testversionen: e.target.checked }); await api("/api/update?aktion=pruefen"); await statusLaden(); zeichnen(); } catch (er) { fehlerToast(er); } };
  $("#e-pruef").onclick = async (e) => { knopfLaedt(e.currentTarget, true, "Prüfe …"); try { await api("/api/update?aktion=pruefen"); await statusLaden(); zeichnen(); } catch (er) { fehlerToast(er); knopfLaedt(e.currentTarget, false); } };
  $("#e-upd") && ($("#e-upd").onclick = (e) => updateInstallieren(e.currentTarget));
  $("#e-daten").onclick = () => api("/api/datenordner", {}).catch(fehlerToast);
  $("#e-ende").onclick = async () => {
    const offen = Z.listen.filter((l) => l.syncOffen && l.empfaenger).length;
    if (!(await frage("Programm beenden?", offen && Z.status.autoSync ? `${offen} Liste(n) mit Änderungen werden vorher an die Kollegen gesendet.` : "Das Programm wird geschlossen.", "Beenden"))) return;
    await api("/api/beenden", {}).catch(() => {});
    document.body.innerHTML = `<div class="leerseite"><img src="icon.svg" alt=""><h1>Programm beendet</h1><p>Dieses Fenster kann geschlossen werden.</p></div>`;
    setTimeout(() => window.close(), 800);
  };
  h.addEventListener("click", async (e) => {
    const b = e.target.closest("button[data-g]"); if (!b) return;
    const g = Z.geloescht[+b.dataset.g];
    const id = g.datei.slice(0, g.datei.lastIndexOf("_", g.datei.lastIndexOf("_") - 1));
    try { await api("/api/wiederherstellen", { id, datei: g.datei }); await listenLaden(); toast("Liste wiederhergestellt.", "gut"); zeichnen(); } catch (er) { fehlerToast(er); }
  });
}

// ----- Anleitung -----
function hilfeZeichnen(h) {
  h.innerHTML = updateBand() + `<div class="kopf"><div class="kopf-zeile"><div><h1>So funktioniert die Kundengeräteliste</h1><div class="unterzeile"><span>Alles, was die Excel-Geräteliste konnte – plus Übersicht und Truck-Reports</span></div></div></div><div style="height:14px"></div></div>
  <div class="inhalt anleitung">
    <h3>Excel-Listen übernehmen</h3>
    <p>Über „Excel-Listen importieren“ (oder die .xlsm-Dateien einfach ins Fenster ziehen) werden die Gerätelisten mit allen Registern, Kommentaren, der E-Mail-Adresse des Kollegen und dem Abgleich-Betreff übernommen. Gibt es die Liste schon, kannst du sie abgleichen, ersetzen oder als neue Liste anlegen.</p>
    <h3>Neue Geräteliste anlegen</h3>
    <p>„+ Neu“ in der Seitenleiste: Kundennamen eintragen, Aufbau wählen (Standard oder mit Hersteller/Kunde/Bereich) und bei Bedarf mehrere Standorte als Register anlegen.</p>
    <h3>Bearbeiten</h3>
    <ul><li>In eine Zelle klicken und tippen. <kbd>Enter</kbd> speichert und springt eine Zeile tiefer, <kbd>Tab</kbd> eine Spalte weiter, <kbd>Esc</kbd> bricht ab. Alles wird sofort gespeichert.</li>
      <li>Datumsfelder als <kbd>MM/JJJJ</kbd> (z. B. 06/2026) oder <kbd>TT.MM.JJJJ</kbd> eingeben. Bei „Wartung“ ist auch Text wie „fällig 1708h“ erlaubt.</li>
      <li>Nächste UVV wird automatisch berechnet (letzte UVV + 12 Monate): rot = überfällig, orange = fällig in den nächsten 30 Tagen. Graue Felder fehlen noch.</li>
      <li>Rechtsklick auf eine Zeile oder „⋯“: Kommentar, Truck-Reports des Geräts, in anderes Register verschieben, Gerät löschen.</li>
      <li>Auf eine Spaltenüberschrift klicken sortiert die Liste. Register per Doppelklick umbenennen oder löschen.</li></ul>
    <h3>Abgleich mit dem Kollegen</h3>
    <ul><li>Einmal in den Listen-Einstellungen die E-Mail des Kollegen eintragen. Der Abgleich läuft über das klassische Outlook – genau wie bei der Excel-Liste, auch mit Kollegen, die noch die Excel-Datei nutzen.</li>
      <li>Liste öffnen: neue Daten vom Kollegen werden automatisch übernommen. Programm schließen: geänderte Listen werden automatisch gesendet.</li>
      <li>„Synchronisieren“ holt sofort neue Daten und schickt die eigene Liste los.</li>
      <li>Haben beide dasselbe Feld geändert, gilt die neuere Änderung. Leere Felder löschen nichts.</li>
      <li>Geräte werden an der Seriennummer erkannt – gibt es eine Seriennummer doppelt (z. B. bei verschiedenen Herstellern), zusätzlich am Typ. Gelöschte Geräte werden nicht übertragen – ein Gerät, das raus soll, bitte bei beiden löschen.</li>
      <li>Die Mails mit dem Abgleich-Betreff nicht löschen oder verschieben, bevor die Liste geöffnet bzw. synchronisiert wurde.</li>
      <li>Notlösung, falls Outlook streikt: Anhang aus der Mail speichern und unter „Mehr → Datei einlesen“ auswählen.</li></ul>
    <h3>Sicherungen und Protokoll</h3>
    <p>Vor jeder Übernahme, jedem Einlesen und jedem Löschen wird eine Sicherung angelegt (die letzten 30 bleiben). Unter „Mehr → Sync-Protokoll“ steht jede übernommene Änderung mit altem und neuem Wert.</p>
    <h3>Truck-Reports</h3>
    <p>Unter Einstellungen den Ordner mit den Truck-Reports wählen. In jeder Liste zeigt der Reiter „Truck-Reports“ alle Reports, deren Dateiname (oder Inhalt) eine Seriennummer der Liste enthält. Anhaken und per E-Mail (Outlook öffnet eine fertige Mail mit Anhängen) oder WhatsApp senden: Die Dateien werden kopiert, WhatsApp öffnet sich – Chat wählen und <kbd>Strg</kbd>+<kbd>V</kbd>.</p>
    <h3>Excel und Drucken</h3>
    <p>„Mehr → Als Excel-Datei speichern“ erzeugt eine Excel-Datei im gewohnten Aufbau (mit Formel und Farben). „Drucken“ druckt das aktuelle Register im Querformat oder speichert es als PDF.</p>
  </div>`;
  updateBandVerbinden(h);
}

// ---------- Status, Update, Lebenszeichen ----------
async function statusLaden() {
  Z.status = await api("/api/status");
  $("#version").textContent = "Version " + Z.status.version;
  $("#nav-update").hidden = !(Z.status.update && Z.status.update.verfuegbar);
}

let updateTimer = null;
function updatePlanen() {
  const u = Z.status && Z.status.update;
  if (!u || !u.verfuegbar || Z.updateSpaeter || updateTimer) return;
  updateTimer = setTimeout(() => {
    updateTimer = null;
    if (Z.updateSpaeter || document.querySelector(".edit, .schleier")) return updatePlanen();
    updateInstallieren(null);
  }, 15000);
}

async function updateInstallieren(btn) {
  knopfLaedt(btn, true, "Wird installiert …");
  toast("Update wird geladen und installiert …");
  try {
    const j = await api("/api/update?aktion=installieren", {});
    toast(j.text, "gut");
    document.body.innerHTML = `<div class="leerseite"><img src="icon.svg" alt=""><h1>Update wird installiert</h1><p>Das Programm startet gleich in der neuen Version neu. Dieses Fenster kann geschlossen werden.</p></div>`;
    setTimeout(() => window.close(), 2500);
  } catch (e) { Z.updateSpaeter = true; fehlerToast(e); knopfLaedt(btn, false); zeichnen(); }
}

setInterval(() => api("/api/ping", {}).catch(() => {}), 5000);
addEventListener("pagehide", () => navigator.sendBeacon("/api/schliessen?token=" + TOKEN));
setInterval(async () => { try { const alt = JSON.stringify(Z.status && Z.status.update); await statusLaden(); if (JSON.stringify(Z.status.update) !== alt && !document.querySelector(".edit, .schleier")) zeichnen(); updatePlanen(); } catch {} }, 60000);

// ---------- Ereignisse ----------
$("#marke").onclick = () => { Z.ansicht = "start"; Z.liste = null; seitenleiste(); zeichnen(); };
$("#listen").addEventListener("click", (e) => { const b = e.target.closest(".liste-eintrag"); if (b) listeOeffnen(b.dataset.id); });
$("#listen-suche").addEventListener("input", seitenleiste);
$("#btn-neu").onclick = neueListe;
$("#btn-import").onclick = importWahl;
$("#datei-import").onchange = (e) => { const f = [...e.target.files]; e.target.value = ""; if (f.length) importieren(f); };
$("#datei-einlesen").onchange = (e) => { const f = e.target.files[0]; e.target.value = ""; if (f) einlesen(f); };
$$(".nav[data-ansicht]").forEach((b) => (b.onclick = () => { Z.ansicht = b.dataset.ansicht; if (Z.ansicht !== "liste") Z.liste = null; seitenleiste(); zeichnen(); }));

let ziehZaehler = 0;
addEventListener("dragenter", (e) => { if ([...e.dataTransfer.types].includes("Files")) { ziehZaehler++; $("#ablage").hidden = false; } });
addEventListener("dragleave", () => { if (--ziehZaehler <= 0) { ziehZaehler = 0; $("#ablage").hidden = true; } });
addEventListener("dragover", (e) => e.preventDefault());
addEventListener("drop", (e) => {
  e.preventDefault(); ziehZaehler = 0; $("#ablage").hidden = true;
  const f = [...e.dataTransfer.files].filter((x) => /\.xls[xm]$/i.test(x.name));
  if (f.length) importieren(f); else if (e.dataTransfer.files.length) toast("Bitte .xlsm- oder .xlsx-Dateien ablegen.", "fehler");
});
addEventListener("keydown", (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "f" && Z.ansicht === "liste") { const s = $("#suche") || $("#rf"); if (s) { e.preventDefault(); s.focus(); s.select(); } }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "p" && Z.ansicht === "liste") { /* Browser-Druck */ }
});

(async function start() {
  try {
    await statusLaden();
    api("/api/ping", {}).catch(() => {});
    await listenLaden();
    zeichnen();
    updatePlanen();
  } catch (e) {
    $("#haupt").innerHTML = `<div class="leerseite"><h1>Keine Verbindung</h1><p>${esc(e.message)}</p></div>`;
  }
})();
