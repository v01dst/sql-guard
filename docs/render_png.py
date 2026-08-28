#!/usr/bin/env python3
"""Render ANSI-colored terminal output to a PNG (pure stdlib, no deps)."""
import sys, re, zlib, struct

def ansitorgb(code):
    # dracula palette
    return {
        '30':'#282a36','31':'#ff5555','32':'#50fa7b','33':'#f1fa8c','34':'#bd93f9',
        '35':'#ff79c6','36':'#8be9fd','37':'#f8f8f2','90':'#6272a4','2':'#6272a4',
    }.get(code, '#f8f8f2')

def hex2rgb(h):
    h = h.lstrip('#')
    return (int(h[0:2],16), int(h[2:4],16), int(h[4:6],16))

# 5x7 pixel font for chars we need (subset). fallback = filled block-ish.
FONT = {
 'A':[0,6,6,6,6,9,9],'B':[0,7,9,7,9,9,7],'C':[0,6,9,1,1,9,6],'D':[0,7,9,9,9,9,7],
 'E':[0,15,1,7,1,1,15],'F':[0,15,1,1,7,1,1],'G':[0,6,9,1,13,9,6],'H':[0,9,9,15,9,9,9],
 'I':[0,7,2,2,2,2,7],'J':[0,4,4,4,4,9,6],'K':[0,9,10,12,10,9,9],'L':[0,1,1,1,1,1,15],
 'M':[0,17,27,21,17,17,17],'N':[0,9,13,15,11,9,9],'O':[0,6,9,9,9,9,6],'P':[0,7,9,9,7,1,1],
 'Q':[0,6,9,9,9,10,5],'R':[0,7,9,9,7,9,9],'S':[0,6,9,1,6,8,7],'T':[0,7,2,2,2,2,2],
 'U':[0,9,9,9,9,9,6],'V':[0,9,9,9,9,6,6],'W':[0,17,17,17,21,27,17],'X':[0,9,9,6,6,9,9],
 'Y':[0,9,9,6,2,2,2],'Z':[0,15,8,4,2,1,15],
 'a':[0,0,6,9,9,9,6],'b':[0,1,1,7,9,9,7],'c':[0,0,6,9,1,9,6],'d':[0,8,8,14,9,9,14],
 'e':[0,6,9,15,1,9,6],'f':[0,12,2,7,2,2,2],'g':[0,6,9,9,14,8,7],'h':[0,1,1,7,9,9,9],
 'i':[0,2,0,2,2,2,2],'j':[0,4,0,4,4,4,3],'k':[0,1,1,9,14,9,9],'l':[0,2,2,2,2,2,2],
 'm':[0,0,0,27,21,21,21],'n':[0,0,0,7,9,9,9],'o':[0,0,0,6,9,9,6],'p':[0,0,0,7,9,9,7],
 'q':[0,0,0,14,9,9,14],'r':[0,0,0,7,9,1,1],'s':[0,0,0,14,1,6,15],'t':[0,2,2,7,2,2,12],
 'u':[0,0,0,9,9,9,14],'v':[0,0,0,9,9,6,6],'w':[0,0,0,17,17,21,27],'x':[0,0,0,9,6,6,9],
 'y':[0,0,0,9,9,14,7],'z':[0,0,0,15,4,2,15],
 '0':[0,6,9,11,13,9,6],'1':[0,2,6,2,2,2,7],'2':[0,6,9,8,4,2,15],'3':[0,14,1,2,1,9,6],
 '4':[0,8,12,10,15,8,8],'5':[0,15,1,7,8,9,6],'6':[0,6,1,7,9,9,6],'7':[0,15,8,4,2,2,2],
 '8':[0,6,9,6,6,9,6],'9':[0,6,9,9,14,8,7],
 ' ':[0,0,0,0,0,0,0],'.':[0,0,0,0,0,6,6],',':[0,0,0,0,6,2,4],':':[0,0,6,0,6,0,0],
 ';':[0,0,6,0,6,2,4],'-':[0,0,0,15,0,0,0],'_':[0,0,0,0,0,0,15],'/':[0,8,4,2,1,0,0],
 '+':[0,0,2,7,2,0,0],'!':[0,2,2,2,2,0,2],'?':[0,14,1,2,4,0,4],'(': [0,4,2,2,2,2,4],
 ')':[0,2,4,4,4,4,2],'=':[0,0,15,0,15,0,0],'*':[0,0,21,14,21,0,0],'@':[0,6,9,13,15,1,6],
 '#':[0,10,10,31,10,31,10],'~':[0,0,12,3,0,0,0],'>':[0,8,4,2,4,8,0],'<':[0,2,4,8,4,2,0],
 '[': [0,6,2,2,2,2,6],']':[0,6,4,4,4,4,6],'"':[0,10,10,0,0,0,0],"'":[0,2,2,0,0,0,0],
 '`':[0,2,4,0,0,0,0],'\\':[0,1,2,4,8,0,0],'|':[0,2,2,2,2,2,2],'^':[0,4,10,0,0,0,0],
 '&':[0,4,10,4,22,9,22],'%':[0,17,18,4,9,17,17],'$':[0,4,14,5,10,7,4],'→':[0,0,8,15,7,0,0],
 '✓':[0,0,0,0,8,4,2],'✗':[0,0,0,0,9,6,9],'△':[0,4,10,0,0,0,0],'·':[0,0,0,6,0,0,0],
}
g = lambda c: FONT.get(c, [0,15,15,15,15,15,15])

