package vpkpolicy

import (
	"path"
	"strings"
)

func ShouldRemove(rel string) bool {
	base, ext := path.Base(rel), strings.ToLower(path.Ext(rel))
	return base == "" || ext == "" || ext == ".vtf" || ext == ".mp3" || ext == ".wav" || ext == ".vmf" || ext == ".vmx"
}
