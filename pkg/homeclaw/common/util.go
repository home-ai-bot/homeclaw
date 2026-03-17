// Package common provides common utility functions.
package common

import (
	"fmt"
	"time"
)

// GenerateUUID 生成简单的UUID（基于当前时间纳秒）
func GenerateUUID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
