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
| `/integrations` | APIs | `IntegrationDirectoryView` through `IntegrationsView` | Header, toolbar, API `DataTable`. |
| `/recipes` | Recipes | `RecipesView` | Header, metrics, evidence workbench, review queue. Uses the same route stack and width as every other route. |
| `/integrations/documentation` | Sources | `SourcesView` | Header, summary, toolbar, `DataTable`. |
| `/tools/connections` | MCP connections | `MCPConnectionsView` | Header, policy notice, token-authenticated upstream panel. |
| `/tools` | Tools | `ToolsView` | Header, policy notice, reviewed tool collection. |
| `/agent-access` | Agent access | `DistributionView` | Header, public MCP block, setup blocks, resource `DataTable`. |
| `/operations/outbox` | Support outbox | `OutboxView` | Header, plaintext queued reports and recent audit. |
| `/settings` | Settings overview | `SettingsView` | Header, shared settings tabs, settings collection, identity and administrator panels. |
| `/settings/ai` | AI configuration | `AISettingsView` | Header, shared settings tabs, AI readiness, provider connection, model selection, advanced prompts. |

### Context and entity routes

`/integration/:uid` has shared Quick Start, Documentation, Keys & Access, Tools, Test, and History tabs; `IntegrationWorkspaceView` owns their contextual content.

The generic `EntityDetailView` covers resource sets, sources, tools, MCP connections, reports, audit events, and root users. Invalid paths and unavailable records use `ConsoleNotFoundView` or the entity missing state without exposing internal routing errors.

### Dialog and transient views

`ConsoleApp` also owns Integration/resource/SDK editors, runtime access configuration, MCP import, visibility confirmation, support-report detail, AI provider/model configuration, and root administrator enrolment dialogs. They all use the owned `Dialog`, form, button, badge, and switch compositions. Toast messages use a polite status region; blocking authentication failures use an alert region.

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

### Knowledge library and imported evidence

The Knowledge route opens Reviewed content with Needs attention beside it.
Reviewed files come from each source’s latest included publication; a pending
import does not displace them. Import history and documentation sets are secondary
disclosures. Pending rows expose source names, actionable states and exact setup
continuation, including imports with no files. Both lists are bounded; pending
rows do not hydrate document bodies, source histories, AI or diagnostic payloads.
File content loads on selection, and previous included content only when the
operator requests a comparison. Focus refreshes the visible list; pending failures
remain actionable errors rather than an empty-success state. Review history, navigation maps, and processing
metadata live in secondary disclosure panels. A review decision is not a claim
that the content is currently served by an API.

Source creation keeps errors beside the input and preserves its request identity
across retry and reload. Recovery stores only a nonce and input digest for seven
days in the current deployment/reviewer browser scope; it never stores file text
or URLs. Storage failure is visible. Configured AI details use the shared compact
disclosure, while blockers stay expanded. Knowledge search controls wrap within
the available width, including with long source names and additional actions.

**Add content** opens `DocumentationSetupWorkspace` from Knowledge, source
maintenance, or API Resources. It keeps input, import state, required processing,
readable review, audience and the finish action in one page composed with
`PageHeader`, `PanelHeader`, owned controls and semantic spacing. The URL retains
the source, import and originating API. Import history is secondary, older
unpublished imports require the current review, and transient queue errors clear
when the committed job is observed. Source metadata refreshes in the library
after setup. Uploaded storage paths live under provenance; document rows show
their title and an Imported file label. Public confirmation and final review
acknowledgement remain explicit. Existing reviewed custom-set creation is
available within Attach existing rather than the primary Add content action.

`core/evidence-content.tsx` renders imported text using React text nodes with
headings, prose, lists, code, and validated HTTP(S) links. It never renders raw
HTML, images, scripts, or embeds. The shared `core-evidence-*` styles use the
existing theme, typography, spacing and focus tokens. Diff sections identify
removed and added text with headings as well as border colors.

