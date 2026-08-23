# DokoSoko UI system audit

Updated 23 August 2026. This document is the source inventory and the contract for new console work.

## Audit result

The pre-normalisation browser audit found 14 rendered text sizes (8–32 px), 11 numeric font weights, three effective font stacks, and a different top-level block sequence on every primary route. Recipes also used a private width wrapper while other routes filled the content region. Several 8–10 px labels and low-contrast `--subtle` text failed the intended readability standard.

The normalised contract is now:

- one `ViewStack` route container on every console route;
- one `PageHeader`, `PageTabs`, `SectionHeader`, and `PanelHeader` hierarchy;
- six text sizes, four weights, and two font families;
- one semantic light scheme and one semantic dark scheme;
- `DataTable` for responsive grid tables and the native `Table` primitive for conventional tables;
- semantic tokens for every application colour outside the two theme declarations;
- a 320 px minimum layout, a single responsive page gutter, visible focus, reduced motion, and named controls.

## View catalogue

### Entry and authentication views

| View | Component | Notes |
| --- | --- | --- |
| Loading and session gate | `RootGate` | Authentication-safe server shell and client session resolution. |
| Initial workspace | `WorkspaceSetup` | Organisation and deployment creation. |
| Root enrolment | `SetupScreen` | Password, MFA, recovery codes, and completion states. |
| Root sign-in | `LoginScreen` | Email, password, and TOTP. |
| Shared auth frame | `AuthShell` | One title, description, problem, field, and action hierarchy. |

### Section routes

| Route | Section | View component | Primary composition |
| --- | --- | --- | --- |
| `/integrations` | APIs | `IntegrationDirectoryView` through `IntegrationsView` | Header, toolbar, grouped `DataTable`s. |
| `/recipes` | Recipes | `RecipesView` | Header, metrics, evidence workbench, review queue. Uses the same route stack and width as every other route. |
| `/integrations/documentation` | Sources | `SourcesView` | Header, summary, toolbar, `DataTable`. |
| `/access` | Service connections | `AccessView` | Header, panel lists, advanced disclosure. |
| `/integrations/mcp` | MCP connections | `MCPConnectionsView` | Header, policy notice, managed-upstream panel. |
| `/integrations/tools` | Tools | `ToolsView` | Header, policy notice, tool collection. |
| `/distribution/releases` | Compatibility snapshots | `ConnectorReleasesView` | Header, notice, metrics, published list. |
| `/agent-access` | Agent access | `DistributionView` | Header, public MCP block, setup blocks, resource `DataTable`. |
| `/widgets` | Widgets | `WidgetsView` | Header, security principle, `DataTable`. |
| `/activity` | Activity | `ActivityHubView` | Header, shared segmented filter, summary, run/report/audit panels. |
| `/operations/reporting` | Reporting | `ReportingView` | Header, policy block, connections and policy lists. |
| `/settings` | Settings overview | `SettingsView` | Header, shared settings tabs, settings collection, identity and administrator panels. |
| `/settings/ai` | AI providers | `AISettingsView` | Header, shared settings tabs, workload and provider tables. |

### Context and entity routes

`/integration/:uid` has the shared Overview, Resources, Access, and History tabs; `IntegrationWorkspaceView` owns their contextual content. `/widget/:uid` uses `WidgetDetailView` for configuration, install, security, empty, and loading states.

The generic `EntityDetailView` covers resource sets, sources, tools, MCP connections, access definitions, access connections, installations, releases, runs, support routes, reports, audit events, and root users. Invalid paths and unavailable records use `ConsoleNotFoundView` or the entity missing state without exposing internal routing errors.

### Dialog and transient views

`ConsoleApp` also owns the product importer, integration/resource editors, access configuration, MCP import, widget creation and credential reveal, visibility confirmation, support reporting, run creation, AI provider/model configuration, root administrator enrolment, and release lifecycle dialogs. They all use the owned `Dialog`, form, button, badge, and switch compositions. Toast messages use a polite status region; blocking authentication failures use an alert region.

## Component catalogue

### Route composition (`core/layout.tsx`)

| Primitive | Required use |
| --- | --- |
| `ViewStack` | The only console route root. It owns width and vertical rhythm. |
| `PageHeader` | One per route; eyebrow, `h1`, description, optional primary action. |
| `PageTabs` | Route-level navigation only; scrolls at narrow widths and exposes a navigation label. |
| `SectionHeader` | Introduces a standalone route section with `h2`. |
| `PanelHeader` | Introduces content inside a bounded panel; defaults to `h2`, supports `h3` only under a real `h2` section. |
| `SegmentedControl` | In-view filtering, never route navigation. Uses pressed button semantics. |
| `DataTable`, `DataTableHeader`, `DataTableRow`, `DataTableEmpty` | Responsive tabular directories. Own table, row, column-header, cell, and empty-state semantics. Interactive controls stay inside cells so their native roles are preserved. |

### Product compositions (`core/control.tsx`)

`Button`, `Badge`, `Switch`, and `Dialog` are the application-facing compositions. They provide stable `core-*` hooks and delegate behaviour to the lower-level owned primitives. New feature code should import these before reaching for the lower-level modules.

### Owned primitive inventory (`core/`)

| Family | Modules and exports |
| --- | --- |
| Actions | `button` (`Button`, `TouchTarget`), `link`, `dropdown`, `pagination`. |
| Forms | `input`, `textarea`, `select`, `checkbox`, `radio`, `switch`, `combobox`, `listbox`, `fieldset`. |
| Feedback | `alert`, `dialog`, `badge`, `avatar`. |
| Data and type | `table`, `description-list`, `heading`, `text`, `divider`. |
| Navigation and shell | `navbar`, `sidebar`, `sidebar-layout`, `stacked-layout`, `auth-layout`. |

