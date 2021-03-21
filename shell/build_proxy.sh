#!/usr/bin/env bash

# set -x
echo 'Start compiling "vx-proxy".'

# 获取git tag版本号
GitTag=`git tag --sort=version:refname | tail -n 1`
# 获取源码最近一次git commit log，包含commit sha 值，以及commit message
GitCommitLog=`git log --pretty=oneline -n 1`
# 将log原始字符串中的单引号替换成双引号
GitCommitLog=${GitCommitLog//\'/\"}
# 检查源码在git commit基础上，是否有本地修改，且未提交的内容
GitStatus=`git status -s`
# 获取当前时间
BuildTime=`date +'%Y.%m.%d.%H%M%S'`
# 获取Go的版本
BuildGoVersion=`go version`

# 将以上变量序列化至 LDFlags 变量中
LDFlags=" \
    -X 'vxway/apps/vx-proxy/info/info.GitTag=${GitTag}' \
    -X 'vxway/apps/vx-proxy/info/info.GitCommitLog=${GitCommitLog}' \
    -X 'vxway/apps/vx-proxy/info/info.GitStatus=${GitStatus}' \
    -X 'vxway/apps/vx-proxy/info/info.BuildTime=${BuildTime}' \
    -X 'vxway/apps/vx-proxy/info/info.BuildGoVersion=${BuildGoVersion}' \
"

ROOT_DIR=`pwd`

# 如果可执行程序输出目录不存在，则创建
if [ ! -d ${ROOT_DIR}/bin ]; then
  mkdir bin
fi

# 编译多个可执行程序
echo  ${ROOT_DIR}
#if [ -d ${ROOT_DIR}/demo/add_blog_license ]; then
#  cd ${ROOT_DIR}/demo/add_blog_license && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/add_blog_license
#fi

#if [ -d ${ROOT_DIR}/demo/add_go_license ]; then
#  cd ${ROOT_DIR}/demo/add_go_license && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/add_go_license
#fi

#if [ -d ${ROOT_DIR}/demo/myapp ]; then
#  cd ${ROOT_DIR}/demo/myapp && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/myapp
#fi

#if [ -d ${ROOT_DIR}/demo/slicebytepool ]; then
#  cd ${ROOT_DIR}/demo/slicebytepool && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/slicebytepool
#fi

#if [ -d ${ROOT_DIR}/demo/taskpool ]; then
#  cd ${ROOT_DIR}/demo/taskpool && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/taskpool
#fi

if [ -d ${ROOT_DIR}/../apps/vx-proxy ]; then
  cd ${ROOT_DIR}/../apps/vx-proxy && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/vx-proxy
fi

if [ -d ${ROOT_DIR}/../apps/vx-admin ]; then
  cd ${ROOT_DIR}/../apps/vx-admin && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/vx-admin
fi

if [ -d ${ROOT_DIR}/../apps/vx-service ]; then
  cd ${ROOT_DIR}/../apps/vx-service && go build -ldflags "$LDFlags" -o ${ROOT_DIR}/bin/vx-service
fi

ls -lrt ${ROOT_DIR}/bin &&

echo
cd ${ROOT_DIR} && ./bin/vx-proxy -version &&
cd ${ROOT_DIR} && ./bin/vx-admin -version &&
cd ${ROOT_DIR} && ./bin/vx-service -version &&

echo 'build done.'
