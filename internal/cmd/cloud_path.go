package cmd

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/gowsp/cloud189/pkg/drive"
)

type cloudPath struct {
	familyID string
	path     string
}

func parseCloudPath(value string) (cloudPath, error) {
	return parseCloudPathWithFamily(value, "")
}

func parseCloudPathWithFamily(value, defaultFamilyID string) (cloudPath, error) {
	if value == "" {
		value = "/"
	}
	if defaultFamilyID != "" && !isFamilyID(defaultFamilyID) {
		return cloudPath{}, fmt.Errorf("无效的家庭 ID: %s", defaultFamilyID)
	}
	separator := strings.IndexByte(value, ':')
	if separator > 0 {
		prefix := value[:separator]
		rest := value[separator+1:]
		if isFamilyID(prefix) {
			if rest == "" {
				rest = "/"
			}
			if !strings.HasPrefix(rest, "/") {
				return cloudPath{}, fmt.Errorf("云盘路径必须为绝对路径: %s", value)
			}
			location := cloudPath{path: path.Clean(rest)}
			location.familyID = prefix
			return location, nil
		}
	}
	if !path.IsAbs(value) {
		return cloudPath{}, errors.New("云盘路径必须以 / 开头，家庭云路径格式为 家庭ID:/路径")
	}
	return cloudPath{familyID: defaultFamilyID, path: path.Clean(value)}, nil
}

func isFamilyID(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range []byte(value) {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (p cloudPath) key() string {
	if p.familyID == "" {
		return "personal"
	}
	return "family:" + p.familyID
}

func (p cloudPath) sameSpace(other cloudPath) bool {
	return p.familyID == other.familyID
}

func (p cloudPath) name() string {
	if p.path == "/" {
		return "."
	}
	return strings.TrimPrefix(p.path, "/")
}

func (p cloudPath) display(value string) string {
	if p.familyID == "" {
		return value
	}
	return p.familyID + ":" + value
}

func resolveCloudPath(value string) (*drive.FS, cloudPath, error) {
	location, err := parseCloudPathWithFamily(value, familySelector)
	if err != nil {
		return nil, cloudPath{}, err
	}
	client, err := driveFor(location)
	return client, location, err
}

func ResolveCloudPath(value string) (*drive.FS, string, error) {
	drive, location, err := resolveCloudPath(value)
	if err != nil {
		return nil, "", err
	}
	return drive, location.name(), nil
}

func NormalizeCloudPath(value string) (string, error) {
	location, err := parseCloudPathWithFamily(value, familySelector)
	if err != nil {
		return "", err
	}
	return location.display(location.path), nil
}
