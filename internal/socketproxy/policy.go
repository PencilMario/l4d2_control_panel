package socketproxy

import (
	"net/http"
	"regexp"
	"strings"
)

var versionPrefix = regexp.MustCompile(`^/v[0-9]+\.[0-9]+`)
var containerItem = regexp.MustCompile(`^/containers/[^/]+/(json|stats|logs)$`)
var containerAction = regexp.MustCompile(`^/containers/[^/]+/(start|stop|wait|exec)$`)
var execAction = regexp.MustCompile(`^/exec/[^/]+/(start|resize)$`)
var containerDelete = regexp.MustCompile(`^/containers/[^/]+$`)

func Allowed(method, path string) bool {
	path = versionPrefix.ReplaceAllString(path, "")
	if path == "/_ping" || path == "/version" {
		return method == http.MethodGet || method == http.MethodHead
	}
	switch method {
	case http.MethodGet:
		return path == "/info" || path == "/containers/json" || path == "/images/json" || containerItem.MatchString(path) || strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json")
	case http.MethodPost:
		return path == "/containers/create" || path == "/images/create" || containerAction.MatchString(path) || execAction.MatchString(path)
	case http.MethodDelete:
		return containerDelete.MatchString(path)
	default:
		return false
	}
}
