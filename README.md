# ClassOrbit

ClassOrbit（智创课堂）是面向小学信息科技教师的轻量班级积分、课堂考勤与课程导航系统。Go 单进程提供 API 并托管 React 前端，数据存放在本地 SQLite，适合教师电脑、校内局域网或小型服务器部署。

当前稳定版本为 `v1.4.0`。版本变更见 [`CHANGELOG.md`](CHANGELOG.md)，开发、提交和 Tag 发布规则见 [`CONTRIBUTING.md`](CONTRIBUTING.md)。

## 已实现

- 多班级管理，Excel 批量导入学号和姓名
- 一至六年级与班号结构化管理，升年级后历史考勤自动同步班级名称
- 学生积分卡快速加减分、自定义分值与原因、积分流水
- 按学号或积分排序，按姓名和学号搜索
- 课堂随机点名，抽中后可直接加减分
- 随机点名支持调整人数，同一次抽取不会出现重复学生
- 学生自助签到页，重复签到保护
- 签到名单支持姓名、拼音首字母、完整拼音和部分学号搜索
- 签到成功后 5 秒自动进入学生学习导航页，教学网站在新标签页打开
- 教师后台可维护学习网站标题、链接、在线图标及显示顺序
- 教师考勤台实时查看已到和缺席，支持迟到、请假等手动修正
- 考勤场次支持便捷删除，删除进行中场次会立即停止该班签到
- 删除考勤进入回收站，可恢复或二次确认后永久删除
- 考勤保存班级、学号和姓名快照，修改或删除名单不会污染历史
- 按服务器时间、课表和临时换课自动识别当前课时，教师确认后发起签到
- 考勤场次支持课程、上课日期时间以及班级/日期组合筛选
- 场次名称统一按“班级名 · 日期 时间”自动生成
- 后台自定义站点主标题和副标题
- 教师后台悬浮课程按钮，展开后自动显示当前/下节课和当天课表
- 本学期校历支持自定义起止日期，并自动计算周次与单双周
- 周一至周五、每日 7 节的网格课表，支持编辑节次时间和单双周上课地点
- 常规课表支持点击格子编辑、Excel 批量导入，以及带备注的换课或被占课标记
- 首次部署创建教师账号密码，教师 API 与公开学生 API 隔离
- 支持修改密码并注销其他会话、操作审计和登录/签到/名单接口限流
- 支持一致性数据库下载/恢复、每日自动备份及 Excel 名单/积分/考勤报表
- 积分流水支持可审计撤销，不直接删除原始记录
- 桌面和手机响应式界面，动效尊重 `prefers-reduced-motion`

## 项目结构

```text
classorbit/
├── .github/workflows/      # 自动测试与 GHCR 镜像发布
├── backend/                # Go API、SQLite 数据层、迁移与测试
├── docs/                   # 对外接口和改进路线文档
├── frontend/               # React、Radix UI 前端
├── VERSION                 # 单一应用版本号来源
├── CHANGELOG.md            # 按 SemVer 维护的发布记录
├── CONTRIBUTING.md         # 分支、提交与 Tag 发布规范
├── Dockerfile              # 前后端多阶段构建
├── compose.yaml            # 从源码构建并部署
├── compose.deploy.yaml     # 直接使用预构建镜像部署
├── .env.example            # 部署环境变量示例
└── Makefile                # 本地构建、测试和容器命令
```

Go 后端原先位于项目根目录的 `main.go` 和 `store.go`，并非缺少后端。本次已迁入 `backend/`，目录边界更加清晰。生产环境仍然只有一个 Go 进程：同时提供 API 和前端静态文件。

数据使用单个 SQLite 文件。积分变更、签到和名单级联需要事务一致性，拆分多个 SQLite 会增加跨库失败风险，因此保留单库并使用 WAL、外键和索引。考勤列表已优化为固定两次查询，避免随场次数增长产生 N+1 查询。

## GitHub Actions 镜像发布

仓库建议命名为 `classorbit`。工作流 [`.github/workflows/publish-container.yml`](.github/workflows/publish-container.yml) 会执行以下流程：

1. 使用 Node.js 22 安装依赖，运行 Vitest/React Testing Library 测试并构建 React 前端。
2. 使用 `go.mod` 指定的 Go 版本运行后端与 API 测试。
3. 构建 `linux/amd64` 和 `linux/arm64` 多架构镜像。
4. 发布到 `ghcr.io/<GitHub 用户名或组织>/classorbit`。
5. 为镜像附加 SBOM、构建来源以及版本标签。

