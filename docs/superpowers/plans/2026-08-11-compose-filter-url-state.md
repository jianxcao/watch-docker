# Compose Filter URL State Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Compose list URL query the only source of truth for search, status filtering, and sorting, while preserving that query across detail navigation.

**Architecture:** Add a pure TypeScript module that parses and serializes the Compose list's four owned query parameters while preserving unrelated parameters. `ComposeView` derives all list state from `route.query` and updates it with `router.replace`; card and detail navigation copy the current query so returning restores the same list.

**Tech Stack:** Vue 3.5, Composition API, TypeScript 5.8, Vue Router 4.6, Vitest 3.2, pnpm.

## Global Constraints

- URL query is the only source of truth; do not add Pinia, local storage, `<KeepAlive>`, or duplicate local filter state.
- Query parameters are exactly `search`, `status`, `sort`, and `order`.
- Valid statuses are `running`, `exited`, and `other`; valid sort fields are `name` and `status`; valid orders are `asc` and `desc`.
- Defaults are empty search, all statuses, name sorting, and ascending order; default values are omitted from the URL.
- Preserve query parameters not owned by the Compose list.
- Keep existing filtering, sorting, data loading, and UI behavior unchanged apart from URL persistence.

---

### Task 1: Pure Compose list query state

**Files:**
- Create: `frontend/src/pages/composeListQuery.ts`
- Create: `frontend/src/pages/composeListQuery.test.ts`

**Interfaces:**
- Consumes: Vue Router `LocationQuery` values, including string, null, and array forms.
- Produces: `ComposeListState`, `parseComposeListQuery(query: LocationQuery): ComposeListState`, and `buildComposeListQuery(currentQuery: LocationQuery, state: ComposeListState): LocationQueryRaw`.

- [ ] **Step 1: Write failing parsing tests**

Create `frontend/src/pages/composeListQuery.test.ts` with tests that specify complete parsing, fallback behavior, and array handling:

```ts
import { describe, expect, it } from 'vitest'
import { buildComposeListQuery, parseComposeListQuery } from './composeListQuery'

describe('parseComposeListQuery', () => {
  it('parses every supported Compose list parameter', () => {
    expect(
      parseComposeListQuery({
        search: 'nginx',
        status: 'running',
        sort: 'status',
        order: 'desc',
      }),
    ).toEqual({ search: 'nginx', statusFilter: 'running', sortBy: 'status', sortOrder: 'desc' })
  })

  it('uses defaults for missing and invalid enum parameters', () => {
    expect(
      parseComposeListQuery({ status: 'paused', sort: 'created', order: 'sideways' }),
    ).toEqual({ search: '', statusFilter: '', sortBy: 'name', sortOrder: 'asc' })
  })

  it('uses the first string from array query values', () => {
    expect(
      parseComposeListQuery({
        search: [null, 'api'],
        status: ['exited', 'running'],
        sort: [null, 'status'],
        order: ['desc', 'asc'],
      }),
    ).toEqual({ search: 'api', statusFilter: 'exited', sortBy: 'status', sortOrder: 'desc' })
  })
})
```

- [ ] **Step 2: Run the parsing tests and verify RED**

Run:

```bash
cd frontend && pnpm test src/pages/composeListQuery.test.ts
```

Expected: FAIL because `./composeListQuery` does not exist.

- [ ] **Step 3: Write failing serialization tests**

Append tests that require default omission, non-default output, unrelated query preservation, and removal of previously owned values:

```ts
describe('buildComposeListQuery', () => {
  it('omits all default values', () => {
    expect(
      buildComposeListQuery({}, {
        search: '',
        statusFilter: '',
        sortBy: 'name',
        sortOrder: 'asc',
      }),
    ).toEqual({})
  })

  it('serializes every non-default value', () => {
    expect(
      buildComposeListQuery({}, {
        search: 'nginx',
        statusFilter: 'other',
        sortBy: 'status',
        sortOrder: 'desc',
      }),
    ).toEqual({ search: 'nginx', status: 'other', sort: 'status', order: 'desc' })
  })

  it('preserves unrelated values and removes owned values restored to defaults', () => {
    expect(
      buildComposeListQuery(
        { search: 'old', status: 'running', sort: 'status', order: 'desc', tab: 'summary' },
        { search: '', statusFilter: '', sortBy: 'name', sortOrder: 'asc' },
      ),
    ).toEqual({ tab: 'summary' })
  })
})
```

- [ ] **Step 4: Implement the minimal pure query module**

Create `frontend/src/pages/composeListQuery.ts`:

```ts
import type { LocationQuery, LocationQueryRaw, LocationQueryValue } from 'vue-router'

export type ComposeStatusFilter = '' | 'running' | 'exited' | 'other'
export type ComposeSortBy = 'name' | 'status'
export type ComposeSortOrder = 'asc' | 'desc'

export interface ComposeListState {
  search: string
  statusFilter: ComposeStatusFilter
  sortBy: ComposeSortBy
  sortOrder: ComposeSortOrder
}

const firstString = (
  value: LocationQueryValue | LocationQueryValue[] | undefined,
): string | undefined => {
  if (Array.isArray(value)) {
    return value.find((item): item is string => typeof item === 'string')
  }
  return typeof value === 'string' ? value : undefined
}

export const parseComposeListQuery = (query: LocationQuery): ComposeListState => {
  const status = firstString(query.status)
  const sort = firstString(query.sort)
  const order = firstString(query.order)

  return {
    search: firstString(query.search) ?? '',
    statusFilter:
      status === 'running' || status === 'exited' || status === 'other' ? status : '',
    sortBy: sort === 'status' ? 'status' : 'name',
    sortOrder: order === 'desc' ? 'desc' : 'asc',
  }
}

export const buildComposeListQuery = (
  currentQuery: LocationQuery,
  state: ComposeListState,
): LocationQueryRaw => {
  const query: LocationQueryRaw = { ...currentQuery }
  delete query.search
  delete query.status
  delete query.sort
  delete query.order

  if (state.search) query.search = state.search
  if (state.statusFilter) query.status = state.statusFilter
  if (state.sortBy !== 'name') query.sort = state.sortBy
  if (state.sortOrder !== 'asc') query.order = state.sortOrder

  return query
}
```

