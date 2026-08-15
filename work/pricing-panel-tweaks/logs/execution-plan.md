# Execution Plan: pricing-panel-tweaks

**Created:** 2026-08-15

---

## Wave 1 (independent)

### Task 1: Rename and reorder calculator modes, rename discount label (Increment 1)
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** open poshivon.ru/panel → «Настройки» — plan order swapped, labels read "Продвинутый"/"Быстрый", discount section reads "Скидки за количество"

## Wave 2 (depends on Wave 1 — sequenced for file-safety, no logical dependency)

### Task 2: Add/delete foundation — constants, handlers, validation, unit tests
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `cd client && npx vitest run` → all new tests pass

## Wave 3 (depends on Wave 2)

### Task 3: Add/delete rows — Изделия (Increment 2)
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** add/remove a garment on the panel, confirm persistence and correct pricing in both calculator modes

### Task 4: Add/delete rows — Усложнения/Операции (Increment 2)
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** add/remove an operation on the panel, confirm persistence in both calculator modes

### Task 5: Add/delete rows — Скидки за количество (Increment 2)
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** add/delete a discount tier, confirm last-tier delete is disabled, confirm persistence

## Wave 4 — Audit Wave (depends on Wave 3)

### Task 6: Code Audit
- **Skill:** code-reviewing
- **Reviewers:** none (auditor IS the review)

### Task 7: Security Audit
- **Skill:** security-auditor
- **Reviewers:** none
- **Verify-smoke:** grep/git-log checks per task file

### Task 8: Test Audit
- **Skill:** test-master
- **Reviewers:** none

## Wave 5 — Final Wave (depends on Wave 4)

### Task 9: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify-user:** confirm the full user-spec "How to Verify" checklist (both increments) was walked through

## Checks requiring user involvement

- [ ] Task 1: user verifies plan order/labels/discount label on the panel (Инкремент 1)
- [ ] Task 3: user verifies add/delete of Изделия rows across both calculator modes
- [ ] Task 4: user verifies add/delete of Усложнения rows across both calculator modes
- [ ] Task 5: user verifies add/delete of Скидки rows, including last-tier delete guard
- [ ] Task 9: user confirms the full manual "How to Verify" checklist was completed
- [ ] After all waves: final review of the live/dev panel before this ships (deploy is a separate, deliberate git-tag action outside this feature's scope — not automated here)
