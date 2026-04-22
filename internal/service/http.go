package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"time"

	xproxy "golang.org/x/net/proxy"
)

func newHTTPClient(
	tlsVerify bool,
	timeout time.Duration,
	proxyConfig *ProxyConfig,
) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: !tlsVerify,
	}

	if err := applyProxyConfig(transport, timeout, proxyConfig); err != nil {
		return nil, err
	}

	jar, _ := cookiejar.New(nil)

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		Jar:       jar,
	}, nil
}

func applyProxyConfig(
	transport *http.Transport,
	timeout time.Duration,
	proxyConfig *ProxyConfig,
) error {
	if proxyConfig == nil || !proxyConfig.Enabled {
		return nil
	}

	switch proxyConfig.Type {
	case "http", "https":
		proxyURL, err := proxyConfig.URL()
		if err != nil {
			return err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
		return nil
	case "socks5":
		return applySOCKS5Proxy(transport, timeout, proxyConfig)
	default:
		return badRequest("unsupported proxy type")
	}
}

func applySOCKS5Proxy(
	transport *http.Transport,
	timeout time.Duration,
	proxyConfig *ProxyConfig,
) error {
	address := net.JoinHostPort(proxyConfig.Host, strconv.Itoa(proxyConfig.Port))
	baseDialer := &net.Dialer{Timeout: timeout}

	var auth *xproxy.Auth
	if proxyConfig.Username != "" {
		auth = &xproxy.Auth{
			User:     proxyConfig.Username,
			Password: proxyConfig.Password,
		}
	}

	dialer, err := xproxy.SOCKS5("tcp", address, auth, baseDialer)
	if err != nil {
		return err
	}

	if contextDialer, ok := dialer.(xproxy.ContextDialer); ok {
		transport.DialContext = contextDialer.DialContext
		return nil
	}

	transport.DialContext = func(ctx context.Context, network string, addr string) (net.Conn, error) {
		type dialResult struct {
			conn net.Conn
			err  error
		}

		resultCh := make(chan dialResult, 1)
		go func() {
			conn, dialErr := dialer.Dial(network, addr)
			resultCh <- dialResult{conn: conn, err: dialErr}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-resultCh:
			return result.conn, result.err
		}
	}

	return nil
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
	} else {
		reader = nil
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
