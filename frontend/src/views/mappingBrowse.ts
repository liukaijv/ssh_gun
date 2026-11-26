export type BrowseRow = {
  name: string
  relPath: string
  isDir: boolean
  skipped: boolean
  status?: string
}

export function changeStatusClass(row: BrowseRow): string {
  if (row.isDir || !row.status) return ''
  if (row.status === 'added') return 'change-status-added'
  if (row.status === 'modified') return 'change-status-modified'
  if (row.status === 'renamed') return 'change-status-renamed'
  return ''
}

export function visibleFiles(rows: BrowseRow[], expanded: ReadonlySet<string>): BrowseRow[] {
  return rows.filter((row) => {
    if (row.isDir) return false
    const parent = row.relPath.includes('/') ? row.relPath.slice(0, row.relPath.lastIndexOf('/')) : ''
    if (!parent) return true
    const parts = parent.split('/')
    let acc = ''
    for (const part of parts) {
      acc = acc ? `${acc}/${part}` : part
      if (!expanded.has(acc)) return false
    }
    return true
  })
}

export function nextFileSelection(
  selected: string[],
  anchor: string | null,
  row: BrowseRow,
  visible: BrowseRow[],
  ctrl: boolean,
  shift: boolean,
): { selected: string[]; anchor: string | null } {
  if (row.isDir) return { selected, anchor }
  if (shift && anchor) {
    const paths = visible.map((item) => item.relPath)
    const start = paths.indexOf(anchor)
    const end = paths.indexOf(row.relPath)
    if (start >= 0 && end >= 0) {
      const [from, to] = start < end ? [start, end] : [end, start]
      return { selected: paths.slice(from, to + 1), anchor }
    }
  }
  if (ctrl) {
    const has = selected.includes(row.relPath)
    return {
      selected: has ? selected.filter((path) => path !== row.relPath) : [...selected, row.relPath],
      anchor: row.relPath,
    }
  }
  return { selected: [row.relPath], anchor: row.relPath }
}

export function menuSync(
  row: BrowseRow,
  selected: string[],
  directoryEnabled: boolean,
): { kind: 'files'; rels: string[]; clear: boolean } | { kind: 'directory'; rel: string; clear: true } | { kind: 'disabled' } {
  if (row.isDir) {
    if (!directoryEnabled || row.skipped) return { kind: 'disabled' }
    return { kind: 'directory', rel: row.relPath, clear: true }
  }
  if (selected.includes(row.relPath)) return { kind: 'files', rels: [...selected], clear: false }
  return { kind: 'files', rels: [row.relPath], clear: false }
}

export type ChangeNode = BrowseRow & { children: ChangeNode[]; open: boolean }

// nestChanges builds the changes tree. Rows are ordered by depth so a parent
// exists before its children. Order within one depth is kept, which preserves
// folders-first and case-insensitive name order from the backend.
export function nestChanges(rows: BrowseRow[]): ChangeNode[] {
  const nodes = new Map<string, ChangeNode>()
  const roots: ChangeNode[] = []
  const ordered = [...rows].sort((a, b) => a.relPath.split('/').length - b.relPath.split('/').length)
  for (const row of ordered) {
    const node: ChangeNode = { ...row, children: [], open: row.isDir }
    nodes.set(row.relPath, node)
    const slash = row.relPath.lastIndexOf('/')
    const parent = slash >= 0 ? nodes.get(row.relPath.slice(0, slash)) : undefined
    if (parent) parent.children.push(node)
    else roots.push(node)
  }
  return roots
}

export function changeDirectoryPaths(rows: BrowseRow[]): string[] {
  return rows.filter((row) => row.isDir).map((row) => row.relPath)
}

export function selectionWithout(
  selected: string[],
  anchor: string | null,
  dir: string,
): { selected: string[]; anchor: string | null } {
  const prefix = `${dir}/`
  const next = selected.filter((path) => path !== dir && !path.startsWith(prefix))
  const anchorHidden = anchor != null && (anchor === dir || anchor.startsWith(prefix))
  return { selected: next, anchor: anchorHidden ? (next[0] ?? null) : anchor }
}

export function directoryHasSyncableChange(rows: BrowseRow[], rel: string): boolean {
  const prefix = `${rel}/`
  return rows.some((row) => !row.isDir && !row.skipped && (row.relPath === rel || row.relPath.startsWith(prefix)))
}
