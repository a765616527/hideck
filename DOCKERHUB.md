# HiDeck GHCR 镜像

镜像地址：`ghcr.io/a765616527/hideck`。`main` 推送触发源码构建并发布 `latest`，发布新版本 tag 时还会生成对应版本镜像。

支持架构：

- `linux/amd64`
- `linux/arm64`

## 快速启动（推荐）

直接通过 curl 运行部署脚本，默认安装到当前目录下的 `hideck/`：

```bash
curl -fsSL https://raw.githubusercontent.com/a765616527/hideck/main/deploy.sh | sh
```

自定义安装目录：

```bash
curl -fsSL https://raw.githubusercontent.com/a765616527/hideck/main/deploy.sh | HIDECK_DIR=/opt/hideck sh
```

脚本会下载 `docker-compose.yml` 和配置模板，创建持久化目录并拉取 `latest`；不会覆盖已有的部署文件和 `config/config.yaml`。原来安装上游版的部署目录，须先手动将 `docker-compose.yml` 镜像改为 `ghcr.io/a765616527/hideck:latest` 再升级。

## 手工部署

```bash
mkdir -p hideck/{config,data,logs}
cd hideck
```

创建 `config/config.yaml`：

```yaml
server:
  port: 7575
  debug: false
  https_enabled: false

web:
  username: admin
  password: admin

devices: []

proxy:
  instances: []

vowifi:
  enabled: false
```

创建 `docker-compose.yml`：

```yaml
services:
  hideck:
    image: ghcr.io/a765616527/hideck:latest
    container_name: hideck
    restart: unless-stopped
    init: true
    stop_grace_period: 30s
    network_mode: host
    privileged: true
    volumes:
      - ./config:/app/config
      - ./data:/app/data
      - ./logs:/app/logs
      - /dev:/dev
    environment:
      TZ: Asia/Shanghai
      CONFIG_PATH: /app/config/config.yaml
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: "3"
```

启动：

```bash
docker compose up -d
```

Web 入口：`http://YOUR_IP:7575`

默认账号：`admin` / `admin`

首次登录后请立即修改密码。

## 维护者发布

`main` 推送会触发 `.github/workflows/docker-build.yml`，从当前源码构建并发布 `ghcr.io/a765616527/hideck:latest`。DNS 镜像由 `.github/workflows/caddy-dns-build.yml` 构建。请确认 GitHub Packages 中的容器包设置为 public，才可匿名一键安装。

二进制安装走独立的 `.github/workflows/binary-release.yml`：在修复完成后推送新的 `vX.Y.Z` tag，等待工作流成功并确认 Release 包含 Linux/OpenWrt 二进制及 SHA256SUMS。复制过来的上游 tag 没有 fork 的 Release 资产，不能拿来安装。

服务器部署仅拉取镜像，不在服务器上编译。需要手动构建镜像时，使用根目录 `Dockerfile.github` 和自己的 GHCR 地址；历史 Docker Hub 发布流程不用于本 fork。

## 更新镜像

```bash
docker compose pull
docker compose up -d
```

应用内二进制热更新在这个源码整合构建中已禁用。Docker 部署请通过拉取新镜像升级。

## 配置说明

| 路径 | 说明 |
| --- | --- |
| `/app/config` | 配置文件目录 |
| `/app/data` | SQLite 数据与运行数据 |
| `/app/logs` | 日志目录 |

容器默认时区为 `Asia/Shanghai`。Compose 文件也显式设置了同一时区，方便在不同运行方式下保持一致。

## 许可证提示

本仓库是源码整合树，不是单一 MIT 许可项目。根项目来自 PolyForm Noncommercial 1.0.0，`third_party/vowifi-go` 为 AGPL-3.0，其它第三方源码按各自许可证授权。发布公开二进制或 Docker 镜像前，请先确认组合分发的许可证义务。

## PC/SC smart-card readers

HiDeck supports PC/SC readers without a cellular modem. The Docker images include
only the PC/SC client library; the Linux host owns the USB reader through `pcscd`
and its reader driver. Passing `/dev` alone does not expose this service.

On Debian/Ubuntu, install and start the host dependencies:

```sh
sudo apt-get update
sudo apt-get install -y pcscd libccid pcsc-tools
sudo systemctl enable --now pcscd.socket
sudo systemctl start pcscd.service
sudo pcsc_scan
```

Confirm that the reader and inserted card appear, then exit `pcsc_scan` with
Ctrl-C. Some readers need a vendor driver instead of `libccid`. On other Linux
distributions use the equivalent packages/service; the commands above assume
systemd. Do not run a second `pcscd` in the container against the same reader.

After the normal Docker installation, save `docker-compose.pcsc.yml` from this
repository alongside your existing `docker-compose.yml`, then recreate HiDeck:

```sh
curl -fsSL https://raw.githubusercontent.com/a765616527/hideck/main/docker-compose.pcsc.yml \
  -o docker-compose.pcsc.yml
test -S /run/pcscd/pcscd.comm
docker compose -f docker-compose.yml -f docker-compose.pcsc.yml up -d
```

The override mounts `/run/pcscd` read-only. Unix socket connections still work;
mounting the directory instead of the socket allows `pcscd` to recreate it after
a restart. The directory must already exist (`create_host_path: false`); a missing
host service should not silently create an empty directory. Ensure `pcscd` is
started before HiDeck after a host reboot. If your distribution uses a different
socket directory, adjust the bind source and check the client socket path.

Use the same `-f` arguments for subsequent `pull`, `up`, and `logs` commands.
Include any existing Caddy overrides as well. The one-click `deploy.sh` preserves
existing files but does not automatically select this optional override; rerunning
it alone will omit the PC/SC mount. A plain modem-only installation does not need
this override or a running host `pcscd`.

In HiDeck, discover/add the physical PC/SC reader, then read its EID/profile list.
A blank eUICC may return no profiles. This verifies card access, not carrier
activation or VoWiFi registration. If no reader appears, check host `pcsc_scan`,
the bind mount, and host `pcscd` logs/access policy. Keep other SIM applications
from holding the card during operations; do not disable host access controls as
a workaround.
