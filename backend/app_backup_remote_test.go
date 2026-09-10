package backend

import (
	"testing"
	"time"
)

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
