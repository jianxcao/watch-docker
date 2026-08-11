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
    expect(parseComposeListQuery({ status: 'paused', sort: 'created', order: 'sideways' })).toEqual(
      { search: '', statusFilter: '', sortBy: 'name', sortOrder: 'asc' },
    )
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

describe('buildComposeListQuery', () => {
  it('omits all default values', () => {
    expect(
      buildComposeListQuery(
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
      buildComposeListQuery(
        {},
        {
          search: 'nginx',
          statusFilter: 'other',
          sortBy: 'status',
          sortOrder: 'desc',
        },
      ),
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
