# Page override: redeem

- Base authority: ../MASTER.md
- Audience: customers completing a redemption or reviewing a renewal bill.
- Goal: reduce uncertainty. The page should feel like a clear service counter, not an administrator console.

## Direction

- Use a pale mint background, white service cards and one restrained emerald action color.
- Keep the two-column relationship: the active task on the left, human support or payment evidence on the right.
- Lead with a compact service hero, then the two entry choices, then only the fields needed for the current task.
- Keep technical language as quiet metadata (`ACCESS REQUEST`, service status); Chinese remains the primary content language.
- Treat the floating entitlement references as optional help. They must remain reachable without competing with the task.

## Component rules

- Cards use soft 18px corners, a subtle mint border and a low, stable shadow. Do not scale cards on hover.
- Form field indices are round and sequential; input focus uses a high-contrast emerald outline.
- The active entry card has a light mint surface plus a short bottom indicator; inactive cards stay visibly clickable.
- The right panel starts with support/payment state, shows one concise trust block, then the QR code/contact details.
- Preserve all existing redemption, renewal, validation and dialog behavior. This page override is presentation-only.

## Responsive rule

- Desktop keeps both cards aligned in one row.
- Below the desktop breakpoint, the action card stays first; support and payment remain available from the header dialog rather than forcing a long page.
