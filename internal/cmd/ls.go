package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/gowsp/cloud189/internal/session"
	"github.com/gowsp/cloud189/pkg/file"
	"github.com/spf13/cobra"
)

var recursiveList bool

func init() {
	lsCmd.Flags().BoolVarP(&recursiveList, "recursive", "R", false, "递归列出")
}

type dirReader interface {
	ReadDirContext(context.Context, string) ([]fs.DirEntry, error)
}

var lsCmd = &cobra.Command{
	Use:    "ls [path]",
	PreRun: session.Parse,
	Short:  "列出文件",
	Args:   cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := "/"
		if session.Pwd() != "" {
			name = session.Pwd()
		}
		if len(args) == 0 {
		} else if len(args) > 0 {
			name = args[0]
		}
		client, location, err := resolveCloudPath(name)
		if err != nil {
			return err
		}
		name = location.name()
		if err := file.CheckPath(location.path); err != nil {
			return err
		}
		if jsonOutput {
			entries, err := listJSONEntries(cmd.Context(), client, name, recursiveList)
			if err != nil {
				return err
			}
			for i := range entries {
				entries[i].Path = location.display(entries[i].Path)
			}
			return writeJSON(entries)
		}
		files, err := client.ReadDirContext(cmd.Context(), name)
		if err != nil {
			return err
		}
		for _, v := range files {
			info, _ := v.Info()
			fmt.Println(file.ReadableFileInfo(info))
		}
		return nil

	},
}

func listJSONEntries(ctx context.Context, client dirReader, root string, recursive bool) ([]JSONFileEntry, error) {
	rootPath := cleanCloudPath(root)
	entries, err := listJSONEntriesFrom(ctx, client, root, rootPath, rootPath, recursive)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func listJSONEntriesFrom(ctx context.Context, client dirReader, currentName, currentPath, base string, recursive bool) ([]JSONFileEntry, error) {
	files, err := client.ReadDirContext(ctx, currentName)
	if err != nil {
		return nil, err
	}
	entries := make([]JSONFileEntry, 0, len(files))
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			return nil, err
		}
		entryName := path.Join(currentName, info.Name())
		entryPath := joinCloudPath(currentPath, info.Name())
		if recursive && info.IsDir() {
			childEntries, err := listJSONEntriesFrom(ctx, client, entryName, entryPath, base, recursive)
			if err != nil {
				return nil, err
			}
			entries = append(entries, childEntries...)
			continue
		}
		entries = append(entries, fileToJSONEntryWithBase(entryPath, base, info))
	}
	return entries, nil
}
