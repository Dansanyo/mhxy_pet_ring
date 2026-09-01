# 宠环助手

梦幻西游召唤兽修炼任务的本地记录、积分预测、成本比较和策略决策网页。

## 当前能力

- 11 类系统任务积分规则；
- 指定变异的三种交付方式，以及所有任务可用的跳过本环（-20 分）；
- 当前积分至少 20 分才允许跳过；三药/烹饪仅在品质不达标时填写品质；
- 当前任务各处理方案的确定性积分、成本对比；
- PC 宽屏双栏与手机单栏响应式布局；
- 烹饪、三级药品质扣分；
- 当前积分、成本与平均值；
- 公共任务概率蒙特卡洛预测；
- 90–150 级书铁积分门槛；
- 公共奖励概率 × 用户本地价格的收益估算；
- 当前周期、最多 100 期本地历史和本地物价；
- 首次明确选择是否贡献匿名统计；
- Go + SQLite 匿名事件 API；
- 重复检测、字段校验、请求限流和只公开聚合数据；
- 独立 Docker/SQLite/网络部署材料。

## 本地运行

Windows PowerShell：

```powershell
$env:PET_RING_DEVICE_SALT='local-development-salt'
$env:PET_RING_DB="$PWD\work\pet-ring.db"
go run ./cmd/pet-ring
```

打开 <http://127.0.0.1:8080>。

Linux/macOS：

```bash
PET_RING_DEVICE_SALT=local-development-salt \
PET_RING_DB=./work/pet-ring.db \
go run ./cmd/pet-ring
```

## 测试

```powershell
go test ./...
node --test web/*.test.mjs
```

如果当前 Windows 环境限制用户缓存目录，可临时将 `GOCACHE`、`GOPATH`、`GOMODCACHE` 和 `GOTMPDIR` 指向项目的 `work/` 目录。

## 数据说明

个人周期、历史、成本和物价保存在浏览器 `localStorage`，不会写入服务器。用户同意匿名统计后，服务端仅接收任务概率和奖励概率所需字段；系统要求的任务与玩家处理方式分开记录，跳过不会被误计为随机任务。设备随机 ID 会使用服务器盐做 SHA-256 后再写入 SQLite。

应用不提供账号或跨设备同步。清理浏览器网站数据会清除个人历史。

## 配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PET_RING_ADDR` | `:8080` | HTTP 监听地址 |
| `PET_RING_DB` | `./work/pet-ring.db` | SQLite 文件路径 |
| `PET_RING_DEVICE_SALT` | 开发警告值 | 生产环境必须设置随机值 |

## API

- `GET /api/v1/health`
- `GET /api/v1/model`
- `POST /api/v1/events/tasks`
- `POST /api/v1/events/rewards`

公共模型接口只返回聚合数量，不返回事件 ID、设备哈希或单条样本。

## 部署

参见 [deploy/README.md](deploy/README.md)。目标域名为 `ring.ddctl.com`，只复用现有 `supabase-caddy` 入口，其他组件完全独立。

## 规则待确认

当前按原资料允许烹饪/三级药品质差距导致负积分。该口径仍需要更多玩家确认；规则调整时应同时修改 Go 和浏览器端测试。