推送到 `main` 后会生成 `latest`、`main` 和 `sha-xxxxxxx` 标签；推送 `v1.2.3` 形式的 Git 标签还会生成 `1.2.3` 与 `1.2` 标签。Pull Request 只构建和测试，不发布镜像。

Tag 发布前，Actions 会校验根目录 `VERSION`、前端包版本、`CHANGELOG.md` 和 Git Tag 完全一致。版本镜像发布成功后会自动创建 GitHub Release。已经公开的版本 Tag 不应移动或覆盖。

首次发布后，在 GitHub 仓库的 Packages 页面把镜像设为 Public，即可在服务器上免登录拉取；如果保持 Private，需要先在服务器登录 GHCR。

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u 你的GitHub用户名 --password-stdin
```

## 使用预构建镜像部署

服务器只需要 `compose.deploy.yaml` 和 `.env`，不需要安装 Go、Node.js，也不需要上传完整源码。

```bash
cp .env.example .env
# 编辑 .env，把 CLASSORBIT_IMAGE 改为实际 GHCR 镜像地址并设置密码、共享密钥
docker compose -f compose.deploy.yaml up -d
```

`.env` 中最关键的配置如下：

```dotenv
CLASSORBIT_IMAGE=ghcr.io/你的GitHub用户名/classorbit
CLASSORBIT_TAG=latest
APP_PORT=8080
DATA_VOLUME_NAME=classorbit_data
TEACHER_USERNAME=teacher
TEACHER_PASSWORD=请设置强密码
CLASS_SYSTEM_TOKEN=请设置一个随机共享密钥
TZ=Asia/Shanghai
AUTO_BACKUP_ENABLED=true
BACKUP_RETENTION_DAYS=14
TRUST_PROXY_HEADERS=false
```

更新镜像：

```bash
docker compose -f compose.deploy.yaml pull
docker compose -f compose.deploy.yaml up -d
```

查看状态和日志：

```bash
docker compose -f compose.deploy.yaml ps
docker compose -f compose.deploy.yaml logs -f classorbit
```

默认通过服务器 `8080` 端口访问。公网部署必须在前面配置 Caddy、Nginx 或其他 HTTPS 反向代理，不要直接通过明文 HTTP 登录教师后台。只有反向代理会覆盖客户端来源地址时才设置 `TRUST_PROXY_HEADERS=true`。

## 从源码构建部署

VPS 安装 Docker 与 Compose 插件后，把整个项目目录上传并执行：

```bash
docker compose up -d
```

首次打开教师后台时，系统会要求创建教师账号和密码；账号信息保存在 SQLite 数据库中。

如需无人值守部署，可在首次启动前同时设置 `TEACHER_USERNAME` 和 `TEACHER_PASSWORD`；它们只用于空数据库的首次初始化，之后不会覆盖数据库中的账号。未设置时直接使用网页初始化即可。

如需修改外部端口，复制环境变量示例后再启动：

```bash
cp .env.example .env
docker compose up -d
```

源码构建默认使用 `goproxy.cn`、`registry.npmmirror.com` 和阿里云 Alpine 镜像加速 Go/npm/Alpine 依赖下载。构建缓存会保留 Go 模块和 npm 包；海外服务器可在 `.env` 中改为官方源。

`.env` 示例：

```dotenv
APP_PORT=9000
DATA_VOLUME_NAME=classorbit_data
ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
TEACHER_USERNAME=teacher
TEACHER_PASSWORD=请设置强密码
# 与 KeySprint 中 CLASS_SYSTEM_TOKEN 保持一致的随机共享密钥
CLASS_SYSTEM_TOKEN=请设置一个随机共享密钥
TZ=Asia/Shanghai
AUTO_BACKUP_ENABLED=true
BACKUP_RETENTION_DAYS=14
TRUST_PROXY_HEADERS=false
```

此时访问 `http://VPS_IP:9000`，学生签到地址为 `http://VPS_IP:9000/checkin`。也可以不创建 `.env`，临时指定端口：

```bash
APP_PORT=9000 ALPINE_MIRROR=https://mirrors.aliyun.com/alpine TEACHER_USERNAME=teacher TEACHER_PASSWORD='请设置强密码' CLASS_SYSTEM_TOKEN='请设置一个随机共享密钥' docker compose up -d
```

### KeySprint 班级名单同步

系统提供只读接口 `GET /api/integration/classes`，供 KeySprint 获取教师管理的班级和学生名单。接口使用独立的共享 Bearer 密钥，不使用教师后台 Cookie，也不接收教师密码。请求中的 `teacher_username` 必须与本系统教师账号用户名一致；如果发送 `X-Teacher-Username`，它也必须与查询参数一致。部署时设置 `CLASS_SYSTEM_TOKEN`，并在 KeySprint 中配置相同的 `CLASS_SYSTEM_TOKEN`。接口支持 ETag/`If-None-Match`，名单未变化时返回 `304`，适合定时同步。

