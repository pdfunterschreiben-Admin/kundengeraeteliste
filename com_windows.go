//go:build windows

package main

// Minimaler COM/IDispatch-Zugriff (fuer Outlook, Shell, Verknuepfungen) ohne Fremdbibliotheken.

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	ole32               = syscall.NewLazyDLL("ole32.dll")
	oleaut32            = syscall.NewLazyDLL("oleaut32.dll")
	procCoInitializeEx  = ole32.NewProc("CoInitializeEx")
	procCoUninitialize  = ole32.NewProc("CoUninitialize")
	procCLSIDFromProgID = ole32.NewProc("CLSIDFromProgID")
	procCoCreateInst    = ole32.NewProc("CoCreateInstance")
	procSysAllocString  = oleaut32.NewProc("SysAllocString")
	procSysFreeString   = oleaut32.NewProc("SysFreeString")
	procVariantClear    = oleaut32.NewProc("VariantClear")
	procGetActiveObject = oleaut32.NewProc("GetActiveObject")
)

type guid struct {
	D1 uint32
	D2 uint16
	D3 uint16
	D4 [8]byte
}

var (
	iidIDispatch = guid{0x00020400, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidNull      = guid{}
)

const (
	vtEmpty    = 0
	vtNull     = 1
	vtI2       = 2
	vtI4       = 3
	vtR4       = 4
	vtR8       = 5
	vtDate     = 7
	vtBstr     = 8
	vtDispatch = 9
	vtError    = 10
	vtBool     = 11
	vtVariant  = 12
	vtUnknown  = 13
	vtI1       = 16
	vtUI1      = 17
	vtUI2      = 18
	vtUI4      = 19
	vtI8       = 20
	vtUI8      = 21
	vtInt      = 22
	vtUint     = 23

	dispMethod      = 1
	dispPropertyGet = 2
	dispPropertyPut = 4
)

// VARIANT (64 und 32 Bit: 16 bzw. 24 Byte; wir nutzen die groessere Form)
type variant struct {
	VT        uint16
	r1, r2, r3 uint16
	Val       int64
	_         int64
}

type dispParams struct {
	Args      uintptr
	Named     uintptr
	CArgs     uint32
	CNamed    uint32
}

type excepInfo struct {
	wCode             uint16
	wReserved         uint16
	bstrSource        *uint16
	bstrDescription   *uint16
	bstrHelpFile      *uint16
	dwHelpContext     uint32
	pvReserved        uintptr
	pfnDeferredFillIn uintptr
	scode             int32
}

type Disp struct{ p uintptr }

func vtbl(p uintptr, i int) uintptr {
	tab := *(*uintptr)(unsafe.Pointer(p))
	return *(*uintptr)(unsafe.Pointer(tab + uintptr(i)*unsafe.Sizeof(uintptr(0))))
}

func (d *Disp) Release() {
	if d != nil && d.p != 0 {
		syscall.SyscallN(vtbl(d.p, 2), d.p)
		d.p = 0
	}
}

func bstr(s string) uintptr {
	p, _ := syscall.UTF16PtrFromString(s)
	r, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(p)))
	return r
}

func bstrText(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; ptr = unsafe.Add(ptr, 2) {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

func hrFehler(hr uintptr, was string) error {
	return fmt.Errorf("%s (Fehler 0x%08X)", was, uint32(hr))
}

// COM-Arbeit immer im selben Betriebssystem-Thread
func ComAusfuehren(f func() error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		hr, _, _ := procCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
		if int32(hr) < 0 && uint32(hr) != 0x80010106 {
			done <- hrFehler(hr, "COM konnte nicht gestartet werden")
			return
		}
		defer procCoUninitialize.Call()
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("interner Fehler: %v", r)
			}
		}()
		done <- f()
	}()
	return <-done
}

func clsid(progID string) (*guid, error) {
	var c guid
	p, _ := syscall.UTF16PtrFromString(progID)
	hr, _, _ := procCLSIDFromProgID.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&c)))
	if hr != 0 {
		return nil, hrFehler(hr, progID+" ist auf diesem PC nicht installiert")
	}
	return &c, nil
}

func ComErstellen(progID string) (*Disp, error) {
	c, err := clsid(progID)
	if err != nil {
		return nil, err
	}
	var p uintptr
	hr, _, _ := procCoCreateInst.Call(uintptr(unsafe.Pointer(c)), 0, 0x15, uintptr(unsafe.Pointer(&iidIDispatch)), uintptr(unsafe.Pointer(&p)))
	if hr != 0 || p == 0 {
		return nil, hrFehler(hr, progID+" konnte nicht gestartet werden")
	}
	return &Disp{p}, nil
}

// laufendes Programm holen (z. B. Outlook), sonst nil
func ComAktiv(progID string) *Disp {
	c, err := clsid(progID)
	if err != nil {
		return nil
	}
	var unk uintptr
	hr, _, _ := procGetActiveObject.Call(uintptr(unsafe.Pointer(c)), 0, uintptr(unsafe.Pointer(&unk)))
	if hr != 0 || unk == 0 {
		return nil
	}
	var p uintptr
	hr, _, _ = syscall.SyscallN(vtbl(unk, 0), unk, uintptr(unsafe.Pointer(&iidIDispatch)), uintptr(unsafe.Pointer(&p)))
	syscall.SyscallN(vtbl(unk, 2), unk)
	if hr != 0 || p == 0 {
		return nil
	}
	return &Disp{p}
}

func (d *Disp) id(name string) (int32, error) {
	n, _ := syscall.UTF16PtrFromString(name)
	var id int32
	hr, _, _ := syscall.SyscallN(vtbl(d.p, 5), d.p, uintptr(unsafe.Pointer(&iidNull)), uintptr(unsafe.Pointer(&n)), 1, 0x0400, uintptr(unsafe.Pointer(&id)))
	if hr != 0 {
		return 0, hrFehler(hr, "Unbekannte Eigenschaft "+name)
	}
	return id, nil
}

