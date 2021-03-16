// Copyright 2021, 昌徽（上海）信息技术有限公司.  All rights reserved.
// https://www.vxway.cn
//
// Author: Tony (tony@isysful.com)

// Package info
// 将编译时源码的git版本信息（当前tag，commit log的sha值和commit message，是否有未提交的修改），编译时间，Go版本，编译、运行平台打入程序中

package info

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	// 程序版本
	Version = "1.0.1"

	// 初始化为 unknown，如果编译时没有传入这些值，则为 unknown
	GitTag         = "unknown"
	GitCommitLog   = "unknown"
	GitStatus      = "unknown"
	BuildTime      = "unknown"
	BuildGoVersion = "unknown"
	// 程序运行时的平台
	Runtime = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)

func PrintVersion() {
	// fmt.Printf("CPU: %d",runtime.NumCPU())
	fmt.Println(StringifySingleLine())
}

// 返回单行格式
func StringifySingleLine() string {
	return fmt.Sprintf("VxWay Proxy %s. \nGitTag=%s. GitCommitLog=%s. GitStatus=%s. BuildTime=%s. GoVersion=%s. Runtime=%s.",
		Version, GitTag, GitCommitLog, GitStatus, BuildTime, BuildGoVersion, Runtime)
}

// 返回多行格式
func StringifyMultiLine() string {
	return fmt.Sprintf("VxWay Proxy %s. \nGitTag=%s\nGitCommitLog=%s\nGitStatus=%s\nBuildTime=%s\nGoVersion=%s\nRuntime=%s\n",
		Version, GitTag, GitCommitLog, GitStatus, BuildTime, BuildGoVersion, Runtime)
}

// 对一些值做美化处理
func beauty() {
	if GitStatus == "" {
		// GitStatus 为空时，说明本地源码与最近的 commit 记录一致，无修改
		// 为它赋一个特殊值
		GitStatus = "cleanly"
	} else {
		// 将多行结果合并为一行
		GitStatus = strings.Replace(strings.Replace(GitStatus, "\r\n", " |", -1), "\n", " |", -1)
	}
}

func init() {
	beauty()
}
