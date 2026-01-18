# Cwatch 视频平台 API 文档

## 基础信息

- **Base URL**: `http://localhost:5000`
- **Content-Type**: `application/json`
- **认证方式**: Bearer Token (JWT)

---

## 1. 用户认证相关

### 1.1 用户注册

**接口地址**: `POST /api/register`

**请求参数**:
```json
{
    "username": "string",  // 用户名，至少3个字符
    "password": "string"   // 密码，至少6个字符
}
```

**请求示例**:
```javascript
const res = await fetch("http://localhost:5000/api/register", {
    method: "POST",
    headers: {
        "Content-Type": "application/json"
    },
    body: JSON.stringify({
        username: "testuser",
        password: "123456"
    })
});
```

**成功响应** (200):
```json
{
    "message": "注册成功",
    "user": {
        "id": 1,
        "username": "testuser",
        "avatar_url": "https://i0.hdslb.com/bfs/static/jinkela/long/images/live.gif",
        "email": "",
        "phone": ""
    }
}
```

**错误响应** (400):
```json
{
    "error": "用户名已存在" // 或其他错误信息
}
```

---

### 1.2 用户登录

**接口地址**: `POST /api/login`

**请求参数**:
```json
{
    "username": "string",  // 用户名
    "password": "string"   // 密码
}
```

**请求示例**:
```javascript
const res = await fetch("http://localhost:5000/api/login", {
    method: "POST",
    headers: {
        "Content-Type": "application/json"
    },
    body: JSON.stringify({
        username: "testuser",
        password: "123456"
    })
});
```

**成功响应** (200):
```json
{
    "message": "登录成功",
    "user": {
        "id": 1,
        "username": "testuser",
        "avatar_url": "https://i0.hdslb.com/bfs/static/jinkela/long/images/live.gif",
        "email": "",
        "phone": ""
    },
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."  // JWT Token，有效期24小时
}
```

**错误响应** (401):
```json
{
    "error": "用户名或密码错误"
}
```

---

### 1.3 获取用户信息

**接口地址**: `GET /api/user/info`

**请求头**:
```
Authorization: Bearer <token>
```

**请求示例**:
```javascript
const res = await fetch("http://localhost:5000/api/user/info", {
    method: "GET",
    headers: {
        "Authorization": `Bearer ${token}`
    }
});
```

**成功响应** (200):
```json
{
    "user": {
        "id": 1,
        "username": "testuser",
        "avatar_url": "https://i0.hdslb.com/bfs/static/jinkela/long/images/live.gif",
        "email": "",
        "phone": ""
    }
}
```

**错误响应** (401):
```json
{
    "error": "无效的认证令牌"  // Token无效或已过期
}
```

---

### 1.4 用户登出

**接口地址**: `POST /api/logout`

**请求头**:
```
Authorization: Bearer <token>
```

**请求示例**:
```javascript
const res = await fetch("http://localhost:5000/api/logout", {
    method: "POST",
    headers: {
        "Authorization": `Bearer ${token}`
    }
});
```

**成功响应** (200):
```json
{
    "message": "登出成功"
}
```

**说明**: 登出后Token会被加入黑名单，无法再次使用

---

## 2. 视频相关

### 2.1 获取视频列表

**接口地址**: `GET /api/videos`

**查询参数**:
- `page`: 页码，默认为1
- `page_size`: 每页数量，默认为12，最大50

**请求示例**:
```javascript
const res = await fetch("http://localhost:5000/api/videos?page=1&page_size=12");
```

**成功响应** (200):
```json
{
    "videos": [
        {
            "id": 1,
            "title": "我的第一个视频",
            "description": "",
            "url": "http://101.132.25.34:9000/cwatch/a1b2c3d4-e5f6-7890-abcd-ef1234567890.mp4?X-Amz-Algorithm=...",
            "cover_url": "",
            "user_id": 1,
            "username": "testuser",
            "avatar_url": "https://i0.hdslb.com/bfs/static/jinkela/long/images/live.gif",
            "created_at": "2026-01-17 15:30",
            "likes": 0,
            "comments": 0
        }
    ],
    "total": 1,
    "page": 1,
    "page_size": 12
}
```

**说明**: 
- 只返回状态为"上传完成"的视频
- 按创建时间倒序排列
- 包含作者信息和统计数据

---

### 2.2 获取视频上传凭证

**接口地址**: `POST /api/video/upload-url`

**请求头**:
```
Authorization: Bearer <token>
Content-Type: application/json
```

**请求参数**:
```json
{
    "filename": "string",  // 原始文件名，如"我的视频.mp4"
    "filesize": number,    // 文件大小（字节），如52428800
    "title": "string"      // 视频标题（可选），如"我的第一个视频"
}
```