These components live under `app/components/core`; no vendor kit is a public concept or import path. `core/README.md` defines contribution rules.

### Local product components

`ConsoleLink`, `EntityLink`, `CopyButton`, `ThemeToggle`, `AgentSetupCard`, `IntegrationSwitcher`, `CrawlBadge`, `AIWorkloadRow`, `AIProviderLogo`, `WarningContent`, `Confirmation`, `SummaryItem`, `Metric`, and `SettingsCard` encode product-specific behaviour or compact compositions. They should graduate to `core` only when a second unrelated feature needs the same contract.

## Typography catalogue

Only Geist and JetBrains Mono are loaded. Geist is used for interface copy and controls. JetBrains Mono is limited to code, identifiers, endpoints, hashes, and machine-readable values.

| Token | CSS value | Browser size | Use |
| --- | --- | --- | --- |
| `--text-caption` | `.75rem` | 12 px | Metadata, table headers, badges. Never smaller. |
| `--text-body` | `.8125rem` | 13 px | Default product copy and dense rows. |
| `--text-label` | `.875rem` | 14 px | Controls, descriptions, navigation, prominent labels. |
| `--text-heading` | `1rem` | 16 px | Section and panel headings. |
| `--text-metric` | `1.25rem` | 20 px | Summary values and compact empty-state titles. |
| `--text-title` | `1.75rem` | 28 px | The single page `h1`. |

The only weights are regular 400, medium 500, semibold 600, and bold 700. Raw feature-level font sizes and numeric weights are prohibited by `tests/ui-system.test.mjs`.

## Container catalogue

| Type | Token or primitive | Contract |
| --- | --- | --- |
| App shell | `.app-shell` | 248 px sticky sidebar plus a fluid main region; collapses to top navigation. |
| Top bar | `.topbar` | 64 px, same horizontal gutter as content. |
| Route stack | `ViewStack` | Full available width, 1 rem gap, no route-specific max width. |
| Page inset | `.content` | `--page-block` vertically and `--page-gutter` horizontally; mobile is 24/16/48 px. |
| Panel | `.panel` | One line, `--radius-panel`, semantic surface, restrained `--shadow-sm`. |
| Table | `DataTable` or `Table` | 56 px data rows, 40 px headers, explicit accessible name. |
| Section | `.section-block` + `SectionHeader` | Semantic grouping; route rhythm comes from `ViewStack`, not local margins. |
| Notice | `.notice` and semantic variants | Inline state or policy context; not a general decorative card. |
| Summary | `.summary-strip`, `.metrics-grid`, `.compact-metrics` | Structured values only; not a default page wrapper. |
| Toolbar | `.toolbar` | Search, filtering, and adjacent actions. |
| Dialog | owned `Dialog` | Labelled modal, focus-managed by Headless UI, responsive bottom sheet on mobile. |

Spacing uses `--space-section` (1 rem), `--space-view-section` (1.5 rem), `--space-panel` (1.125 rem), `--space-dialog` (1.5 rem), and the responsive page tokens. Feature views may use a smaller internal grid gap, but must not create an alternative page gutter or route width.

## Colour schemes

Both schemes expose the same semantic API:

- content: `--ink`, `--text-strong`, `--text`, `--muted`, `--subtle`;
- boundaries and surfaces: `--line`, `--line-strong`, `--surface`, `--surface-elevated`, `--surface-subtle`, `--soft`;
- interaction: `--accent`, `--accent-strong`, `--accent-fill`, `--accent-highlight`, `--accent-soft`, `--accent-line`, `--on-accent`;
- status: `--success*`, `--warning*`, `--danger*`, `--violet*`;
- inverse and code: `--inverse-*`, `--code-*`, `--overlay`;
- navigation: `--sidebar*`;
- provider marks and elevation: `--provider-*-soft`, `--shadow-sm`, `--shadow-overlay`.

Every raw colour is declared inside `:root` or `html[data-theme="dark"]`; feature selectors use semantic tokens. Inputs, textareas, selects, options, disabled fields, read-only fields, placeholders, and code surfaces therefore resolve consistently in both modes. `--subtle` maintains at least 4.5:1 contrast on both the base and subtle surfaces, enforced by the UI-system test.

## Accessibility and interaction contract

- Use native controls and keep their native roles; never turn a link or button into a table cell or row role.
- Every icon-only action needs a useful accessible name.
- Every table needs an accessible label and explicit header/row/cell semantics.
- Route changes move focus to `#main-content`; the skip link is the first keyboard path into content.
- Focus indicators use `--accent-highlight` and remain visible in both themes.
- Dense switches are at least 40 by 24 px; row icon actions are 32 by 32 px; primary controls are at least 36 px high.
- Empty, loading, error, disabled, read-only, and working states must retain their layout and plain-language status.
- Motion is limited to state transitions and the running indicator, and is disabled under `prefers-reduced-motion: reduce`.
- Responsive tables hide secondary columns only; the record name and its native link remain available.

## Adding a new view

Start with `ViewStack` (provided by `ConsoleApp`), then `PageHeader`. Add `PageTabs` only for URL-addressable sibling views, `SegmentedControl` only for local filters, `SectionHeader` for new semantic sections, and `PanelHeader` inside panels. Use `DataTable` for responsive record directories. Do not add a page max width, negative route margin, new font size, new numeric weight, or raw colour.
