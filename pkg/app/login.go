package app

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/gowsp/cloud189/internal/invoker"
)

func (api *Client) beforLogin() url.Values {
	params := url.Values{}
	params.Set("appId", "9317140619")
	params.Set("clientType", "10020")
	params.Set("timeStamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("returnURL", "https://m.cloud.189.cn/zhuanti/2020/loginErrorPc/index.html")
	return params
}

func (api *Client) Login(ctx context.Context, username, password string) (err error) {
	params := api.beforLogin()
	user := &invoker.User{Name: username, Password: password}
	resp, err := api.invoker.PwdLogin(ctx, "https://cloud.189.cn/unifyLoginForPC.action", params, user)
	if err != nil {
		return err
	}
	api.conf.User = user
	return api.afterLogin(ctx, resp)
}

func (api *Client) QRLogin(ctx context.Context) (err error) {
	params := api.beforLogin()
	resp, err := api.invoker.QrLogin(ctx, "https://cloud.189.cn/unifyLoginForPC.action", params)
	if err != nil {
		return err
	}
	return api.afterLogin(ctx, resp)
}

func (api *Client) afterLogin(ctx context.Context, resp *invoker.LoginResult) error {
	var userSession invoker.Session
	params := url.Values{}
	params.Set("redirectURL", resp.ToUrl)
	if err := api.invoker.PostContext(ctx, "/getSessionForPC.action", params, &userSession); err != nil {
		return err
	}
	api.conf.Session = &userSession
	return api.conf.Save()
}
