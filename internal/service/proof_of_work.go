package service

import (
	"crypto/sha3"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	scriptSrcPattern = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
	dataBuildPattern = regexp.MustCompile(`data-build="([^"]+)"`)

	powCache struct {
		mu      sync.Mutex
		scripts []string
		dpl     string
	}
)

var navigatorKeys = []string{
	"appCodeName",
	"appName",
	"appVersion",
	"hardwareConcurrency",
	"language",
	"languages",
	"platform",
	"userAgent",
	"vendor",
}

var documentKeys = []string{
	"characterSet",
	"compatMode",
	"contentType",
	"dir",
	"fullscreenEnabled",
	"hidden",
	"visibilityState",
}

var windowKeys = []string{
	"innerHeight",
	"innerWidth",
	"outerHeight",
	"outerWidth",
	"devicePixelRatio",
	"origin",
	"screenX",
	"screenY",
}

func updateProofBuildInfoFromHTML(html string) {
	powCache.mu.Lock()
	defer powCache.mu.Unlock()

	if match := dataBuildPattern.FindStringSubmatch(html); len(match) == 2 {
		powCache.dpl = match[1]
	}

	scripts := []string{}
	for _, match := range scriptSrcPattern.FindAllStringSubmatch(html, -1) {
		if len(match) != 2 {
			continue
		}
		src := strings.TrimSpace(match[1])
		if src == "" {
			continue
		}
		scripts = append(scripts, src)
	}
	if len(scripts) > 0 {
		powCache.scripts = scripts
	}
}

func getRequirementsToken(userAgent string) string {
	payload := []any{
		rand.Float64(),
		time.Now().UnixMilli(),
		4294705152,
		0,
		userAgent,
	}
	data, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(data)
}

func generateProofToken(seed string, difficulty string, userAgent string) string {
	config := proofConfig(userAgent)
	configJSON, _ := json.Marshal(config)

	for answer := 0; answer < 500000; answer++ {
		payload := fmt.Sprintf("%s|%s|%d", seed, string(configJSON), answer)
		sum := sha3.Sum512([]byte(payload))
		hexHash := hex.EncodeToString(sum[:])
		if strings.Compare(hexHash[:len(difficulty)], difficulty) <= 0 {
			result := []any{
				"g",
				time.Now().UnixMilli(),
				answer,
				config,
			}
			data, _ := json.Marshal(result)
			return "gAAAAAB" + base64.StdEncoding.EncodeToString(data)
		}
	}

	return "gAAAAAB" + getRequirementsToken(userAgent)
}

func proofConfig(userAgent string) []any {
	powCache.mu.Lock()
	scripts := append([]string(nil), powCache.scripts...)
	dpl := powCache.dpl
	powCache.mu.Unlock()

	if len(scripts) == 0 {
		scripts = []string{
			"/_next/static/chunks/main-app.js",
			"/_next/static/chunks/webpack.js",
		}
	}

	if dpl == "" {
		dpl = "prod"
	}

	randomItem := func(items []string) string {
		return items[rand.Intn(len(items))]
	}

	est := time.FixedZone("EST", -5*3600)
	now := time.Now().In(est)

	return []any{
		3,
		fmt.Sprintf("%d", now.Unix()),
		3640,
		0,
		userAgent,
		randomItem(scripts),
		dpl,
		"en-US",
		"en-US,zh-CN,zh",
		0,
		randomItem(navigatorKeys),
		randomItem(documentKeys),
		randomItem(windowKeys),
		now.Format(time.RFC1123Z),
	}
}
