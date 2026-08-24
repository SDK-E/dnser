# DNS.er — brand variant specification

Derives entirely from the SDK Enterprises canonical guideline
(`sdk-e/app → docs/design/brand.md`). Palette, geometry rules and the
never-redraw constraint are inherited unchanged; only the wordmark text
differs (`SDK.` → `DNS.er`).

## Approved color variants

### Dark (primary)

| Element | Value |
|---|---|
| Background | `#082003` |
| Letters | `#d7e8d3` |
| Period/accent | `#2cdb16` |

### Light

| Element | Value |
|---|---|
| Background | `#d7e8d3` |
| Letters | `#082003` |
| Period/accent | `#2cdb16` |

## Asset policy

- The canonical `DNS.er` letterforms are a fixed graphic asset owned by
  SDK Enterprises. Agents MUST NOT re-type, approximate or redraw them.
- Runtime contract: the dashboard header serves
  `public/brand/dnser-logo-dark.png` when the owner has placed it there;
  until then a plain-text UI label is shown as chrome (not a logo).
- Once the owner supplies the PNG/SVG exports (dark + light, matching the
  table above), drop them at:
  - `internal/dashboard/webapp/public/brand/dnser-logo-dark.png`
  - `internal/dashboard/webapp/public/brand/dnser-logo-light.png`

## Interface palette mapping

Dashboard theme consumes the same tokens:

- primary/accent: `#2cdb16`
- dark surfaces: `#082003`
- muted foreground on dark: `#d7e8d3`
