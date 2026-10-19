# Stroppy kit 0.1.0

Vendored styles, fonts, and mark from:
https://github.com/stroppy-io/stroppy-cloud/releases/download/v0.1.0/stroppy-kit-0.1.0.tar.gz

Archive SHA-256: `ba1bd6342e5333aa11e54f4e84810ef6f3e95caf6a96f3de6c4c89787aa57b22`.

The upstream CSS and assets are kept unchanged. The report adapter in `kit.go`
inlines font files and adds system-light and print token overrides. Report layout
lives in `style.css`; plain-HTML components come from `base.css`. Charts use the
same tokens on SVG, retaining numeric tables and native hover/focus without a
JavaScript runtime. Optional interactive charts embed the kit's `chart.js` and
`chart.css` (uPlot 1.6.32). `chart_runtime.go` adds three version-checked hooks to
the unchanged bundle at initialization: option/data configuration, mount
notification, and render-failure notification. `charts.js` uses those hooks for
elapsed axes, straight lines, gaps, bars, phase markers, and fallback lifecycle.
Replace this compatibility seam when the kit publishes equivalent supported APIs.

uPlot is MIT-licensed; see `UPLOT-LICENSE.txt`, also embedded in exported HTML.
The server permits the exact bundled script by CSP SHA-256, not arbitrary inline
scripts. Source values are JSON-encoded into escaped HTML attributes.

Inter and Roboto Mono are distributed under the SIL Open Font License 1.1;
see `OFL.txt`. The license is also included in generated HTML source.
