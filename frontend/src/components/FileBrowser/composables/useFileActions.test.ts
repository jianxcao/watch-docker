import { describe, expect, it, vi } from 'vitest'

vi.mock('@/common/api', () => ({
  createContainerPath: vi.fn(),
  renameContainerPath: vi.fn(),
}))

import { validateEntryName } from './useFileActions'

describe('validateEntryName', () => {
  it('allows filenames with leading or trailing spaces', () => {
    expect(validateEntryName('report ', '文件')).toBeNull()
    expect(validateEntryName(' report', '文件')).toBeNull()
    expect(validateEntryName('my..report', '文件')).toBeNull()
  })

  it('rejects empty, whitespace-only, dot, slash, and null names', () => {
    expect(validateEntryName('', '文件')).not.toBeNull()
    expect(validateEntryName('   ', '文件')).not.toBeNull()
    expect(validateEntryName('.', '文件')).not.toBeNull()
    expect(validateEntryName('..', '文件')).not.toBeNull()
    expect(validateEntryName('a/b', '文件')).not.toBeNull()
    expect(validateEntryName('a\\b', '文件')).not.toBeNull()
    expect(validateEntryName('a\0b', '文件')).not.toBeNull()
  })
})