- [ ] **Step 5: Run focused tests and verify GREEN**

Run:

```bash
cd frontend && pnpm test src/pages/composeListQuery.test.ts
```

Expected: 6 tests PASS with no TypeScript transform errors.

- [ ] **Step 6: Commit the pure state module**

```bash
git add frontend/src/pages/composeListQuery.ts frontend/src/pages/composeListQuery.test.ts
git commit -m "feat: add compose list query state"
```

---

### Task 2: Route-backed list controls and detail return

**Files:**
- Modify: `frontend/src/pages/ComposeView.vue:123-276`
- Modify: `frontend/src/components/ComposeCard.vue:158-185,313-321`
- Modify: `frontend/src/pages/ComposeDetailView.vue:560-563`

**Interfaces:**
- Consumes: `parseComposeListQuery()` and `buildComposeListQuery()` from Task 1.
- Produces: writable route-backed control models and query-preserving list/detail navigation.

- [ ] **Step 1: Replace local filter refs with route-derived state**

In `ComposeView.vue`, import `useRoute`, the query helpers, and their state types. Keep modal refs local, but replace the four filter refs with this route-backed structure:

```ts
import { useRoute, useRouter } from 'vue-router'
import {
  buildComposeListQuery,
  parseComposeListQuery,
  type ComposeListState,
  type ComposeSortOrder,
  type ComposeStatusFilter,
} from './composeListQuery'

const route = useRoute()
const router = useRouter()

const composeListState = computed(() => parseComposeListQuery(route.query))

const updateComposeListState = (patch: Partial<ComposeListState>) => {
  const state = { ...composeListState.value, ...patch }
  void router.replace({ query: buildComposeListQuery(route.query, state) })
}

const searchKeyword = computed({
  get: () => composeListState.value.search,
  set: (search: string) => updateComposeListState({ search }),
})

const statusFilter = computed({
  get: () => composeListState.value.statusFilter,
  set: (statusFilter: ComposeStatusFilter) => updateComposeListState({ statusFilter }),
})

const sortBy = computed(() => composeListState.value.sortBy)
const sortOrder = computed(() => composeListState.value.sortOrder)
```

This preserves URL-only state: no watcher copies route values into refs.

- [ ] **Step 2: Make status and sort handlers update the route atomically**

Type the dropdown values and update sorting with one router replacement so `sortBy` and `sortOrder` cannot race:

```ts
const handleFilterSelect = (key: string | number) => {
  if (key === '' || key === 'running' || key === 'exited' || key === 'other') {
    statusFilter.value = key
  }
}

const handleSortSelect = (key: string | number) => {
  if (key !== 'name' && key !== 'status') {
    return
  }
  const nextOrder: ComposeSortOrder =
    sortBy.value === key ? (sortOrder.value === 'asc' ? 'desc' : 'asc') : 'asc'
  updateComposeListState({ sortBy: key, sortOrder: nextOrder })
}
```

- [ ] **Step 3: Preserve the query when entering Compose detail**

In `ComposeCard.vue`, import `useRoute`, initialize it beside the router, and include the query in the existing navigation:

```ts
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()

router.push({
  name: 'compose-detail',
  params: { projectName: props.project.name },
  query: route.query,
})
```

- [ ] **Step 4: Preserve the query when using the detail back button**

Update `ComposeDetailView.vue`:

```ts
const handleBack = () => {
  router.push({ name: 'compose', query: route.query })
}
```

- [ ] **Step 5: Run focused and full frontend verification**

Run:

```bash
cd frontend && pnpm test src/pages/composeListQuery.test.ts
cd frontend && pnpm test
cd frontend && pnpm check
cd frontend && pnpm build
```

Expected: all Vitest tests pass, `vue-tsc -b` exits 0, and Vite produces `frontend/dist` successfully.

- [ ] **Step 6: Perform route behavior QA**

Verify these exact cases in a local or deployed build with Compose enabled:

1. Open `/compose?search=nginx&status=running&sort=status&order=desc`; the controls and visible list match all four values.
2. Clear search, choose all statuses, and restore name ascending; owned query parameters disappear.
3. Keep an unrelated `tab=summary` query while changing filters; `tab=summary` remains.
4. Enter a Compose detail; the detail URL retains the list query.
5. Click the detail back button; `/compose` retains the same query and filtered list.
6. Use browser back/forward; controls and results follow the URL.

- [ ] **Step 7: Review and commit the Vue integration**

```bash
git diff --check
git diff -- frontend/src/pages/ComposeView.vue frontend/src/components/ComposeCard.vue frontend/src/pages/ComposeDetailView.vue
git add frontend/src/pages/ComposeView.vue frontend/src/components/ComposeCard.vue frontend/src/pages/ComposeDetailView.vue
git commit -m "feat: persist compose filters in URL"
```
