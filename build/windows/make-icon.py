from PIL import Image, ImageDraw

# TionHarness brand mark: "TH" monogram on the brand purple badge.
# Drawn with plain rectangles so it matches frontend/public/favicon.svg exactly.
S = 512
sc = S / 48.0
img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
d = ImageDraw.Draw(img)
r = int(11 * sc)
d.rounded_rectangle([0, 0, S - 1, S - 1], radius=r, fill=(134, 59, 255, 255))

bars = [
    (7, 14, 15, 4.4),
    (12.3, 14, 4.4, 20),
    (26, 14, 4.4, 20),
    (36.6, 14, 4.4, 20),
    (26, 21.8, 15, 4.4),
]
for x, y, w, h in bars:
    d.rounded_rectangle(
        [x * sc, y * sc, (x + w) * sc - 1, (y + h) * sc - 1],
        radius=int(1 * sc),
        fill=(255, 255, 255, 255),
    )

img.save("build/windows/icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
img.resize((256, 256), Image.LANCZOS).save("build/windows/icon.png")
