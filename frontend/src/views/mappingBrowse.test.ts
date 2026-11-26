import { describe, expect, it } from 'vitest'
import {
  absoluteLocalPath,
  changeDirectoryPaths,
  changeStatusClass,
  directoryHasSyncableChange,
  menuSync,
  nestChanges,
  nextFileSelection,
  selectionWithout,
  shouldListDirChildren,
  sortBrowseRows,
  visibleFiles,
  type BrowseRow,
} from './mappingBrowse'

function row(partial: Partial<BrowseRow> & Pick<BrowseRow, 'name' | 'relPath' | 'isDir'>): BrowseRow {
  return {
    skipped: false,
    size: 0,
    modTime: '',
    ...partial,
  }
}

const rows: BrowseRow[] = [
  row({ name: 'src', relPath: 'src', isDir: true }),
  row({ name: 'a.txt', relPath: 'src/a.txt', isDir: false }),
  row({ name: 'b.txt', relPath: 'src/b.txt', isDir: false, skipped: true }),
  row({ name: 'c.txt', relPath: 'c.txt', isDir: false }),
]

describe('mappingBrowse', () => {
  it('skips ListMappingDir when already loaded or loading', () => {
    expect(shouldListDirChildren(false, false)).toBe(true)
    expect(shouldListDirChildren(true, false)).toBe(false)
    expect(shouldListDirChildren(false, true)).toBe(false)
    expect(shouldListDirChildren(true, true)).toBe(false)
  })

  it('lists only visible files in tree order', () => {
    expect(visibleFiles(rows, new Set(['src'])).map((item) => item.relPath)).toEqual(['src/a.txt', 'src/b.txt', 'c.txt'])
    expect(visibleFiles(rows, new Set()).map((item) => item.relPath)).toEqual(['c.txt'])
  })

  it('hides nested files when an ancestor directory is collapsed', () => {
    const nested: BrowseRow[] = [
      row({ name: 'src', relPath: 'src', isDir: true }),
      row({ name: 'sub', relPath: 'src/sub', isDir: true }),
      row({ name: 'deep.txt', relPath: 'src/sub/deep.txt', isDir: false }),
    ]
    expect(visibleFiles(nested, new Set(['src/sub'])).map((item) => item.relPath)).toEqual([])
    expect(visibleFiles(nested, new Set(['src', 'src/sub'])).map((item) => item.relPath)).toEqual(['src/sub/deep.txt'])
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
    const skippedDir = row({ name: 'dist', relPath: 'dist', isDir: true, skipped: true })
    expect(menuSync(skippedDir, ['c.txt'], true)).toEqual({ kind: 'disabled' })
    expect(menuSync(rows[2], [], false)).toEqual({ kind: 'files', rels: ['src/b.txt'], clear: false })
  })

  it('keeps folders first and name order while nesting changes', () => {
    const flat: BrowseRow[] = [
      row({ name: 'src', relPath: 'src', isDir: true }),
      row({ name: 'directory', relPath: 'src/directory', isDir: true }),
      row({ name: 'aa.go', relPath: 'src/aa.go', isDir: false }),
      row({ name: 'b.go', relPath: 'src/b.go', isDir: false }),
    ]
    const tree = nestChanges(flat)
    expect(tree.map((node) => node.relPath)).toEqual(['src'])
    expect(tree[0].children.map((node) => node.name)).toEqual(['directory', 'aa.go', 'b.go'])
    expect(tree[0].open).toBe(true)
    expect(tree[0].children.find((node) => node.relPath === 'src/directory')?.open).toBe(true)
    expect(tree[0].children.find((node) => node.relPath === 'src/aa.go')?.open).toBe(false)
    expect(changeDirectoryPaths(flat)).toEqual(['src', 'src/directory'])
  })

  it('sorts by name, mtime, and size with directories first', () => {
    const items: BrowseRow[] = [
      row({ name: 'b.txt', relPath: 'b.txt', isDir: false, size: 100, modTime: '2024-01-01T00:00:00Z' }),
      row({ name: 'a.txt', relPath: 'a.txt', isDir: false, size: 30, modTime: '2024-01-03T00:00:00Z' }),
      row({ name: 'zdir', relPath: 'zdir', isDir: true, size: 50, modTime: '2024-01-02T00:00:00Z' }),
      row({ name: 'adir', relPath: 'adir', isDir: true, size: 5, modTime: '2024-01-04T00:00:00Z' }),
    ]
    expect(sortBrowseRows(items, 'name').map((item) => item.name)).toEqual(['adir', 'zdir', 'a.txt', 'b.txt'])
    expect(sortBrowseRows(items, 'mtime').map((item) => item.name)).toEqual(['adir', 'zdir', 'a.txt', 'b.txt'])
    expect(sortBrowseRows(items, 'size').map((item) => item.name)).toEqual(['zdir', 'adir', 'b.txt', 'a.txt'])
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

  it('joins absolute paths using the local root separator', () => {
    expect(absoluteLocalPath('D:\\Work\\project', 'Library/Api/foo.php')).toBe(
      'D:\\Work\\project\\Library\\Api\\foo.php',
    )
    expect(absoluteLocalPath('D:\\Work\\project\\', 'src/a.txt')).toBe('D:\\Work\\project\\src\\a.txt')
    expect(absoluteLocalPath('/home/u/project', 'src/a.txt')).toBe('/home/u/project/src/a.txt')
    expect(absoluteLocalPath('/home/u/project/', '')).toBe('/home/u/project')
  })

  it('maps git change kinds to color classes and skips directories', () => {
    expect(changeStatusClass(row({ name: 'src', relPath: 'src', isDir: true, status: 'added' }))).toBe('')
    expect(changeStatusClass(row({ name: 'a.txt', relPath: 'a.txt', isDir: false }))).toBe('')
    expect(changeStatusClass(row({ name: 'a.txt', relPath: 'a.txt', isDir: false, status: 'added' }))).toBe(
      'change-status-added',
    )
    expect(changeStatusClass(row({ name: 'b.txt', relPath: 'b.txt', isDir: false, status: 'modified' }))).toBe(
      'change-status-modified',
    )
    expect(changeStatusClass(row({ name: 'c.txt', relPath: 'c.txt', isDir: false, status: 'renamed' }))).toBe(
      'change-status-renamed',
    )
  })

  it('detects whether a directory has non-skipped changed files', () => {
    expect(directoryHasSyncableChange(rows, 'src')).toBe(true)
    expect(directoryHasSyncableChange([rows[0], rows[2]], 'src')).toBe(false)
    expect(directoryHasSyncableChange([row({ name: 'x.txt', relPath: 'src2/x.txt', isDir: false })], 'src')).toBe(false)
    expect(directoryHasSyncableChange(rows, '')).toBe(false)
  })
})
