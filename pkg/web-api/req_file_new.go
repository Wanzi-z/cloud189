package web

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gowsp/cloud189/pkg"
	"github.com/gowsp/cloud189/pkg/drive"
)

func (client *Api) Upload(upload pkg.UploadFile, part pkg.UploadPart) error {
	upload.Prepare(func() {
		client.init(upload, upload.ParentId())
	})
	if upload.IsExists() {
		log.Println("file exists, fast upload")
		client.commit(upload, upload.UploadId(), "0")
		return nil
	}
	err := client.UploadPart(part, upload.UploadId())
	if err != nil {
		return err
	}
	if upload.IsComplete() {
		client.commit(upload, upload.UploadId(), "1")
	}
	return nil
}

type uploadInfo struct {
	UploadType     int    `json:"uploadType,omitempty"`
	UploadHost     string `json:"uploadHost,omitempty"`
	UploadFileId   string `json:"uploadFileId,omitempty"`
	FileDataExists int    `json:"fileDataExists,omitempty"`
}
type initResp struct {
	Code string     `json:"code,omitempty"`
	Data uploadInfo `json:"data,omitempty"`
}

func (r *initResp) GetCode() string {
	return r.Code
}

func (c *Api) init(i pkg.UploadFile, parentId string) error {
	params := make(url.Values)
	params.Set("parentFolderId", parentId)
	params.Set("fileName", i.Name())
	params.Set("fileSize", strconv.FormatInt(i.Size(), 10))
	params.Set("sliceSize", strconv.Itoa(drive.Slice))

	if i.SliceNum() > 1 {
		params.Set("lazyCheck", "1")
	} else {
		params.Set("fileMd5", i.FileMD5())
		params.Set("sliceMd5", i.SliceMD5())
	}
	var upload initResp
	c.do("/person/initMultiUpload", params, &upload)
	fileId := upload.Data.UploadFileId
	if fileId == "" {
		return errors.New("error get upload fileid")
	}
	i.SetUploadId(fileId)
	i.SetExists(upload.Data.FileDataExists == 1)
	return nil
}
func (client *Api) check(i pkg.UploadFile, fileId string) error {
	var upload initResp
	params := make(url.Values)
	params.Set("fileMd5", i.FileMD5())
	params.Set("sliceMd5", i.SliceMD5())
	params.Set("uploadFileId", fileId)
	err := client.do("/person/checkTransSecond", params, &upload)
	if err != nil {
		return err
	}
	i.SetExists(upload.Data.FileDataExists == 1)
	return nil
}

type urlResp struct {
	Code string                `json:"code,omitempty"`
	Data map[string]uploadUrls `json:"uploadUrls,omitempty"`
}

func (r *urlResp) GetCode() string {
	return r.Code
}

type uploadUrls struct {
	RequestURL    string `json:"requestURL,omitempty"`
	RequestHeader string `json:"requestHeader,omitempty"`
}

func (client *Api) UploadPart(part pkg.UploadPart, fileId string) error {
	p := make(url.Values)
	num := strconv.Itoa(part.Num() + 1)
	p.Set("partInfo", fmt.Sprintf("%s-%s", num, part.Name()))
	p.Set("uploadFileId", fileId)

	var urlResp urlResp
	if err := client.do("/person/getMultiUploadUrls", p, &urlResp); err != nil {
		return err
	}
	log.Printf("start uploading part %s\n", num)

	upload := urlResp.Data["partNumber_"+num]
	req, _ := http.NewRequest(http.MethodPut, upload.RequestURL, part.Data())
	headers := strings.Split(upload.RequestHeader, "&")
	for _, v := range headers {
		i := strings.Index(v, "=")
		req.Header.Set(v[0:i], v[i+1:])
	}
	resp, err := client.invoker.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload error, %s", string(data))
	}
	log.Printf("part %s upload completed\n", num)
	return nil
}

type UploadResult struct {
	Code string       `json:"code,omitempty"`
	File UploadDetail `json:"file,omitempty"`
}

func (r *UploadResult) GetCode() string {
	return r.Code
}

type UploadDetail struct {
	Id         string `json:"userFileId,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	FileMd5    string `json:"file_md_5,omitempty"`
	CreateDate string `json:"create_date,omitempty"`
}

func (client *Api) commit(i pkg.UploadFile, fileId, lazyCheck string) error {
	var result UploadResult
	params := make(url.Values)
	params.Set("fileMd5", i.FileMD5())
	params.Set("sliceMd5", i.SliceMD5())
	params.Set("lazyCheck", lazyCheck)
	params.Set("uploadFileId", fileId)
	return client.do("/person/commitMultiUploadFile", params, &result)
}