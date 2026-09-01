# ring.ddctl.com 部署说明

本目录仅提供部署材料。首次本地实现不会自动连接或修改服务器。

## 隔离边界

- 宠环服务使用独立容器 `pet-ring`；
- 使用独立网络 `pet-ring-edge`；
- SQLite 位于宠环项目自己的 `data/`；
- 不使用 `supabase_default`、PostgreSQL、Kong、Auth 或现有数据卷；
- 只让现有 `supabase-caddy` 额外连接 `pet-ring-edge`；
- Caddy 是与现有项目共享的唯一组件。

## 服务器部署前检查

1. 给 `ring.ddctl.com` 添加指向上海服务器公网 IP 的 DNS A 记录。
2. 确认 `/opt/supabase/volumes/proxy/caddy` 中的实际 Caddyfile。
3. 确认 Supabase Compose 中 Caddy 的服务名：

```bash
cd /opt/supabase
sudo docker compose config --services
```

4. 备份现有 Caddyfile 和 Compose 文件。

## 建议安装目录

```text
/opt/pet-ring/
├── compose.yaml
├── Dockerfile
├── .env
├── data/
└── 源代码
```

创建数据目录后，将其交给容器内的非 root 用户 `10001`：

```bash
sudo mkdir -p /opt/pet-ring/data
sudo chown 10001:10001 /opt/pet-ring/data
```

生成匿名设备哈希盐：

```bash
openssl rand -hex 32
```

将结果写入 `/opt/pet-ring/.env`：

```dotenv
PET_RING_DEVICE_SALT=生成的64位十六进制内容
```

## 启动独立应用

```bash
cd /opt/pet-ring
sudo docker compose config
sudo docker compose build
sudo docker compose up -d
sudo docker compose ps
```

此时应用没有绑定宿主机公网端口，只存在于 `pet-ring-edge` 网络中。

## 让现有 Caddy 连接独立网络

持久方案是在 Supabase Compose 中给 Caddy 服务添加外部网络。先复制并根据实际服务名检查 `caddy-network.override.example.yml`，再与现有两个 Compose 文件一起执行 `docker compose config`。必须保证原有 `default` 网络仍然存在。

临时连通性测试可以使用：

```bash
sudo docker network connect pet-ring-edge supabase-caddy
```

该命令只增加网络连接，不应作为长期配置；Caddy 容器被重新创建后连接会丢失。

## Caddy 路由

将 `Caddyfile.snippet` 的站点块追加到实际 Caddyfile。修改前备份：

```bash
sudo cp /opt/supabase/volumes/proxy/caddy/Caddyfile \
  /opt/supabase/volumes/proxy/caddy/Caddyfile.backup.$(date +%Y%m%d-%H%M%S)
```

先在容器中校验，不要直接重启：

```bash
sudo docker exec supabase-caddy caddy validate --config /etc/caddy/Caddyfile
```

只有校验成功后才无中断加载：

```bash
sudo docker exec supabase-caddy caddy reload --config /etc/caddy/Caddyfile
```

## 回滚

若新站点异常，恢复备份 Caddyfile并再次 validate/reload，然后停止宠环容器：

```bash
cd /opt/pet-ring
sudo docker compose stop
```

停止或删除宠环容器不会删除 bind mount 中的 SQLite 文件，也不会影响 Supabase。
