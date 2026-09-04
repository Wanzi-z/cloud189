package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gowsp/cloud189/pkg/drive"
)

type uploadConfig struct {
	Parallel uint32
	Pattern  string
	Policy   string
}

func (c uploadConfig) options() (drive.PutOptions, error) {
	if c.Parallel == 0 {
		return drive.PutOptions{}, errors.New("upload parallel must be positive")
	}
	switch strings.TrimSpace(c.Policy) {
	case "", "skip":
		return drive.PutOptions{}, nil
	case "overwrite":
		return drive.PutOptions{Overwrite: true}, nil
	default:
		return drive.PutOptions{}, errors.New("policy must be skip or overwrite")
	}
}

func putFile(ctx context.Context, client *drive.FS, local, name string, options drive.PutOptions) (drive.Entry, error) {
	return client.PutFile(ctx, name, local, options)
}

var digestPattern = regexp.MustCompile(`^fast://([A-Fa-f0-9]{32})(?::([A-Fa-f0-9]{32}))?:(\d+)/(.+)$`)

func putInput(ctx context.Context, client *drive.FS, destination, input, pattern string, options drive.PutOptions) error {
	if match := digestPattern.FindStringSubmatch(input); match != nil {
		size, err := strconv.ParseInt(match[3], 10, 64)
		if err != nil {
			return err
		}
		if size > 10<<20 && match[2] == "" {
			return errors.New("slice MD5 is required for files larger than 10 MiB")
		}
		sliceMD5 := match[2]
		if sliceMD5 == "" {
			sliceMD5 = match[1]
		}
		_, err = client.PutDigest(ctx, path.Join(destination, match[4]), size, drive.Digest{MD5: strings.ToUpper(match[1]), SliceMD5: strings.ToUpper(sliceMD5)}, options)
		return err
	}
	if strings.HasPrefix(input, "fast://") {
		return errors.New("invalid digest upload URL")
	}
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
		if err != nil {
			return err
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("download source returned %s", response.Status)
		}
		u, _ := url.Parse(input)
		_, err = client.Put(ctx, path.Join(destination, path.Base(u.Path)), response.Body, response.ContentLength, options)
		return err
	}
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		_, err = client.PutFile(ctx, path.Join(destination, filepath.Base(input)), input, options)
		return err
	}
	return putDirectory(ctx, client, destination, input, pattern, options)
}

func putDirectory(ctx context.Context, client *drive.FS, destination, local, pattern string, options drive.PutOptions) error {
	if pattern == "" {
		pattern = "*"
	}
	matches, err := filepath.Glob(filepath.Join(local, pattern))
	if err != nil {
		return err
	}
	for _, match := range matches {
		err := filepath.WalkDir(match, func(localName string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(local, localName)
			if err != nil {
				return err
			}
			remote := path.Join(destination, filepath.ToSlash(relative))
			if entry.IsDir() {
				return mkdirAll(ctx, client, remote)
			}
			_, err = client.PutFile(ctx, remote, localName, options)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func uploadInputs(ctx context.Context, client *drive.FS, destination string, inputs []string, config uploadConfig) error {
	options, err := config.options()
	if err != nil {
		return err
	}
	if _, err := client.StatContext(ctx, destination); errors.Is(err, fs.ErrNotExist) {
		if err := mkdirAll(ctx, client, destination); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	jobs := make(chan string)
	var group sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for i := uint32(0); i < config.Parallel; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for input := range jobs {
				if err := putInput(ctx, client, destination, input, config.Pattern, options); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s: %w", input, err))
					mu.Unlock()
				}
			}
		}()
	}
send:
	for _, input := range inputs {
		select {
		case jobs <- input:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	group.Wait()
	return errors.Join(errs...)
}

func mkdirAll(ctx context.Context, client *drive.FS, name string) error {
	if name == "." {
		return nil
	}
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = path.Join(current, part)
		if _, err := client.StatContext(ctx, current); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := client.Mkdir(ctx, current, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}

func downloadFile(ctx context.Context, client *drive.FS, name, local string) (drive.Entry, error) {
	entry, err := client.StatContext(ctx, name)
	if err != nil {
		return nil, err
	}
	if entry.IsDir() {
		return nil, errors.New("remote path is a directory")
	}
	if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
		return nil, err
	}
	destination, err := os.OpenFile(local, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	downloaded, downloadErr := client.Download(ctx, name, destination)
	return downloaded, errors.Join(downloadErr, destination.Close())
}

func downloadTree(ctx context.Context, client *drive.FS, name, local string) error {
	entry, err := client.StatContext(ctx, name)
	if err != nil {
		return err
	}
	if !entry.IsDir() {
		if info, statErr := os.Stat(local); statErr == nil && info.IsDir() {
			local = filepath.Join(local, entry.Name())
		}
		_, err = downloadFile(ctx, client, name, local)
		return err
	}
	directory := filepath.Join(local, entry.Name())
	if name == "." {
		directory = local
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	entries, err := client.ReadDirContext(ctx, name)
	if err != nil {
		return err
	}
	for _, child := range entries {
		if err := downloadTree(ctx, client, path.Join(name, child.Name()), directory); err != nil {
			return err
		}
	}
	return nil
}
