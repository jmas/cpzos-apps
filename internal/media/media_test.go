package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	state := t.TempDir()
	mnt := filepath.Join(t.TempDir(), "USB Disk")
	if err := os.Mkdir(mnt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "sda1.mountpoint"), []byte(mnt+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_AUTOMOUNT_STATE", state)

	list := List()
	if len(list) != 1 {
		t.Fatalf("list = %#v", list)
	}
	if list[0].Device != "/dev/sda1" || list[0].Mountpoint != mnt || list[0].Label != "USB Disk" {
		t.Fatalf("entry = %#v", list[0])
	}
}

func TestUnmountCallsHelper(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	helper := filepath.Join(dir, "media-umount")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CPZ_ARGS_LOG\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_MEDIA_UMOUNT", helper)
	t.Setenv("CPZ_MEDIA_DIRECT", "1")
	t.Setenv("CPZ_ARGS_LOG", logPath)

	if err := Unmount(context.Background(), Entry{Device: "/dev/sda1"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "/dev/sda1" {
		t.Fatalf("args = %q", b)
	}
}

func TestRejectsTraversal(t *testing.T) {
	if err := Unmount(context.Background(), Entry{Device: "/dev/../etc/shadow"}); err == nil {
		t.Fatal("traversal device accepted")
	}
}