func zuVariant(v interface{}) (variant, func()) {
	var x variant
	frei := func() {}
	switch a := v.(type) {
	case nil:
		x.VT = vtEmpty
	case string:
		b := bstr(a)
		x.VT = vtBstr
		x.Val = int64(b)
		frei = func() { procSysFreeString.Call(b) }
	case bool:
		x.VT = vtBool
		if a {
			x.Val = 0xFFFF
		}
	case int:
		x.VT = vtI4
		x.Val = int64(int32(a))
	case int32:
		x.VT = vtI4
		x.Val = int64(a)
	case float64:
		x.VT = vtR8
		x.Val = int64(math.Float64bits(a))
	case *Disp:
		x.VT = vtDispatch
		x.Val = int64(a.p)
	default:
		x.VT = vtEmpty
	}
	return x, frei
}

func (d *Disp) invoke(flags uint16, name string, args ...interface{}) (variant, error) {
	var res variant
	if d == nil || d.p == 0 {
		return res, errors.New("Objekt nicht verfügbar")
	}
	id, err := d.id(name)
	if err != nil {
		return res, err
	}
	n := len(args)
	vars := make([]variant, n)
	var frees []func()
	for i, a := range args {
		v, f := zuVariant(a)
		vars[n-1-i] = v // umgekehrte Reihenfolge
		frees = append(frees, f)
	}
	defer func() {
		for _, f := range frees {
			f()
		}
	}()
	dp := dispParams{CArgs: uint32(n)}
	if n > 0 {
		dp.Args = uintptr(unsafe.Pointer(&vars[0]))
	}
	named := int32(-3) // DISPID_PROPERTYPUT
	if flags == dispPropertyPut {
		dp.Named = uintptr(unsafe.Pointer(&named))
		dp.CNamed = 1
	}
	var ex excepInfo
	var argErr uint32
	hr, _, _ := syscall.SyscallN(vtbl(d.p, 6), d.p, uintptr(id), uintptr(unsafe.Pointer(&iidNull)), 0x0400, uintptr(flags),
		uintptr(unsafe.Pointer(&dp)), uintptr(unsafe.Pointer(&res)), uintptr(unsafe.Pointer(&ex)), uintptr(unsafe.Pointer(&argErr)))
	runtime.KeepAlive(vars)
	if hr != 0 {
		msg := bstrText(ex.bstrDescription)
		if ex.bstrDescription != nil {
			procSysFreeString.Call(uintptr(unsafe.Pointer(ex.bstrDescription)))
		}
		if ex.bstrSource != nil {
			procSysFreeString.Call(uintptr(unsafe.Pointer(ex.bstrSource)))
		}
		if ex.bstrHelpFile != nil {
			procSysFreeString.Call(uintptr(unsafe.Pointer(ex.bstrHelpFile)))
		}
		if msg == "" {
			msg = name
		}
		return res, fmt.Errorf("%s (0x%08X)", msg, uint32(hr))
	}
	return res, nil
}

func varFrei(v *variant) { procVariantClear.Call(uintptr(unsafe.Pointer(v))) }

func (d *Disp) Call(name string, args ...interface{}) error {
	v, err := d.invoke(dispMethod|dispPropertyGet, name, args...)
	if err == nil {
		varFrei(&v)
	}
	return err
}

func (d *Disp) Put(name string, wert interface{}) error {
	v, err := d.invoke(dispPropertyPut, name, wert)
	if err == nil {
		varFrei(&v)
	}
	return err
}

// Objekt holen (Eigenschaft oder Methode mit Rueckgabe IDispatch)
func (d *Disp) Obj(name string, args ...interface{}) (*Disp, error) {
	v, err := d.invoke(dispMethod|dispPropertyGet, name, args...)
	if err != nil {
		return nil, err
	}
	if v.VT != vtDispatch || v.Val == 0 {
		varFrei(&v)
		return nil, errors.New(name + " liefert kein Objekt")
	}
	// Referenz behalten (kein VariantClear)
	return &Disp{uintptr(v.Val)}, nil
}

func (d *Disp) Str(name string, args ...interface{}) (string, error) {
	v, err := d.invoke(dispMethod|dispPropertyGet, name, args...)
	if err != nil {
		return "", err
	}
	defer varFrei(&v)
	switch v.VT {
	case vtBstr:
		return bstrText((*uint16)(unsafe.Pointer(uintptr(v.Val)))), nil
	case vtEmpty, vtNull:
		return "", nil
	}
	return fmt.Sprint(varZahl(v)), nil
}

func varZahl(v variant) float64 {
	switch v.VT {
	case vtI2:
		return float64(int16(v.Val))
	case vtI4, vtInt:
		return float64(int32(v.Val))
	case vtUI4, vtUint:
		return float64(uint32(v.Val))
	case vtI8:
		return float64(v.Val)
	case vtUI1:
		return float64(uint8(v.Val))
	case vtUI2:
		return float64(uint16(v.Val))
	case vtR8, vtDate:
		return math.Float64frombits(uint64(v.Val))
	case vtR4:
		return float64(math.Float32frombits(uint32(v.Val)))
	case vtBool:
		if int16(v.Val) != 0 {
			return 1
		}
	}
	return 0
}

func (d *Disp) Num(name string, args ...interface{}) (float64, error) {
	v, err := d.invoke(dispMethod|dispPropertyGet, name, args...)
	if err != nil {
		return 0, err
	}
	defer varFrei(&v)
	return varZahl(v), nil
}
