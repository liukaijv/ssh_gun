package syncengine

import (
	"io/fs"
	"path/filepath"
)

func ScanLocal(root string, matcher *ExcludeMatcher) ([]FileEntry, error) {
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
		if entry.IsDir() && matcher != nil && matcher.ShouldSkip(relPath, true) {
			return filepath.SkipDir
		}
		if matcher != nil && matcher.ShouldSkip(relPath, entry.IsDir()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		entries = append(entries, FileEntry{
			RelPath: relPath,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
