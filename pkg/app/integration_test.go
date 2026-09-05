//go:build integration

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gowsp/cloud189/pkg/drive"
)

func TestCloudLifecycle(t *testing.T) {
	if os.Getenv("CLOUD189_INTEGRATION") != "1" {
		t.Skip("set CLOUD189_INTEGRATION=1 to run live cloud tests")
	}
	configPath := os.Getenv("CLOUD189_CONFIG")
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		configPath = filepath.Join(home, ".config", "cloud189", "config.json")
	}
	client, err := Open(configPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	personal := client.Personal()
	t.Run("personal-read-only", func(t *testing.T) {
		testDriveReadOnly(t, ctx, personal)
	})
	t.Run("personal-lifecycle", func(t *testing.T) {
		testDriveLifecycle(t, personal)
	})
	if familyID := os.Getenv("CLOUD189_INTEGRATION_FAMILY"); familyID != "" {
		family, err := client.Family(ctx, familyID)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("family-read-only", func(t *testing.T) {
			testDriveReadOnly(t, ctx, family)
		})
		t.Run("family-lifecycle", func(t *testing.T) {
			testDriveLifecycle(t, family)
		})
		t.Run("family-delete-status", func(t *testing.T) {
			testFamilyDeleteStatus(t, family)
		})
		t.Run("cross-space-copy", func(t *testing.T) {
			testCrossSpaceCopy(t, personal, family)
		})
		t.Run("conflict-keep-both", func(t *testing.T) {
			testConflictResolution(t, personal)
			testConflictResolution(t, family)
		})
	}
	if os.Getenv("CLOUD189_INTEGRATION_SIGN") == "1" {
		t.Run("sign", func(t *testing.T) {
			if err := client.Sign(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func testFamilyDeleteStatus(t *testing.T, family *drive.FS) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := fmt.Sprintf("cloud189-test-delete-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if err := removeAndVerify(family, root); err != nil {
			t.Errorf("cleanup %q: %v", root, err)
		}
	})
	if err := family.Mkdir(ctx, root, 0755); err != nil {
		t.Fatalf("mkdir delete root: %v", err)
	}
	files := make([]string, 3)
	for index := range files {
		files[index] = fmt.Sprintf("%s/file-%02d.txt", root, index)
		content := fmt.Sprintf("delete status test %d", index)
		if _, err := family.Put(ctx, files[index], strings.NewReader(content), int64(len(content)), drive.PutOptions{}); err != nil {
			t.Fatalf("put %s: %v", files[index], err)
		}
	}
	entries, err := family.ReadDirContext(ctx, root)
	if err != nil {
		t.Fatalf("list before delete: %v", err)
	}
	if len(entries) != len(files) {
		t.Fatalf("entries before delete=%d, want %d", len(entries), len(files))
	}
	statuses := make([]batchTaskResponse, 0, 2)
	deleteCtx := withTaskObserver(ctx, func(response batchTaskResponse) {
		statuses = append(statuses, response)
	})
	if err := family.Remove(deleteCtx, files...); err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("batch delete returned no task status")
	}
	final := statuses[len(statuses)-1]
	if final.TaskStatus != taskDone || final.SubTaskCount != len(files) || final.SucceededCount != len(files) || final.FailedCount != 0 || final.SkipCount != 0 {
		t.Fatalf("final delete status: status=%d subtasks=%d success=%d failed=%d skipped=%d",
			final.TaskStatus, final.SubTaskCount, final.SucceededCount, final.FailedCount, final.SkipCount)
	}
	t.Logf("delete task %s statuses=%v subtasks=%d success=%d failed=%d skipped=%d",
		final.TaskID, taskStatusCodes(statuses), final.SubTaskCount, final.SucceededCount, final.FailedCount, final.SkipCount)
	for _, name := range files {
		if _, err := family.StatContext(ctx, name); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("stat deleted %s: %v", name, err)
		}
	}
	if err := removeAndVerify(family, root); err != nil {
		t.Fatalf("remove empty delete root: %v", err)
	}
}

func taskStatusCodes(statuses []batchTaskResponse) []int {
	codes := make([]int, len(statuses))
	for index, status := range statuses {
		codes[index] = status.TaskStatus
	}
	return codes
}

