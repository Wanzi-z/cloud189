package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func (c *Client) space(ctx context.Context) (space pkg.Space, err error) {
	err = c.invoker.GetContext(ctx, "/getUserInfo.action", nil, &space)
	return
}

type result struct {
	Result    int    `json:"result,omitempty"`
	ResultTip string `json:"resultTip,omitempty"`
}

func (client *Client) Sign(ctx context.Context) error {
	params := url.Values{}
	var r result
	err := client.invoker.GetContext(ctx, "/mkt/userSign.action", params, &r)
	if err == nil {
		if r.Result == -1 {
			fmt.Print("已签到 ")
		}
		fmt.Println(r.ResultTip)
	}
	client.signReq(ctx, "https://m.cloud.189.cn/v2/drawPrizeMarketDetails.action?taskId=TASK_SIGNIN&activityId=ACT_SIGNIN")
	client.signReq(ctx, "https://m.cloud.189.cn/v2/drawPrizeMarketDetails.action?taskId=TASK_SIGNIN_PHOTOS&activityId=ACT_SIGNIN")
	return ctx.Err()
}

type signResp struct {
	ErrorCode string `json:"errorCode,omitempty"`
	PrizeName string `json:"prizeName,omitempty"`
}

func (a *Client) signReq(ctx context.Context, url string) {
	var e signResp
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	err := a.invoker.Do(req, &e, 3)
	if err == nil {
		switch e.ErrorCode {
		case "User_Not_Chance":
			log.Println("signed")
		case "TimeOut":
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Millisecond * 200):
			}
			_ = a.refreshContext(ctx)
			a.signReq(ctx, url)
		default:
			log.Printf("obtain: %s", e.PrizeName)
		}
	} else {
		log.Println(err)
	}

}
