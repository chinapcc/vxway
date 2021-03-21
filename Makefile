BASEDIR = $(shell pwd)

test = $(shell if [ ! -d ${BASEDIR}/bin ]; then mkdir ${BASEDIR}/bin fi)

# build with verison infos
versionDir = ./src/utils/version
gitTag = $(shell if [ "`git describe --tags --abbrev=0 2>/dev/null`" != "" ];then git describe --tags --abbrev=0; else git log --pretty=format:'%h' -n 1; fi)
#buildDate = $(shell TZ=Asia/Shanghai date +"%Y-%m-%d %H:%M:%S")
buildDate = $(shell TZ=UTC date +%FT%T%z)
gitCommit = $(shell git log --pretty=format:'%H' -n 1)
gitTreeState = $(shell if git status|grep -q 'clean';then echo clean; else echo dirty; fi)

xxx="-w -X ${versionDir}.gitTag=${gitTag} -X ${versionDir}.buildDate=${buildDate} -X ${versionDir}.gitCommit=${gitCommit} -X ${versionDir}.gitTreeState=${gitTreeState}"

all: gotool build
build:
	go build -ldflags ${xxx} -o ./bin/ ./apps/vx-proxy/
run:
	go run -ldflags ${ldflags} ./apps/vx-proxy/
clean:
	rm -f web
	find . -name "[._]*.s[a-w][a-z]" | xargs -i rm -f {}
gotool:
	go fmt ./
	go vet ./
ca:
	MSYS_NO_PATHCONV=1 openssl req -new -nodes -x509 -out conf/server.crt -keyout conf/server.key -days 3650 -subj "/C=CN/ST=SH/L=SH/O=iSysful/OU=iSysful Software/CN=127.0.0.1/emailAddress=tony@isysful.com"

help:
	@echo "make - 格式化 Go 代码, 并编译生成二进制文件"
	@echo "make build - 编译 Go 代码, 生成二进制文件"
	@echo "make run - 直接运行 Go 代码"
	@echo "make clean - 移除二进制文件和 vim swap files"
	@echo "make gotool - 运行 Go 工具 'fmt' and 'vet'"
	@echo "make ca - 生成证书文件"

.PHONY: run clean gotool ca help
