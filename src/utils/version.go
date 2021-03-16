package utils

import (
	"fmt"
)

// 通过 Shell 脚本来设置 编译时的数据
var (
	// 获取源码最近一次commit时间
	GitCommit = "2021-03-13 12:23:49"
	// 程序的编译时间
	BuildTime = "2021-03-14 01:52:38"
	// 编译使用的 Go 版本
	GoVersion = "1.16"
	// 程序版本
	Version   = "1.0.1"
	// 程序运行时的平台
	Runtime   = "darwin/amd64"
)

// PrintVersion Print out version information
func PrintVersion() {
	fmt.Println("VX Way Version  : ", Version)
	fmt.Println("GitCommit: ", GitCommit)
	fmt.Println("BuildTime: ", BuildTime)
	fmt.Println("GoVersion: ", GoVersion)
	fmt.Println("Runtime  :", Runtime)
}
