# 随机 Feed（Random Feed）后端实现流程（含布隆过滤器去重）

> 目标：登录后为每个用户提供“随机起点 + 不重复（概率去重）+ 滚到底继续加载”的 Feed 流。

---

## 1. 总体调用链路

1. 前端点击“随机 Feed”
2. 前端发起 `POST /api/random-feed/next`（`init: true`）
3. 后端通过 Controller 取出登录用户名 `username`
4. Service `NextRandomFeed()`：
   - 为该用户生成当日 Bloom Key
   - 按“随机起点页”抽取候选视频
   - 用 BloomFilter 做“是否可能已看过”的去重判断
   - 满足条件后返回 `5` 条视频，并把返回的视频 ID 写入 Bloom
5. 前端渲染到 `#feed`，并把视频列表滚动加载行为绑定在“最后一条 feed-item”上
6. 滚到最后一条附近时，前端继续发起 `POST /api/random-feed/next`（`init: false, page: next_page`）
7. Service 重复执行：仍然使用用户当日 Bloom Key 去重、返回下一批 `5` 条，并写入 Bloom

---

## 2. 路由层（Gin）

文件：`backend/routes/routes.go`

- 新增路由（需要登录态）：
  - `POST /api/random-feed/next`
- 路由依赖：
  - `middlewares.AuthMiddleware()`：解析 JWT 并把 `username` 注入到 `c.Set("username", ...)`

---

## 3. Controller 层：接收请求并调用 Service

文件：`backend/controllers/random_feed.go`

### 3.1 请求体

MVP 采用 JSON body：

- 初始化随机起点：
  - `{ "init": true }`
- 后续加载：
  - `{ "init": false, "page": 2 }`

### 3.2 处理逻辑

1. 从 `c.Get("username")` 读取当前登录用户名
2. 解析请求体得到：
   - `init`：是否随机起点初始化
   - `page`：后续扫描起始页码
3. 调用：
   - `randomFeedService.NextRandomFeed(usernameStr, req.Page, req.Init, 5)`
4. 将 Service 返回的响应直接 JSON 化给前端：
   - `videos: []VideoListItem`
   - `next_page: int`
   - `has_more: bool`

---

## 4. Service 层：随机起点 + Bloom 去重 + 返回下一批

文件：`backend/services/random_feed_service.go`

### 4.1 关键输入参数

- `username`：用于定位用户 ID
- `page`：候选页扫描起点
- `init`：
  - `true`：随机起点（由后端决定随机 page）
  - `false`：按前端传入的 page 继续扫描
- `pageSize`：固定为 `5`（MVP 每次返回 5 条）

### 4.2 Bloom Key（按用户 + 当天轮转）

- 当日 Key：`dayKey = time.Now().Format("20060102")`
- Bloom Key 组织为：
  - `bf:randomfeed:<userID>:<dayKey>`

这样可保证：
- 同一个用户在同一天内尽量不会重复
- 不同用户不会互相污染去重结果
- 明天自动轮转（Bloom 不会无限增长导致误判迅速膨胀）

### 4.3 获取用户 & 初始化点赞状态

Service 中会调用：
- `utils.GetUserByUsername(username)` -> 得到 `userID`
- `utils.GetUserLikedVideoIDs(userID)` -> 构造 `likedMap`

用于把返回的视频条目补齐 `is_liked`（前端点赞按钮依赖该字段）。

### 4.4 随机起点（init=true）

Service 先粗略计算已上传视频总量（用于估算 maxPage）：
- `utils.GetUploadedVideoCount()`

然后：
- `maxPage = ceil(totalCount / pageSize)`
- 若 `init=true`：
  - `page = random(1..maxPage)`

### 4.5 候选扫描与 Bloom 去重（核心逻辑）

参数：
- `maxScanPages = 10`：最多扫描 10 页的候选，控制开销/避免长时间扫不到足够数量

循环扫描：

1. 获取候选页：
   - `utils.GetVideoListPage(curPage, pageSize)`
   - 数据来源：`status = Uploaded`，`created_at DESC` 分页
2. 对每个候选视频 `cand.ID`：
   - `BloomMightContain(userID, dayKey, cand.ID)`
     - Bloom 判定“可能存在” => 认为可能已看过 => 跳过
     - Bloom 判定“肯定不存在” => 接受并写入 Bloom
   - `BloomAdd(userID, dayKey, cand.ID)`（写入位图）
   - 补齐 `cand.IsLiked = likedMap[cand.ID]`
   - 收集到 `collected`，直到达到 `pageSize=5`

### 4.6 fallback（避免返回 0 条导致前端空白）

当 Bloom 去重扫描后 `collected` 仍为 0 时，为避免前端 Feed 直接空白：
- 进行 fallback 扫描：仍按页获取候选、仍写入 Bloom

> 说明：这是 MVP 体验兜底；Bloom 本质是概率结构，可能出现误判导致短时返回空。

### 4.7 has_more / next_page 的近似策略

- `nextPage`：在成功收集足够条数时设置为当前扫描结束页的下一页
- `has_more`：近似判断
  - 若 `collected` 为 0 -> `has_more = false`
  - 若未满 `pageSize` -> 允许 `nextPage <= maxPage` 的情况仍返回 `has_more = true`

---

## 5. 数据访问层：候选视频分页查询

文件：`backend/utils/mysql.go`

### 5.1 `GetVideoListPage(page, pageSize)`

- 条件：
  - `models.VideoStatusUploaded`
- 排序：
  - `created_at DESC`
- 分页：
  - `offset = (page - 1) * pageSize`
  - `limit = pageSize`
- 组装字段：
  - `VideoListItem`：包含 `ID/title/url720p/url1080p/cover_url/username/avatar/created_at/likes/comments`

### 5.2 `GetUploadedVideoCount()`

- 用于计算 maxPage（粗略 has_more）

---

## 6. 布隆过滤器实现（Redis bitmap）

文件：`backend/utils/bloom.go`

### 6.1 使用 SETBIT/GETBIT 操作位图

- `BloomMightContain()`：
  - 对 k 个索引位做 `GETBIT`
  - 如果存在任意一个位为 0 => “肯定不存在”（返回 false）
  - 如果 k 个都为 1 => “可能存在”（返回 true）

- `BloomAdd()`：
  - 对 k 个索引位执行 `SETBIT(..., 1)`

### 6.2 哈希与参数

参数来自 `backend/config/config.go`：
- `BloomRandomFeedBitsM`：bitmap bit 数（m）
- `BloomRandomFeedHashK`：哈希次数（k）

哈希方式：
- 基于 `fnv1a64` 做双重哈希：
  - `index_i = (h1 + i*h2) % m`

---

## 7. 前端“流式播放”的触发方式（补充说明）

后端返回的视频一旦被前端追加到 `#feed` 后：
- 前端通过 IntersectionObserver 监听“最后一条 feed-item”进入视野
- 触发后端 `POST /api/random-feed/next (init=false, page=next_page)`

因此后端提供的关键字段是：
- `next_page`：下一次扫描起点页
- `has_more`：是否继续加载

---

## 8. 可能的行为差异/边界情况

1. Bloom 是概率结构：
   - 可能误判“已看过”导致少量视频被跳过
2. fallback：
   - 在 Bloom 导致短时返回为空时，仍会尽量保证每次返回 5 条
3. has_more 是近似策略：
   - 可能在接近末尾时多请求一次，或提前停止一次

