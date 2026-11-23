package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func xesc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\n':
			b.WriteString("&#10;")
		case '\r', '\t':
			b.WriteRune(r)
		default:
			if r < 0x20 || r == 0xFFFE || r == 0xFFFF {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

type sstBau struct {
	idx  map[string]int
	list []string
}

func (s *sstBau) id(t string) int {
	if i, ok := s.idx[t]; ok {
		return i
	}
	s.idx[t] = len(s.list)
	s.list = append(s.list, t)
	return len(s.list) - 1
}

// Stil-Nummern in styles.xml (cellXfs)
const (
	stStandard = 0
	stTitel    = 1
	stUnter    = 2
	stKopf     = 3
	stText     = 4 // Textformat @ (Nummer)
	stMonat    = 5 // MM/JJJJ
	stStunden  = 6 // #.##0
	stFett     = 7
	stFettR    = 8
	stDatumZeit = 9
	stDatum    = 10
	stWrap     = 11
)

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<numFmts count="3"><numFmt numFmtId="164" formatCode="mm\/yyyy"/><numFmt numFmtId="165" formatCode="dd/mm/yyyy\ hh:mm"/><numFmt numFmtId="166" formatCode="dd/mm/yyyy"/></numFmts>
<fonts count="5">
<font><sz val="11"/><name val="Calibri"/><family val="2"/></font>
<font><b/><sz val="14"/><color rgb="FF1F4E78"/><name val="Calibri"/><family val="2"/></font>
<font><i/><sz val="9"/><color rgb="FF595959"/><name val="Calibri"/><family val="2"/></font>
<font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="Calibri"/><family val="2"/></font>
<font><b/><sz val="11"/><name val="Calibri"/><family val="2"/></font>
</fonts>
<fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF1F4E78"/><bgColor indexed="64"/></patternFill></fill></fills>
<borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color rgb="FFBFBFBF"/></left><right style="thin"><color rgb="FFBFBFBF"/></right><top style="thin"><color rgb="FFBFBFBF"/></top><bottom style="thin"><color rgb="FFBFBFBF"/></bottom><diagonal/></border></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="12">
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="0" fontId="3" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf>
<xf numFmtId="49" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="top"/></xf>
<xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="top"/></xf>
<xf numFmtId="3" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="top"/></xf>
<xf numFmtId="0" fontId="4" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="0" fontId="4" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment horizontal="right"/></xf>
<xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="166" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="top" wrapText="1"/></xf>
</cellXfs>
<cellStyles count="1"><cellStyle name="Standard" xfId="0" builtinId="0"/></cellStyles>
<dxfs count="4">
<dxf><fill><patternFill><bgColor rgb="FFFFFFC0"/></patternFill></fill></dxf>
<dxf><fill><patternFill><bgColor rgb="FFD9D9D9"/></patternFill></fill></dxf>
<dxf><fill><patternFill><bgColor rgb="FFF8CBAD"/></patternFill></fill></dxf>
<dxf><fill><patternFill><bgColor rgb="FFFFE699"/></patternFill></fill></dxf>
</dxfs>
<tableStyles count="0" defaultTableStyle="TableStyleMedium2" defaultPivotStyle="PivotStyleLight16"/>
</styleSheet>`

func spaltenBreite(name string) float64 {
	switch {
	case gleich(name, "Nummer"):
		return 14.7
	case gleich(name, "Typ"):
		return 26
	case gleich(name, "Seriennummer"):
		return 19.7
	case gleich(name, "Anbaugerät"):
		return 32.4
	case gleich(name, "Info durchgeführte Arbeiten"):
		return 45
	case gleich(name, "Vermerk"):
		return 40.3
	case gleich(name, "Betriebsstunden"):
		return 22.6
	case IstDatumSpalte(name):
		return 17
	}
	return 16
}

type zellSchreiber struct {
	b   *strings.Builder
	sst *sstBau
}

func (z zellSchreiber) text(ref string, s string, stil int) {
	if s == "" {
		fmt.Fprintf(z.b, `<c r="%s" s="%d"/>`, ref, stil)
		return
	}
	fmt.Fprintf(z.b, `<c r="%s" s="%d" t="s"><v>%d</v></c>`, ref, stil, z.sst.id(s))
}

func (z zellSchreiber) zahl(ref string, f float64, stil int) {
	fmt.Fprintf(z.b, `<c r="%s" s="%d"><v>%s</v></c>`, ref, stil, strconv.FormatFloat(f, 'f', -1, 64))
}

func (z zellSchreiber) wert(ref string, w Wert, stil int) {
	switch w.T {
	case "n":
		if f, err := strconv.ParseFloat(w.V, 64); err == nil {
			z.zahl(ref, f, stil)
			return
		}
	case "d":
		if t, err := time.Parse("2006-01-02", w.V); err == nil {
			z.zahl(ref, ZuSerial(t), stil)
			return
		}
	}
	z.text(ref, w.V, stil)
}

func blattName(s string, vergeben map[string]bool) string {
	for _, c := range []string{"[", "]", ":", "*", "?", "/", "\\"} {
		s = strings.ReplaceAll(s, c, "_")
	}
	s = strings.Trim(strings.TrimSpace(s), "'")
	if s == "" {
		s = "Geräteliste"
	}
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	basis := s
	for i := 2; vergeben[strings.ToLower(s)]; i++ {
		suf := " " + strconv.Itoa(i)
		r := []rune(basis)
		if len(r)+len(suf) > 31 {
			r = r[:31-len(suf)]
		}
		s = string(r) + suf
	}
	vergeben[strings.ToLower(s)] = true
	return s
}

func blattRef(name string) string {
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

// Schreibt die Liste als .xlsx im Aufbau der Excel-Geraetelisten
// (Tabelle "Geraete"/"Geraete_...", Ueberschriften in Zeile 4, Blatt "SyncDaten").
// Diese Datei koennen auch Kollegen mit der Excel-Version einlesen.
func ListeAlsXlsx(l *Liste, mitSync bool) ([]byte, error) {
	TabellenPruefen(l)
	sst := &sstBau{idx: map[string]int{}}
	var blaetter []string
	var blattNamen []string
	var tabellen []string
	vergeben := map[string]bool{}
	tabVergeben := map[string]bool{}
	jetzt := time.Now()

	for ri, r := range l.Register {
		name := blattName(r.Name, vergeben)
		blattNamen = append(blattNamen, name)
		tab := r.Tabelle
		tabVergeben[strings.ToLower(tab)] = true
		nSp := len(r.Spalten)
		lastCol := spaltenName(nSp)
		var b strings.Builder
		zs := zellSchreiber{&b, sst}
		b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
		b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
		b.WriteString(`<sheetPr><pageSetUpPr fitToPage="1"/></sheetPr>`)
		zeilen := len(r.Zeilen)
		if zeilen == 0 {
			zeilen = 1
		}
		endRow := 4 + zeilen
		fmt.Fprintf(&b, `<dimension ref="A1:%s%d"/>`, lastCol, endRow+2)
		sel := ""
		if ri == 0 {
			sel = ` tabSelected="1"`
		}
		fmt.Fprintf(&b, `<sheetViews><sheetView workbookViewId="0"%s><pane ySplit="4" topLeftCell="A5" activePane="bottomLeft" state="frozen"/><selection pane="bottomLeft" activeCell="A5" sqref="A5"/></sheetView></sheetViews>`, sel)
		b.WriteString(`<sheetFormatPr defaultRowHeight="15"/><cols>`)
		for i, s := range r.Spalten {
			fmt.Fprintf(&b, `<col min="%d" max="%d" width="%.2f" customWidth="1"/>`, i+1, i+1, spaltenBreite(s))
		}
		b.WriteString(`</cols><sheetData>`)
		// Zeile 1-3
		fmt.Fprintf(&b, `<row r="1" ht="21" customHeight="1">`)
		zs.text("A1", l.Titel, stTitel)
		b.WriteString(`</row>`)
		unter := l.Untertitel
		if unter == "" || strings.HasPrefix(unter, "Stand:") || strings.HasPrefix(unter, "Zuerst in A1") {
			unter = "Stand: " + jetzt.Format("02.01.2006") + " · Nächste UVV = letzte UVV + 12 Monate (Formel) · Wartung als Datum eingeben (Anzeige MM/JJJJ) · rot = UVV überfällig, orange = fällig in den nächsten 30 Tagen"
		}
		b.WriteString(`<row r="2">`)
		zs.text("A2", unter, stUnter)
		b.WriteString(`</row>`)
		iUVV := r.Spalte(spLetzteUVV)
		iNae := r.Spalte(spNaechste)
		iStd := r.Spalte(spStunden)
		iSN := r.Spalte(spSN)
		// Empfaenger wie in Excel (H3/I3) nur im ersten Register
		if ri == 0 && l.Empfaenger != "" {
			b.WriteString(`<row r="3">`)
			zs.text("H3", "E-Mail an:", stFettR)
			zs.text("I3", l.Empfaenger, stStandard)
			b.WriteString(`</row>`)
		}
		fmt.Fprintf(&b, `<row r="4" ht="30" customHeight="1">`)
		for i, s := range r.Spalten {
			zs.text(fmt.Sprintf("%s4", spaltenName(i+1)), s, stKopf)
		}
		b.WriteString(`</row>`)
		for zi := 0; zi < zeilen; zi++ {
			rowN := 5 + zi
			fmt.Fprintf(&b, `<row r="%d">`, rowN)
			var z *Zeile
			if zi < len(r.Zeilen) {
				z = r.Zeilen[zi]
			}
			for i, s := range r.Spalten {
				ref := fmt.Sprintf("%s%d", spaltenName(i+1), rowN)
				stil := stWrap
				switch {
				case gleich(s, spNummer):
					stil = stText
				case IstDatumSpalte(s):
					stil = stMonat
				case gleich(s, spStunden):
					stil = stStunden
				}
				if i == iNae && iUVV >= 0 {
					u := spaltenName(iUVV+1) + strconv.Itoa(rowN)
					fmt.Fprintf(&b, `<c r="%s" s="%d"><f>IF(%s="","",EDATE(%s,12))</f></c>`, ref, stMonat, u, u)
					continue
				}
				var w Wert
				if z != nil && i < len(z.Werte) {
					w = z.Werte[i]
				}
				if gleich(s, spNummer) && w.T == "n" {
					w.T = "s"
				}
				zs.wert(ref, w, stil)
			}
			b.WriteString(`</row>`)
		}
		anz := endRow + 2
		fmt.Fprintf(&b, `<row r="%d">`, anz)
		zs.text(fmt.Sprintf("A%d", anz), "Anzahl Geräte: ", stFett)
		if iSN >= 0 {
			fmt.Fprintf(&b, `<c r="C%d" s="%d"><f>COUNTA(%s[%s])</f><v>%d</v></c>`, anz, stFett, tab, spSN, anzahlRegister(r))
		}
		if r.Notiz != "" {
			zs.text(fmt.Sprintf("D%d", anz), strings.ReplaceAll(r.Notiz, "\n", " · "), stStandard)
		}
		b.WriteString(`</row></sheetData>`)
		// bedingte Formatierung wie in der Vorlage
		rng := func(c int) string { return fmt.Sprintf("%s5:%s%d", spaltenName(c+1), spaltenName(c+1), endRow) }
		prio := 1
		fmt.Fprintf(&b, `<conditionalFormatting sqref="A5:%s%d"><cfRule type="expression" dxfId="0" priority="%d"><formula>MOD(ROW(),2)=0</formula></cfRule></conditionalFormatting>`, lastCol, endRow, 10)
		snRef := "$" + spaltenName(iSN+1) + "5"
		if iUVV >= 0 {
			c := spaltenName(iUVV+1) + "5"
			fmt.Fprintf(&b, `<conditionalFormatting sqref="%s"><cfRule type="expression" dxfId="1" priority="%d"><formula>AND(%s&lt;&gt;"",%s="")</formula></cfRule></conditionalFormatting>`, rng(iUVV), prio, snRef, c)
			prio++
		}
		if iNae >= 0 {
			c := spaltenName(iNae+1) + "5"
			fmt.Fprintf(&b, `<conditionalFormatting sqref="%s"><cfRule type="expression" dxfId="2" priority="%d"><formula>AND(%s&lt;&gt;"",%s&lt;TODAY())</formula></cfRule><cfRule type="expression" dxfId="3" priority="%d"><formula>AND(%s&lt;&gt;"",%s&lt;=TODAY()+30)</formula></cfRule></conditionalFormatting>`, rng(iNae), prio, c, c, prio+1, c, c)
			prio += 2
		}
		if iStd >= 0 {
			c := spaltenName(iStd+1) + "5"
			fmt.Fprintf(&b, `<conditionalFormatting sqref="%s"><cfRule type="expression" dxfId="1" priority="%d"><formula>AND(%s&lt;&gt;"",%s="")</formula></cfRule></conditionalFormatting>`, rng(iStd), prio, snRef, c)
		}
		b.WriteString(`<pageMargins left="0.4" right="0.4" top="0.6" bottom="0.6" header="0.3" footer="0.3"/>`)
		b.WriteString(`<pageSetup paperSize="9" orientation="landscape" fitToHeight="0"/>`)
		b.WriteString(`<tableParts count="1"><tablePart r:id="rId1"/></tableParts></worksheet>`)
		blaetter = append(blaetter, b.String())

		var t strings.Builder
		fmt.Fprintf(&t, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<table xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" id="%d" name="%s" displayName="%s" ref="A4:%s%d" totalsRowShown="0" headerRowDxfId="-1"><autoFilter ref="A4:%s%d"/><tableColumns count="%d">`,
			ri+1, tab, tab, lastCol, endRow, lastCol, endRow, nSp)
		for i, s := range r.Spalten {
			fmt.Fprintf(&t, `<tableColumn id="%d" name="%s"/>`, i+1, xesc(s))
		}
		t.WriteString(`</tableColumns><tableStyleInfo showFirstColumn="0" showLastColumn="0" showRowStripes="1" showColumnStripes="0"/></table>`)
		tabellen = append(tabellen, strings.Replace(t.String(), ` headerRowDxfId="-1"`, "", 1))
	}

	// SyncDaten (Zeitstempel je Feld)
	syncIdx := -1
	if mitSync {
		var b strings.Builder
		zs := zellSchreiber{&b, sst}
		b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
		b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
		keys := sortedKeys(l.Zeit)
		for i, k := range keys {
			fmt.Fprintf(&b, `<row r="%d">`, i+1)
			zs.text(fmt.Sprintf("A%d", i+1), k, stText)
			zs.zahl(fmt.Sprintf("B%d", i+1), l.Zeit[k], stStandard)
			b.WriteString(`</row>`)
		}
		b.WriteString(`</sheetData></worksheet>`)
		blaetter = append(blaetter, b.String())
		blattNamen = append(blattNamen, "SyncDaten")
		syncIdx = len(blaetter) - 1
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, inhalt string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(inhalt))
		return err
	}
	var ct strings.Builder
	ct.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`)
	for i := range blaetter {
		fmt.Fprintf(&ct, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i+1)
	}
	for i := range tabellen {
		fmt.Fprintf(&ct, `<Override PartName="/xl/tables/table%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"/>`, i+1)
	}
	ct.WriteString(`</Types>`)
	if err := add("[Content_Types].xml", ct.String()); err != nil {
		return nil, err
	}
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`)
	add("docProps/app.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Microsoft Excel</Application></Properties>`)
	add("docProps/core.xml", fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>%s</dc:title><dc:creator>Kundengeräteliste</dc:creator><dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created></cp:coreProperties>`, xesc(l.Titel), jetzt.UTC().Format("2006-01-02T15:04:05Z")))

	var wb strings.Builder
	wb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><bookViews><workbookView activeTab="0"/></bookViews><sheets>`)
	for i, n := range blattNamen {
		state := ""
		if i == syncIdx {
			state = ` state="hidden"`
		}
		fmt.Fprintf(&wb, `<sheet name="%s" sheetId="%d"%s r:id="rId%d"/>`, xesc(n), i+1, state, i+1)
	}
	wb.WriteString(`</sheets><definedNames>`)
	for i := range l.Register {
		fmt.Fprintf(&wb, `<definedName name="_xlnm.Print_Titles" localSheetId="%d">%s!$4:$4</definedName>`, i, xesc(blattRef(blattNamen[i])))
	}
	if l.ListenID != "" {
		fmt.Fprintf(&wb, `<definedName name="Listen_ID" hidden="1">"%s"</definedName>`, xesc(l.ListenID))
	}
	wb.WriteString(`</definedNames><calcPr calcId="191029" fullCalcOnLoad="1"/></workbook>`)
	add("xl/workbook.xml", wb.String())

	var wr strings.Builder
	wr.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := range blaetter {
		fmt.Fprintf(&wr, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
	}
	n := len(blaetter)
	fmt.Fprintf(&wr, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/></Relationships>`, n+1, n+2)
	add("xl/_rels/workbook.xml.rels", wr.String())
	add("xl/styles.xml", stylesXML)
	for i, s := range blaetter {
		add(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), s)
		if i < len(tabellen) {
			add(fmt.Sprintf("xl/worksheets/_rels/sheet%d.xml.rels", i+1), fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table%d.xml"/></Relationships>`, i+1))
		}
	}
	for i, t := range tabellen {
		add(fmt.Sprintf("xl/tables/table%d.xml", i+1), t)
	}
	var ss strings.Builder
	fmt.Fprintf(&ss, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`, len(sst.list), len(sst.list))
	for _, s := range sst.list {
		sp := ""
		if strings.TrimSpace(s) != s || strings.Contains(s, "\n") {
			sp = ` xml:space="preserve"`
		}
		fmt.Fprintf(&ss, `<si><t%s>%s</t></si>`, sp, xesc(s))
	}
	ss.WriteString(`</sst>`)
	add("xl/sharedStrings.xml", ss.String())
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func anzahlRegister(r *Register) int {
	n := 0
	for _, z := range r.Zeilen {
		if !r.Wert(z, spSN).Leer() {
			n++
		}
	}
	return n
}

func tabellenName(s string, vergeben map[string]bool) string {
	var b strings.Builder
	for _, c := range OhneUmlaute(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	basis := "Geraete_" + b.String()
	if basis == "Geraete_" {
		basis = "Geraete_Register"
	}
	t := basis
	for i := 2; vergeben[strings.ToLower(t)]; i++ {
		t = basis + strconv.Itoa(i)
	}
	return t
}

func sortedKeys(m map[string]float64) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sortStrings(k)
	return k
}
