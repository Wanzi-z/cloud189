package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gowsp/cloud189/internal/invoker"
	"github.com/gowsp/cloud189/internal/util"
)

type Client struct {
	invoker *invoker.Invoker
	conf    *invoker.Config
}

func Open(path string) (*Client, error) {
	conf, err := invoker.OpenConfig(path)
	if err != nil {
		return nil, err
	}
	return newClient(conf), nil
}

func New(username, password string) *Client {
	conf := &invoker.Config{User: &invoker.User{Name: username, Password: password}}
	return newClient(conf)
}

func newClient(conf *invoker.Config) *Client {
	client := &Client{conf: conf}
	client.invoker = invoker.NewInvoker("https://api.cloud.189.cn", client.refreshContext, conf)
	client.invoker.SetPrepareE(client.sign)
	return client
}

func (api *Client) refreshContext(ctx context.Context) error {
	s := api.conf.Session
	if s.Login() {
		params := url.Values{}
		params.Set("appId", "9317140619")
		params.Set("accessToken", s.AccessToken)
		var newSession invoker.Session
		if err := api.invoker.PostContext(ctx, "/getSessionForPC.action", params, &newSession); err != nil {
			return err
		}
		s.Merge(newSession)
		return api.conf.Save()
	}
	user := api.conf.User
	if user == nil || user.Name == "" || user.Password == "" {
		return errors.New("扫码不支持自动重新登录")
	}
	return api.Login(ctx, api.conf.User.Name, api.conf.User.Password)
}

func (api *Client) sign(req *http.Request) error {
	now := time.Now()
	query := req.URL.Query()
	// 填充客户端参数
	query.Set("rand", strconv.FormatInt(now.UnixMilli(), 10))
	query.Set("clientType", "TELEPC")
	query.Set("version", "7.1.8.0")
	query.Set("channelId", "web_cloud.189.cn")
	req.URL.RawQuery = query.Encode()
	if req.URL.Path == "/getSessionForPC.action" {
		return nil
	}

	// sha1(SessionKey=相应的值&Operate=相应值&RequestURI=相应值&Date=相应的值”, SessionSecret)
	session := api.conf.Session
	if session == nil {
		return errors.New("请先登录")
	}
	key, secret := session.Key, session.Secret
	if invoker.RequestScope(req) == invoker.FamilyScope {
		key, secret = session.FamilyKey, session.FamilySecret
	}
	if key == "" || secret == "" {
		return errors.New("登录会话无效，请重新登录")
	}
	date := now.Format(time.RFC1123)
	data := fmt.Sprintf("SessionKey=%s&Operate=%s&RequestURI=%s&Date=%s",
		key, req.Method, req.URL.Path, date)
	// 追加上传参数
	params := invoker.RequestSignParams(req)
	if req.Host == "upload.cloud.189.cn" || params != "" {
		if params == "" {
			params = query.Get("params")
		}
		data += "&params=" + params
	}
	req.Header.Set("Date", date)
	req.Header.Set("user-agent", "desktop")
	req.Header.Set("SessionKey", key)
	req.Header.Set("Signature", util.Sha1(data, secret))
	req.Header.Set("X-Request-ID", util.Random("xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"))
	return nil
}
