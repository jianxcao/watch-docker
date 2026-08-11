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
    statusFilter: status === 'running' || status === 'exited' || status === 'other' ? status : '',
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
