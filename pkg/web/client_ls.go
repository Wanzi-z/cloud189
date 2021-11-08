package web

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gowsp/cloud189-cli/pkg"
	"github.com/gowsp/cloud189-cli/pkg/file"
)

type fileList struct {
	Count   int              `json:"count,omitempty"`
	Files   []*file.FileInfo `json:"fileList,omitempty"`
	Folders []*file.FileInfo `json:"folderList,omitempty"`
}
type listResp struct {
	Code json.Number `json:"res_code,omitempty"`
	Data fileList    `json:"fileListAo,omitempty"`
}

type searchResp struct {
	ErrorCode string           `json:"errorCode,omitempty"`
	Code      string           `json:"res_code,omitempty"`
	Count     int              `json:"count,omitempty"`
	Files     []*file.FileInfo `json:"fileList,omitempty"`
	Folders   []*file.FileInfo `json:"folderList,omitempty"`
}

func (client *Client) Ls(path string) {
	info, err := client.Stat(path)
	if err != nil {
		log.Fatalln(err)
	}
	if info.IsDir() {
		files := client.list(info.Id(), 1)
		for _, v := range files {
			if v.IsDir() {
				fmt.Printf("- %s\n", v.Name())
			} else {
				fmt.Printf("f %s\n", v.Name())
			}
		}
	} else {
		fmt.Printf("f %s\n", info.Name())
	}
}

func (client *Client) finds(paths ...string) []*file.FileInfo {
	files := make([]*file.FileInfo, 0, len(paths))
	for _, path := range paths {
		info, err := client.Stat(path)
		if err != nil {
			log.Printf("%s not found, skip\n", path)
			continue
		}
		files = append(files, info.(*file.FileInfo))
	}
	return files
}
func (client *Client) Stat(cloud string) (pkg.FileInfo, error) {
	file.CheckPath(cloud)
	if cloud == "/" {
		return &file.FileInfo{FileId: file.Root.Id, FileName: file.Root.Name, IsFolder: true}, nil
	}
	info := &file.FileInfo{FileId: file.Root.Id, FileName: file.Root.Name, IsFolder: true}
	paths := strings.Split(cloud, "/")
	count := len(paths)
	for i := 1; i < count; i++ {
		path := paths[i]
		if i == 1 {
			if v, f := file.DefaultNameDir()[path]; f {
				info = &file.FileInfo{FileId: v.Id, FileName: v.Name, IsFolder: true}
				continue
			}
		}
		files := client.list(info.FileId.String(), 1)
		for _, v := range files {
			if v.Name() == path {
				info = v
				break
			}
		}
	}
	base := filepath.Base(cloud)
	if base == info.Name() {
		return info, nil
	}
	return nil, os.ErrNotExist
}

func (client *Client) Search(id, name string, page int, includAll bool) []*file.FileInfo {
	params := make(url.Values)
	params.Set("noCache", fmt.Sprintf("%v", rand.Float64()))
	params.Set("folderId", id)
	params.Set("pageNum", strconv.Itoa(page))
	params.Set("pageSize", "100")
	params.Set("filename", name)
	if includAll {
		params.Set("recursive", "1")
	} else {
		params.Set("recursive", "0")
	}
	params.Set("iconOption", "5")
	params.Set("descending", "true")
	params.Set("orderBy", "lastOpTime")

	req, _ := http.NewRequest(http.MethodGet, "https://cloud.189.cn/api/open/file/searchFiles.action?"+params.Encode(), nil)
	req.Header.Add("accept", "application/json;charset=UTF-8")
	var files searchResp
	resp, err := client.api.Do(req)
	if resp.StatusCode != 200 || err != nil {
		log.Fatalln("list file error")
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(&files)
	for _, v := range files.Folders {
		v.IsFolder = true
	}
	result := make([]*file.FileInfo, 0, files.Count)
	result = append(result, files.Files...)
	result = append(result, files.Folders...)
	if files.Count > len(files.Files)+len(files.Folders) {
		result = append(result, client.Search(id, name, page+1, includAll)...)
	}
	return result
}

func (client *Client) Readdir(id string, count int) []fs.FileInfo {
	data := client.list(id, 1)
	result := make([]fs.FileInfo, len(data))
	for i, v := range data {
		result[i] = v
	}
	return result
}
func (client *Client) list(id string, page int) []*file.FileInfo {
	params := make(url.Values)
	params.Set("folderId", id)
	params.Set("pageNum", strconv.Itoa(page))
	params.Set("pageSize", "100")
	params.Set("iconOption", "5")
	params.Set("descending", "true")
	params.Set("orderBy", "lastOpTime")
	params.Set("mediaType", strconv.Itoa(int(file.ALL)))

	req, _ := http.NewRequest(http.MethodGet, "https://cloud.189.cn/api/open/file/listFiles.action?"+params.Encode(), nil)
	req.Header.Add("accept", "application/json;charset=UTF-8")

	resp, _ := client.api.Do(req)
	body, _ := io.ReadAll(resp.Body)
	var list listResp
	json.Unmarshal(body, &list)
	if list.Code.String() == "" {
		var errorResp errorResp
		json.Unmarshal(body, &errorResp)
		if errorResp.IsInvalidSession() {
			client.initSesstion()
			return client.list(id, page)
		}
	}

	data := list.Data
	for _, v := range data.Folders {
		v.IsFolder = true
	}
	result := make([]*file.FileInfo, 0, data.Count)
	result = append(result, data.Files...)
	result = append(result, data.Folders...)
	if data.Count > len(data.Files)+len(data.Folders) {
		result = append(result, client.list(id, page+1)...)
	}
	return result
}
