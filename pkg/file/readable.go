package file

import (
	"fmt"
	"io/fs"
)

const (
	KB = 1 << 10
	MB = 1 << 20
	GB = 1 << 30
	TB = 1 << 40
)

func ReadableSize(size uint64) string {
	result, unit := float64(size), ""
	switch {
	case size >= TB:
		result, unit = result/TB, "T"
	case size >= GB:
		result, unit = result/GB, "G"
	case size >= MB:
		result, unit = result/MB, "M"
	case size >= KB:
		result, unit = result/KB, "K"
	}
	return fmt.Sprintf("%.2f%s", result, unit)
}

func ReadableFileInfo(info fs.FileInfo) string {
	size := "-"
	if !info.IsDir() {
		size = ReadableSize(uint64(info.Size()))
	}
	return fmt.Sprintf("%-10s%-22s%s", size, info.ModTime().Format("2006-01-02 15:04:05"), info.Name())
}
