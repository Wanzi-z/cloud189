package app

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/gowsp/cloud189/internal/invoker"
	pkg "github.com/gowsp/cloud189/pkg/drive"
)

func testClient() *Client {
	client, err := Open(invoker.DefaultPath())
	if err != nil {
		panic(err)
	}
	return client
}

func init() {
	os.Setenv("189_MODE", "1")
}
func TestLogin(t *testing.T) {
	testClient().Login(context.Background(), "xxxxxxx", "xxxxxxxxxxx")
}
func TestQrLogin(t *testing.T) {
	testClient().QRLogin(context.Background())
}
func TestSpace(t *testing.T) {
	space, _ := testClient().space(context.Background())
	fmt.Println(space.Available, space.Capacity)
}
func TestSign(t *testing.T) {
	testClient().Sign(context.Background())
}
func TestListFile(t *testing.T) {
	f, _ := testClient().listEntries(context.Background(), personalRoot, pkg.RegularFile)
	fmt.Println(f)
}
func TestListDir(t *testing.T) {
	f, _ := testClient().listEntries(context.Background(), personalRoot, pkg.Directory)
	fmt.Println(f)
}
func TestSearchFile(t *testing.T) {
	f, _ := testClient().searchEntries(context.Background(), personalRoot, pkg.RegularFile, "1")
	fmt.Println(f)
}
func TestSearchDir(t *testing.T) {
	f, _ := testClient().searchEntries(context.Background(), personalRoot, pkg.Directory, "我")
	fmt.Println(f)
}
func TestMkdir(t *testing.T) {
	testClient().mkdir(context.Background(), personalRoot, "/demo/1/2/3")
}
func TestDelete(t *testing.T) {
	api := testClient()
	dir, _ := api.searchEntries(context.Background(), personalRoot, pkg.Directory, "demo")
	api.remove(context.Background(), dir...)
}
func TestCopy(t *testing.T) {
	api := testClient()
	f, _ := api.mkdir(context.Background(), personalRoot, "/demo/1/2/3")
	api.copyEntries(context.Background(), personalRoot, f)
}
func TestRename(t *testing.T) {
	api := testClient()
	demo, _ := api.searchEntries(context.Background(), personalRoot, pkg.Directory, "demo")
	api.rename(context.Background(), demo[0], "demo")
}
func TestMove(t *testing.T) {
	api := testClient()
	f, _ := api.mkdir(context.Background(), personalRoot, "/demo/1/2/3")
	api.moveEntries(context.Background(), personalRoot, f)
}
func TestGetFolder(t *testing.T) {
	api := testClient()
	f, _ := api.stat(context.Background(), "/demo/1/2/3")
	if f.IsDir() {
		api.usage(context.Background(), f)
	}
}
func TestGetFile(t *testing.T) {
	api := testClient()
	api.stat(context.Background(), "/我的图片")
}