完整的请求参数、响应字段、错误码和接入注意事项见 [`docs/integration-classes-api.md`](docs/integration-classes-api.md)。

请求示例：

```bash
curl -G "https://积分系统地址/api/integration/classes" \
  --data-urlencode "teacher_username=teacher001" \
  -H "Accept: application/json" \
  -H "X-Teacher-Username: teacher001" \
  -H "Authorization: Bearer 你的共享密钥"
```

成功响应格式为 `{"classes":[{"id":"1","name":"三 1 班","students":[{"id":"1001","name":"张三"}]}]}`。班级 `id` 在改名后保持稳定，KeySprint 会据此维持历史成绩关联。未提供凭证返回 `401`，共享密钥错误返回 `403`，教师账号不存在返回 `404`。

常用运维命令：

```bash
docker compose ps
docker compose logs -f classorbit
docker compose build --pull
docker compose up -d --build
docker compose down
```

`docker compose down` 不会删除数据。SQLite 数据（包括教师账号、导航配置和登录会话）保存在具名卷 `classorbit_data`；不要使用 `docker compose down -v`，除非确定要删除全部数据。

如果从旧版 ClassPoint 升级，在 `.env` 中设置 `DATA_VOLUME_NAME=classpoint_data` 即可继续挂载原数据卷。程序也会自动识别数据卷内已有的 `classpoint.db`；新安装则使用 `classorbit.db`。

SQLite 使用 WAL 模式。后台“系统设置”可以在线下载、校验和恢复一致性备份；程序默认每天在数据卷的 `backups/` 下保存一份备份并保留 14 天，恢复前还会额外保存安全副本。不要在容器运行时直接复制单个数据库主文件。

## 本地运行

需要 Go 1.24+ 和 Node.js 20+。

```bash
cd frontend
npm install
npm run build
cd ..
PUBLIC_DIR=frontend/dist go run ./backend
```

默认地址为 `http://127.0.0.1:8080`。首次访问时在页面中创建教师账号和密码。

健康接口同时返回运行版本和构建提交，便于核对部署内容：

```bash
curl http://127.0.0.1:8080/api/health
# {"commit":"...","database":true,"ok":true,"version":"1.1.0"}
```

容器内也可以直接查询二进制版本：

```bash
docker compose exec classorbit /app/classorbit --version
# ClassOrbit 1.1.0 (<构建提交号>)
```

要让同一局域网内的学生访问，可监听所有网卡：

```bash
PUBLIC_DIR=frontend/dist ADDR=0.0.0.0:8080 go run ./backend
```

学生签到地址是 `http://教师电脑IP:8080/checkin`，学习导航地址是 `http://教师电脑IP:8080/navigation`。新安装的数据默认保存在 `data/classorbit.db`；旧版数据目录中的 `classpoint.db` 会继续被自动使用。也可用 `DATA_DIR` 指定其他目录。

## Excel 格式

学生名单支持 `.xlsx`、`.xlsm` 和旧版 `.xls`，读取第一个工作表，单次最多 200 人。推荐格式：

| 学号 | 姓名 |
| --- | --- |
| 2026001 | 张同学 |
| 2026002 | 李同学 |

也识别 `student_no/name`、`student no/student name` 等英文表头。没有表头时默认读取前两列。班级内重复学号会跳过，不覆盖原学生信息。

课程表也支持 `.xlsx` 和 `.xlsm`，第一行需包含以下表头；“班级”应与后台已创建的班级一致：

| 星期 | 开始时间 | 结束时间 | 班级 | 课程 | 单周地点 | 双周地点 |
| --- | --- | --- | --- | --- | --- | --- |
| 周一 | 08:00 | 08:40 | 三 2 班 | 信息课 | 机房 1 | 机房 2 |
| 周五 | 14:20 | 15:00 | 四 1 班 | 人工智能 | 教室 | 教室 |

导入时间需要与“校历与节次”中设置的某一节课完全一致。地点支持“机房 1”“机房 2”和“教室”；也可以只使用“地点”一列，此时单双周地点相同。

## 验证

```bash
make test
make release-check
make version
```

`make test` 会执行前端生产构建、Vitest/React Testing Library 测试以及 Go 后端/API 测试。`make release-check` 校验版本文件和更新日志一致性，`make version` 显示本地构建将写入的版本和提交号。

更完整的性能、安全和功能改进建议见 [`docs/improvement-roadmap.md`](docs/improvement-roadmap.md)。
