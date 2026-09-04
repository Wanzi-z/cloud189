package invoker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

type Invoker struct {
	url     string
	http    *http.Client
	conf    *Config
	prepare func(*http.Request) error
	Refresh func(context.Context) error
}

type Scope uint8

const (
	PersonalScope Scope = iota
	FamilyScope
)

type scopeKey struct{}
type signParamsKey struct{}

func WithScope(req *http.Request, scope Scope) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), scopeKey{}, scope))
}

func RequestScope(req *http.Request) Scope {
	if scope, ok := req.Context().Value(scopeKey{}).(Scope); ok {
		return scope
	}
	return PersonalScope
}

func WithSignParams(req *http.Request, params string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), signParamsKey{}, params))
}

func RequestSignParams(req *http.Request) string {
	params, _ := req.Context().Value(signParamsKey{}).(string)
	return params
}

func NewInvoker(apiUrl string, refresh func(context.Context) error, conf *Config) *Invoker {
	jar, _ := cookiejar.New(nil)
	sson := []*http.Cookie{{Name: "SSON", Value: conf.SSON}}
	user := []*http.Cookie{{Name: "COOKIE_LOGIN_USER", Value: conf.Auth}}
	jar.SetCookies(&url.URL{Scheme: "https", Host: "e.189.cn"}, sson)
	jar.SetCookies(&url.URL{Scheme: "https", Host: "cloud.189.cn"}, user)
	jar.SetCookies(&url.URL{Scheme: "https", Host: "m.cloud.189.cn"}, user)
	return &Invoker{url: apiUrl, Refresh: refresh, http: &http.Client{Jar: jar}, conf: conf}
}

func (i *Invoker) SetPrepare(prepare func(req *http.Request)) {
	i.prepare = func(req *http.Request) error {
		prepare(req)
		return nil
	}
}
func (i *Invoker) SetPrepareE(prepare func(req *http.Request) error) {
	i.prepare = prepare
}
func (i *Invoker) Cookies(url *url.URL) []*http.Cookie {
	return i.http.Jar.Cookies(url)
}
func (i *Invoker) Cookie(raw, name string) string {
	url, _ := url.Parse(raw)
	cookies := i.http.Jar.Cookies(url)
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func (i *Invoker) DoWithResp(req *http.Request) (*http.Response, error) {
	if i.prepare != nil {
		if err := i.prepare(req); err != nil {
			return nil, err
		}
	}
	resp, err := i.http.Do(req)
	val := os.Getenv("189_MODE")
	if val == "1" {
		rdata, _ := httputil.DumpRequest(req, true)
		fmt.Println(string(rdata))
		data, _ := httputil.DumpResponse(resp, true)
		fmt.Println(string(data))
	}
	return resp, err
}
func (i *Invoker) Do(req *http.Request, data any, retry int) error {
	attempt := 0
	return i.DoBuilt(func() (*http.Request, error) {
		if attempt > 0 {
			req.Header.Del("Cookie")
			if req.GetBody != nil {
				req.Body, _ = req.GetBody()
			}
		}
		attempt++
		return req, nil
	}, data, retry)
}

func (i *Invoker) DoBuilt(build func() (*http.Request, error), data any, retry int) error {
	if retry == 0 {
		return os.ErrInvalid
	}
	req, err := build()
	if err != nil {
		return err
	}
	resp, err := i.DoWithResp(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		err := json.NewDecoder(resp.Body).Decode(data)
		if err != nil {
			return err
		}
		if rsp, ok := data.(OkRsp); ok && !rsp.IsSuccess() {
			return rsp
		}
		return nil

	case http.StatusBadRequest, http.StatusForbidden:
		rsp := new(strCodeRsp)
		err := json.NewDecoder(resp.Body).Decode(rsp)
		if err == nil && rsp.isBusinessErr() {
			return rsp
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return req.Context().Err()
		case <-timer.C:
		}
		err = i.Refresh(req.Context())
		if err != nil {
			return err
		}
		return i.DoBuilt(build, data, retry-1)
	default:
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}
}

func (i *Invoker) Send(req *http.Request) (*http.Response, error) {
	return i.http.Do(req)
}
func (i *Invoker) Fetch(path string) (*http.Response, error) {
	return i.http.Get(path)
}
func (i *Invoker) Get(path string, params url.Values, data any) error {
	return i.GetContext(context.Background(), path, params, data)
}
func (i *Invoker) GetContext(ctx context.Context, path string, params url.Values, data any) error {
	return i.GetScopedContext(ctx, PersonalScope, path, params, data)
}
func (i *Invoker) GetScoped(scope Scope, path string, params url.Values, data any) error {
	return i.GetScopedContext(context.Background(), scope, path, params, data)
}
func (i *Invoker) GetScopedContext(ctx context.Context, scope Scope, path string, params url.Values, data any) error {
	url := i.url + path
	if len(params) > 0 {
		url += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req = WithScope(req, scope)
	return i.Do(req, data, 3)
}
func (i *Invoker) Post(path string, params url.Values, data any) error {
	return i.PostContext(context.Background(), path, params, data)
}
func (i *Invoker) PostContext(ctx context.Context, path string, params url.Values, data any) error {
	return i.PostScopedContext(ctx, PersonalScope, path, nil, params, data)
}
func (i *Invoker) PostScoped(scope Scope, path string, query, params url.Values, data any) error {
	return i.PostScopedContext(context.Background(), scope, path, query, params, data)
}
func (i *Invoker) PostScopedContext(ctx context.Context, scope Scope, path string, query, params url.Values, data any) error {
	u := i.url + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = WithScope(req, scope)
	return i.Do(req, data, 3)
}
