package service

import (
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func newUUID() string {
	buf := make([]byte, 16)
	if _, err := crand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		buf[:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	)
}

func newHexID(length int) string {
	size := length/2 + 1
	buf := make([]byte, size)
	if _, err := crand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)[:length]
}

func encodeJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeJWTBody(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}

	result := map[string]any{}
	if err := json.Unmarshal(body, &result); err != nil {
		return map[string]any{}
	}

	return result
}

func normalizeStringList(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := unique[value]; ok {
			continue
		}
		unique[value] = struct{}{}
		items = append(items, value)
	}
	return items
}

func formatTimePointer(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02 15:04:05")
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func AsString(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case json.Number:
		return current.String()
	case float64:
		return fmt.Sprintf("%.0f", current)
	default:
		return ""
	}
}

func ParseImageCount(rawValue any, defaultValue int) (int, error) {
	if rawValue == nil {
		return defaultValue, nil
	}

	switch current := rawValue.(type) {
	case json.Number:
		number, err := current.Int64()
		if err != nil {
			return 0, badRequest("n must be an integer")
		}
		if number < 1 || number > 4 {
			return 0, badRequest("n must be between 1 and 4")
		}
		return int(number), nil
	case float64:
		if current != float64(int(current)) {
			return 0, badRequest("n must be an integer")
		}
		if current < 1 || current > 4 {
			return 0, badRequest("n must be between 1 and 4")
		}
		return int(current), nil
	case int:
		if current < 1 || current > 4 {
			return 0, badRequest("n must be between 1 and 4")
		}
		return current, nil
	case int64:
		if current < 1 || current > 4 {
			return 0, badRequest("n must be between 1 and 4")
		}
		return int(current), nil
	case string:
		current = strings.TrimSpace(current)
		if current == "" {
			return defaultValue, nil
		}
		number, err := json.Number(current).Int64()
		if err != nil {
			return 0, badRequest("n must be an integer")
		}
		if number < 1 || number > 4 {
			return 0, badRequest("n must be between 1 and 4")
		}
		return int(number), nil
	default:
		return 0, badRequest("n must be an integer")
	}
}

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, has := ctx.Deadline(); has {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
