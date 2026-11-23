# Rendert web/icon.svg in PNG-Größen (für .ico und .syso)
import sys
from playwright.sync_api import sync_playwright
svg = open(sys.argv[1]).read()
out = sys.argv[2]
with sync_playwright() as p:
    b = p.chromium.launch(executable_path='/usr/bin/google-chrome', args=['--no-sandbox'])
    for s in [16, 24, 32, 48, 64, 128, 256]:
        pg = b.new_page(viewport={'width': s, 'height': s})
        pg.set_content(f'<html><body style="margin:0;background:transparent">{svg.replace("<svg ", f"<svg width=\"{s}\" height=\"{s}\" ",1)}</body></html>')
        pg.screenshot(path=f'{out}/icon_{s}.png', omit_background=True)
        pg.close()
    b.close()
