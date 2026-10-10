# README artwork

`banner-ru.webp` and `flow-ru.webp` were generated with OpenAI's image generator for
Kinkan's Russian README, using the earlier SVG banner and diagram as references; they keep
the original Russian wording and the TrustTunnel fallback direction. The English README
uses the SVGs.

The panel's logo (`web/src/assets/logo.webp`, `web/public/favicon.png`,
`web/public/apple-touch-icon.png`) is the same kumquat, cut out on a transparent
background; the touch icon sits on Mikan's navy, as Mikan's mandarin does.

`scripts/readme/make-assets.py` rebuilds the SVGs and the demo screenshot video; it does
not recreate the generated artwork. `.github/README*.md` are the README sources (GitHub
shows `.github/README.md` first); the root README files stay as upstream writes them.
