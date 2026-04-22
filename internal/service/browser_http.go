package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	tls "github.com/bogdanfinn/utls"
)

func newBrowserHTTPClient(
	proxyConfig *ProxyConfig,
	timeout time.Duration,
	impersonate string,
) (tls_client.HttpClient, error) {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(int(timeout.Seconds())),
		tls_client.WithClientProfile(resolveClientProfile(impersonate)),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
	}

	if proxyConfig != nil && proxyConfig.Enabled {
		proxyURL, err := proxyConfig.URL()
		if err != nil {
			return nil, err
		}
		options = append(options, tls_client.WithProxyUrl(proxyURL.String()))
	}

	return tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
}

func resolveClientProfile(impersonate string) profiles.ClientProfile {
	switch strings.ToLower(strings.TrimSpace(impersonate)) {
	case "", "edge101", "edge", "edge_101":
		base := profiles.Chrome_131
		return profiles.NewClientProfile(
			tls.HelloEdge_Auto,
			base.GetSettings(),
			base.GetSettingsOrder(),
			base.GetPseudoHeaderOrder(),
			base.GetConnectionFlow(),
			base.GetPriorities(),
			base.GetHeaderPriority(),
			base.GetStreamID(),
			base.GetAllowHTTP(),
			base.GetHttp3Settings(),
			base.GetHttp3SettingsOrder(),
			base.GetHttp3PriorityParam(),
			base.GetHttp3PseudoHeaderOrder(),
			base.GetHttp3SendGreaseFrames(),
		)
	case "chrome131", "chrome_131", "chrome":
		return profiles.Chrome_131
	default:
		return profiles.Chrome_131
	}
}

func newBrowserRequest(
	ctx context.Context,
	method string,
	url string,
	headers map[string]string,
	body any,
) (*fhttp.Request, error) {
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	return newBrowserRawRequest(ctx, method, url, headers, reader)
}

func newBrowserRawRequest(
	ctx context.Context,
	method string,
	url string,
	headers map[string]string,
	body io.Reader,
) (*fhttp.Request, error) {
	req, err := fhttp.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

func doBrowserRequest(client tls_client.HttpClient, req *fhttp.Request) (resp *fhttp.Response, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("browser client panic: %v", recovered)
		}
	}()
	return client.Do(req)
}

func decodeBrowserJSONResponse(resp *fhttp.Response, dst any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func readBrowserResponseSnippet(resp *fhttp.Response, limit int) string {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
	if err != nil {
		return ""
	}
	return string(body)
}
