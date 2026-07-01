package records

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

func newArchiveMetaTestDB(t *testing.T) (*ArchiveMetaService, func()) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	return NewArchiveMetaService(database.Conn()), func() { database.Close() }
}

func TestArchiveMetaService_GetDefaultSeeds(t *testing.T) {
	svc, cleanup := newArchiveMetaTestDB(t)
	defer cleanup()
	ctx := context.Background()

	shared, err := svc.Get(ctx, ArchiveKindShared)
	if err != nil {
		t.Fatalf("Get(shared): %v", err)
	}
	if shared.IncludeTags {
		t.Errorf("shared_archive include_tags = true, want false (default off)")
	}

	backup, err := svc.Get(ctx, ArchiveKindBackup)
	if err != nil {
		t.Fatalf("Get(backup): %v", err)
	}
	if !backup.IncludeTags {
		t.Errorf("backup_archive include_tags = false, want true (full snapshot)")
	}

	staticKind, err := svc.Get(ctx, ArchiveKindStatic)
	if err != nil {
		t.Fatalf("Get(static): %v", err)
	}
	if staticKind.IncludeTags {
		t.Errorf("static_archive include_tags = true, want false")
	}
}

func TestArchiveMetaService_SetIncludeTagsRoundTrip(t *testing.T) {
	svc, cleanup := newArchiveMetaTestDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := svc.SetIncludeTags(ctx, ArchiveKindShared, true); err != nil {
		t.Fatalf("SetIncludeTags: %v", err)
	}
	got, err := svc.Get(ctx, ArchiveKindShared)
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if !got.IncludeTags {
		t.Errorf("IncludeTags = false after set true")
	}
}

func TestArchiveMetaService_IncludeTagsUnknownKind(t *testing.T) {
	svc, cleanup := newArchiveMetaTestDB(t)
	defer cleanup()
	if v := svc.IncludeTags(context.Background(), "not_a_kind"); v {
		t.Errorf("IncludeTags on unknown kind = true, want false")
	}
}

func TestArchiveMetaService_GetMissing(t *testing.T) {
	svc, cleanup := newArchiveMetaTestDB(t)
	defer cleanup()
	_, err := svc.Get(context.Background(), "ghost")
	if !errors.Is(err, ErrArchiveMetaNotFound) {
		t.Errorf("Get on unknown kind err = %v, want ErrArchiveMetaNotFound", err)
	}
}
