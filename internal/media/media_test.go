package media

import (
	"context"
	"os"
	"os/exec"
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

func TestUnmountFallsBackToMountpoint(t *testing.T) {
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

	if err := Unmount(context.Background(), Entry{Mountpoint: "/media/USB"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(logPath)
	if strings.TrimSpace(string(b)) != "/media/USB" {
		t.Fatalf("args = %q", b)
	}
}

func TestRejectsTraversal(t *testing.T) {
	if err := Unmount(context.Background(), Entry{Device: "/dev/../etc/shadow"}); err == nil {
		t.Fatal("traversal device accepted")
	}
}

func TestMediaUmountValidation(t *testing.T) {
	helper := filepath.Join("..", "..", "system", "media-umount")
	cases := []struct {
		arg  string
		ok   bool
		want string
	}{
		{"/dev/sda1", true, "/dev/sda1\n"},
		{"/dev/mmcblk0p1", true, "/dev/mmcblk0p1\n"},
		{"/dev/disk/by-id/foo", false, ""},
		{"/dev/../etc/passwd", false, ""},
		{"../../etc/shadow", false, ""},
	}
	for _, c := range cases {
		cmd := exec.Command("sh", helper, c.arg)
		cmd.Env = append(os.Environ(), "CPZ_MEDIA_UMOUNT_TEST=1")
		out, err := cmd.Output()
		if c.ok {
			if err != nil {
				t.Fatalf("%s: %v", c.arg, err)
			}
			if string(out) != c.want {
				t.Fatalf("%s: out %q, want %q", c.arg, out, c.want)
			}
			continue
		}
		if err == nil {
			t.Fatalf("%s: accepted, want rejection", c.arg)
		}
	}
}