func testDriveReadOnly(t *testing.T, ctx context.Context, cloud *drive.FS) {
	t.Helper()
	if _, err := cloud.Space(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.StatContext(ctx, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.ReadDirContext(ctx, "."); err != nil {
		t.Fatal(err)
	}
}

func testDriveLifecycle(t *testing.T, cloud *drive.FS) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := fmt.Sprintf("cloud189-test-%d", time.Now().UnixNano())
	cleanup := func() error { return removeAndVerify(cloud, root) }
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %q: %v", root, err)
		}
	})

	if err := cloud.Mkdir(ctx, root, 0755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	content := "cloud189 integration test"
	fileName := root + "/source.txt"
	if _, err := cloud.Put(ctx, fileName, strings.NewReader(content), int64(len(content)), drive.PutOptions{}); err != nil {
		t.Fatalf("put source: %v", err)
	}
	copyDir := root + "/copy"
	if err := cloud.Mkdir(ctx, copyDir, 0755); err != nil {
		t.Fatalf("mkdir copy dir: %v", err)
	}
	if err := cloud.Copy(ctx, copyDir, fileName); err != nil {
		t.Fatalf("copy source: %v", err)
	}
	if _, err := cloud.StatContext(ctx, copyDir+"/source.txt"); err != nil {
		t.Fatalf("stat copy: %v", err)
	}
	renamed := root + "/renamed.txt"
	if err := cloud.Rename(ctx, fileName, renamed); err != nil {
		t.Fatalf("rename source: %v", err)
	}
	var downloaded bytes.Buffer
	if _, err := cloud.Download(ctx, renamed, &downloaded); err != nil {
		t.Fatalf("download renamed: %v", err)
	}
	if downloaded.String() != content {
		t.Fatalf("downloaded %q, want %q", downloaded.String(), content)
	}
	if _, err := cloud.Usage(ctx, root); err != nil {
		t.Fatalf("usage root: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("final cleanup: %v", err)
	}
}

func testCrossSpaceCopy(t *testing.T, personal, family *drive.FS) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	suffix := time.Now().UnixNano()
	personalRoot := fmt.Sprintf("cloud189-test-personal-%d", suffix)
	familyRoot := fmt.Sprintf("cloud189-test-family-%d", suffix)
	t.Cleanup(func() {
		if err := removeAndVerify(personal, personalRoot); err != nil {
			t.Errorf("cleanup personal %q: %v", personalRoot, err)
		}
		if err := removeAndVerify(family, familyRoot); err != nil {
			t.Errorf("cleanup family %q: %v", familyRoot, err)
		}
	})
	if err := personal.Mkdir(ctx, personalRoot, 0755); err != nil {
		t.Fatalf("mkdir personal root: %v", err)
	}
	if err := family.Mkdir(ctx, familyRoot, 0755); err != nil {
		t.Fatalf("mkdir family root: %v", err)
	}
	personalFile := personalRoot + "/personal.txt"
	familyFile := familyRoot + "/family.txt"
	if _, err := personal.Put(ctx, personalFile, strings.NewReader("personal"), 8, drive.PutOptions{}); err != nil {
		t.Fatalf("put personal source: %v", err)
	}
	if _, err := family.Put(ctx, familyFile, strings.NewReader("family"), 6, drive.PutOptions{}); err != nil {
		t.Fatalf("put family source: %v", err)
	}
	if err := family.CopyFrom(ctx, personal, familyRoot, personalFile); err != nil {
		t.Fatalf("copy personal to family: %v", err)
	}
	if err := personal.CopyFrom(ctx, family, personalRoot, familyFile); err != nil {
		t.Fatalf("copy family to personal: %v", err)
	}
	if _, err := family.StatContext(ctx, familyRoot+"/personal.txt"); err != nil {
		t.Fatalf("stat family copy: %v", err)
	}
	if _, err := personal.StatContext(ctx, personalRoot+"/family.txt"); err != nil {
		t.Fatalf("stat personal copy: %v", err)
	}
	if err := removeAndVerify(personal, personalRoot); err != nil {
		t.Fatal(err)
	}
	if err := removeAndVerify(family, familyRoot); err != nil {
		t.Fatal(err)
	}
}

