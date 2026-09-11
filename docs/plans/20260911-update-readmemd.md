---
# Update README.md

## Overview
Update the README.md to document the new subscription management functionality, including the SubsUpdater service and the update-subs CLI command.

## Context
- Files involved: README.md
- Related patterns: Existing README structure and style.
- Dependencies: None.

## Development Approach
- **Testing approach**: Regular (updates then manual verification)
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests** (verification of documentation accuracy)
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Update Architecture Overview

**Files:**
- Modify: `README.md`

- [x] Add a "Subscription Management" section.
- [x] Describe the role of SubsUpdater: fetching brands from Smotrim, filtering by accessibility/type (Radiobroadcast), and grouping by channel.
- [x] Explain that it generates the `etc/subscriptions.smotrim.yml` file used by the main provider.
- [x] Verify the new section is clear and accurate.

### Task 2: Update Building and Running section

**Files:**
- Modify: `README.md`

- [x] Add a subsection "Updating Subscriptions".
- [x] Document the command: `go run cmd/update-subs/main.go`.
- [x] Document the available flags: `-config` (Path to the YAML config file, defaults to `etc/config.yml`) and `-prod` (Use production data instead of test data).
- [x] Verify the command and flags are correct by checking `cmd/update-subs/main.go`.

### Task 3: Update Configuration section

**Files:**
- Modify: `README.md`

- [ ] Modify the description of the `subscriptions` field to mention that it can be automatically generated via the `update-subs` utility.
- [ ] Reference the generated file `etc/subscriptions.smotrim.yml`.
- [ ] Verify the reference is correct.

### Task 4: Verify acceptance criteria

- [ ] verify README is logically structured and contains all new functionality
- [ ] verify all documented commands work as described
- [ ] verify formatting is consistent

### Task 5: Update documentation

- [ ] update README.md if user-facing changes
- [ ] update CLAUDE.md if internal patterns changed
---
