# 宠环模拟器 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个可在 `ring.ddctl.com` 自托管的宠环记账、积分预测、收益估算与匿名概率统计网页。

**Architecture:** 一个 Go 进程嵌入并提供前端静态资源，同时暴露匿名任务、奖励和聚合模型 API。个人数据保存在浏览器，公共匿名事件保存在独立 SQLite；生产环境用独立 Docker 网络连接现有 Caddy。

**Tech Stack:** Go 1.24、`net/http`、`modernc.org/sqlite`、原生 HTML/CSS/JavaScript、Node 内置测试运行器、Docker、Caddy。

## Global Constraints

- 不登录，不上传个人历史、本地成本、本地物价、角色名或账号。
- 首次访问必须由用户主动选择是否参与匿名统计。
- 无公共样本时必须使用本周期平均值回退并显示低可信度。
- 不使用现有 Supabase、Kong、PostgreSQL 或业务数据卷。
- 不在本任务中修改或连接生产服务器。
- 未经用户明确要求不创建 Git commit。

---

### Task 1: 积分和奖励领域规则

**Files:**
- Create: `go.mod`
- Create: `internal/domain/rules.go`
- Test: `internal/domain/rules_test.go`

**Interfaces:**
- Produces: `domain.TaskTypes() []TaskRule`
- Produces: `domain.ScoreTask(taskType string, requiredQuality, actualQuality *int) (int, error)`
- Produces: `domain.RewardTier(playerLevel, finalScore int) int`

- [ ] **Step 1: 写积分表、品质扣分和奖励档位的失败测试**

```go
func TestScoreTaskQualityShortfall(t *testing.T) {
    required, actual := 63, 58
    got, err := ScoreTask("medicine", &required, &actual)
    if err != nil || got != 0 { t.Fatalf("got %d, %v", got, err) }
}

func TestRewardTierForLevel175(t *testing.T) {
    if got := RewardTier(175, 202); got != 130 { t.Fatalf("got %d", got) }
}
```

- [ ] **Step 2: 运行测试并确认因符号不存在而失败**

Run: `go test ./internal/domain -run 'TestScoreTask|TestRewardTier'`
Expected: FAIL，提示 `ScoreTask` 或 `RewardTier` 未定义。

- [ ] **Step 3: 实现规则表和纯函数**

规则必须覆盖 PRD 第 7、8 节；人物等级取表中不高于输入等级的最近一档，积分不足 90 档时返回 0。

- [ ] **Step 4: 运行领域测试**

Run: `go test ./internal/domain`
Expected: PASS。

### Task 2: 预测核心

**Files:**
- Create: `internal/domain/prediction.go`
- Test: `internal/domain/prediction_test.go`

**Interfaces:**
- Consumes: `domain.RewardTier`
- Produces: `domain.FallbackProjection(currentRing, score int, cost float64) Projection`
- Produces: `domain.SimulateProjection(input SimulationInput, seed int64) Projection`

- [ ] **Step 1: 写平均值回退和固定种子模拟的失败测试**

```go
func TestFallbackProjection(t *testing.T) {
    got := FallbackProjection(75, 137, 712.8)
    if got.ExpectedScore != 183 || math.Abs(got.ExpectedCost-950.4) > 0.01 { t.Fatalf("%+v", got) }
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./internal/domain -run 'TestFallbackProjection|TestSimulateProjection'`
Expected: FAIL，预测接口未定义。

- [ ] **Step 3: 实现平均值回退与可复现蒙特卡洛模拟**

模拟按未来环号所在十环区间抽取任务积分，输出期望值、P10、P50、P90 和各奖励档位达成概率；无分布时回退到当前平均值。

- [ ] **Step 4: 运行领域测试**

Run: `go test ./internal/domain`
Expected: PASS。