Contract creation opens a continuous import/review workspace, retaining the exact
API and contract in its URL. The catalog shows readable contract content before
advanced inspection. Review groups operation descriptions and definitions with
source evidence, required AI processing, audience and the publication action.
Inherited parameters and changes outside operations remain visible. Imported
text uses the safe evidence renderer; JSON definitions remain inert. The input,
source, and import selectors use explicit accessible names. Source/import status
and advanced provenance remain available without becoming setup prerequisites.

### Required AI setup

`AIReadinessPanel` is the shared administrative prerequisite projection in empty
API setup, contract/source/SDK import, incomplete Knowledge processing, and recipe
authoring. It loads only readiness, not all AI settings or recipe histories. Local
configuration, historical connection tests, and actual processing are separate
states. Missing prerequisites disable generation/processing; a failed explicit
connection test is amber and can be retried. Completed processing remains available.
Provider/model settings have an ordered flow and an internal return-to-setup link.
Unsaved dialogs open configuration in another tab and refresh readiness on focus;
their selections remain in the original tab. The component uses shared controls,
semantic tokens, wrapping actions, and localized dates including reset times.


### SDK guidance workspace

The SDK directory opens a version chooser or exact guidance workspace. The normal
path has package name/URL resolution, bounded text import, required AI processing,
readable files and samples beside decisions, previous publication comparison, and
publish/attach recovery. It does not automatically select a package or expose raw
JSON as its default content. Candidate/publication identity survives advanced
inspection and return. The advanced catalog is loaded on demand and owns only
package metadata, exact release creation, lifecycle history and diagnostic
inspection; duplicate ingestion/review/attachment dialogs were removed.

Configured AI connection details can collapse within SDK input and review. Missing
prerequisites and failed connection tests stay visible. This changes presentation
only: the same readiness result gates processing. Review file lists, code, action
rows and comparisons wrap at narrow widths and use the shared semantic tokens.

Sample rows include language, source path and extracted line ranges to distinguish
repeated headings. The reader preserves literal sample code, including Markdown.
Sample comparison is labeled as a full source-file comparison with previously
included evidence from the same release. Saving disables draft controls without
prematurely labeling their decisions as published. Reload retains the exact
candidate's choices while resetting both review and public-audience confirmation.


### API publication review

Contract and custom documentation-set creation retain their request identity
through failed responses or attachment. Reload requires selecting and reviewing
the same input again. If browser storage is unavailable, a local message explains
that the form must remain open for retry. Existing dialogs and error controls
present recovery; internal request keys are not exposed in the form.

The API **Connect** tab retains the `/test` route for existing links and opens a
task-oriented workspace. It selects a published recipe and audience, checks the
exact task and its bound API publications, and displays readable canonical task
content. The initial audience follows the API's visibility. Collapsed evidence
rows identify their content kind; global publication rows include the revision.
Copying the prompt or downloading a check plan does not add a publication
approval step. Configuration preflight is secondary; the setup checklist opens
the publication review dialog directly.

Task connection uses the owned panel, field, button, badge and safe Markdown
components with semantic spacing and borders. Failed or mismatched previews
remain local actionable errors. Client-report import is scoped to the exact plan
and required checks; implementation fields explicitly record client-reported
results. Downloads retain evidence origin, and selection/reload clears the open
results. The page does not imply that simulated private preview authenticates a
customer or that reported results establish Tested status.

`IntegrationPublicationReviewDialog` owns the exact candidate review and approval.
Normal content groups named documentation, contracts, SDK versions, global
knowledge, tools, permissions and runtime connections. Additions, changes and
removals are visible; historical names come from the frozen snapshot. Version
labels resolve exact immutable records, with scope checks, before approval is
enabled. The row comparison shows changed audience and avoids repeating unchanged
version labels. Execution checks appear when tools are selected. Technical JSON
is secondary. Shared Dialog, controls and semantic tokens provide keyboard,
wrapping, dark-theme and narrow-layout behavior. Refresh clears acknowledgement;
Publish uses the already-reviewed candidate revision/hash.


