# Erzeugt rsrc_windows_amd64.syso (Programmsymbol + Manifest) und Kundengeraeteliste.ico
import struct, sys
src = sys.argv[1]
sizes = [16, 24, 32, 48, 64, 128, 256]
pngs = [open(f'{src}/icon_{s}.png', 'rb').read() for s in sizes]
# .ico-Datei
ico = struct.pack('<HHH', 0, 1, len(pngs)); off = 6 + 16 * len(pngs); body = b''
for s, d in zip(sizes, pngs):
    ico += struct.pack('<BBBBHHII', s % 256, s % 256, 0, 0, 1, 32, len(d), off + len(body)); body += d
open(sys.argv[2] + '/Kundengeraeteliste.ico', 'wb').write(ico + body)
manifest = b'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="1.0.0.0" processorArchitecture="amd64" name="Kundengeraeteliste" type="win32"/>
  <description>Kundengeraeteliste</description>
  <dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges><requestedExecutionLevel level="asInvoker" uiAccess="false"/></requestedPrivileges></security></trustInfo>
  <application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings><dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware><dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness></windowsSettings></application>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1"><application><supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/></application></compatibility>
</assembly>'''
grp = struct.pack('<HHH', 0, 1, len(pngs))
for i, (s, d) in enumerate(zip(sizes, pngs)):
    grp += struct.pack('<BBBBHHIH', s % 256, s % 256, 0, 0, 1, 32, len(d), i + 1)
# Ressourcen: typ -> [(id, daten)]
res = {3: [(i + 1, d) for i, d in enumerate(pngs)], 14: [(1, grp)], 24: [(1, manifest)]}
LANG = 0x0409
types = sorted(res)
# Layout berechnen
def dirsize(n): return 16 + 8 * n
root = 0
pos = dirsize(len(types))
typ_off = {}
for t in types: typ_off[t] = pos; pos += dirsize(len(res[t]))
name_off = {}
for t in types:
    for (i, _) in res[t]: name_off[(t, i)] = pos; pos += dirsize(1)
entry_off = {}
for t in types:
    for (i, _) in res[t]: entry_off[(t, i)] = pos; pos += 16
data_off = {}
for t in types:
    for (i, d) in res[t]:
        pos = (pos + 7) & ~7; data_off[(t, i)] = pos; pos += len(d)
buf = bytearray(pos)
def wdir(o, entries):
    struct.pack_into('<IIHHHH', buf, o, 0, 0, 0, 0, 0, len(entries))
    for k, (ident, target, sub) in enumerate(entries):
        struct.pack_into('<II', buf, o + 16 + 8 * k, ident, target | (0x80000000 if sub else 0))
wdir(root, [(t, typ_off[t], True) for t in types])
relocs = []
for t in types:
    wdir(typ_off[t], [(i, name_off[(t, i)], True) for (i, _) in res[t]])
    for (i, d) in res[t]:
        wdir(name_off[(t, i)], [(LANG, entry_off[(t, i)], False)])
        eo = entry_off[(t, i)]
        struct.pack_into('<IIII', buf, eo, data_off[(t, i)], len(d), 0, 0)
        relocs.append(eo)
        buf[data_off[(t, i)]:data_off[(t, i)] + len(d)] = d
raw = bytes(buf)
hdr_size = 20 + 40
raw_ptr = hdr_size
rel_ptr = raw_ptr + len(raw)
sym_ptr = rel_ptr + 10 * len(relocs)
out = struct.pack('<HHIIIHH', 0x8664, 1, 0, sym_ptr, 1, 0, 0)
out += struct.pack('<8sIIIIIIHHI', b'.rsrc', 0, 0, len(raw), raw_ptr, rel_ptr, 0, len(relocs), 0, 0x40000040)
out += raw
for r in relocs: out += struct.pack('<IIH', r, 0, 3)
out += struct.pack('<8sIhHBB', b'.rsrc', 0, 1, 0, 3, 0)
out += struct.pack('<I', 4)
open(sys.argv[3], 'wb').write(out)
print('ok', len(out))
