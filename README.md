# 畅看 Cwatch

基于 Go、Gin 和原生 JavaScript 的短视频项目。支持注册登录、视频上传播放、点赞、评论、热门排行和随机浏览。

## Docker Compose 部署

从项目根目录复制 `compose/.env.example` 为 `compose/.env`，为每个密码字段填入独立生成的十六进制密码。Linux 上可用 `openssl rand -hex 24` 生成。将 `.env` 权限设置为仅部署用户可读，然后启动：

```sh
docker compose --env-file compose/.env -f compose/docker-compose.yml build app minio
docker compose --env-file compose/.env -f compose/docker-compose.yml up -d --no-build
docker compose --env-file compose/.env -f compose/docker-compose.yml ps
```

Cwatch 使用独立的 Compose 项目、网络和命名数据卷。Docker 将宿主机的 `CWATCH_HTTP_PORT` 直接映射到应用容器的 `5000` 端口。默认访问地址为 `http://服务器IP:8080/`。MySQL、Redis、RabbitMQ 和 MinIO 在项目网络内通信。MySQL、RabbitMQ 和 MinIO 数据保存在各自的数据卷中，Redis 缓存可以重建。部署多个项目时可分别设置项目名和宿主机 HTTP 端口。服务器需要镜像或包下载代理时，可配置 `CWATCH_IMAGE_PREFIX`、`CWATCH_GOPROXY` 和 `CWATCH_APT_MIRROR`。

## 本地启动

需要 Go 1.24.9 和一个可访问的 MySQL 8.0 数据库。FFmpeg 用于生成封面和转码。配置相应连接后，项目使用 Redis 共享缓存、RabbitMQ 投递异步任务和 MinIO 存储视频。开发环境可以留空这些连接，直接查询 MySQL、轮询任务表并使用本地媒体目录。

1. 在已有 MySQL 实例中创建 `cwatch` 数据库，并准备有该库读写及建表权限的账号。
2. 在 PowerShell 中从项目根目录启动后端。把示例 DSN 的账号、密码和端口替换成实际值。

```powershell
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOTOOLCHAIN = 'local'
$env:CWATCH_DSN = 'cwatch:你的密码@tcp(127.0.0.1:3306)/cwatch?parseTime=true&charset=utf8mb4&loc=UTC'
Set-Location backend
go run .
```

