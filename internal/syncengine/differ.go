package syncengine

import "time"

type FileEntry struct {
	RelPath string
	Size    int64
	ModTime time.Time
	IsDir   bool
	Hash    string
}

type Changes struct {
	Upload []FileEntry
	Mkdir  []FileEntry
	Delete []FileEntry
}

func Diff(local, remote []FileEntry, mode string, mtimeSkew time.Duration) Changes {
	remoteByPath := make(map[string]FileEntry, len(remote))
	for _, entry := range remote {
		remoteByPath[entry.RelPath] = entry
	}

	var changes Changes
	for _, entry := range local {
		other, exists := remoteByPath[entry.RelPath]
		if !exists {
			if entry.IsDir {
				changes.Mkdir = append(changes.Mkdir, entry)
			} else {
				changes.Upload = append(changes.Upload, entry)
			}
			continue
		}
		delete(remoteByPath, entry.RelPath)
		if entry.IsDir != other.IsDir {
			changes.Delete = append(changes.Delete, other)
			if entry.IsDir {
				changes.Mkdir = append(changes.Mkdir, entry)
			} else {
				changes.Upload = append(changes.Upload, entry)
			}
			continue
		}
		if entry.IsDir {
			continue
		}
		if mode == "checksum" {
			if entry.Hash != other.Hash {
				changes.Upload = append(changes.Upload, entry)
			}
			continue
		}
		delta := entry.ModTime.Sub(other.ModTime)
		if delta < 0 {
			delta = -delta
		}
		if entry.Size != other.Size || delta > mtimeSkew {
			changes.Upload = append(changes.Upload, entry)
		}
	}

	for _, entry := range remote {
		if _, exists := remoteByPath[entry.RelPath]; exists {
			changes.Delete = append(changes.Delete, entry)
		}
	}
	return changes
}
