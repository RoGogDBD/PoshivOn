# Decisions Log: pricing-panel-tweaks

Agent reports on completed tasks. Each entry is written by the agent that executed the task.

---

<!-- Entries are added by agents as tasks are completed.

Format is strict — use only these sections, do not add others.
Do not include: file lists, findings tables, JSON reports, step-by-step logs.
Review details — in JSON files via links. QA report — in logs/working/.

## Task N: [title]

**Status:** Done
**Commit:** abc1234
**Agent:** [teammate name or "main agent"]
**Summary:** 1-3 sentences: what was done, key decisions. Not a file list.
**Deviations:** None / Deviated from spec: [reason], did [what].

**Reviews:**

*Round 1:*
- code-reviewer: 2 findings → [logs/working/task-N/code-reviewer-1.json]
- security-auditor: OK → [logs/working/task-N/security-auditor-1.json]

*Round 2 (after fixes):*
- code-reviewer: OK → [logs/working/task-N/code-reviewer-2.json]

**Verification:**
- `npm test` → 42 passed
- Manual check → OK

-->

## Task 1: Rename and reorder calculator modes, rename discount label (Increment 1)

**Status:** Done
**Commit:** 4edea63
**Agent:** main agent
**Summary:** Display-text-only edit in `client/src/pages/Panel.jsx`: swapped the two `calculatorModes` array entries so the `quick` entry renders first, renamed their labels to "Быстрый" / "Продвинутый", updated the fallback label string, and renamed the `DiscountsBlock` section title to "Скидки за количество". Per tech-spec Decision 1 the `value` identifiers (`masterpiece`/`quick`) and both `description` strings were left untouched, so calculation logic, storage and `normalizeCalculatorMode` are unaffected.
**Deviations:** None.

**Open question for the user (order ambiguity):** user-spec's narrative step 2 ("сначала «Продвинутый»... затем «Быстрый»") literally describes the pre-change order — i.e. a rename with no swap — while user-spec's own Acceptance Criteria ("бывший второй режим теперь показывается первым") requires an actual swap producing the opposite order. Tech-spec Decision 1 resolves this by explicitly instructing a positional swap, so this task implemented the swap: "Быстрый" now shows first, "Продвинутый" second. This is the single user-visible effect of the task and no automated check can catch a wrong guess, so it needs explicit user confirmation during manual verification. If the user intended the other order, revert the array swap (keeping the renames) and correct whichever user-spec sentence is wrong — Task 9 pre-deploy QA currently asserts the swapped order.

**Reviews:**

*Round 1:*
- code-reviewer: 4 findings (1 major = the order ambiguity above, 3 minor all out-of-scope/pre-existing) → [logs/working/task-1/code-reviewer-1.json](logs/working/task-1/code-reviewer-1.json)
- security-auditor: diff clean; 3 findings, all pre-existing repo debt unrelated to this change → [logs/working/task-1/security-auditor-1.json](logs/working/task-1/security-auditor-1.json)
- test-reviewer: OK, 0 findings — confirmed the no-automated-test decision is justified (no positional reads of `calculatorModes` exist) → [logs/working/task-1/test-reviewer-full-1.json](logs/working/task-1/test-reviewer-full-1.json)

No round 2 — no in-scope findings required a fix.

**Carried forward to Task 2:** `client/package.json` still has no test runner; vitest coverage for Increment 2's new validation logic (tech-spec Decision 7) remains outstanding. Security-auditor also flagged `vite ^5.4.10` high advisories worth folding into Task 2, which already edits that file.

**Verification:**
- `grep -n "Шедевр\|По быстрому\|Скидки по партиям" client/src/pages/Panel.jsx` → no matches
- `grep -n '"masterpiece"\|"quick"' client/src/pages/Panel.jsx` → same 7 occurrences as before (lines 85, 90, 98, 414, 1182, 1241, 1421); none added, removed or altered
- `npx vite build` → passes
- Verify-user (browser check of mode order and labels) → PENDING, awaiting user
