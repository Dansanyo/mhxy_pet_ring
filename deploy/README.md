# 宠环助手部署

## 首次部署

```bash
cd /opt/pet-ring
sudo mkdir -p data
sudo chown 10001:10001 data
openssl rand -hex 32
```

将生成的随机值写入 `.env`：

```dotenv
PET_RING_DEVICE_SALT=生成的64位十六进制内容
```

启动并检查服务：

```bash
sudo docker compose config --quiet
sudo docker compose up -d --build
sudo docker compose ps
sudo docker exec pet-ring wget -qO- http://127.0.0.1:8080/api/v1/health
```

## 自动部署

将 `deploy-pet-ring.sh` 安装为服务器部署命令：

```bash
sudo install -m 0755 deploy/deploy-pet-ring.sh /usr/local/sbin/deploy-pet-ring
sudo /usr/local/sbin/deploy-pet-ring
```

脚本会拉取 `main`、构建镜像、替换容器并执行健康检查；失败时自动尝试回滚。
