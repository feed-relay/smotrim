---
# Review and Fix brandsfilter changes

## Overview
Review the implementation of the brandsfilter feature, fix identified bugs and gaps, and ensure full test coverage for the new components.

## Context
- Files involved:
    - internal/service/subs_updater.go
    - internal/api/graphql/brands.go
    - internal/api/graphql/brand_test.go
    - cmd/update-subs/main.go
- Related patterns: Follow existing GraphQL client testing patterns and use moq for service interfaces.
- Dependencies: moq for mocking.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Fix bugs in SubsUpdater

**Files:**
- Modify: `internal/service/subs_updater.go`

- [x] Update allBrands to use s.client instead of creating a new graphql.Client instance
- [x] Include "Podcast" type in filtering logic (resolve TODO)
- [x] Improve error wrapping using fmt.Errorf and %w
- [x] run project test suite - must pass before task 2

### Task 2: Implement tests for SubsUpdater

**Files:**
- Modify: `internal/service/subs_updater.go`
- Create: `internal/service/subs_updater_test.go`

- [x] run make generate to create necessary mocks
- [x] implement tests for UpdateSubs covering:
    - cache hit and miss scenarios
    - API error handling
    - correct filtering of brands by type, status, and tariff
    - correct generation of subscriptions file
- [x] run project test suite - must pass before task 3

### Task 3: Implement tests for GraphQL Brands API

**Files:**
- Modify: `internal/api/graphql/brand_test.go`

- [ ] implement tests for Client.Brands and Client.BrandsRaw
- [ ] verify correct request construction (query and variables)
- [ ] verify correct response decoding
- [ ] run project test suite - must pass before task 4

### Task 4: Verify acceptance criteria

- [ ] run make test (race + coverage)
- [ ] run make lint
- [ ] verify test coverage meets 80%+

### Task 5: Update documentation

- [ ] update README.md if user-facing changes
- [ ] update CLAUDE.md if internal patterns changed
---
