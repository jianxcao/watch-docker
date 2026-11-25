import { ref, watch, type Ref } from 'vue'
import type {
  LocationQuery,
  LocationQueryRaw,
  LocationQueryValue,
  RouteLocationNormalizedLoaded,
  Router,
} from 'vue-router'

export type ListSortOrder = 'asc' | 'desc'

export interface ListState<Status extends string, SortBy extends string> {
  search: string
  statusFilter: Status
  sortBy: SortBy
  sortOrder: ListSortOrder
}

const CONTAINER_STATUS_FILTERS = [
  'running',
  'stopped',
  'updateable',
  'uptodate',
  'error',
  'skipped',
] as const
const CONTAINER_SORT_BYS = ['name', 'created', 'status'] as const

export type ContainerStatusFilter = '' | (typeof CONTAINER_STATUS_FILTERS)[number]
export type ContainerSortBy = (typeof CONTAINER_SORT_BYS)[number]
export type ContainerSortOrder = ListSortOrder
export type ContainerListState = ListState<ContainerStatusFilter, ContainerSortBy>

const NETWORK_STATUS_FILTERS = [
  'used',
  'unused',
  'builtin',
  'custom',
  'driver-bridge',
  'driver-overlay',
  'driver-host',
  'driver-macvlan',
] as const
const NETWORK_SORT_BYS = ['name', 'created'] as const

export type NetworkStatusFilter = '' | (typeof NETWORK_STATUS_FILTERS)[number]
export type NetworkSortBy = (typeof NETWORK_SORT_BYS)[number]
export type NetworkSortOrder = ListSortOrder
export type NetworkListState = ListState<NetworkStatusFilter, NetworkSortBy>

const firstString = (
  value: LocationQueryValue | LocationQueryValue[] | undefined,
): string | undefined => {
  if (Array.isArray(value)) {
    return value.find((item): item is string => typeof item === 'string')
  }
  return typeof value === 'string' ? value : undefined
}

const parseListQuery = <Status extends string, SortBy extends string>(
  query: LocationQuery,
  validStatuses: readonly Status[],
  validSorts: readonly SortBy[],
  defaultSort: SortBy,
): ListState<'' | Status, SortBy> => {
  const status = firstString(query.status)
  const sort = firstString(query.sort)
  const order = firstString(query.order)

  const statusFilter =
    status && validStatuses.includes(status as Status) ? (status as Status) : ''
  const sortBy = sort && validSorts.includes(sort as SortBy) ? (sort as SortBy) : defaultSort

  return {
    search: firstString(query.search) ?? '',
    statusFilter,
    sortBy,
    sortOrder: order === 'desc' ? 'desc' : 'asc',
  }
}

const buildListQuery = <Status extends string, SortBy extends string>(
  currentQuery: LocationQuery,
  state: ListState<Status, SortBy>,
  defaultSort: SortBy,
): LocationQueryRaw => {
  const query: LocationQueryRaw = { ...currentQuery }
  delete query.search
  delete query.status
  delete query.sort
  delete query.order

  if (state.search) {
    query.search = state.search
  }
  if (state.statusFilter) {
    query.status = state.statusFilter
  }
  if (state.sortBy !== defaultSort) {
    query.sort = state.sortBy
  }
  if (state.sortOrder !== 'asc') {
    query.order = state.sortOrder
  }

  return query
}

export const parseContainerListQuery = (query: LocationQuery): ContainerListState =>
  parseListQuery(query, CONTAINER_STATUS_FILTERS, CONTAINER_SORT_BYS, 'name')

export const buildContainerListQuery = (
  currentQuery: LocationQuery,
  state: ContainerListState,
): LocationQueryRaw => buildListQuery(currentQuery, state, 'name')

export const parseNetworkListQuery = (query: LocationQuery): NetworkListState =>
  parseListQuery(query, NETWORK_STATUS_FILTERS, NETWORK_SORT_BYS, 'name')

export const buildNetworkListQuery = (
  currentQuery: LocationQuery,
  state: NetworkListState,
): LocationQueryRaw => buildListQuery(currentQuery, state, 'name')

const isSameListState = <Status extends string, SortBy extends string>(
  a: ListState<Status, SortBy>,
  b: ListState<Status, SortBy>,
): boolean =>
  a.search === b.search &&
  a.statusFilter === b.statusFilter &&
  a.sortBy === b.sortBy &&
  a.sortOrder === b.sortOrder

export interface UseRouteListStateOptions<Status extends string, SortBy extends string> {
  route: RouteLocationNormalizedLoaded
  router: Router
  parse: (query: LocationQuery) => ListState<Status, SortBy>
  build: (currentQuery: LocationQuery, state: ListState<Status, SortBy>) => LocationQueryRaw
}

export interface RouteListRefs<Status extends string, SortBy extends string> {
  searchKeyword: Ref<string>
  statusFilter: Ref<Status>
  sortBy: Ref<SortBy>
  sortOrder: Ref<ListSortOrder>
}

export function useRouteListState<Status extends string, SortBy extends string>({
  route,
  router,
  parse,
  build,
}: UseRouteListStateOptions<Status, SortBy>): RouteListRefs<Status, SortBy> {
  const initial = parse(route.query)
  const searchKeyword = ref(initial.search)
  const statusFilter = ref(initial.statusFilter) as Ref<Status>
  const sortBy = ref(initial.sortBy) as Ref<SortBy>
  const sortOrder = ref<ListSortOrder>(initial.sortOrder)

  let pendingReplaceCount = 0

  watch([searchKeyword, statusFilter, sortBy, sortOrder], ([search, status, sort, order]) => {
    const currentState: ListState<Status, SortBy> = {
      search,
      statusFilter: status,
      sortBy: sort,
      sortOrder: order,
    }
    if (isSameListState(currentState, parse(route.query))) {
      return
    }
    pendingReplaceCount += 1
    void router.replace({ query: build(route.query, currentState) }).finally(() => {
      pendingReplaceCount = Math.max(0, pendingReplaceCount - 1)
    })
  })

  watch(
    () => route.query,
    (newQuery) => {
      if (pendingReplaceCount > 0) {
        return
      }
      const next = parse(newQuery)
      if (searchKeyword.value !== next.search) {
        searchKeyword.value = next.search
      }
      if (statusFilter.value !== next.statusFilter) {
        statusFilter.value = next.statusFilter
      }
      if (sortBy.value !== next.sortBy) {
        sortBy.value = next.sortBy
      }
      if (sortOrder.value !== next.sortOrder) {
        sortOrder.value = next.sortOrder
      }
    },
  )

  return {
    searchKeyword,
    statusFilter,
    sortBy,
    sortOrder,
  }
}

export const canNavigateBackInApp = (): boolean => {
  const back = typeof window !== 'undefined' ? window.history.state?.back : null
  return typeof back === 'string' && back.length > 0 && !back.startsWith('/login')
}
