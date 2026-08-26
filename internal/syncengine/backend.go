package syncengine

import (
	"context"
	"time"

	"ssh_gun/internal/config"
)

type ChangeOp string

const (
	Upload  ChangeOp = "upload"
	Mkdir   ChangeOp = "mkdir"
	Delete  ChangeOp = "delete"
	Chtimes ChangeOp = "chtimes"
)

type Change struct {
	Op      ChangeOp
	RelPath string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

type Event struct {
	Op          ChangeOp
	RelPath     string
	Transferred int64
	Total       int64
	Err         error
}

type Stats struct {
	Files    int
	Dirs     int
	Deleted  int
	Bytes    int64
	Started  time.Time
	Finished time.Time
}

type Backend interface {
	Name() string
	FullSync(ctx context.Context, mapping *config.SyncMapping, emit func(Event)) (*Stats, error)
	PushChanges(ctx context.Context, mapping *config.SyncMapping, changes []Change, emit func(Event)) (*Stats, error)
}
