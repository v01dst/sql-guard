#!/usr/bin/env python3
"""Render ANSI-colored terminal output as an SVG 'screenshot' for README use."""
import sys, re, html

def ansitorgb(code):
    return {
        '31': '#ff5555', '32': '#50fa7b', '33': '#f1fa8c', '34': '#bd93f9',
        '35': '#ff79c6', '36': '#8be9fd', '37': '#f8f8f2', '90': '#6272a4',
        '1': '#f8f8f2', '2': '#6272a4', '0': '#f8f8f2',
    }.get(code, '#f8f8f2')

def main():
    data = open(sys.argv[1], 'r', encoding='utf-8', errors='replace').read()
    out = sys.argv[2] if len(sys.argv) > 2 else 'out.svg'

    lines = data.split('\n')
    fg = '#f8f8f2'
    bold = False
    spans = []  # list of (x, line_idx, text, fill, bold)

    font_w = 8.2   # px per char (monospace)
    font_h = 18.0  # line height
    x0, y0 = 26, 42

    maxlen = 0
    for li, line in enumerate(lines):
        x = 0.0
        # parse ANSI escapes
        i = 0
        while i < len(line):
            c = line[i]
            if c == '\x1b' and i+1 < len(line) and line[i+1] == '[':
                j = i+2
                while j < len(line) and line[j] not in 'm':
                    j += 1
                code = line[i+2:j]
                if line[j] == 'm':
                    if code == '' or code == '0':
                        fg, bold = '#f8f8f2', False
                    elif code == '1':
                        bold = True
                    elif code == '22':
                        bold = False
                    elif code in ('31','32','33','34','35','36','37','90','2'):
                        fg = ansitorgb(code)
                i = j+1
                continue
            # plain char
            if c == '\t':
                c = '    '
            if c != '\n' and c != '\r':
                spans.append((x, li, c, fg, bold))
                x += font_w
            i += 1
        maxlen = max(maxlen, x/font_w)

    width = x0*2 + maxlen*font_w
    height = y0 + len(lines)*font_h + 20

    svg = []
    svg.append(f'<svg xmlns="http://www.w3.org/2000/svg" width="{width:.0f}" height="{height:.0f}" viewBox="0 0 {width:.0f} {height:.0f}">')
    svg.append(f'<rect width="100%" height="100%" rx="10" fill="#282a36"/>')
    # title bar dots
    for dx, color in [(0, '#ff5f57'), (18, '#febc2e'), (36, '#28c840')]:
        svg.append(f'<circle cx="{x0+10+dx}" cy="20" r="6" fill="{color}"/>')
    svg.append(f'<text x="{width/2:.0f}" y="24" font-family="monospace" font-size="13" fill="#6272a4" text-anchor="middle">sql-guard</text>')
    for x, li, ch, fill, bold in spans:
        esc = html.escape(ch, quote=False)
        if ch == ' ':
            continue
        weight = ' font-weight="bold"' if bold else ''
        svg.append(f'<text x="{x0+x:.1f}" y="{y0+li*font_h:.1f}" font-family="monospace" font-size="15" fill="{fill}"{weight}>{esc}</text>')
    svg.append('</svg>')

    open(out, 'w').write('\n'.join(svg))
    print(f"wrote {out} ({width:.0f}x{height:.0f})")

if __name__ == '__main__':
    main()