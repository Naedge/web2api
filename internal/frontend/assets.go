package frontend

import (
	"embed"
	"errors"
	"io/fs"
	"path"
	"strings"
)

//go:embed all:dist
var files embed.FS

func ReadAsset(requestPath string) ([]byte, string, error) {
	assetPath, err := ResolveAssetPath(requestPath)
	if err != nil {
		return nil, "", err
	}
	data, err := files.ReadFile("dist/" + assetPath)
	if err != nil {
		return nil, "", err
	}
	return data, assetPath, nil
}

func ResolveAssetPath(requestPath string) (string, error) {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		return "", err
	}

	cleanPath := strings.Trim(strings.TrimSpace(requestPath), "/")
	candidates := []string{}
	if cleanPath == "" {
		candidates = []string{"index.html"}
	} else {
		cleanPath = path.Clean(cleanPath)
		if cleanPath == "." {
			cleanPath = ""
		}
		if cleanPath == "" {
			candidates = []string{"index.html"}
		} else {
			candidates = []string{
				cleanPath,
				path.Join(cleanPath, "index.html"),
				cleanPath + ".html",
			}
		}
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, statErr := fs.Stat(sub, candidate)
		if statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	if strings.HasPrefix(cleanPath, "_next/") {
		return "", fs.ErrNotExist
	}

	info, statErr := fs.Stat(sub, "index.html")
	if statErr == nil && !info.IsDir() {
		return "index.html", nil
	}

	return "", errors.New("asset not found")
}
