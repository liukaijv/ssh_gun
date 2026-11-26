import { describe, expect, it } from 'vitest'
import {
  changeDirectoryPaths,
  changeStatusClass,
  directoryHasSyncableChange,
  menuSync,
  nestChanges,
  nextFileSelection,
  selectionWithout,
  visibleFiles,
  type BrowseRow,
} from './mappingBrowse'

const rows: BrowseRow[] = [
  { name: 'src', relPath: 'src', isDir: true, skipped: false },
  { name: 'a.txt', relPath: 'src/a.txt', isDir: false, skipped: false },
  { name: 'b.txt', relPath: 'src/b.txt', isDir: false, skipped: true },
  { name: 'c.txt', relPath: 'c.txt', isDir: false, skipped: false },
]

describe('mappingBrowse', () => {
  it('lists only visible files in tree order', () => {
    expect(visibleFiles(rows, new Set(['src'])).map((row) => row.relPath)).toEqual(['src/a.txt', 'src/b.txt', 'c.txt'])
    expect(visibleFiles(rows, new Set()).map((row) => row.relPath)).toEqual(['c.txt'])
  })

  it('hides nested files when an ancestor directory is collapsed', () => {
    const nested: BrowseRow[] = [
      { name: 'src', relPath: 'src', isDir: true, skipped: false },
      { name: 'sub', relPath: 'src/sub', isDir: true, skipped: false },
      { name: 'deep.txt', relPath: 'src/sub/deep.txt', isDir: false, skipped: false },
    ]
    expect(visibleFiles(nested, new Set(['src/sub'])).map((row) => row.relPath)).toEqual([])
    expect(visibleFiles(nested, new Set(['src', 'src/sub'])).map((row) => row.relPath)).toEqual(['src/sub/deep.txt'])
  })

  it('toggles with ctrl and ranges with shift', () => {
    const visible = visibleFiles(rows, new Set(['src']))
    const first = nextFileSelection([], null, rows[1], visible, false, false)
    expect(first.selected).toEqual(['src/a.txt'])
    const added = nextFileSelection(first.selected, first.anchor, rows[3], visible, true, false)
    expect(added.selected).toEqual(['src/a.txt', 'c.txt'])
    const range = nextFileSelection(['src/a.txt'], 'src/a.txt', rows[3], visible, false, true)
    expect(range.selected).toEqual(['src/a.txt', 'src/b.txt', 'c.txt'])
  })

  it('ignores directory clicks and handles ctrl untoggle, reverse shift range, and hidden anchor', () => {
    const visible = visibleFiles(rows, new Set(['src']))
    expect(nextFileSelection(['c.txt'], 'c.txt', rows[0], visible, false, false)).toEqual({
      selected: ['c.txt'],
      anchor: 'c.txt',
    })
    expect(nextFileSelection(['src/a.txt', 'c.txt'], 'c.txt', rows[1], visible, true, false)).toEqual({
      selected: ['c.txt'],
      anchor: 'src/a.txt',
    })
    expect(nextFileSelection(['c.txt'], 'c.txt', rows[1], visible, false, true)).toEqual({
      selected: ['src/a.txt', 'src/b.txt', 'c.txt'],
      anchor: 'c.txt',
    })
    const collapsedVisible = visibleFiles(rows, new Set())
    expect(nextFileSelection(['src/a.txt'], 'src/a.txt', rows[3], collapsedVisible, false, true)).toEqual({
      selected: ['c.txt'],
      anchor: 'c.txt',
    })
  })

  it('syncs the selection only from a selected file', () => {
    expect(menuSync(rows[1], ['src/a.txt', 'c.txt'], true)).toEqual({ kind: 'files', rels: ['src/a.txt', 'c.txt'], clear: false })
    expect(menuSync(rows[3], ['src/a.txt'], true)).toEqual({ kind: 'files', rels: ['c.txt'], clear: false })
    expect(menuSync(rows[0], ['src/a.txt'], true)).toEqual({ kind: 'directory', rel: 'src', clear: true })
    expect(menuSync(rows[0], [], false)).toEqual({ kind: 'disabled' })
  })

  it('disables menu sync on skipped directories and allows skipped files', () => {
    const skippedDir: BrowseRow = { name: 'dist', relPath: 'dist', isDir: true, skipped: true }
    expect(menuSync(skippedDir, ['c.txt'], true)).toEqual({ kind: 'disabled' })
    expect(menuSync(rows[2], [], false)).toEqual({ kind: 'files', rels: ['src/b.txt'], clear: false })
  })

  it('keeps folders first and name order while nesting changes', () => {
    const flat: BrowseRow[] = [
      { name: 'src', relPath: 'src', isDir: true, skipped: false },
      { name: 'directory', relPath: 'src/directory', isDir: true, skipped: false },
      { name: 'aa.go', relPath: 'src/aa.go', isDir: false, skipped: false },
      { name: 'b.go', relPath: 'src/b.go', isDir: false, skipped: false },
    ]
    const tree = nestChanges(flat)
    expect(tree.map((node) => node.relPath)).toEqual(['src'])
    expect(tree[0].children.map((node) => node.name)).toEqual(['directory', 'aa.go', 'b.go'])
    expect(tree[0].open).toBe(true)
    expect(tree[0].children.find((node) => node.relPath === 'src/directory')?.open).toBe(true)
    expect(tree[0].children.find((node) => node.relPath === 'src/aa.go')?.open).toBe(false)
    expect(changeDirectoryPaths(flat)).toEqual(['src', 'src/directory'])
  })

  it('drops selected files under a collapsed directory', () => {
    expect(selectionWithout(['src/a.txt', 'c.txt'], 'src/a.txt', 'src')).toEqual({
      selected: ['c.txt'],
      anchor: 'c.txt',
    })
    expect(selectionWithout(['c.txt'], 'c.txt', 'src')).toEqual({
      selected: ['c.txt'],
      anchor: 'c.txt',
    })
  })

  it('maps git change kinds to color classes and skips directories', () => {
    expect(changeStatusClass({ name: 'src', relPath: 'src', isDir: true, skipped: false, status: 'added' })).toBe('')
    expect(changeStatusClass({ name: 'a.txt', relPath: 'a.txt', isDir: false, skipped: false })).toBe('')
    expect(changeStatusClass({ name: 'a.txt', relPath: 'a.txt', isDir: false, skipped: false, status: 'added' })).toBe(
      'change-status-added',
    )
    expect(changeStatusClass({ name: 'b.txt', relPath: 'b.txt', isDir: false, skipped: false, status: 'modified' })).toBe(
      'change-status-modified',
    )
    expect(changeStatusClass({ name: 'c.txt', relPath: 'c.txt', isDir: false, skipped: false, status: 'renamed' })).toBe(
      'change-status-renamed',
    )
  })

  it('detects whether a directory has non-skipped changed files', () => {
    expect(directoryHasSyncableChange(rows, 'src')).toBe(true)
    expect(directoryHasSyncableChange([rows[0], rows[2]], 'src')).toBe(false)
    expect(directoryHasSyncableChange([{ name: 'x.txt', relPath: 'src2/x.txt', isDir: false, skipped: false }], 'src')).toBe(false)
    expect(directoryHasSyncableChange(rows, '')).toBe(false)
  })
})
