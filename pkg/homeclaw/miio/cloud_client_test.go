// Package miio provides unit tests for CloudClient
package miio

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// newTestCloudClient creates a CloudClient pointed at the given httptest server URL.
func newTestCloudClient(t *testing.T) (*CloudClient, error) {
	t.Helper()
	token := ""
	if token == "" {
		t.Fatal("token is empty")
		return nil, &OAuthError{Code: -1, Message: fmt.Sprintf("invalid http response, %s", "token is empty")}
	}
	client, err := NewCloudClient("cn", OAuth2ClientID, token)
	if err != nil {
		t.Fatalf("NewCloudClient() error = %v", err)
		return nil, err
	}
	return client, nil
}

func TestCloudClient_SetProps(t *testing.T) {

	client, err := newTestCloudClient(t)
	if err != nil {
		t.Fatalf("newTestCloudClient() error = %v", err)
		return
	}
	result, err := client.SetProps([]map[string]interface{}{{"did": "482654707", "siid": 2, "piid": 1, "value": true}})
	t.Logf("result: %v", result)

	if err != nil {
		t.Errorf("SetProps() error = %v", err)
		return
	}
}
func TestCloudClient_GetProps(t *testing.T) {
	client, err := newTestCloudClient(t)
	if err != nil {
		t.Fatalf("newTestCloudClient() error = %v", err)
		return
	}
	result, err := client.GetProp("482654707", 2, 1)
	if err != nil {
		t.Errorf("getProps() error = %v", err)
		return
	}
	t.Logf("getProps result: %v", result)
}
func TestCloudClient_StarRtspAction(t *testing.T) {

	client, err := newTestCloudClient(t)
	if err != nil {
		t.Fatalf("newTestCloudClient() error = %v", err)
		return
	}
	result, err := client.Action("357864212", 5, 1, []interface{}{2})
	t.Logf("result: %v", result)

	if err != nil {
		t.Errorf("Action() error = %v", err)
		return
	}
	// result["out"] is []interface{}{rtspURL, thumbnailURL, timestamp}
	outList, ok := result["out"].([]interface{})
	if !ok || len(outList) == 0 {
		t.Fatalf("unexpected out field: %v", result["out"])
		return
	}
	hls, ok := outList[0].(string)
	if !ok || hls == "" {
		t.Fatalf("hls url not found in out[0]: %v", outList[0])
		return
	}
	t.Logf("hls url: %s", hls)

	// 构建 HLS 请求头，携带认证信息（小米直播转码服务需要鉴权）
	headers := client.hlsHeaders()
	var headerLines []string
	for k, v := range headers {
		headerLines = append(headerLines, k+": "+v+"\r\n")
	}
	headerStr := strings.Join(headerLines, "")

	// 使用 ffmpeg 录制 2 秒 HLS 流
	cmd := exec.Command("ffmpeg",
		"-headers", headerStr,
		"-i", hls,
		"-t", "2",
		"-c", "copy",
		"-y",
		"output.mp4",
	)
	t.Logf("ffmpeg command: %s", strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("错误: %v\n输出: %s\n", err, output)
		return
	}
	fmt.Println("录制完成: output.mp4")
}
func TestCloudClient_StopRtspAction(t *testing.T) {

	client, err := newTestCloudClient(t)
	if err != nil {
		t.Fatalf("newTestCloudClient() error = %v", err)
		return
	}
	result, err := client.Action("357864212", 5, 2, []interface{}{})
	t.Logf("result: %v", result)

	if err != nil {
		t.Errorf("Action() error = %v", err)
		return
	}

}
