package upload

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSourceHashesAndParts(t *testing.T) {
	name := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(name, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	source, err := NewFile("parent", name, "remote.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if source.MD5() != "8D777F385D3DFEC8815D20F7496026DC" || source.SliceMD5() != source.MD5() {
		t.Fatalf("unexpected digests: %s %s", source.MD5(), source.SliceMD5())
	}
	if source.SliceCount() != 1 || source.Name() != "remote.txt" || !source.Overwrite() {
		t.Fatal("unexpected source metadata")
	}
	data, err := io.ReadAll(source.Part(0).Data())
	if err != nil || string(data) != "data" {
		t.Fatalf("part = %q, %v", data, err)
	}
}

func TestDigestRequiresSliceForLargeFile(t *testing.T) {
	_, err := NewDigest("parent", "large.bin", SliceSize+1, "D41D8CD98F00B204E9800998ECF8427E", "", false)
	if err == nil {
		t.Fatal("large digest source accepted without slice MD5")
	}
}
