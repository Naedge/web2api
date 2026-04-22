package service

import (
	"bytes"
	"crypto/sha3"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	proofScriptPattern = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
	proofBuildPattern  = regexp.MustCompile(`<html[^>]*data-build="([^"]*)"`)
	proofDPLPattern    = regexp.MustCompile(`c/[^/]*/_`)

	proofCache struct {
		mu      sync.Mutex
		scripts []string
		dpl     string
		updated time.Time
	}
)

var navigatorKeys = []string{
	"registerProtocolHandler−function registerProtocolHandler() { [native code] }",
	"storage−[object StorageManager]",
	"locks−[object LockManager]",
	"appCodeName−Mozilla",
	"permissions−[object Permissions]",
	"share−function share() { [native code] }",
	"webdriver−false",
	"managed−[object NavigatorManagedData]",
	"canShare−function canShare() { [native code] }",
	"vendor−Google Inc.",
	"mediaDevices−[object MediaDevices]",
	"vibrate−function vibrate() { [native code] }",
	"storageBuckets−[object StorageBucketManager]",
	"mediaCapabilities−[object MediaCapabilities]",
	"getGamepads−function getGamepads() { [native code] }",
	"bluetooth−[object Bluetooth]",
	"cookieEnabled−true",
	"product−Gecko",
	"pdfViewerEnabled−true",
	"language−zh-CN",
	"userAgentData−[object NavigatorUAData]",
	"hardwareConcurrency−32",
}

var documentKeys = []string{
	"_reactListeningo743lnnpvdg",
	"location",
}

var windowKeys = []string{
	"0",
	"window",
	"self",
	"document",
	"name",
	"location",
	"history",
	"navigation",
	"navigator",
	"origin",
	"screen",
	"innerWidth",
	"innerHeight",
	"scrollX",
	"scrollY",
	"screenX",
	"screenY",
	"outerWidth",
	"outerHeight",
	"devicePixelRatio",
	"performance",
	"crypto",
	"indexedDB",
	"sessionStorage",
	"localStorage",
	"chrome",
	"caches",
	"speechSynthesis",
}

func updateProofBuildInfoFromHTML(html string) {
	proofCache.mu.Lock()
	defer proofCache.mu.Unlock()

	scripts := []string{}
	for _, match := range proofScriptPattern.FindAllStringSubmatch(html, -1) {
		if len(match) != 2 {
			continue
		}
		src := strings.TrimSpace(match[1])
		if src == "" {
			continue
		}
		scripts = append(scripts, src)
		if scriptMatch := proofDPLPattern.FindString(src); scriptMatch != "" {
			proofCache.dpl = scriptMatch
		}
	}
	if len(scripts) == 0 {
		scripts = append(scripts, "https://chatgpt.com/backend-api/sentinel/sdk.js")
	}
	proofCache.scripts = scripts

	if proofCache.dpl == "" {
		if match := proofBuildPattern.FindStringSubmatch(html); len(match) == 2 {
			proofCache.dpl = strings.TrimSpace(match[1])
		}
	}
	proofCache.updated = time.Now()
}

func proofParseTime() string {
	now := time.Now().In(time.FixedZone("EST", -5*3600))
	return now.Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)"
}

func proofConfig(userAgent string) []any {
	proofCache.mu.Lock()
	scripts := append([]string(nil), proofCache.scripts...)
	dpl := proofCache.dpl
	proofCache.mu.Unlock()

	if len(scripts) == 0 {
		scripts = []string{"https://chatgpt.com/backend-api/sentinel/sdk.js"}
	}

	return []any{
		randomChoiceInt([]int{3000, 4000, 3120, 4160}),
		proofParseTime(),
		4294705152,
		0,
		userAgent,
		randomChoiceString(scripts),
		dpl,
		"en-US",
		"en-US,es-US,en,es",
		0,
		randomChoiceString(navigatorKeys),
		randomChoiceString(documentKeys),
		randomChoiceString(windowKeys),
		browserUptimeMillis(),
		newUUID(),
		"",
		randomChoiceInt([]int{8, 16, 24, 32}),
		float64(time.Now().UnixNano())/1_000_000 - browserUptimeMillis(),
	}
}

func getRequirementsToken(userAgent string) string {
	config := proofConfig(userAgent)
	require, _ := generateAnswer(formatRandomFloat(), "0fffff", config)
	return "gAAAAAC" + require
}

func generateProofToken(seed string, difficulty string, userAgent string) string {
	config := proofConfig(userAgent)
	answer, _ := generateAnswer(seed, difficulty, config)
	return "gAAAAAB" + answer
}

func generateAnswer(seed string, difficulty string, config []any) (string, bool) {
	diffLen := len(difficulty)
	targetDiff, err := hex.DecodeString(difficulty)
	if err != nil {
		return fallbackProof(seed), false
	}

	seedBytes := []byte(seed)
	part1 := []byte(jsonPrefix(config[:3]) + ",")
	part2 := []byte("," + jsonMiddle(config[4:9]) + ",")
	part3 := []byte("," + jsonSuffix(config[10:]))

	for i := 0; i < 500000; i++ {
		left := []byte(fmt.Sprintf("%d", i))
		right := []byte(fmt.Sprintf("%d", i>>1))
		payload := append(append(append(append([]byte{}, part1...), left...), part2...), right...)
		payload = append(payload, part3...)
		encoded := base64.StdEncoding.EncodeToString(payload)
		digest := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if bytes.Compare(digest[:diffLen], targetDiff) <= 0 {
			return encoded, true
		}
	}

	return fallbackProof(seed), false
}

func fallbackProof(seed string) string {
	return "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%q", seed)))
}

func jsonPrefix(values []any) string {
	data, _ := json.Marshal(values)
	return strings.TrimSuffix(string(data), "]")
}

func jsonMiddle(values []any) string {
	data, _ := json.Marshal(values)
	return strings.TrimSuffix(strings.TrimPrefix(string(data), "["), "]")
}

func jsonSuffix(values []any) string {
	data, _ := json.Marshal(values)
	return strings.TrimPrefix(string(data), "[")
}

func bytesLEQ(left []byte, right []byte) bool {
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] < right[i] {
			return true
		}
		if left[i] > right[i] {
			return false
		}
	}
	return len(left) <= len(right)
}

func formatRandomFloat() string {
	return strconv.FormatFloat(rand.Float64(), 'g', -1, 64)
}

func randomChoiceInt(values []int) int {
	return values[rand.Intn(len(values))]
}

func randomChoiceString(values []string) string {
	return values[rand.Intn(len(values))]
}
