package main

import (
	"encoding/binary"
	"errors"
	"strings"
	"unicode/utf16"
)

// Liest den Quelltext der VBA-Module aus vbaProject.bin (OLE-Datei, MS-OVBA-Kompression).
func VBAModule(data []byte) (map[string]string, error) {
	st, err := oleStreams(data)
	if err != nil {
		return nil, err
	}
	dirC, ok := st["VBA/dir"]
	if !ok {
		return nil, errors.New("kein VBA-Verzeichnis")
	}
	dir, err := ovbaEntpacken(dirC)
	if err != nil {
		return nil, err
	}
	type mod struct {
		name string
		off  uint32
	}
	var mods []mod
	var cur mod
	for i := 0; i+6 <= len(dir); {
		id := binary.LittleEndian.Uint16(dir[i:])
		sz := int(binary.LittleEndian.Uint32(dir[i+2:]))
		if id == 9 {
			sz = 6
		}
		if i+6+sz > len(dir) {
			break
		}
		val := dir[i+6 : i+6+sz]
		i += 6 + sz
		switch id {
		case 0x19:
			cur = mod{name: string(val)}
		case 0x31:
			if len(val) >= 4 {
				cur.off = binary.LittleEndian.Uint32(val)
			}
		case 0x2B:
			mods = append(mods, cur)
		}
	}
	res := map[string]string{}
	for _, m := range mods {
		s, ok := st["VBA/"+m.name]
		if !ok || int(m.off) >= len(s) {
			continue
		}
		code, err := ovbaEntpacken(s[m.off:])
		if err != nil {
			continue
		}
		res[m.name] = cp1252(code)
	}
	return res, nil
}

func cp1252(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

func oleStreams(data []byte) (map[string][]byte, error) {
	if len(data) < 512 || binary.LittleEndian.Uint64(data) != 0xE11AB1A1E011CFD0 {
		return nil, errors.New("keine OLE-Datei")
	}
	le := binary.LittleEndian
	ss := 1 << le.Uint16(data[30:])
	mss := 1 << le.Uint16(data[32:])
	nfat := int(le.Uint32(data[44:]))
	dirStart := le.Uint32(data[48:])
	cutoff := le.Uint32(data[56:])
	miniStart := le.Uint32(data[60:])
	difatStart := le.Uint32(data[68:])
	ndifat := int(le.Uint32(data[72:]))
	sektor := func(n uint32) []byte {
		a := 512 + int(n)*ss
		if a < 0 || a+ss > len(data) {
			return nil
		}
		return data[a : a+ss]
	}
	var difat []uint32
	for i := 0; i < 109; i++ {
		difat = append(difat, le.Uint32(data[76+i*4:]))
	}
	s := difatStart
	for i := 0; i < ndifat && s < 0xFFFFFFFA; i++ {
		sec := sektor(s)
		if sec == nil {
			break
		}
		for j := 0; j < ss/4-1; j++ {
			difat = append(difat, le.Uint32(sec[j*4:]))
		}
		s = le.Uint32(sec[ss-4:])
	}
	var fat []uint32
	for i := 0; i < nfat && i < len(difat); i++ {
		sec := sektor(difat[i])
		for j := 0; sec != nil && j < ss/4; j++ {
			fat = append(fat, le.Uint32(sec[j*4:]))
		}
	}
	kette := func(s uint32) []byte {
		var out []byte
		for n := 0; s < 0xFFFFFFFA && int(s) < len(fat) && n < len(fat); n++ {
			out = append(out, sektor(s)...)
			s = fat[s]
		}
		return out
	}
	d := kette(dirStart)
	type eintrag struct {
		name                     string
		typ                      byte
		left, right, child       int32
		start, size              uint32
	}
	var e []eintrag
	for i := 0; i+128 <= len(d); i += 128 {
		x := d[i : i+128]
		nl := int(le.Uint16(x[64:]))
		if nl > 64 {
			nl = 64
		}
		u := make([]uint16, 0, nl/2)
		for j := 0; j+1 < nl; j += 2 {
			u = append(u, le.Uint16(x[j:]))
		}
		e = append(e, eintrag{strings.TrimRight(string(utf16.Decode(u)), "\x00"), x[66],
			int32(le.Uint32(x[68:])), int32(le.Uint32(x[72:])), int32(le.Uint32(x[76:])), le.Uint32(x[116:]), le.Uint32(x[120:])})
	}
	if len(e) == 0 {
		return nil, errors.New("leeres OLE-Verzeichnis")
	}
	mini := kette(e[0].start)
	var minifat []uint32
	if miniStart < 0xFFFFFFFA {
		mf := kette(miniStart)
		for j := 0; j+4 <= len(mf); j += 4 {
			minifat = append(minifat, le.Uint32(mf[j:]))
		}
	}
	miniKette := func(s uint32, size uint32) []byte {
		var out []byte
		for n := 0; s < 0xFFFFFFFA && int(s) < len(minifat) && n < len(minifat); n++ {
			a := int(s) * mss
			if a+mss > len(mini) {
				break
			}
			out = append(out, mini[a:a+mss]...)
			s = minifat[s]
		}
		if int(size) < len(out) {
			out = out[:size]
		}
		return out
	}
	res := map[string][]byte{}
	besucht := map[int32]bool{}
	var lauf func(i int32, pfad string)
	lauf = func(i int32, pfad string) {
		if i < 0 || int(i) >= len(e) || besucht[i] {
			return
		}
		besucht[i] = true
		x := e[i]
		lauf(x.left, pfad)
		lauf(x.right, pfad)
		p := x.name
		if pfad != "" {
			p = pfad + "/" + x.name
		}
		switch x.typ {
		case 2:
			if x.size < cutoff {
				res[p] = miniKette(x.start, x.size)
			} else {
				b := kette(x.start)
				if int(x.size) < len(b) {
					b = b[:x.size]
				}
				res[p] = b
			}
		case 1:
			lauf(x.child, p)
		}
	}
	lauf(e[0].child, "")
	return res, nil
}

func ovbaEntpacken(c []byte) ([]byte, error) {
	if len(c) == 0 || c[0] != 1 {
		return nil, errors.New("unbekannte Kompression")
	}
	var out []byte
	for i := 1; i+2 <= len(c); {
		h := int(binary.LittleEndian.Uint16(c[i:]))
		size := (h & 0xFFF) + 3
		end := i + size
		if end > len(c) {
			end = len(c)
		}
		chunk := c[i+2 : end]
		i += size
		if h&0x8000 == 0 {
			if len(chunk) > 4096 {
				chunk = chunk[:4096]
			}
			out = append(out, chunk...)
			continue
		}
		var dec []byte
		for j := 0; j < len(chunk); {
			flags := chunk[j]
			j++
			for b := 0; b < 8 && j < len(chunk); b++ {
				if flags&(1<<b) == 0 {
					dec = append(dec, chunk[j])
					j++
					continue
				}
				if j+2 > len(chunk) {
					j = len(chunk)
					break
				}
				tok := int(binary.LittleEndian.Uint16(chunk[j:]))
				j += 2
				dl := len(dec)
				bc := 4
				for (1 << bc) < dl {
					bc++
				}
				if bc > 12 {
					bc = 12
				}
				lm := 0xFFFF >> bc
				ln := (tok & lm) + 3
				off := (tok >> (16 - bc)) + 1
				if off > len(dec) {
					return out, errors.New("defekte Daten")
				}
				for k := 0; k < ln; k++ {
					dec = append(dec, dec[len(dec)-off])
				}
			}
		}
		out = append(out, dec...)
	}
	return out, nil
}