**请求示例**:
```javascript
const res = await fetch("http://localhost:5000/api/video/upload-url", {
    method: "POST",
    headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${token}`
    },
    body: JSON.stringify({
        filename: "my_video.mp4",
        filesize: 52428800,
        title: "我的第一个视频"
    })
});
```

**成功响应** (200):
```json
{
    "upload_url": "http://101.132.25.34:9000/cwatch/a1b2c3d4-e5f6-7890-abcd-ef1234567890.mp4?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=...",
    "video_id": 1
}
```

**错误响应** (400):
```json
{
    "error": "不支持的视频格式，仅支持 mp4, webm, mov, avi, mkv"
}
```

**说明**:
- 支持的视频格式: `.mp4`, `.webm`, `.mov`, `.avi`, `.mkv`
- 文件大小限制: 最大500MB
- `upload_url`有效期: 15分钟
- 返回的`video_id`用于后续确认上传

---

### 2.3 直传视频到MinIO

**接口地址**: 使用上一步返回的`upload_url`

**请求方法**: `PUT`

**请求体**: 视频文件的二进制数据

**请求示例**:
```javascript
const xhr = new XMLHttpRequest();

// 监听上传进度
xhr.upload.addEventListener("progress", (e) => {
    if (e.lengthComputable) {
        const percent = Math.round((e.loaded / e.total) * 100);
        console.log(`上传进度: ${percent}%`);
    }
});

// 上传完成处理
xhr.onload = () => {
    if (xhr.status >= 200 && xhr.status < 300) {
        console.log("上传成功");
    } else {
        console.log("上传失败");
    }
};

xhr.open("PUT", upload_url);
xhr.send(videoFile);  // videoFile 是 File 对象
```

**成功响应**: HTTP 200 OK

**说明**:
- 直接上传到MinIO服务器，不经过后端
- 支持进度监听
- 上传完成后需要调用确认接口

---

### 2.4 确认视频上传完成

**接口地址**: `POST /api/video/upload-complete`

**请求头**:
```
Authorization: Bearer <token>
Content-Type: application/json
```

**请求参数**:
```json
{
    "video_id": number  // 获取上传凭证时返回的video_id
}
```

**请求示例**:
```javascript
const res = await fetch("http://localhost:5000/api/video/upload-complete", {
    method: "POST",
    headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${token}`
    },
    body: JSON.stringify({
        video_id: 1
    })
});
```

**成功响应** (200):
```json
{
    "success": true,
    "video_url": "http://101.132.25.34:9000/cwatch/a1b2c3d4-e5f6-7890-abcd-ef1234567890.mp4?X-Amz-Algorithm=..."
}
```

**错误响应** (400):
```json
{
    "error": "文件未上传成功，请重新上传"
}
```

**说明**:
- 验证文件是否真的上传到MinIO
- 更新数据库中视频状态为"上传完成"
- 返回视频播放URL，有效期24小时

---

## 3. 错误码说明

| HTTP状态码 | 说明 | 常见原因 |
|-----------|------|---------|
| 200 | 成功 | 请求处理成功 |
| 400 | 请求错误 | 参数格式错误、业务逻辑错误 |
| 401 | 未授权 | Token无效、未登录、Token过期 |
| 403 | 禁止访问 | 无权限操作资源 |
| 404 | 资源不存在 | 请求的资源不存在 |
| 500 | 服务器错误 | 服务器内部错误 |

---

## 4. 前端使用示例

### 4.1 完整的视频上传流程

```javascript
async function uploadVideo(videoFile, title) {
    const token = localStorage.getItem("cwatchToken");
    
    try {
        // 1. 获取上传凭证
        const urlRes = await fetch("http://localhost:5000/api/video/upload-url", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${token}`
            },
            body: JSON.stringify({
                filename: videoFile.name,
                filesize: videoFile.size,
                title: title
            })
        });
        
        const { upload_url, video_id } = await urlRes.json();
        
        // 2. 上传文件到MinIO
        const xhr = new XMLHttpRequest();
        await new Promise((resolve, reject) => {
            xhr.onload = () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    resolve();
                } else {
                    reject(new Error("上传失败"));
                }
            };
            xhr.onerror = () => reject(new Error("网络错误"));
            xhr.open("PUT", upload_url);
            xhr.send(videoFile);
        });
        
        // 3. 确认上传完成
        const confirmRes = await fetch("http://localhost:5000/api/video/upload-complete", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${token}`
            },
            body: JSON.stringify({ video_id })
        });
        
        const result = await confirmRes.json();
        console.log("上传成功:", result.video_url);
        
    } catch (error) {
        console.error("上传失败:", error.message);
    }
}
```

### 4.2 获取并显示视频列表

```javascript
async function loadVideoList(page = 1) {
    try {
        const res = await fetch(`http://localhost:5000/api/videos?page=${page}&page_size=12`);
        const data = await res.json();
        
        data.videos.forEach(video => {
            console.log(`视频: ${video.title}, 作者: ${video.username}, 点赞: ${video.likes}`);
        });
        
        return data;
    } catch (error) {
        console.error("获取视频列表失败:", error);
    }
}
```

---

## 5. 注意事项

1. **Token管理**: JWT Token有效期为24小时，过期后需要重新登录
2. **文件限制**: 视频文件最大500MB，支持格式有限
3. **URL有效期**: 
   - 上传URL有效期15分钟
   - 播放URL有效期24小时
4. **错误处理**: 前端需要妥善处理各种错误情况
5. **进度显示**: 上传大文件时建议显示进度条
6. **网络重试**: 建议在网络错误时实现重试机制

---

## 6. 服务器配置

- **后端服务**: `http://localhost:5000`
- **MinIO服务**: `http://101.132.25.34:9000`
- **MySQL数据库**: `101.132.25.34:3306`
- **Redis缓存**: `101.132.25.34:6379`