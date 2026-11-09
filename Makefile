.PHONY: help install dev dev-api build check test test-go test-e2e preview fmt

help:
	@echo "install   安装 pnpm 和 Go 依赖"
	@echo "dev       启动 API、console、screen（Ctrl+C 停止全部）"
	@echo "dev-api   单独启动 API"
	@echo "build     构建 Go 服务及两个应用"
	@echo "check     格式、Go vet、ESLint、TypeScript、接口类型检查"
	@echo "test      Go 测试与浏览器端到端测试"
	@echo "preview   预览已构建应用并启动 API"

install:
	pnpm install --frozen-lockfile
	node scripts/go.mjs download

dev:
	node scripts/dev.mjs

dev-api:
	node scripts/dev.mjs --api-only

build:
	pnpm build

check:
	pnpm check

test:
	pnpm test

test-go:
	node scripts/go.mjs test

test-e2e:
	pnpm test:e2e

preview:
	pnpm preview

fmt:
	node scripts/go.mjs fmt