The API publication delivery panel consumes the same server status as overview
and review. It names the serving revision, marks an incomplete newer publication
as needing attention, and retries the exact saved revision. Pending delivery uses
amber status indicators and an explicit finish-delivery checklist step. History
labels the publication actually serving through MCP rather than assuming the
newest row is active. Tab/focus refresh and affected mutation callbacks update
readiness; late responses from an earlier API cannot replace the current status.


Recipe reference editing uses an owned Dialog with Checkbox/CheckboxField/Label,
Select, safe EvidenceContent, and compact AI readiness. Names and retained text
precede collapsible version/ID/fingerprint details. Save errors remain in the
dialog's polite status region; choices persist until the user closes or explicitly
reloads. Reload discards local choices and rebinds the picker to the current saved
revision. Empty selections are supported. AI authoring/rework remains a separate
text prompt; the reference editor contains no JSON or instruction textarea.

### Correcting blocked source imports

Documentation setup exposes **Replace uploaded file** beside a failed or
quarantined upload. It retains the source/audience and supports exact committed
request recovery on reload. Corrected-file selection is scoped to that source;
public uploads require renewed audience acknowledgement for the replacement.
Blocked imports say they are blocked and show correction actions before AI
processing/publication actions. An older import offers **Open current import**.
Website recovery reuses the original source after the operator corrects its
content. These flows use existing details, labels, buttons and semantic panel
spacing, including narrow and keyboard layouts. New imports continue through
the existing required-AI and immutable publication review sequence.

### Contract review recovery

Contract setup keeps the primary-attachment choice beside exact contract/source
content, required AI evidence and the intended attachment audience. The choice
survives reload for the same candidate and API; final acknowledgement does not.
An unavailable browser store shows a local recovery limitation beside the choice
instead of claiming it was saved. The open review retains the choice during
automatic recovery. After publication succeeds, the remaining action reads
**Attach exact revision** and explains that publication is already saved.
Completion reflects the actual exact attachment and clears a recovered response
error. Choice and acknowledgement controls are disabled while finishing. These
states use the existing controls, semantic panels and narrow/keyboard behavior.

### MCP discovery pagination

Connect follows resource discovery pages before reviewing the selected task;
partial or inconsistent discovery cannot produce a check plan. The technical
MCP preview uses existing Button controls, heading actions and translated
pagination labels for each exact response. Audience, method, simulated grant
and refresh changes reset the page. Production-console browser checks covered a
task on page two, matching client-report import, keyboard next/previous actions,
exact-page copy and audience/method/refresh resets. Desktop/light and 390px/dark
screenshots were inspected, with no horizontal overflow or JavaScript errors.

Tool discovery uses the same controls and context reset behavior. Catalog version
2 is the preview default, showing compact deployment routing instead of repeated
API manifests. A second production-console fixture checked 42 tools over pages
of 32 and 10, and 43 over pages of 32 and 11 after selecting a simulated grant.
Removing that grant restarted the list and excluded the restricted tool; public
preview showed only its seven available built-ins. Exact-page copy, keyboard
navigation, method/audience changes and desktop/light plus 390px/dark layouts
passed. Acceptance client 0.3.0 also found a later-page tool and called the API
catalog through this actual local service. The fixture used demo identities;
its private preview correctly did not claim production OAuth was live.


### Compact publication review

Connect now renders compact exact publication maps and resolves supporting
references through exact source filters. It retains the existing publication and
evidence disclosures, keyboard controls and report import/export workflow.
Production-console checks used a private API index with 9,005 units and a public
one with 9,003. Private review made five exact lookups; public review made four
and retained historical global publication 1 after publication 2 became current.
Each exported seven exact resources and imported a passing client 0.3.0 report.
Desktop/light and 390px/dark screenshots were inspected without overflow or
JavaScript errors. These were local fixtures with synthetic extra evidence and
demo identities; the results do not establish production OAuth or application
implementation success.
