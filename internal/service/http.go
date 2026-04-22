package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

func newHTTPClient(tlsVerify bool, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: !tlsVerify,
	}

	jar, _ := cookiejar.New(nil)

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		Jar:       jar,
	}
}

func newJSONRequest(
	ctx context.Context,
	method string,
	url string,
	headers map[string]string,
	body any,
) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

func decodeJSONResponse(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func readResponseSnippet(resp *http.Response, limit int) string {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
	if err != nil {
		return ""
	}
	return string(body)
}
