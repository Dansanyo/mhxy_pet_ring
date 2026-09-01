#!/usr/bin/env bash
set -Eeuo pipefail

# --------------------------------------------------------------------
# 宠环生产部署
# 只更新 /opt/pet-ring，并保留上一个可运行镜像用于失败回滚。
# --------------------------------------------------------------------

readonly APP_DIR="/opt/pet-ring"
readonly BRANCH="main"
readonly CONTAINER="pet-ring"
readonly IMAGE="pet-ring:local"
readonly ROLLBACK_IMAGE="pet-ring:rollback"
readonly LOCK_FILE="/run/lock/deploy-pet-ring.lock"
readonly HEALTH_ATTEMPTS=20
readonly HEALTH_INTERVAL=3

OLD_REVISION=""

log() {
	printf '[deploy-pet-ring] %s\n' "$*"
}

fail() {
	printf '[deploy-pet-ring] 错误：%s\n' "$*" >&2
	exit 1
}

health_check() {
	local attempt

	for ((attempt = 1; attempt <= HEALTH_ATTEMPTS; attempt++)); do
		if docker exec "$CONTAINER" \
			wget -q -O /dev/null http://127.0.0.1:8080/api/v1/health 2>/dev/null; then
			return 0
		fi
		log "等待健康检查：${attempt}/${HEALTH_ATTEMPTS}"
		sleep "$HEALTH_INTERVAL"
	done

	return 1
}

rollback() {
	log "恢复代码 ${OLD_REVISION}"
	git reset --hard "$OLD_REVISION"

	if ! docker image inspect "$ROLLBACK_IMAGE" >/dev/null 2>&1; then
		log "没有可用的回滚镜像"
		return 1
	fi

	docker image tag "$ROLLBACK_IMAGE" "$IMAGE"
	docker compose up -d --no-deps --force-recreate app
	health_check
}

main() {
	exec 9>"$LOCK_FILE"
	flock -n 9 || fail "已有部署正在执行"

	[[ -d "$APP_DIR/.git" ]] || fail "项目目录不是 Git 仓库：$APP_DIR"
	[[ -f "$APP_DIR/.env" ]] || fail "缺少生产配置：$APP_DIR/.env"
	cd "$APP_DIR"

	if ! git diff --quiet || ! git diff --cached --quiet; then
		fail "存在未提交的已跟踪文件修改，拒绝覆盖"
	fi

	OLD_REVISION="$(git rev-parse HEAD)"
	log "获取 origin/${BRANCH}"
	git fetch --prune origin "$BRANCH"
	local new_revision
	new_revision="$(git rev-parse "origin/${BRANCH}")"

	if ! git merge-base --is-ancestor "$OLD_REVISION" "$new_revision"; then
		fail "远端历史不是当前版本的快进更新"
	fi

	if docker image inspect "$IMAGE" >/dev/null 2>&1; then
		docker image tag "$IMAGE" "$ROLLBACK_IMAGE"
	fi

	log "更新 ${OLD_REVISION:0:7} -> ${new_revision:0:7}"
	git merge --ff-only "$new_revision"
	if ! docker compose config --quiet; then
		git reset --hard "$OLD_REVISION"
		fail "Compose 配置无效，代码已恢复"
	fi

	log "构建生产镜像"
	if ! docker compose build app; then
		git reset --hard "$OLD_REVISION"
		fail "镜像构建失败，代码已恢复"
	fi

	log "替换应用容器"
	if ! docker compose up -d --no-deps --force-recreate app; then
		rollback || true
		fail "容器启动失败，已尝试回滚"
	fi

	if ! health_check; then
		docker logs --tail 100 "$CONTAINER" >&2 || true
		rollback || true
		fail "新版本健康检查失败，已尝试回滚"
	fi

	log "部署成功：$(git rev-parse --short HEAD)"
}

main "$@"
