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

## GitHub Actions 自动部署

仓库的 `.github/workflows/deploy-production.yml` 在 `main` 推送后通过受限 SSH Key 触发服务器部署。GitHub Actions 不发送任意远程命令；服务器的 `authorized_keys` 强制执行 root 持有的 `/usr/local/sbin/deploy-pet-ring`。

部署脚本只操作 `/opt/pet-ring` 和 `pet-ring` 容器，不执行 `docker compose down`、Docker prune，不修改 Supabase、Caddy、WoW 或共享网络。它要求 Git 历史可快进、拒绝覆盖已跟踪的本地修改，并在新容器健康检查失败时恢复旧提交和旧镜像。

### 1. 安装服务器脚本

每次仓库中的部署脚本发生变更，都必须由管理员重新安装；不要直接让部署用户执行仓库内可修改的脚本。

```bash
sudo install -o root -g root -m 0755 \
  /opt/pet-ring/deploy/deploy-pet-ring.sh \
  /usr/local/sbin/deploy-pet-ring
```

检查：

```bash
sudo bash -n /usr/local/sbin/deploy-pet-ring
sudo stat /usr/local/sbin/deploy-pet-ring
```

### 2. 创建 Actions 专用 SSH Key

在可信电脑生成一把独立密钥，不要复用服务器拉取 GitHub 的只读 Deploy Key：

```bash
ssh-keygen -t ed25519 -f ./pet_ring_actions -C github-actions-pet-ring -N ""
```

- 私钥 `pet_ring_actions` 只写入 GitHub Secret `PET_RING_DEPLOY_SSH_KEY`；
- 公钥 `pet_ring_actions.pub` 只写入服务器部署用户的 `authorized_keys`；
- 不要把任一密钥提交到仓库。

以下示例使用已有的 `ubuntu` 用户。编辑 `/home/ubuntu/.ssh/authorized_keys`，把公钥内容放在同一行末尾：

```text
command="sudo -n /usr/local/sbin/deploy-pet-ring",restrict ssh-ed25519 AAAA... github-actions-pet-ring
```

确认权限：

```bash
sudo chown -R ubuntu:ubuntu /home/ubuntu/.ssh
sudo chmod 700 /home/ubuntu/.ssh
sudo chmod 600 /home/ubuntu/.ssh/authorized_keys
```

### 3. 限定 sudo 权限

使用 `visudo` 创建规则：

```bash
sudo visudo -f /etc/sudoers.d/pet-ring-deploy
```

仅写入：

```sudoers
ubuntu ALL=(root) NOPASSWD: /usr/local/sbin/deploy-pet-ring
```

然后：

```bash
sudo chmod 440 /etc/sudoers.d/pet-ring-deploy
sudo visudo -cf /etc/sudoers.d/pet-ring-deploy
sudo -l -U ubuntu
```

### 4. 核对服务器 SSH 主机指纹

在服务器读取 ED25519 指纹：

```bash
sudo ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

在可信电脑获取公网入口的主机密钥，并用 `ssh-keygen -lf` 核对指纹一致：

```bash
ssh-keyscan -p SSH端口 服务器地址 > ./pet-ring-known-hosts
ssh-keygen -lf ./pet-ring-known-hosts
```

从核对成功的 ED25519 行中取出 `ssh-ed25519 AAAA...` 部分，作为 `PET_RING_DEPLOY_HOST_KEY`。不要关闭工作流中的严格主机校验。

### 5. 配置 GitHub Secrets

在仓库 `Settings → Secrets and variables → Actions` 中创建：

| Secret | 内容 |
|---|---|
| `PET_RING_DEPLOY_SSH_KEY` | Actions 专用私钥全文 |
| `PET_RING_DEPLOY_HOST_KEY` | 已核对的 `ssh-ed25519 AAAA...` 主机公钥 |
| `PET_RING_DEPLOY_HOST` | 服务器公网 IP 或域名 |
| `PET_RING_DEPLOY_PORT` | SSH 端口 |
| `PET_RING_DEPLOY_USER` | `ubuntu` |

服务器和 Secrets 全部验证完成后，再在同一页面的 `Variables` 中创建
`PET_RING_DEPLOY_ENABLED=true`。开关未启用时工作流会安全跳过，避免首次提交工作流时因配置尚未完成而失败。

启用后可在 Actions 页面手动运行 `Deploy pet-ring to production`。之后每次推送 `main` 都会串行部署；同一时间只允许一个生产部署任务。

### 6. 验证与恢复

服务器检查：

```bash
sudo docker compose -f /opt/pet-ring/compose.yaml ps
curl -i https://ring.ddctl.com/api/v1/health
```

如果 Actions 失败，查看任务日志；服务器脚本会输出构建、健康检查及回滚结果。固定回滚镜像标签为 `pet-ring:rollback`，不会无限积累部署镜像标签。
