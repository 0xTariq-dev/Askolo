# Keyboard accessibility baseline

## Scope and baseline findings

This pass covers the signed-in application shell, primary navigation, the new
command menu, profile shortcut preferences, route changes, and reduced-motion
behavior. It is an incremental WCAG 2.2 AA implementation target, not a claim
that the full application conforms.

Before this pass, the shell had a skip link and focus styling on some navigation
controls, but no app command surface, no shortcut preference, and no deliberate
focus move after client-side route changes. The skip link also lacked a clear
focus ring. The mobile sheet already uses the shared dialog primitive, which
provides modal keyboard containment and Escape handling; the app-level duplicate
Escape listener has been removed so the dialog owns dismissal and focus return.

## Keyboard behavior

- The shared design-system command dialog handles arrow-key navigation, Enter
  activation, Escape dismissal, and dialog focus containment.
- The always-visible Commands button is the keyboard alternative to the
  shortcut. The button exposes its active binding with `aria-keyshortcuts`.
- The default is Alt/Option+Shift+K to avoid common browser search and developer
  tools shortcuts. Profile settings allow Ctrl/Cmd+K or disabling the binding.
  Ctrl/Cmd+K is handled only when enabled, with no extra modifiers, during
  ordinary key input, and outside editable controls or another dialog. Some
  browsers use Ctrl/Cmd+K for native search; on those browsers, keep the
  recommended Alt/Option+Shift+K binding or disable the app shortcut.
- Navigating to another route moves focus to that route's first heading (or the
  main landmark if it has no heading). The skip link moves focus to the main
  landmark.
- Shortcut preferences are stored in this browser's local storage; they do not
  sync between browsers or devices.

## Verification evidence

- Automated: `pnpm --filter @workspace/personal-assistant run test:a11y:keyboard`
  covers platform modifier matching, exact modifier behavior, disabled mode,
  editable targets, composition/repeat events, shortcut labels, and reduced
  motion CSS.
- Automated: `pnpm --filter @workspace/personal-assistant run typecheck` and
  `pnpm --filter @workspace/personal-assistant run build`.
- Manual keyboard checklist: Tab/Shift+Tab from the first page element; activate
  skip link; open and search Commands; use Up/Down, Enter, and Escape; confirm
  focus returns; navigate through desktop links and the mobile menu; change the
  binding in Profile; verify the browser's native search shortcut remains
  available when the app binding is disabled or shifted.
- Screen reader checklist: confirm the skip link, main landmark, active
  navigation page, Commands button and binding, named command dialog/search
  field, result names, and the shortcut preference/status. Verify Escape closes
  the dialog and focus returns to its opener.
- WAVE and Lighthouse were not available in this workspace session. Run both on
  the published landing page and a representative signed-in route before
  release; target Lighthouse accessibility 90+ on affected public surfaces.
  Record route-specific findings here and resolve critical blockers. This
  environment did not provide an authenticated browser session or a WAVE
  browser extension, so the manual keyboard and screen-reader checklists remain
  release checks rather than claims of completion.

## Reduced motion

The app disables smooth scrolling and shortens CSS animation and transition
durations when `prefers-reduced-motion: reduce` is active. The design-system
dialog also includes its own reduced-motion behavior. Verify both system
preference states during browser testing.