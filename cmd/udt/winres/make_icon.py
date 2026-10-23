"""Draws the program icon (icon.ico) - the same one as in the interface header.

Needed only when the icon changes: the finished icon.ico and the
rsrc_windows_*.syso files built from it are in the repository, and an
ordinary `go build` picks them up. How to rebuild - see README.md here.

The picture: a diagnostic socket (the OBD-II trapezoid) with a pulse line,
on the dark rounded square shared with the wheel-alignment program. The
small sizes (title bar, 16-24 px) leave out the contacts and draw thicker
lines, so they stay sharp.
"""
from PIL import Image, ImageDraw

S = 1024  # drawn large and scaled down, so the edges come out smooth

BG_TOP, BG_BOTTOM = (19, 30, 51), (10, 16, 29)
LIGHT = (232, 238, 248, 255)
PULSE = (43, 212, 106, 255)

# The socket outline and the pulse, in fractions of the side.
SOCKET = [(0.165, 0.295), (0.835, 0.295), (0.725, 0.725), (0.275, 0.725)]
PINS_Y = 0.385
PULSE_LINE = [(0.285, 0.565), (0.405, 0.565), (0.455, 0.465), (0.535, 0.665), (0.585, 0.565), (0.715, 0.565)]

SOCKET_SMALL = [(0.12, 0.25), (0.88, 0.25), (0.75, 0.76), (0.25, 0.76)]
PULSE_SMALL = [(0.27, 0.53), (0.35, 0.53), (0.43, 0.37), (0.57, 0.66), (0.65, 0.53), (0.73, 0.53)]


def background(radius):
    grad = Image.new('RGBA', (S, S))
    gd = ImageDraw.Draw(grad)
    for y in range(S):
        k = y / (S - 1)
        gd.line([(0, y), (S, y)], fill=tuple(round(a + (b - a) * k) for a, b in zip(BG_TOP, BG_BOTTOM)) + (255,))
    mask = Image.new('L', (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=int(S * radius), fill=255)
    im = Image.new('RGBA', (S, S), (0, 0, 0, 0))
    im.paste(grad, (0, 0), mask)
    return im


def stroke(d, pts, color, width, closed=False):
    pts = [(x * S, y * S) for x, y in pts]
    if closed:
        pts = pts + pts[:2]  # the join at the first corner is rounded too
    d.line(pts, fill=color, width=width, joint='curve')
    if not closed:
        for x, y in (pts[0], pts[-1]):
            d.ellipse([x - width / 2, y - width / 2, x + width / 2, y + width / 2], fill=color)


def draw():
    im = background(0.22)
    d = ImageDraw.Draw(im)
    stroke(d, SOCKET, LIGHT, int(S * 0.062), closed=True)
    # The upper row of contacts, as on the real socket.
    r = S * 0.026
    for i in range(5):
        x = S * (0.335 + i * 0.0825)
        d.ellipse([x - r, S * PINS_Y - r, x + r, S * PINS_Y + r], fill=LIGHT)
    stroke(d, PULSE_LINE, PULSE, int(S * 0.058))
    return im


def draw_small():
    im = background(0.18)
    d = ImageDraw.Draw(im)
    stroke(d, SOCKET_SMALL, LIGHT, int(S * 0.095), closed=True)
    stroke(d, PULSE_SMALL, PULSE, int(S * 0.085))
    return im


if __name__ == '__main__':
    big, small = draw(), draw_small()
    sizes = [16, 20, 24, 32, 40, 48, 64, 128, 256]
    # Every size is given ready-made: Pillow would otherwise shrink the
    # last given image, not the big one.
    frames = [(small if s <= 24 else big).resize((s, s), Image.LANCZOS) for s in sizes]
    frames[-1].save('icon.ico', sizes=[(s, s) for s in sizes], append_images=frames[:-1])
    big.resize((512, 512), Image.LANCZOS).save('icon-512.png')
    print('icon.ico, icon-512.png')
