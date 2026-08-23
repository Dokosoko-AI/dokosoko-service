# DokoSoko Core UI

This directory is the application-owned component system.

The complete route, component, typography, container, colour, and accessibility inventory is in [`docs/ui-system-audit.md`](../../../docs/ui-system-audit.md).

- Feature views use the composed controls from `control.tsx`.
- Lower-level primitives remain available from their individual modules.
- Visible colors, borders, focus states, spacing, and disabled states use the semantic tokens in `app/globals.css`.
- Product-specific styling uses the stable `core-*` hooks rather than utility overrides in feature views.
- New primitives must support light and dark themes, keyboard use, disabled states, and narrow layouts before adoption.
