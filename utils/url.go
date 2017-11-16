package utils

import (
	"net/url"
	"path"
)

func AddURLPath(urlStr string, paths ...string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	allPaths := []string{u.Path}
	allPaths = append(allPaths, paths...)
	p, err := url.Parse(path.Join(allPaths...))
	if err != nil {
		return "", err
	}
	return u.ResolveReference(p).String(), nil
}
