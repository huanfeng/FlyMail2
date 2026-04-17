# FlyMail Makefile

.PHONY: build clean run init test help

# 默认目标
help:
	@echo "FlyMail 项目构建工具"
	@echo ""
	@echo "可用目标:"
	@echo "  build    - 构建项目"
	@echo "  clean    - 清理构建文件"
	@echo "  run      - 运行服务器"
	@echo "  init     - 初始化数据库"
	@echo "  migrate  - 应用数据库迁移"
	@echo "  test     - 运行测试"
	@echo "  deps     - 安装依赖"
	@echo "  docs     - 显示文档访问信息"
	@echo "  help     - 显示此帮助信息"

# 构建项目
build:
	@echo "构建 FlyMail..."
	@mkdir -p bin
	go build -o bin/flymail cmd/flymail/main.go
	@echo "构建完成: bin/flymail"

# 清理构建文件
clean:
	@echo "清理构建文件..."
	rm -rf bin/
	rm -f data/flymail.db
	@echo "清理完成"

# 运行服务器
run: build
	@echo "启动 FlyMail 服务器..."
	./bin/flymail server

# 初始化数据库
init: build
	@echo "初始化数据库..."
	./bin/flymail db init

# 应用数据库迁移
migrate:
	@echo "应用数据库迁移..."
	./scripts/apply_migrations.sh

# 重置管理员密码
reset-password: build
	@echo "重置管理员密码..."
	./bin/flymail db reset-admin-password

# 安装依赖
deps:
	@echo "安装 Go 依赖..."
	go mod tidy
	go mod download

# 运行测试
test:
	@echo "运行测试..."
	go test ./...

# 生成开发环境
dev: init
	@echo "启动开发环境..."
	@echo "数据库已初始化"
	@echo "默认管理员账户: admin / admin123"
	@echo "服务器地址: http://localhost:8080"
	@echo "API文档: http://localhost:8080/api/v1"
	@echo ""
	@echo "示例客户端: examples/client.html"
	@echo ""
	./bin/flymail server

# 格式化代码
fmt:
	@echo "格式化代码..."
	go fmt ./...

# 代码检查
lint:
	@echo "运行代码检查..."
	golangci-lint run

# 显示文档信息
docs:
	@echo "FlyMail API 文档"
	@echo ""
	@echo "启动服务器后，访问以下地址查看文档："
	@echo "  Swagger UI: http://localhost:8080/api/swagger"
	@echo "  OpenAPI 规范: http://localhost:8080/api/v1/openapi.yaml"
	@echo ""
	@echo "使用 'make run' 启动服务器"

# 构建 Docker 镜像
docker-build:
	@echo "构建 Docker 镜像..."
	docker build -t flymail:latest .

# 运行 Docker 容器
docker-run:
	@echo "运行 Docker 容器..."
	docker run -p 8080:8080 -v $(PWD)/data:/app/data flymail:latest

# 发布版本
release: clean build
	@echo "创建发布版本..."
	@mkdir -p release
	cp bin/flymail release/
	cp -r data release/
	cp README.md release/
	cp -r docs release/
	cp -r examples release/
	cd release && tar -czf flymail-$(shell date +%Y%m%d).tar.gz *
	@echo "发布包已创建: release/flymail-$(shell date +%Y%m%d).tar.gz"
