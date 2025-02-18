package openstackphy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"k8s.io/klog/v2"
	"net/http"
)

func (op *OpenStackPhy) GetNodeIdForECS(nodeId string) (string, error) {
	url := fmt.Sprintf("http://%s:5000/ecsnode_id", nodeId)
	res, code := postHTTPUrl(url, "{}")
	if code != 200 {
		klog.Error("Failed to get node connector")
		return "", errors.New("Failed to get node id")
	}

	return res, nil

}

func (op *OpenStackPhy) GetConnector(nodeId string) (result map[string]any, err error) {
	url := fmt.Sprintf("http://%s:5000/connector", nodeId)
	res, code := postHTTPUrl(url, "{}")
	if code != 200 {
		klog.Error("Failed to get node connector")
		return nil, errors.New("Failed to get node connector")
	}
	err = json.Unmarshal([]byte(res), &result)
	if err != nil {
		klog.Fatalf("JSON unmarshaling failed: %s", err)
		return nil, err
	}
	return result, nil
}

func (op *OpenStackPhy) connectVolume(connectionInfo map[string]any, nodeId string) (result map[string]string, err error) {
	url := fmt.Sprintf("http://%s:5000/attachvolume", nodeId)

	connection_info, err := json.Marshal(connectionInfo)
	if err != nil {
		klog.Fatalf("JSON marshaling failed: %s", err)
		return nil, err
	}

	res, code := postHTTPUrl(url, string(connection_info))
	if code != 200 {
		klog.Error("Failed to connect volume")
		return nil, errors.New("Failed to connect volume")
	}
	err = json.Unmarshal([]byte(res), &result)
	if err != nil {
		klog.Fatalf("JSON unmarshaling failed: %s", err)
		return nil, err
	}
	return result, nil
}

func (op *OpenStackPhy) disconnectVolume(connectionInfo map[string]any, nodeId string) (err error) {
	url := fmt.Sprintf("http://%s:5000/detachvolume", nodeId)
	connection_info, err := json.Marshal(connectionInfo)
	if err != nil {
		klog.Fatalf("JSON marshaling failed: %s", err)
		return err
	}

	_, code := postHTTPUrl(url, string(connection_info))
	if code != 200 {
		klog.Error("Failed to disconnect volume")
		return errors.New("Failed to disconnect volume")
	}
	return nil
}

func postHTTPUrl(url, jsonStr string) (string, int) {
	buf := bytes.NewBufferString(jsonStr)

	// 创建一个新的HTTP请求
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		klog.Fatalf("Error creating request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		klog.Fatalf("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		klog.Fatalf("Error reading response body: %v", err)
	}

	return string(body), resp.StatusCode
}