3. 打开 [http://127.0.0.1:5000](http://127.0.0.1:5000)。后端会提供页面、API 和已确认的视频文件，并启动一个转码 Worker。服务会自动迁移表结构。

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `CWATCH_DSN` | `root@tcp(127.0.0.1:3306)/cwatch?parseTime=true&charset=utf8mb4&loc=UTC` | MySQL 连接 |
| `CWATCH_REDIS_URL` | 空 | Redis 连接，如 `redis://主机:6379/0` 或 `rediss://主机:6380/0`，支持 ACL 用户名和密码 |
| `CWATCH_RABBITMQ_URL` | 空 | AMQP 地址，支持 `amqp://` 和 `amqps://`，使用分配给项目的用户和 vhost |
| `CWATCH_RABBITMQ_QUEUE` | `cwatch.video-tasks.v1` | 视频转码和清理的持久队列 |
| `CWATCH_MINIO_ENDPOINT` | 空 | 带 `http://` 或 `https://` 的 MinIO API 地址 |
| `CWATCH_MINIO_ACCESS_KEY`、`CWATCH_MINIO_SECRET_KEY` | 空 | MinIO 凭据。配置 MinIO 地址时需要设置 |
| `CWATCH_MINIO_BUCKET` | `cwatch` | 已创建的 MinIO 存储桶，需允许读、写、列举和删除对象 |
| `CWATCH_MINIO_REGION` | 空 | 对象存储区域。留空时由 SDK 查询 |
| `CWATCH_MEDIA_DIR` | `../data/media` | 本地媒体及上传和转码暂存目录，相对于运行目录 |
| `CWATCH_WEB_DIR` | `../web` | 前端文件目录，相对于运行目录 |
| `CWATCH_ADDR` | `127.0.0.1:5000` | API 和前端的监听地址 |
| `CWATCH_FFMPEG` | `ffmpeg` | PATH 中的命令或 FFmpeg 的完整路径 |
| `CWATCH_WORKERS` | `1` | 进程内 Worker 数，API 允许设为 `0` |

需要单独运行 Worker 时，将后端的 `CWATCH_WORKERS` 设为 `0`，从 `worker` 目录执行 `go run .`。先启动 API 完成迁移。两个进程使用同一数据库和中间件配置。本地存储模式还要求共享同一个媒体目录。

上面的命令使用本机 Go 工具链和已有的模块缓存进行离线构建。

## 数据和任务

- MySQL 保存用户、会话、视频、点赞、评论和任务。用户重新登录即可获取新的会话令牌。点赞关系和计数在一个短事务里更新，数据库用联合唯一约束保证同一个用户与视频只有一条点赞关系。
- 浏览器拿到上传地址后携带 Bearer 令牌上传原文件。API 将文件写入该视频创建时选定的存储位置。配置 MinIO 后，新视频的原文件、封面和转码产物存入 MinIO，浏览器仍经 API 播放并支持 Range 请求。启用 MinIO 前上传的视频继续从原来的本地目录读取。
- 确认上传时，在更新视频状态的同一个事务里创建转码任务。RabbitMQ 发布器从任务表读取待投递任务，发送固定任务 ID，收到发布确认且消息已路由后记录投递时间。连接中断时，任务留在 MySQL 等待恢复。确认后未被领取的任务也会重新投递。
- Worker 按任务 ID 用 MySQL 行锁领取任务，并在租约内续期。重复投递由任务状态和租约控制，旧执行令牌无法发布产物。任务完成或重试状态已落库后，Worker 才确认消息。临时失败最多执行四次，已失败的转码可由视频作者调用 `POST /api/video/retry-processing`，请求体为 `{"video_id": 1}`。删除视频会取消转码并登记存储清理任务。
- 首页和热门榜优先读 Redis，未命中时查询 MySQL 并回填。视频确认、转码完成和删除后使共享缓存失效。Redis 超时或缺少连接配置时，直接查询 MySQL。同一进程内并发到达的相同回源请求合并为一次查询，热点回源有并发上限。榜单和计数可有数秒延迟，当前用户的点赞状态从数据库批量查询。随机 Feed 在每次进入时随机选起始页，按页加载。
- 迁移旧库时先备份数据。首次添加点赞唯一约束会合并重复关系、保留一条记录并重算视频计数。数据库备份和媒体目录需要一起保存，旧对象存储的历史视频需要从原文件备份另行恢复。

## 常用接口

| 方法和路径 | 用途 |
| --- | --- |
| `POST /api/register`、`POST /api/login`、`POST /api/logout` | 注册、登录和登出 |
| `GET /api/videos`、`POST /api/videos/hot` | 视频列表和热门榜 |
| `POST /api/random-feed/next` | 随机浏览 |
| `POST /api/video/upload-url` → `PUT /api/video/upload/:videoid` → `POST /api/video/upload-complete` | 申请上传、上传文件、确认 |
| `POST /api/video/like`、`DELETE /api/video/like` | 点赞和取消点赞 |
| `GET /api/video/:videoid/comments`、`POST /api/video/comment/:videoid`、`DELETE /api/video/comment/:commentid` | 评论列表、发表和删除 |
| `GET /api/user/:userid/videos`、`POST /api/videos/delete` | 个人视频和批量删除 |

## 测试

在 `backend` 和 `worker` 目录分别执行 `go test ./...`。后端并发集成测试使用 `CWATCH_TEST_DSN` 指定独立的空 MySQL 测试数据库。测试会创建表、测试用户和任务。
