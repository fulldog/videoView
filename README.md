# 视频上传转码系统

Gin + JWT + Zap + GORM + FFmpeg。视频存本地磁盘，元数据存 MySQL。支持分片上传、断点续传，后台有界协程转码为 480p / 720p / 1080p。

## 环境

- Go 1.27+
- MySQL 8（库名 `videoview`）
- 本机 `ffmpeg`、`ffprobe`（见下方安装）

```bash
docker compose up -d
```

修改 [configs/config.yaml](configs/config.yaml) 中的 `mysql.dsn`。可用环境变量覆盖：`APP_MODE`、`APP_ADDR`、`MYSQL_DSN`、`JWT_SECRET`、`LOG_LEVEL`。

## 安装 FFmpeg

转码依赖本机 [FFmpeg](https://ffmpeg.org/) 的 `ffmpeg` 与 `ffprobe`。官方说明见 [Download FFmpeg](https://ffmpeg.org/download.html)：项目本身只提供源码，预编译包由页面列出的发行渠道提供。

安装完成后需能在命令行执行：

```bash
ffmpeg -version
ffprobe -version
```

若未加入 PATH，可在 [configs/config.yaml](configs/config.yaml) 里写成绝对路径，例如 Windows：

```yaml
transcode:
  ffmpeg: "C:\\ffmpeg\\bin\\ffmpeg.exe"
  ffprobe: "C:\\ffmpeg\\bin\\ffprobe.exe"
```

### Windows（官方推荐的预编译）

按 [下载页](https://ffmpeg.org/download.html) 的 **Windows EXE Files**：

1. 打开 [Windows builds from gyan.dev](https://www.gyan.dev/ffmpeg/builds/)（或 [Windows builds by BtbN](https://github.com/BtbN/FFmpeg-Builds/releases)）。
2. 下载完整包（gyan.dev 常用 `ffmpeg-git-full.7z` / `ffmpeg-release-full.7z`）。
3. 用 7-Zip 解压，将目录放到例如 `C:\ffmpeg`，保证存在 `C:\ffmpeg\bin\ffmpeg.exe` 与 `ffprobe.exe`。
4. 把 `C:\ffmpeg\bin` 加入系统或用户 **PATH**：设置 → 系统 → 关于 → 高级系统设置 → 环境变量 → Path → 新建。
5. **重新打开** 终端后再执行 `ffmpeg -version`。

### Linux（发行版官方包）

按 [下载页](https://ffmpeg.org/download.html) 的 **Linux Packages**，用发行版仓库安装（含 `ffmpeg` 与 `ffprobe`）：

```bash
# Debian / Ubuntu
sudo apt update
sudo apt install ffmpeg

# Fedora
sudo dnf install ffmpeg

# Arch
sudo pacman -S ffmpeg
```

各发行版包的入口都在官方 [Linux Packages](https://ffmpeg.org/download.html#build-linux) 一节。

### macOS

按 [下载页](https://ffmpeg.org/download.html) 的 **macOS Static builds**，或用 Homebrew：

```bash
brew install ffmpeg
```

### 从源码编译（可选）

官方仓库：

```bash
git clone https://git.ffmpeg.org/ffmpeg.git ffmpeg
```

发行版校验与编译步骤见 [ffmpeg.org/download.html](https://ffmpeg.org/download.html) 的 Get the Sources / Release Verification。日常运行本服务使用预编译包即可。

## 日志

- 级别：`log.level`（debug / info / warn / error）
- 文件：`logs/app-YYYY-MM-DD.log`，按自然日切割
- **仅 `app.mode=dev` 时同时输出控制台**；`prod` 只写文件

## 启动

在仓库根目录：

```bash
go run ./cmd/server -config configs/config.yaml
```

健康检查：`GET http://127.0.0.1:8080/healthz`

默认管理员：`admin` / `Admin@123`

## 清晰度策略

不根据源分辨率跳档。每个视频都用 FFmpeg 转出 **480p、720p、1080p** 三档（源更低时会上采样）。ffprobe 只用于读取时长等元数据，以及估算转码进度。

## API

统一响应：`{"code":0,"message":"ok","data":...}`。除登录注册重置外，请求头 `Authorization: Bearer <token>`。

### 认证

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/auth/register` | `{"username","password"}` |
| POST | `/api/v1/auth/login` | 同上 |
| POST | `/api/v1/auth/password/change` | JWT；`{"old_password","new_password"}` |
| POST | `/api/v1/auth/password/reset-request` | `{"username"}`；**dev 模式返回 token** |
| POST | `/api/v1/auth/password/reset` | `{"token","new_password"}` |

### RBAC（需 `rbac:manage`）

| 方法 | 路径 |
| --- | --- |
| GET | `/api/v1/rbac/users` |
| GET | `/api/v1/rbac/roles` |
| POST | `/api/v1/rbac/roles` |
| PUT | `/api/v1/rbac/roles/:id` |
| GET | `/api/v1/rbac/permissions` |
| PUT | `/api/v1/rbac/users/:id/roles` |

种子权限：`video:upload`、`video:read`、`video:download`、`video:manage`、`rbac:manage`。注册用户默认 `user` 角色（上传/观看/下载）。

### 分片上传（需 `video:upload`）

1. `POST /api/v1/uploads`  
   `{"filename":"demo.mp4","size":12345678,"chunk_size":5242880}`  
   返回 `id`、`stored_basename`（磁盘名）、`original_filename`、`chunk_total`。
2. `PUT /api/v1/uploads/:id/chunks/:index`  
   Body 为该分片原始字节（index 从 0 开始）。
3. `GET /api/v1/uploads/:id`  
   返回 `uploaded_chunks` 与 `upload_progress`，用于断点续传（只补传缺失分片）。
4. `POST /api/v1/uploads/:id/complete`  
   合并到 `storage/originals/{stored_basename}.{ext}`，写入视频表并触发转码。

支持扩展名：mp4 / mov / mkv / webm / avi / flv / m4v / mpeg / mpg / wmv / ts。

### 视频

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/videos` | `video:read` | 普通用户看自己的；`video:manage` 看全部 |
| GET | `/api/v1/videos/:id` | `video:read` | 含上传/转码进度与各档状态 |
| GET | `/api/v1/videos/:id/play?quality=720` | `video:read` | Range 播放，观看 +1 |
| GET | `/api/v1/videos/:id/download?quality=720` | `video:download` | 附件下载，下载 +1 |

`quality` 可选 480 / 720 / 1080；省略则取最高已就绪档，否则回源文件。

## curl 示例

将 `TOKEN`、`FILE` 换成实际值。PowerShell 下分片上传示例：

```powershell
$login = Invoke-RestMethod -Method Post http://127.0.0.1:8080/api/v1/auth/login -ContentType application/json -Body '{"username":"admin","password":"Admin@123"}'
$token = $login.data.token
$headers = @{ Authorization = "Bearer $token" }

$file = Get-Item .\demo.mp4
$chunkSize = 5MB
$initBody = @{ filename = $file.Name; size = $file.Length; chunk_size = $chunkSize } | ConvertTo-Json
$sess = Invoke-RestMethod -Method Post http://127.0.0.1:8080/api/v1/uploads -Headers $headers -ContentType application/json -Body $initBody
$sid = $sess.data.id

$fs = [IO.File]::OpenRead($file.FullName)
$buf = New-Object byte[] $chunkSize
$index = 0
while (($n = $fs.Read($buf, 0, $buf.Length)) -gt 0) {
  $slice = $buf[0..($n-1)]
  Invoke-RestMethod -Method Put "http://127.0.0.1:8080/api/v1/uploads/$sid/chunks/$index" -Headers $headers -ContentType application/octet-stream -Body $slice
  $index++
}
$fs.Close()

Invoke-RestMethod -Method Post "http://127.0.0.1:8080/api/v1/uploads/$sid/complete" -Headers $headers

# 查询进度
Invoke-RestMethod http://127.0.0.1:8080/api/v1/videos -Headers $headers
```

重置密码（仅 dev 返回 token）：

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/auth/password/reset-request -H "Content-Type: application/json" -d "{\"username\":\"admin\"}"
curl -s -X POST http://127.0.0.1:8080/api/v1/auth/password/reset -H "Content-Type: application/json" -d "{\"token\":\"<token>\",\"new_password\":\"Admin@1234\"}"
```

## 目录

```
storage/chunks/       分片临时目录
storage/originals/    合并后的源文件（stored_basename）
storage/transcoded/   {basename}_480.mp4 等
logs/                 按天切割的应用日志
```
