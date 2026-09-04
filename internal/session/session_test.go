package session

import "testing"

func TestJoinScopedWorkingDirectory(t *testing.T) {
	old := Pwd()
	defer SetWorkDir(old)
	SetWorkDir("123:/dir")

	if got := Join("file.txt"); got != "123:/dir/file.txt" {
		t.Fatalf("relative path = %q", got)
	}
	if got := Join("/file.txt"); got != "/file.txt" {
		t.Fatalf("personal absolute path = %q", got)
	}
	if got := Join("456:/file.txt"); got != "456:/file.txt" {
		t.Fatalf("explicit family path = %q", got)
	}
	if got := Join("456:"); got != "456:" {
		t.Fatalf("family root shorthand = %q", got)
	}
	SetWorkDir("123:")
	if got := Join("file.txt"); got != "123:/file.txt" {
		t.Fatalf("relative path from shorthand root = %q", got)
	}
}
