# yomi-boot Makefile
# 请在 yomi-boot 根目录执行 make 命令

GO               ?= go
GOLANGCI_VERSION ?= v2.1.6
GOLANGCI         := $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

.DEFAULT_GOAL := check

# check
# 作用: 本地提交前门禁
# 用法: make check
.PHONY: check
check: fmt vet lint

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
