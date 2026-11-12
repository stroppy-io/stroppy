# Stroppy kit 0.7.21

Vendored styles, fonts, and mark from:
https://github.com/stroppy-io/stroppy-cloud/releases/download/v0.7.21/stroppy-kit-0.7.21.tar.gz

Archive SHA-256: `f0f9552f7d58f73792d9629dc678c500180a345e74e278ce41c28b0a8d1bc634`.

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
