package client

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	"k8s.io/klog/v2"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/utils/leveledlogger"
)

// doJSONNoData calls doJSON[struct{}] when you don't care about Data.
func doJSONNoData(c *Client, method, url string, payload any) error {
	_, err := doJSON[struct{}](c, method, url, payload)
	return err
}

// Generic JSON std-envelope request returning Data as T.
func doJSON[T any](c *Client, method, url string, payload any) (T, error) {
	var zero T

	resp, err := c.request(method, url, payload)
	if err != nil {
		klog.Errorf("HTTP request failed, method(%s) url(%s), err: %v", method, url, err)
		return zero, err
	}
	defer resp.Body.Close()

	// Try to decode std envelope
	var stdResp api.ResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&stdResp); err != nil {
		// If server returned non-200 + non-JSON body, prefer status
		if resp.StatusCode != http.StatusOK {
			klog.Errorf("Server error (non-JSON), method(%s) url(%s): %s", method, url, resp.Status)
			return zero, fmt.Errorf("server error: %s", resp.Status)
		}
		klog.Errorf("Decode response body failed, method(%s) url(%s), err: %v", method, url, err)
		return zero, err
	}

	// Non-200 -> use server message if present
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return zero, ErrNotFound
		}

		msg := stdResp.Message
		if msg == "" {
			msg = resp.Status
		}
		klog.Errorf("Server error, method(%s) url(%s): %s", method, url, msg)
		return zero, fmt.Errorf("server error: %s", msg)
	}

	// Marshal stdResp.Data back to JSON then into T (robust to interface{} shape)
	dataBytes, err := json.Marshal(stdResp.Data)
	if err != nil {
		klog.Errorf("Marshal stdResp.Data failed, method(%s) url(%s): %v", method, url, err)
		return zero, err
	}
	var out T
	// tolerate null / empty
	if len(dataBytes) > 0 && string(dataBytes) != "null" {
		if err := json.Unmarshal(dataBytes, &out); err != nil {
			klog.Errorf("Unmarshal to target type failed, method(%s) url(%s): %v", method, url, err)
			return zero, err
		}
	}
	return out, nil
}

// request builds and executes an HTTP request.
// If reqBody is []byte or json.RawMessage, it is sent as-is (no re-marshal).
// Otherwise, reqBody is JSON-marshaled.
func (c *Client) request(method string, url string, reqBody any) (*http.Response, error) {
	var (
		httpReq *retryablehttp.Request
		err     error
	)

	switch b := reqBody.(type) {
	case nil:
		httpReq, err = c.newHTTPReq(method, url, nil)
	case []byte:
		httpReq, err = c.newHTTPReq(method, url, b)
		httpReq.Header.Set("Content-Type", "application/json")
	case json.RawMessage:
		httpReq, err = c.newHTTPReq(method, url, b)
		httpReq.Header.Set("Content-Type", "application/json")
	default:
		reqBodyJSON, mErr := json.Marshal(reqBody)
		if mErr != nil {
			klog.Errorf("Failed to marshal request body, method(%s) url(%s): %v", method, url, mErr)
			return nil, mErr
		}
		httpReq, err = c.newHTTPReq(method, url, reqBodyJSON)
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if err != nil {
		klog.Errorf("Failed to create http request, method(%s) url(%s): %v", method, url, err)
		return nil, err
	}

	client := c.retryClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		klog.Errorf("Failed to send http request, method(%s) url(%s): %v", method, url, err)
		return nil, err
	}
	return resp, nil
}

// requestData sends gzipped []byte and transparently ungzips response if needed.
func (c *Client) requestData(method string, url string, data []byte) (*http.Response, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data is empty")
	}

	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(data); err != nil {
		klog.Errorf("Failed to compress request body, method(%s) url(%s): %v", method, url, err)
		return nil, err
	}
	if err := gz.Close(); err != nil {
		klog.Errorf("Failed to close gzip writer, method(%s) url(%s): %v", method, url, err)
		return nil, err
	}

	httpReq, err := c.newHTTPReq(method, url, compressed.Bytes())
	if err != nil {
		klog.Errorf("Failed to create http request, method(%s) url(%s): %v", method, url, err)
		return nil, err
	}
	httpReq.Header.Set("Content-Encoding", "gzip")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept-Encoding", "gzip, identity")

	client := c.retryClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		klog.Errorf("Failed to send http request, method(%s) url(%s): %v", method, url, err)
		return nil, err
	}

	// transparently unwrap gzipped response body to a new ReadCloser
	if resp.Header.Get("Content-Encoding") == "gzip" && resp.Body != nil {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			klog.Errorf("Failed to create gzip reader, method(%s) url(%s): %v", method, url, err)
			_ = resp.Body.Close()
			return nil, err
		}
		// read all & replace body, ensure closing original body
		body, readErr := io.ReadAll(gr)
		_ = gr.Close()
		_ = resp.Body.Close()
		if readErr != nil {
			klog.Errorf("Failed to read gzipped response body, method(%s) url(%s): %v", method, url, readErr)
			return nil, readErr
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}

	return resp, nil
}

// Build the request with common headers
func (c *Client) newHTTPReq(method, url string, body []byte) (*retryablehttp.Request, error) {
	httpReq, err := retryablehttp.NewRequest(method, url, body)
	if err != nil {
		klog.Errorf("Failed to create http request: %v", err)
		return nil, err
	}
	httpReq.Header.Set("X-API-KEY", c.APIKEY)
	return httpReq, nil
}

func (c *Client) retryClient() *retryablehttp.Client {
	if c.rc != nil {
		return c.rc
	}
	rc := retryablehttp.NewClient()
	rc.Logger = leveledlogger.NewKlogLeveledLogger()
	c.rc = rc
	return rc
}