// testConflictResolution exercises all three dealWay strategies against the
// real conflict flow: duplicate COPY reports taskStatus=2, then
// manageBatchTask resolves it per policy and the task completes.
func testConflictResolution(t *testing.T, cloud *drive.FS) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := fmt.Sprintf("cloud189-test-conflict-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if err := removeAndVerify(cloud, root); err != nil {
			t.Errorf("cleanup %q: %v", root, err)
		}
	})
	if err := cloud.Mkdir(ctx, root, 0755); err != nil {
		t.Fatalf("mkdir conflict root: %v", err)
	}
	skipSource := root + "/skip-source.txt"
	if _, err := cloud.Put(ctx, skipSource, strings.NewReader("skip conflict"), int64(len("skip conflict")), drive.PutOptions{}); err != nil {
		t.Fatalf("put skip source: %v", err)
	}
	overwriteSource := root + "/overwrite-source.txt"
	if _, err := cloud.Put(ctx, overwriteSource, strings.NewReader("overwrite conflict"), int64(len("overwrite conflict")), drive.PutOptions{}); err != nil {
		t.Fatalf("put overwrite source: %v", err)
	}

	// dealWay=1 (skip): target keeps the pre-existing file untouched.
	skipDir := root + "/skip"
	if err := cloud.Mkdir(ctx, skipDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Put(ctx, skipDir+"/skip-source.txt", strings.NewReader("existing"), int64(len("existing")), drive.PutOptions{}); err != nil {
		t.Fatalf("put skip existing: %v", err)
	}
	if err := cloud.CopyOptions(ctx, drive.ConflictSkip, skipDir, skipSource); err != nil {
		t.Fatalf("conflict skip: %v", err)
	}
	skipEntries, err := cloud.ReadDirContext(ctx, skipDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipEntries) != 1 {
		t.Fatalf("skip policy produced %d entries, want 1", len(skipEntries))
	}
	var downloaded bytes.Buffer
	if _, err := cloud.Download(ctx, skipDir+"/skip-source.txt", &downloaded); err != nil {
		t.Fatal(err)
	}
	if downloaded.String() != "existing" {
		t.Fatalf("skip policy content=%q, want pre-existing %q", downloaded.String(), "existing")
	}

	// dealWay=3 (overwrite): the incoming copy replaces the target file.
	overwriteDir := root + "/overwrite"
	if err := cloud.Mkdir(ctx, overwriteDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Put(ctx, overwriteDir+"/overwrite-source.txt", strings.NewReader("old"), int64(len("old")), drive.PutOptions{}); err != nil {
		t.Fatalf("put overwrite old: %v", err)
	}
	if err := cloud.CopyOptions(ctx, drive.ConflictOverwrite, overwriteDir, overwriteSource); err != nil {
		t.Fatalf("conflict overwrite: %v", err)
	}
	overwriteEntries, err := cloud.ReadDirContext(ctx, overwriteDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(overwriteEntries) != 1 {
		t.Fatalf("overwrite policy produced %d entries, want 1", len(overwriteEntries))
	}
	downloaded.Reset()
	if _, err := cloud.Download(ctx, overwriteDir+"/overwrite-source.txt", &downloaded); err != nil {
		t.Fatal(err)
	}
	if downloaded.String() != "overwrite conflict" {
		t.Fatalf("overwrite policy content=%q, want incoming %q", downloaded.String(), "overwrite conflict")
	}

	// dealWay=2 (keep both): the incoming copy is auto-renamed by the server.
	keepDir := root + "/keep"
	if err := cloud.Mkdir(ctx, keepDir, 0755); err != nil {
		t.Fatal(err)
	}
	keepSource := root + "/keep-source.txt"
	if _, err := cloud.Put(ctx, keepDir+"/keep-source.txt", strings.NewReader("original"), int64(len("original")), drive.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Put(ctx, keepSource, strings.NewReader("incoming"), int64(len("incoming")), drive.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := cloud.CopyOptions(ctx, drive.ConflictKeepBoth, keepDir, keepSource); err != nil {
		t.Fatalf("conflict keep both: %v", err)
	}
	keepEntries, err := cloud.ReadDirContext(ctx, keepDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(keepEntries) != 2 {
		names := make([]string, len(keepEntries))
		for i, e := range keepEntries {
			names[i] = e.Name()
		}
		t.Fatalf("keep-both produced %d entries (%v), want 2", len(keepEntries), names)
	}

	if err := removeAndVerify(cloud, root); err != nil {
		t.Fatal(err)
	}
}

func removeAndVerify(cloud *drive.FS, name string) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err := cloud.RemoveAll(ctx, name)
		cancel()
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			_, statErr := cloud.Stat(name)
			if errors.Is(statErr, fs.ErrNotExist) {
				return nil
			}
			if statErr == nil {
				lastErr = fmt.Errorf("%s still exists", name)
			} else {
				lastErr = statErr
			}
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	return lastErr
}
