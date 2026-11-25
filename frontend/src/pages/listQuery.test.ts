import { nextTick, reactive } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { LocationQuery, Router } from 'vue-router'
import {
  buildContainerListQuery,
  buildNetworkListQuery,
  parseContainerListQuery,
  parseNetworkListQuery,
  useRouteListState,
} from './listQuery'

describe('parseContainerListQuery', () => {
  it('parses every supported Container list parameter', () => {
    expect(
      parseContainerListQuery({
        search: 'nginx',
        status: 'updateable',
        sort: 'created',
        order: 'desc',
      }),
    ).toEqual({
      search: 'nginx',
      statusFilter: 'updateable',
      sortBy: 'created',
      sortOrder: 'desc',
    })
  })

  it('uses defaults for missing and invalid enum parameters', () => {
    expect(
      parseContainerListQuery({ status: 'paused', sort: 'size', order: 'sideways' }),
    ).toEqual({
      search: '',
      statusFilter: '',
      sortBy: 'name',
      sortOrder: 'asc',
    })
  })

  it('uses the first string from array query values', () => {
    expect(
      parseContainerListQuery({
        search: [null, 'redis'],
        status: ['running', 'stopped'],
        sort: [null, 'status'],
        order: ['desc', 'asc'],
      }),
    ).toEqual({
      search: 'redis',
      statusFilter: 'running',
      sortBy: 'status',
      sortOrder: 'desc',
    })
  })
})

describe('buildContainerListQuery', () => {
  it('omits all default values', () => {
    expect(
      buildContainerListQuery(
        {},
        {
          search: '',
          statusFilter: '',
          sortBy: 'name',
          sortOrder: 'asc',
        },
      ),
    ).toEqual({})
  })

  it('serializes every non-default value', () => {
    expect(
      buildContainerListQuery(
        {},
        {
          search: 'nginx',
          statusFilter: 'error',
          sortBy: 'status',
          sortOrder: 'desc',
        },
      ),
    ).toEqual({ search: 'nginx', status: 'error', sort: 'status', order: 'desc' })
  })

  it('preserves unrelated values and removes owned values restored to defaults', () => {
    expect(
      buildContainerListQuery(
        { search: 'old', status: 'running', sort: 'created', order: 'desc', tab: 'overview' },
        { search: '', statusFilter: '', sortBy: 'name', sortOrder: 'asc' },
      ),
    ).toEqual({ tab: 'overview' })
  })
})

describe('parseNetworkListQuery', () => {
  it('parses every supported Network list parameter', () => {
    expect(
      parseNetworkListQuery({
        search: 'bridge',
        status: 'driver-bridge',
        sort: 'created',
        order: 'desc',
      }),
    ).toEqual({
      search: 'bridge',
      statusFilter: 'driver-bridge',
      sortBy: 'created',
      sortOrder: 'desc',
    })
  })

  it('uses defaults for missing and invalid enum parameters', () => {
    expect(
      parseNetworkListQuery({ status: 'running', sort: 'status', order: 'invalid' }),
    ).toEqual({
      search: '',
      statusFilter: '',
      sortBy: 'name',
      sortOrder: 'asc',
    })
  })

  it('uses the first string from array query values', () => {
    expect(
      parseNetworkListQuery({
        search: [null, '172.18'],
        status: ['custom', 'builtin'],
        sort: [null, 'created'],
        order: ['desc', 'asc'],
      }),
    ).toEqual({
      search: '172.18',
      statusFilter: 'custom',
      sortBy: 'created',
      sortOrder: 'desc',
    })
  })
})

describe('buildNetworkListQuery', () => {
  it('omits all default values', () => {
    expect(
      buildNetworkListQuery(
        {},
        {
          search: '',
          statusFilter: '',
          sortBy: 'name',
          sortOrder: 'asc',
        },
      ),
    ).toEqual({})
  })

  it('serializes every non-default value', () => {
    expect(
      buildNetworkListQuery(
        {},
        {
          search: '172.17',
          statusFilter: 'unused',
          sortBy: 'created',
          sortOrder: 'desc',
        },
      ),
    ).toEqual({ search: '172.17', status: 'unused', sort: 'created', order: 'desc' })
  })

  it('preserves unrelated values and removes owned values restored to defaults', () => {
    expect(
      buildNetworkListQuery(
        { search: 'old', status: 'used', sort: 'created', order: 'desc', foo: 'bar' },
        { search: '', statusFilter: '', sortBy: 'name', sortOrder: 'asc' },
      ),
    ).toEqual({ foo: 'bar' })
  })
})

describe('useRouteListState', () => {
  it('syncs rapid change-and-revert across ticks even while an earlier router.replace is pending', async () => {
    const route = reactive<{ query: LocationQuery }>({ query: {} })
    const resolvers: Array<() => void> = []
    const replaceMock = vi.fn(({ query }: { query: LocationQuery }) => {
      return new Promise<void>((resolve) => {
        resolvers.push(() => {
          route.query = { ...query }
          resolve()
        })
      })
    })
    const router = { replace: replaceMock } as unknown as Router

    const { searchKeyword } = useRouteListState({
      route: route as any,
      router,
      parse: parseContainerListQuery,
      build: buildContainerListQuery,
    })

    searchKeyword.value = 'nginx'
    await nextTick()
    expect(replaceMock).toHaveBeenCalledTimes(1)
    expect(replaceMock).toHaveBeenLastCalledWith({ query: { search: 'nginx' } })

    // Revert before first async router.replace commits route.query
    searchKeyword.value = ''
    await nextTick()
    expect(replaceMock).toHaveBeenCalledTimes(2)
    expect(replaceMock).toHaveBeenLastCalledWith({ query: {} })

    while (resolvers.length > 0) {
      resolvers.shift()?.()
      await nextTick()
    }
    expect(route.query).toEqual({})
    expect(searchKeyword.value).toBe('')
  })
})
