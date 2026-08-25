# stage-rig-clearance

`stage-rig-clearance` 是面向剧场技术团队的舞台吊挂装置安全放行工作台。它把吊挂方案、吊点与设备登记、载荷试验、阻断风险、整改复测、安全终审、冻结和放行凭据收束为一条可追溯的同源浏览器流程。

项目只使用 Go 标准库。服务直接提供 `internal/httpapi/web/index.html`、`app.css` 和 `app.js`，不需要 Node 或前端构建链。

## 构建

```text
go build ./cmd/server
```

## 运行

默认只监听高位回环地址 `127.0.0.1:19091`：

```text
go run ./cmd/server
```

可显式指定其他回环端口和数据目录：

```text
go run ./cmd/server -addr=127.0.0.1:19092 -data=./data
```

未传 `-addr` 时，也可通过 `PORT` 提供端口号，服务会绑定 `127.0.0.1:<PORT>`。显式 `-addr` 优先于 `PORT`。服务拒绝 `0.0.0.0` 等非回环监听地址。

启动后访问 `http://127.0.0.1:19091/`。关闭服务时会在限定时间内优雅退出。

## 业务流程

1. 创建包含场地、演出日期、额定总载荷和负责人的方案。
2. 在冻结前按版本修订方案基础信息；日期和总载荷变化会保留既有事实并立即重算风险。
3. 登记或版本化修订吊点额定载荷、计划载荷、设备、钢索、证书与独立冗余关系；旧配置试验仅作为历史保留，无引用和历史的吊点可受控移除。
4. 逐吊点或按最多 50 行的原子批次记录初次载荷试验；系统按载荷、冗余、证书、当前配置摘要和试验结果生成阻断风险。
5. 对风险提交带前后差异的整改修订并获得唯一复测任务，再用绑定吊点、配置摘要和最低条件的针对性复测精准关单。
6. 安全负责人批准后冻结吊点与试验清单；冻结后所有配置写入都会被拒绝。
7. 签发递增序号的不可变放行凭据；工作台无需先选择方案即可用完整凭据标识和摘要核验冻结清单、凭据与审计链。

所有写请求都要求 `expectedVersion` 和 `idempotencyKey`。版本不一致返回 `409 version_conflict`；同一幂等键重试不会重复追加业务事件。

公开主链路新增 `PATCH /api/plans/{planID}`、`PATCH|DELETE /api/plans/{planID}/points/{pointID}`、`POST /api/plans/{planID}/tests/batch` 和 `GET /api/credentials/{credentialID}/verify?digest=<完整摘要>`。请求继续严格拒绝未知 JSON 字段，批量校验错误会带 `tests[n].field` 行号字段。

## 持久化与完整性

默认数据位于 `./data`：

- `domain/events.log` 是带 4 字节长度前缀、`schemaVersion`、连续序号和 SHA-256 校验和的只追加事件日志。
- `domain/projection.json` 是通过临时文件同步和原子替换维护的带校验和投影。
- `audit/audit.chain` 使用前序摘要和全局连续序号形成审计链。
- `audit/credentials.log` 只追加保存放行凭据及其递增序号。

启动会完整扫描事件与审计帧，校验帧长度、序号、摘要、聚合版本和投影不变量；发现截断、乱序或内容损坏时拒绝启动，不会静默跳过。

## 测试与自检

运行全部回归测试：

```text
go test ./...
```

运行可自行结束的真实 HTTP 冒烟链路：

```text
go run ./cmd/server -selfcheck -addr=127.0.0.1:19091
```

`-selfcheck` 使用临时数据目录，实际启动监听器并经公开 HTTP API 完成创建、双吊点登记、初试、风险整改、复测、批准冻结、凭据签发、凭据核验和审计验链，随后优雅关闭服务。
