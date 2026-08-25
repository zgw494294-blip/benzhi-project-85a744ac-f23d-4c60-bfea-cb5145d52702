# BENZHI_README

基于 Go 实现的stage-rig-clearance Web 项目，一款后端服务，已完整实现面向剧场技术团队的舞台吊挂安全放行工作台，覆盖配置登记、载荷试验、风险整改复测、终审冻结、不可变凭据签发及审计核验。

## 项目说明
- 项目：benzhi-project-85a744ac-f23d-4c60-bfea-cb5145d52702
- 项目用途：已完整实现面向剧场技术团队的舞台吊挂安全放行工作台，覆盖配置登记、载荷试验、风险整改复测、终审冻结、不可变凭据签发及审计核验。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19091
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-85a744ac-f23d-4c60-bfea-cb5145d52702-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-85a744ac-f23d-4c60-bfea-cb5145d52702-arm64 linux/arm64
docker run -it benzhi-project-85a744ac-f23d-4c60-bfea-cb5145d52702-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19091`
