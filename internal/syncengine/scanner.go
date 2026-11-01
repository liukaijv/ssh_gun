package syncengine

import (
	"io/fs"
	"path/filepath"
)

func ScanLocal(root string, matcher *ExcludeMatcher) ([]FileEntry, error) {
	return ScanLocalWithProgress(root, matcher, nil)
}

func ScanLocalWithProgress(root string, matcher *ExcludeMatcher, onEntry func(FileEntry)) ([]FileEntry, error) {
	entries := make([]FileEntry, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && isVCSMetaDir(entry.Name()) {
			return filepath.SkipDir
		}
		if isReparsePoint(entry, path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, err := ToSlashRel(root, path)
		if err != nil {
			return err
		}
		if relPath == "" {
			return nil
		}
		if entry.IsDir() && matcher != nil && matcher.ShouldPruneDir(relPath) {
			return filepath.SkipDir
		}
		if matcher != nil && matcher.ShouldSkip(relPath, entry.IsDir()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := FileEntry{
			RelPath: relPath,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		}
		entries = append(entries, item)
		if onEntry != nil {
			onEntry(item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
