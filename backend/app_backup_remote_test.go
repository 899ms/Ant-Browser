package backend

import (
	"ant-chrome/backend/internal/backup/channels"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

type backupRemoteMetadataTestClient struct {
	metadata map[string][]byte
}

func (client *backupRemoteMetadataTestClient) DownloadMetadata(_ context.Context, fileName, localPath string) error {
	data, ok := client.metadata[fileName]
	if !ok {
		return os.ErrNotExist
	}
	return os.WriteFile(localPath, data, 0o644)
}

func TestFormatBackupFileSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{size: 1023, want: `1023 B`},
		{size: 1024, want: `1.00 KB`},
		{size: 1024 * 1024, want: `1.00 MB`},
		{size: 29631128, want: `28.26 MB`},
	}

	for _, test := range tests {
		if got := formatBackupFileSize(test.size); got != test.want {
			t.Errorf(`formatBackupFileSize(%d) = %q, want %q`, test.size, got, test.want)
		}
	}
}

func TestLockBackupMaintenanceWaitsBriefly(t *testing.T) {
	app := NewApp(t.TempDir())
	app.maintenanceMu.Lock()
	released := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		app.maintenanceMu.Unlock()
		close(released)
	}()

	if err := app.lockBackupMaintenance(); err != nil {
		t.Fatalf(`lockBackupMaintenance returned error: %v`, err)
	}
	app.maintenanceMu.Unlock()
	<-released
}

func TestBackupRemoteHistoryEntriesReadsMetadata(t *testing.T) {
	metadata, err := json.Marshal(backupMetadata{
		Format:       `ant-chrome-backup-metadata`,
		Version:      backupMetadataVersion,
		BackupFile:   `ant-chrome-profile-backup-multi-2--20260912-093115.zip`,
		PackageType:  `profile`,
		ProfileCount: 2,
		ProfileNames: []string{`工作`, `个人`},
	})
	if err != nil {
		t.Fatalf(`marshal metadata: %v`, err)
	}

	app := NewApp(t.TempDir())
	items := app.backupRemoteHistoryEntries(&backupRemoteMetadataTestClient{
		metadata: map[string][]byte{
			`ant-chrome-profile-backup-multi-2--20260912-093115.json`: metadata,
		},
	}, []channels.File{{
		Name:       `ant-chrome-profile-backup-multi-2--20260912-093115.zip`,
		Size:       86 * 1024 * 1024,
		ModifiedAt: `2026-09-12T09:31:15Z`,
	}}, time.Second)

	if len(items) != 1 {
		t.Fatalf(`remote history item count = %d, want 1`, len(items))
	}
	item := items[0]
	if item[`packageType`] != `profile` || item[`profileCount`] != 2 {
		t.Fatalf(`remote history metadata = %+v, want profile metadata`, item)
	}
	profileNames, ok := item[`profileNames`].([]string)
	if !ok || len(profileNames) != 2 || profileNames[0] != `工作` || profileNames[1] != `个人` {
		t.Fatalf(`remote history profile names = %#v, want [工作 个人]`, item[`profileNames`])
	}
}

func TestBackupDownloadRemoteMetadataKeepsExistingFileOnDownloadFailure(t *testing.T) {
	metadataPath := t.TempDir() + string(os.PathSeparator) + `backup.json`
	original := []byte(`existing metadata`)
	if err := os.WriteFile(metadataPath, original, 0o644); err != nil {
		t.Fatalf(`write existing metadata: %v`, err)
	}

	app := NewApp(t.TempDir())
	err := app.backupDownloadRemoteMetadata(&backupRemoteMetadataTestClient{}, time.Second, `backup.json`, metadataPath, `backup.zip`)
	if err == nil {
		t.Fatal(`backupDownloadRemoteMetadata returned nil error for missing metadata`)
	}
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf(`read existing metadata: %v`, err)
	}
	if string(content) != string(original) {
		t.Fatalf(`existing metadata changed after failed download: %q`, content)
	}
}

func TestBackupDownloadRemoteMetadataKeepsExistingFileOnInvalidMetadata(t *testing.T) {
	metadataPath := t.TempDir() + string(os.PathSeparator) + `backup.json`
	original := []byte(`existing metadata`)
	if err := os.WriteFile(metadataPath, original, 0o644); err != nil {
		t.Fatalf(`write existing metadata: %v`, err)
	}

	app := NewApp(t.TempDir())
	err := app.backupDownloadRemoteMetadata(&backupRemoteMetadataTestClient{
		metadata: map[string][]byte{`backup.json`: []byte(`{`)},
	}, time.Second, `backup.json`, metadataPath, `backup.zip`)
	if err == nil {
		t.Fatal(`backupDownloadRemoteMetadata returned nil error for invalid metadata`)
	}
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf(`read existing metadata: %v`, err)
	}
	if string(content) != string(original) {
		t.Fatalf(`existing metadata changed after invalid download: %q`, content)
	}
}

func TestBackupDownloadRemoteMetadataPublishesValidatedMetadata(t *testing.T) {
	metadataPath := t.TempDir() + string(os.PathSeparator) + `backup.json`
	if err := os.WriteFile(metadataPath, []byte(`existing metadata`), 0o644); err != nil {
		t.Fatalf(`write existing metadata: %v`, err)
	}
	metadata, err := json.Marshal(backupMetadata{
		Format:  `ant-chrome-backup-metadata`,
		Version: backupMetadataVersion,
	})
	if err != nil {
		t.Fatalf(`marshal metadata: %v`, err)
	}

	app := NewApp(t.TempDir())
	if err := app.backupDownloadRemoteMetadata(&backupRemoteMetadataTestClient{
		metadata: map[string][]byte{`backup.json`: metadata},
	}, time.Second, `backup.json`, metadataPath, `backup.zip`); err != nil {
		t.Fatalf(`backupDownloadRemoteMetadata returned error: %v`, err)
	}
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf(`read published metadata: %v`, err)
	}
	var loaded backupMetadata
	if err := json.Unmarshal(content, &loaded); err != nil {
		t.Fatalf(`parse published metadata: %v`, err)
	}
	if loaded.BackupFile != `backup.zip` {
		t.Fatalf(`published backup file = %q, want backup.zip`, loaded.BackupFile)
	}
}
