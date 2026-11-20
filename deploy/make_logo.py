#!/usr/bin/env python3
# 장승 로고 SVG 만들기 : python3 -I deploy/make_logo.py  -> static/jangseung_*.svg
# 장승 = 세워 둔 통나무 기둥 (갓 + 금색 얼굴판 + 나뭇결). 해태 로고와 색 / 선 스타일은 같고 모양은 다름. 색 = 먹 #1B2A2F, 금 #D9A21B, 크림 #FFF6E0, 회색 #5B6770. 글자는 나눔스퀘어 라운드를 경로로 바꿈
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen

INK, GOLD, CREAM, GREY = "#1B2A2F", "#D9A21B", "#FFF6E0", "#5B6770"
FONT_B = "/usr/share/fonts/truetype/nanum/NanumSquareRoundB.ttf"
FONT_R = "/usr/share/fonts/truetype/nanum/NanumSquareRoundR.ttf"

# 장승 마크 : 세워 둔 통나무 기둥 (갓 + 얼굴 + 나뭇결). 해태의 구름 갈기와 다른 모양, 같은 색 / 선 스타일.
HAT = "M72 62 C 72 36, 128 36, 128 62 Z"                      # 갓 윗부분
BRIM = "M40 66 Q100 50 160 66 Q 160 74 152 74 Q100 62 48 74 Q 40 74 40 66 Z"   # 갓 챙 (넓게)
POST = (58, 64, 84, 122, 14)                                    # 기둥 x, y, 폭, 높이, 둥글기
PLATE = (66, 72, 68, 82, 10)                                    # 금색 얼굴판
EYE = (83, 104)                                                 # 눈 (한쪽, 반대쪽은 뒤집어 씀)
BROW = "M72 85 L96 94"                                          # 눈썹 : 안쪽으로 내려오는 성난 눈썹
NOSE = (93, 112, 14, 15, 6)                                     # 코 : 네모진 둥근 코
MOUTH = (74, 134, 52, 16, 5)                                    # 입 : 넓은 네모 입, 네모난 이빨
TEETH = [(82, 134, 11, 9), (107, 134, 11, 9)]
GRAIN = ["M72 168 q7 -5 14 0 t14 0 t14 0 t14 0", "M80 178 q6 -4 12 0 t12 0 t12 0"]   # 나뭇결

def rrect(r, fill, extra=""):
    x, y, w, h, rx = r
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{rx}" fill="{fill}" {extra}/>'

def mirror(s, ink=None):
    return f'<g transform="translate(200 0) scale(-1 1)">{s}</g>'

def mark(dark):
    """채운 마크 (200 x 200). dark = 어두운 바탕용 (바깥에 크림 테두리, 해태와 같음)"""
    ink, cream = INK, CREAM
    sil = rrect(POST, ink) + f'<path d="{HAT}" fill="{ink}"/><path d="{BRIM}" fill="{ink}"/>'
    halo = (f'<g fill="{cream}" stroke="{cream}" stroke-width="12" stroke-linejoin="round">' + sil.replace(f'fill="{ink}"', f'fill="{cream}"') + '</g>') if dark else ""
    eye = (f'<path d="{BROW}" fill="none" stroke="{ink}" stroke-width="6.5" stroke-linecap="round"/>'
           f'<circle cx="{EYE[0]}" cy="{EYE[1]}" r="10.5" fill="{ink}"/><circle cx="{EYE[0]+3}" cy="{EYE[1]-3}" r="3.5" fill="{cream}"/>')
    mx, my, mw, mh, mr = MOUTH
    face = (rrect(PLATE, GOLD) + f'<g>{eye}</g>{mirror(eye)}' + rrect(NOSE, ink) + rrect(MOUTH, ink) +
            "".join(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="{cream}"/>' for x, y, w, h in TEETH))
    grain = f'<g fill="none" stroke="{GOLD}" stroke-width="3.2" stroke-linecap="round">' + "".join(f'<path d="{g}"/>' for g in GRAIN) + '</g>'
    band = f'<path d="M80 56 H120" stroke="{GOLD}" stroke-width="3.2" stroke-linecap="round" fill="none"/>'   # 갓끈
    return halo + sil + band + face + grain

def line_mark(ink):
    """선 마크 (200 x 200) : 갓 + 기둥 윤곽 + 얼굴 선 + 나뭇결"""
    x, y, w, h, r = POST
    post = f"M{x+6} {y+4} V{y+h-r} Q{x+6} {y+h} {x+6+r} {y+h} H{x+w-6-r} Q{x+w-6} {y+h} {x+w-6} {y+h-r} V{y+4}"
    eye = (f'<path d="{BROW}" transform="translate(0 -4)"/><circle cx="{EYE[0]}" cy="{EYE[1]-4}" r="9"/>'
           f'<circle cx="{EYE[0]}" cy="{EYE[1]-4}" r="2.4" fill="{ink}" stroke="none"/>')
    nx, ny, nw, nh, nr = NOSE
    mx, my, mw, mh, mr = MOUTH
    return (f'<g fill="none" stroke="{ink}" stroke-width="5" stroke-linecap="round" stroke-linejoin="round">'
            f'<path d="{HAT}"/><path d="{BRIM}"/><path d="{post}"/>'
            f'<g>{eye}</g>{mirror(eye)}'
            f'<rect x="{nx}" y="{ny-2}" width="{nw}" height="{nh}" rx="{nr}"/>'
            f'<rect x="{mx}" y="{my}" width="{mw}" height="{mh}" rx="{mr}"/>'
            f'<path d="M90 {my+1} V{my+9} M110 {my+1} V{my+9}"/>'
            + "".join(f'<path d="{g}"/>' for g in GRAIN) + '</g>')

def text_path(font, s, x, y, size, track=0):
    f = TTFont(font); gs = f.getGlyphSet(); cmap = f.getBestCmap(); k = size / f["head"].unitsPerEm
    d, cx = [], x
    for ch in s:
        g = cmap[ord(ch)]; pen = SVGPathPen(gs)
        gs[g].draw(TransformPen(pen, (k, 0, 0, -k, cx, y)))
        d.append(pen.getCommands()); cx += gs[g].width * k + track
    return " ".join(d), cx - x - track

def svg(w, h, body):
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}">{body}</svg>\n'

def write(name, w, h, body):
    open(f"static/jangseung_{name}.svg", "w").write(svg(w, h, body))

def logo(dark):
    ink, sub = (CREAM, "#A9B4BC") if dark else (INK, GREY)
    d1, w1 = text_path(FONT_B, "Jangseung", 215, 110, 78)
    d2, w2 = text_path(FONT_R, "RPKI Web View", 217, 160, 34)
    W = int(max(215 + w1, 217 + w2) + 14)
    return W, svg_body(mark(dark), d1, d2, ink, sub)

def svg_body(m, d1, d2, ink, sub):
    return m + f'<path fill="{ink}" d="{d1}"/><path fill="{sub}" d="{d2}"/>'

if __name__ == "__main__":
    write("mark", 200, 200, mark(False))
    write("mark_dark", 200, 200, mark(True))
    write("line_mark", 200, 200, line_mark(INK))
    write("line_mark_dark", 200, 200, line_mark(CREAM))
    for dark, name in ((False, "logo"), (True, "logo_dark")):
        W, body = logo(dark)
        write(name, W, 200, body)
    print("ok")