CW, CH = 6, 9   # char cell w,h
COL = CW  # advance

def draw_char(px, ox, oy, ch, color):
    rows = g(ch)
    for r in range(7):
        bits = rows[r]
        for c in range(5):
            if bits & (1 << (4-c)):
                for py in range(-1, 2):
                    for _px in range(1):
                        yy = oy + r + py
                        if 0 <= yy < len(px):
                            px[yy][ox+c] = color

def main():
    data = open(sys.argv[1], 'r', encoding='utf-8', errors='replace').read()
    out = sys.argv[2] if len(sys.argv) > 2 else 'out.png'
    lines = data.split('\n')

    fg = None
    bold = False
    # parse into (char, color, bold) per line
    parsed = []
    for line in lines:
        row = []
        xchar = 0
        i = 0
        while i < len(line):
            c = line[i]
            if c == '\x1b' and i+1 < len(line) and line[i+1] == '[':
                j = i+2
                while j < len(line) and line[j] != 'm':
                    j += 1
                code = line[i+2:j]
                if line[j]=='m':
                    if code in ('','0'): fg, bold = None, False
                    elif code=='1': bold=True
                    elif code=='22': bold=False
                    elif code in ('30','31','32','33','34','35','36','37','90','2'): fg=code
                i = j+1
                continue
            if c == '\t':
                c = ' '
            if c not in ('\n','\r'):
                row.append((c, fg, bold))
                xchar += 1
            i += 1
        parsed.append(row)

    maxc = max((len(r) for r in parsed), default=0)
    W = maxc * COL + 20
    H = len(parsed) * CH + 30
    bg = (40, 42, 54)
    px = [[bg for _ in range(W)] for _ in range(H)]

    # title bar
    for dx, col in [(12,(255,95,87)),(24,(254,188,46)),(36,(40,200,64))]:
        for dy in range(-3,4):
            for ddx in range(-3,4):
                xx, yy = dx+ddx, 12+dy
                if dx*dx+dy*dy <= 9:
                    px[yy][xx] = col

    ty0 = 24
    for li, row in enumerate(parsed):
        for ci, (ch, color, bold) in enumerate(row):
            if ch == ' ':
                continue
            col = hex2rgb(ansitorgb(color)) if color else (248,248,242)
            ox, oy = 12 + ci*COL, ty0 + li*CH
            draw_char(px, ox, oy, ch, col)
            if bold:
                draw_char(px, ox+1, oy, ch, col)

    # write PNG
    raw = b''.join(b'\x00' + b''.join(bytes([r,g,b]) for (r,g,b) in row) for row in px)
    def chunk(tag, data):
        return struct.pack('>I', len(data)) + tag + data + struct.pack('>I', zlib.crc32(tag+data) & 0xffffffff)
    ihdr = struct.pack('>IIBBBBB', W, H, 8, 2, 0, 0, 0)
    png = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', ihdr) + chunk(b'IDAT', zlib.compress(raw)) + chunk(b'IEND', b'')
    open(out, 'wb').write(png)
    print(f"wrote {out} ({W}x{H})")

if __name__ == '__main__':
    main()