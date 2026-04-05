# Cyber Command Design System

### 1. Overview & Creative North Star
**Creative North Star: The Kinetic Terminal**
Cyber Command is a high-density, technical design system that prioritizes information velocity and terminal-grade precision. It rejects the "soft" nature of modern consumer web design in favor of a "hard-tech" aesthetic—utilizing sharp corners, neon accents, and mono-spaced data visualization to create an atmosphere of mission-critical oversight.

The system breaks the standard grid by using high-contrast health indicators (neon glows) against an ultra-dark background, creating a sense of infinite depth. It is built for experts who require split-second status recognition.

### 2. Colors
The palette is rooted in an "Obsidian Foundation" (#090A0F), using high-chroma accent colors to signal system health and priority.

- **The "No-Line" Rule:** While the system utilizes 1px borders for card definition (to mimic hardware panels), structural sectioning should be achieved through surface shifts. Avoid standard horizontal rules; use background tonal changes or vertical spacers to separate data clusters.
- **Surface Hierarchy:** 
    - **Surface (Background):** #090A0F
    - **Surface Container (Cards/Inputs):** #13151E
    - **Surface Container High (Hover states):** #1D202D
- **The "Neon Signal" Rule:** Use high-saturation colors (Primary Cyan, Success Green, Danger Pink) exclusively for active status and interaction.
- **Signature Textures:** Utilize a 2px "Health Bar" top-border on cards to indicate urgency without overwhelming the layout.

### 3. Typography
The typography system is a hybrid of **Space Grotesk** for display/navigation and **JetBrains Mono** for data-heavy components.

- **Display (32px):** Used for primary status metrics. Bold, tight tracking (-0.015em).
- **Headline (20px):** For section headers and titles.
- **Data Mono (13px, 11px, 10px):** Used for all technical details, IPs, and hostnames. The use of 10px mono-spaced text in all-caps creates a "technical readout" feel.
- **Body (0.875rem):** Standard Inter font for readable instructional text.

### 4. Elevation & Depth
Elevation is expressed through **Chromatic Glows** rather than physical shadows.

- **The Layering Principle:** UI elements are layered using the `surface` to `surface-container` transition. A border of #2A2D3D provides the "panel" effect common in hardware interfaces.
- **Neon Shadows:** 
    - **Success:** `0 0 8px #39FF14`
    - **Danger:** `0 0 8px #FF0055`
    - **Warning:** `0 0 8px #FFB000`
- **Ghost Border:** Use `outline-variant` (#1D202D) for inactive or low-priority containers to keep the interface from feeling cluttered.

### 5. Components
- **Status Article (Card):** High-density panels with a 1px border. Should include a "Status Light" (neon dot) in the top right.
- **Histogram Sparklines:** 2px wide bars used to show 30-day history. Use `success` for 100% and `danger` for outages.
- **Health Ticker:** A full-width section with high-contrast display type for the "Global Success Rate."
- **Mono Inputs:** Search fields should use `JetBrains Mono` and show a `primary` focus ring to signal an "active terminal" state.
- **Command Buttons:** Small, square-cornered buttons with mono labels. Active filters should use a matching neon dot indicator.

### 6. Do's and Don'ts
- **Do:** Use mono-spaced fonts for any value that includes numbers or technical identifiers.
- **Do:** Use tight tracking on headers to maintain a "dense/technical" aesthetic.
- **Don't:** Use large border radii. The system is capped at a 4px (lg) maximum for buttons/cards; standard elements should be 2px.
- **Don't:** Use gradients for depth. Depth must be conveyed through flat tonal layering or blur-based glows.
- **Do:** Ensure all "Failing" status indicators include the neon-shadow property for maximum visual urgency.

---

### 7. Tailwind Implementation Reference

This section documents the canonical Tailwind CSS class patterns to use in Vue components. Use these consistently — never fall back to light-theme utilities (`bg-white`, `bg-gray-*`, `border-gray-*`, `text-blue-600`).

#### Color Tokens (defined in `style.css` via `@theme`)
| Token | Value | Usage |
|-------|-------|-------|
| `surface-950` | `#07070f` | Page background |
| `surface-900` | `#0e0e1a` | Cards, panels |
| `surface-800` | `#15152a` | Hover states, input backgrounds, secondary buttons |
| `surface-700` | `#1e1e38` | Borders, dividers |
| `surface-600` | `#28284e` | Subtle borders on interactive elements |
| `accent` | `#0ddbf2` | Primary interactions, focus rings, active states |
| `accent-dim` | `#0ab8cc` | Hover state of accent elements |

#### Buttons

**Primary action button** (create, save, trigger):
```html
class="rounded bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 transition-colors hover:bg-accent/20 disabled:opacity-50"
```

**Secondary / cancel button**:
```html
class="rounded border border-surface-600 bg-surface-700 px-4 py-2 text-sm font-medium text-slate-300 transition-colors hover:bg-surface-600"
```

**Small table action button** (View, Edit):
```html
class="rounded bg-surface-800 px-2.5 py-1 text-xs font-medium text-slate-300 hover:bg-surface-700"
```

**Destructive button** (Delete — standalone):
```html
class="rounded bg-red-500/10 px-2.5 py-1 text-xs font-medium text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20"
```

**Approve / success-intent button**:
```html
class="rounded bg-green-500/10 px-2.5 py-1 text-xs font-medium text-green-400 ring-1 ring-green-500/20 hover:bg-green-500/20"
```

**Reject / danger-intent button** (small):
```html
class="rounded bg-red-500/10 px-2.5 py-1 text-xs font-medium text-red-400 ring-1 ring-red-500/20 hover:bg-red-500/20"
```

#### Cards & Panels
```html
class="rounded border border-surface-700 bg-surface-900 p-6"
```
For section headers inside a card:
```html
class="text-lg font-semibold text-slate-100"
```

#### Forms

**Text input / textarea**:
```html
class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
```

**Mono input** (paths, commands, cron):
```html
class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
```

**Select / dropdown**:
```html
class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30"
```

**Disabled input**:
```html
class="... disabled:bg-surface-800 disabled:text-slate-600"
```

**Form field label**:
```html
class="block text-sm font-medium text-slate-400"
```

**Checkbox / radio accent**:
```html
class="text-accent"  <!-- or: accent-accent for native styling -->
```

#### Badges & Pills

**Neutral tag / badge**:
```html
class="rounded-full bg-surface-800 px-2 py-0.5 text-xs text-slate-400"
```

**Global scope badge**:
```html
class="rounded-full bg-cyan-500/10 px-2 py-0.5 text-xs font-medium text-cyan-400 ring-1 ring-cyan-500/20"
```

**Local / default scope badge**:
```html
class="rounded-full bg-surface-800 px-2 py-0.5 text-xs font-medium text-slate-400"
```

**Error / abort badge**:
```html
class="rounded-full bg-red-500/15 px-2 py-0.5 text-xs font-medium text-red-400 ring-1 ring-red-500/30"
```

#### Alerts / Banners

**Error**:
```html
class="rounded border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400"
```

**Success**:
```html
class="rounded border border-green-500/20 bg-green-500/10 p-3 text-sm text-green-400"
```

#### Filter Tabs (segmented control)
```html
<!-- Container -->
class="flex gap-1 rounded bg-surface-800 p-1"
<!-- Active tab -->
class="rounded px-3 py-1.5 text-sm font-medium text-slate-100 bg-surface-900 ring-1 ring-accent/30"
<!-- Inactive tab -->
class="rounded px-3 py-1.5 text-sm font-medium capitalize text-slate-500 hover:text-slate-300"
```

#### Text Links (navigation / inline)
```html
class="text-accent hover:text-accent-dim"
```

#### Inline Code / Mono Snippets
```html
class="rounded bg-surface-800 px-1.5 py-0.5 font-mono text-xs text-slate-300"
```

#### Section Dividers
Use background surface shifts instead of `<hr>`. For inner card divisions:
```html
class="border-t border-surface-700"
```