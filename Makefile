# yomi-boot Makefile
# 请在仓库根目录执行 make 命令

GO               ?= go
GOLANGCI_VERSION ?= v2.1.6
GOLANGCI         := $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

.DEFAULT_GOAL := check

# check
# 作用: 本地提交前门禁
# 用法: make check
.PHONY: check
check: fmt vet lint layers

# deps
# 作用: 下载模块依赖
# 用法: make deps
.PHONY: deps
deps:
	$(GO) mod download

# tidy
# 作用: 整理 go.mod / go.sum
# 用法: make tidy
.PHONY: tidy
tidy:
	$(GO) mod tidy

# fmt
# 作用: go fmt + golangci-lint fmt（含 gofumpt）
# 用法: make fmt
.PHONY: fmt
fmt:
	$(GO) fmt ./...
	$(GOLANGCI) fmt ./...

# vet
# 作用: go vet
# 用法: make vet
.PHONY: vet
vet:
	$(GO) vet ./...

# lint
# 作用: golangci-lint（读取 .golangci.yml）
# 用法: make lint
.PHONY: lint
lint:
	$(GOLANGCI) run --config .golangci.yml ./...

# test
# 作用: 执行单元测试
# 用法: make test
.PHONY: test
test:
	$(GO) test ./...

# layers
# 作用: 校验分层依赖方向（core 无状态；component 三分组；仅 bootstrap/plugin 可依赖 cloud）
# 用法: make layers
.PHONY: layers
layers:
	@$(GO) list -f '{{.ImportPath}} {{join .Imports " "}}' ./core/... | \
	  awk '{ \
	         for (i = 2; i <= NF; i++) \
	           if ($$i ~ /yomi-boot\/(boot|component)\//) { \
	             print "core 禁止依赖 boot/component: " $$1 " -> " $$i; bad = 1 \
	           } \
	       } \
	       END { exit bad }'
	@$(GO) list -f '{{.ImportPath}} {{join .Imports " "}}' ./component/common/... | \
	  awk '{ \
	         for (i = 2; i <= NF; i++) { \
	           if ($$i ~ /yomi-boot\/component\/(cloud|ai)\//) { \
	             print "component/common 禁止依赖 component/cloud|ai: " $$1 " -> " $$i; bad = 1 \
	           } \
	           if ($$i ~ /yomi-boot\/boot\// && ($$1 !~ /\/plugin$$/ || $$i != "gitee.com/hq_project/yomi-boot/boot/plugin")) { \
	             print "component/common 仅 */plugin 可依赖 boot/plugin: " $$1 " -> " $$i; bad = 1 \
	           } \
	         } \
	       } \
	       END { exit bad }'
	@$(GO) list -f '{{.ImportPath}} {{join .Imports " "}}' ./component/cloud/... | \
	  awk '{ \
	         for (i = 2; i <= NF; i++) { \
	           if ($$i ~ /yomi-boot\/component\/ai\//) { \
	             print "component/cloud 禁止依赖 component/ai: " $$1 " -> " $$i; bad = 1 \
	           } \
	           if ($$i ~ /yomi-boot\/boot\// && ($$1 !~ /\/plugin$$/ || $$i != "gitee.com/hq_project/yomi-boot/boot/plugin")) { \
	             print "component/cloud 仅 */plugin 可依赖 boot/plugin: " $$1 " -> " $$i; bad = 1 \
	           } \
	         } \
	       } \
	       END { exit bad }'
	@$(GO) list -f '{{.ImportPath}} {{join .Imports " "}}' ./component/ai/... | \
	  awk '{ \
	         for (i = 2; i <= NF; i++) { \
	           if ($$i ~ /yomi-boot\/component\/cloud\//) { \
	             print "component/ai 禁止依赖 component/cloud: " $$1 " -> " $$i; bad = 1 \
	           } \
	           if ($$i ~ /yomi-boot\/boot\//) { \
	             print "component/ai 禁止依赖 boot: " $$1 " -> " $$i; bad = 1 \
	           } \
	         } \
	       } \
	       END { exit bad }'
	@$(GO) list -f '{{.ImportPath}} {{join .Imports " "}}' ./boot/... | \
	  awk '{ \
	         for (i = 2; i <= NF; i++) { \
	           if ($$i ~ /yomi-boot\/component\/ai\//) { \
	             print "boot 禁止依赖 component/ai: " $$1 " -> " $$i; bad = 1 \
	           } \
	           if ($$i ~ /yomi-boot\/component\/cloud\// && $$1 != "gitee.com/hq_project/yomi-boot/boot/bootstrap" && $$1 != "gitee.com/hq_project/yomi-boot/boot/plugin") { \
	             print "仅 bootstrap/plugin 允许依赖 component/cloud: " $$1 " -> " $$i; bad = 1 \
	           } \
	         } \
	       } \
	       END { exit bad }'
