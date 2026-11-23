// ===== Kundengeräteliste: Texterkennung (Typenschilder) =====
// Einbau in die Live-Datei des Apps-Script-Projekts „PDF Unterschreiben Anmeldung“:
//
// 1) In doPost(e) DIREKT VOR der Zeile
//      if (d && d.key === LISTEN_KEY && /^liste_/.test(String(d.event || ''))) {
//    diesen Block einfügen:
//
//  // Kundengeräteliste: Typenschild lesen – nur für angemeldete, freigeschaltete Service-Mappe-Nutzer
//  if (d && d.key === LISTEN_KEY && d.event === 'liste_ocr') {
//    try { return out(listeOcr(d)); } catch (x) { return out({ ok: false, error: 'Texterkennung: ' + x.message }); }
//  }
//
// 2) Diese Funktion z. B. direkt hinter function ocrAnfrage(d) { … } einfügen:

function listeOcr(d) {
  const users = SpreadsheetApp.getActiveSpreadsheet().getSheetByName('Nutzer');
  const mail = clean(d.email, 150).toLowerCase();
  const row = users ? findRow(users, clean(d.id, 64)) : -1;
  if (row < 0) throw new Error('Service-Mappe bitte neu anmelden.');
  const r = users.getRange(row, 1, 1, 8).getValues()[0];
  if (String(r[1]).trim().toLowerCase() !== mail || String(r[6]).toLowerCase() !== 'ja' || String(r[7]).trim().toLowerCase() === 'ja')
    throw new Error('Service-Mappe bitte neu anmelden.');
  const b64 = String(d.b64 || '').replace(/\s/g, '');
  if (b64.length < 100 || b64.length > 15000000) return { ok: false, error: 'Kein gültiges Foto.' };
  const mime = /^image\/(jpeg|png|gif|bmp|webp)$/.test(String(d.mime || '')) ? d.mime : 'image/jpeg';
  return { ok: true, text: googleOcr(b64, mime) };
}