### Task 3: SQLite 匿名事件存储

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/sqlite.go`
- Test: `internal/store/sqlite_test.go`

**Interfaces:**
- Produces: `store.Open(path string) (*SQLiteStore, error)`
- Produces: `InsertTaskEvent(ctx context.Context, TaskEvent) error`
- Produces: `InsertRewardEvent(ctx context.Context, RewardEvent) error`
- Produces: `AggregateModel(ctx context.Context) (Model, error)`

- [ ] **Step 1: 写临时数据库迁移、幂等事件和聚合测试**

测试插入两个不同任务事件、重复插入同一事件 ID，并断言重复被拒绝且聚合只包含两条。

- [ ] **Step 2: 运行存储测试并确认失败**

Run: `go test ./internal/store`
Expected: FAIL，存储接口未实现。

- [ ] **Step 3: 添加纯 Go SQLite 依赖并实现迁移与查询**

Run: `go get modernc.org/sqlite`

数据库使用 WAL、busy timeout、唯一事件 ID、字段 CHECK 约束和索引。API 只返回聚合计数，不返回原始行。

- [ ] **Step 4: 运行存储测试**

Run: `go test ./internal/store`
Expected: PASS。

### Task 4: HTTP API、校验和限流

**Files:**
- Create: `internal/httpapi/api.go`
- Create: `internal/httpapi/ratelimit.go`
- Test: `internal/httpapi/api_test.go`

**Interfaces:**
- Consumes: store event insertion and aggregation interfaces
- Produces: `httpapi.New(store Store, options Options) http.Handler`
- Routes: `GET /api/v1/health`, `GET /api/v1/model`, `POST /api/v1/events/tasks`, `POST /api/v1/events/rewards`

- [ ] **Step 1: 写健康检查、有效提交、非法字段、重复事件和限流失败测试**

使用 `httptest.NewRecorder` 对真实处理器发请求；断言合法提交为 202、非法任务为 400、重复事件为 409、超限为 429。

- [ ] **Step 2: 运行 API 测试并确认失败**

Run: `go test ./internal/httpapi`
Expected: FAIL，处理器未定义。

- [ ] **Step 3: 实现 JSON API 与内存限流**

设备 ID 使用服务器盐进行 SHA-256 后再传入存储层；IP 只用于内存限流且不持久化。设置请求体上限、安全响应头和 JSON content type。

- [ ] **Step 4: 运行 API 测试**

Run: `go test ./internal/httpapi`
Expected: PASS。

### Task 5: 浏览器模型与本地数据

**Files:**
- Create: `web/model.mjs`
- Test: `web/model.test.mjs`
- Create: `web/storage.mjs`
- Test: `web/storage.test.mjs`

**Interfaces:**
- Produces: `scoreTask`、`rewardTier`、`fallbackProjection`、`simulateProjection`
- Produces: `createRepository(storage)`，管理当前周期、最多 100 个历史周期、物价和匿名选择

- [ ] **Step 1: 写前端规则、预测和存储失败测试**

```js
test('三级药品质不足按差值的一半向下取整扣分', () => {
  assert.equal(scoreTask('medicine', 63, 58), 0)
})
```

- [ ] **Step 2: 运行 Node 测试并确认失败**

Run: `node --test web/*.test.mjs`
Expected: FAIL，模块或导出不存在。

- [ ] **Step 3: 实现纯函数与可注入 Storage 的仓库**

本地 schema 带版本号；读取损坏 JSON 时恢复空状态而不是阻断页面。

- [ ] **Step 4: 运行前端单元测试**

Run: `node --test web/*.test.mjs`
Expected: PASS。

### Task 6: 响应式网页

**Files:**
- Create: `web/index.html`
- Create: `web/styles.css`
- Create: `web/app.mjs`
- Create: `internal/webui/embed.go`
- Create: `cmd/pet-ring/main.go`
- Test: `cmd/pet-ring/main_test.go`

**Interfaces:**
- Consumes: browser model/storage and HTTP API
- Produces: one executable serving `/`, static assets, and `/api/v1/*`

- [ ] **Step 1: 写静态资源和 SPA 回退的失败测试**

断言 `/` 返回包含“宠环模拟器”的 HTML，`/styles.css` 为 CSS，未知前端路径回退首页，API 未被回退覆盖。

- [ ] **Step 2: 运行服务测试并确认失败**

Run: `go test ./cmd/pet-ring`
Expected: FAIL，嵌入资源或服务组合未定义。

- [ ] **Step 3: 实现页面和交互**

页面包含首次匿名提醒、当前周期、预测详情、历史、物价设置、隐私设置；网络失败不阻止本地保存。任务按钮根据类型展示品质字段，奖励完成表单记录实际结果。

- [ ] **Step 4: 运行 Go 与 Node 测试**

Run: `go test ./...`
Expected: PASS。

Run: `node --test web/*.test.mjs`
Expected: PASS。

### Task 7: 容器与部署材料

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `compose.yaml`
- Create: `deploy/Caddyfile.snippet`
- Create: `deploy/README.md`
- Create: `README.md`

**Interfaces:**
- Produces: container `pet-ring` listening on `8080`
- Produces: persistent bind mount `/opt/pet-ring/data:/data`
- Produces: isolated network `pet-ring-edge`

- [ ] **Step 1: 编写部署文件并使用只读配置检查验证**

Compose 必须包含只读根文件系统、`no-new-privileges`、CPU/内存限制、日志轮转、健康检查和独立网络，不包含生产密码。

- [ ] **Step 2: 构建 Linux 可执行文件**

Run: `$env:CGO_ENABLED='0'; $env:GOOS='linux'; $env:GOARCH='amd64'; go build -trimpath -ldflags='-s -w' -o work/pet-ring-linux-amd64 ./cmd/pet-ring`
Expected: exit 0，生成 Linux amd64 文件。

- [ ] **Step 3: 构建并检查 Docker 镜像（本机 Docker 可用时）**

Run: `docker build -t pet-ring:local .`
Expected: exit 0。若本机无 Docker，记录为未执行，不声称镜像已验证。

### Task 8: 完整验证

**Files:**
- Modify only when verification exposes defects

- [ ] **Step 1: 格式化并运行完整自动化测试**

Run: `gofmt -w cmd internal`

Run: `go test ./...`
Expected: PASS，0 failures。

Run: `go test -race ./...`
Expected: PASS，0 data races；若 Windows SQLite 驱动不支持 race，记录真实限制。

Run: `node --test web/*.test.mjs`
Expected: PASS，0 failures。

- [ ] **Step 2: 构建当前平台与 Linux 版本**

Run: `go build ./cmd/pet-ring`
Expected: exit 0。

Run: `$env:CGO_ENABLED='0'; $env:GOOS='linux'; $env:GOARCH='amd64'; go build -trimpath -o work/pet-ring-linux-amd64 ./cmd/pet-ring`
Expected: exit 0。

- [ ] **Step 3: 浏览器验收**

启动本地服务，分别在约 360px 和桌面宽度验证匿名选择、记环、撤销、刷新恢复、历史、物价、预测和网络失败回退；检查控制台无错误。

- [ ] **Step 4: 对照 PRD 验收标准逐条复核**

将无法在本机验证的生产 DNS、Caddy 证书和真实服务器部署明确列为待用户执行，不宣称已上线。

