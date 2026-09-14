// Package cache 实现配置的本地缓存，配置中心不可用时由调用方回退读取。
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/wueasy/waymark-sdk-golang/internal/model"
)

// DefaultSubDir 系统用户缓存目录下的默认子目录名。
const DefaultSubDir = "waymark"

// ResolveDir 解析本地缓存目录；禁用缓存时返回空字符串表示不启用。
func ResolveDir(disable bool, dir string) string {
	if disable {
		return ""
	}
	if dir = strings.TrimSpace(dir); dir != "" {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, DefaultSubDir)
}

// IsUnreachable 判断错误是否为网络层不可达（连接失败、超时等），
// 此类错误说明无法访问配置中心，可回退到本地缓存；服务端返回的业务错误不在其列。
func IsUnreachable(err error) bool {
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// NormalizeKey 去除空白，为空时返回默认值。
func NormalizeKey(value, fallback string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return fallback
}

// Path 返回指定配置的缓存文件路径；缓存未启用时返回空字符串。
// namespace、group 为空时按服务端默认值归一，保证写入与读取使用一致的缓存键。
func Path(dir, namespace, group, dataId string) string {
	if dir == "" {
		return ""
	}
	namespace = NormalizeKey(namespace, model.DefaultNamespace)
	group = NormalizeKey(group, model.DefaultGroup)
	name := escapeSegment(namespace) + "_" + escapeSegment(group) + "_" + escapeSegment(strings.TrimSpace(dataId)) + ".json"
	return filepath.Join(dir, "config", name)
}

// Write 将配置写入本地缓存。缓存为尽力而为，失败时静默忽略，不影响正常读取。
func Write(dir string, item *model.ConfigItem) {
	path := Path(dir, item.Namespace, item.GroupName, item.DataId)
	if path == "" {
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	// 先写临时文件再原子重命名，避免读取到写了一半的内容。
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
	}
}

// Read 读取本地缓存的配置；缓存未启用、不存在或已损坏时返回错误。
func Read(dir, namespace, group, dataId string) (*model.ConfigItem, error) {
	path := Path(dir, namespace, group, dataId)
	if path == "" {
		return nil, errors.New("waymark: 本地缓存未启用")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("waymark: 读取本地配置缓存失败: %w", err)
	}
	var item model.ConfigItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("waymark: 解析本地配置缓存失败: %w", err)
	}
	return &item, nil
}

// escapeSegment 将配置定位片段转义为文件系统安全且不产生歧义的名称。
func escapeSegment(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '-':
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}
