/**
 * Short Video App - Modular Vanilla JS
 * - 首页：预览式缩略图信息流（不自动播放）
 * - 点击缩略图：进入全屏瀑布流播放页（纵向一屏一条）
 * - 进度条拖动 seek
 * - 评论/分享弹层
 * - 手势上滑/下滑切换（移动端）
 * - 双击点赞动画
 */

/* =========================
   1) Data (集中管理)
========================= */
const DATA = {
    shareApps: [],
    videos: [] // 视频数据从服务器获取
};

/* =========================
   2) DOM & State
========================= */
const el = {
    // pages
    previewPage: document.getElementById("previewPage"),
    playerPage: document.getElementById("playerPage"),
    // containers
    previewGrid: document.getElementById("previewGrid"),
    feed: document.getElementById("feed"),
    hint: document.getElementById("hint"),
    // topbar
    btnHome: document.getElementById("btnHome"),
    // sidebar (placeholders)
    navExplore: document.getElementById("navExplore"),
    navUpload: document.getElementById("navUpload"),
    navFav: document.getElementById("navFav"),
    navSetting: document.getElementById("navSetting"),
    // sheets
    sheetMask: document.getElementById("sheetMask"),
    commentSheet: document.getElementById("commentSheet"),
    shareSheet: document.getElementById("shareSheet"),
    commentList: document.getElementById("commentList"),
    commentInput: document.getElementById("commentInput"),
    commentSend: document.getElementById("commentSend"),
    shareGrid: document.getElementById("shareGrid"),
    shareLink: document.getElementById("shareLink"),
    copyBtn: document.getElementById("copyBtn"),
    // 新增：登录注册相关元素
    authPage: document.getElementById("authPage"),
    authClose: document.getElementById("authClose"),
    loginForm: document.getElementById("loginForm"),
    registerForm: document.getElementById("registerForm"),
    loginUsername: document.getElementById("loginUsername"),
    loginPassword: document.getElementById("loginPassword"),
    registerUsername: document.getElementById("registerUsername"),
    registerPassword: document.getElementById("registerPassword"),
    registerPasswordConfirm: document.getElementById("registerPasswordConfirm"),
    showRegister: document.getElementById("showRegister"),
    showLogin: document.getElementById("showLogin"),
    avatar: document.querySelector(".avatar"),
    // 新增：用户下拉菜单相关元素
    avatarWrapper: document.querySelector(".avatar-wrapper"),
    avatarDropdown: document.getElementById("avatarDropdown"),
    dropdownUserInfo: document.getElementById("dropdownUserInfo"),
    dropdownLogout: document.getElementById("dropdownLogout"),
    // 新增：视频上传相关元素
    uploadModal: document.getElementById("uploadModal"),
    uploadModalClose: document.getElementById("uploadModalClose"),
    uploadDropzone: document.getElementById("uploadDropzone"),
    uploadFileInput: document.getElementById("uploadFileInput"),
    uploadFileInfo: document.getElementById("uploadFileInfo"),
    uploadVideoPreview: document.getElementById("uploadVideoPreview"),
    uploadFileName: document.getElementById("uploadFileName"),
    uploadFileSize: document.getElementById("uploadFileSize"),
    uploadChangeFile: document.getElementById("uploadChangeFile"),
    uploadTitle: document.getElementById("uploadTitle"),
    uploadProgress: document.getElementById("uploadProgress"),
    uploadProgressFill: document.getElementById("uploadProgressFill"),
    uploadProgressText: document.getElementById("uploadProgressText"),
    uploadCancel: document.getElementById("uploadCancel"),
    uploadSubmit: document.getElementById("uploadSubmit"),
};

const state = {
    currentIndex: -1,
    currentVideoEl: null,
    currentFeedItemEl: null,
    currentVideoData: null,
    io: null, // IntersectionObserver (player page)
    // 新增：用户登录状态
    isLoggedIn: false,
    currentUser: null,
    eventsInitialized: false, // 标记全局事件是否已初始化
    previewRendered: false, // 标记预览网格是否已渲染
    // 新增：上传状态
    uploadFile: null, // 当前选择的文件
    isUploading: false, // 是否正在上传
    // 新增：视频列表状态
    videoList: [], // 服务器视频列表
    videoPage: 1, // 当前页码
    videoPageSize: 12, // 每页数量
    videoTotal: 0, // 总数
    isLoadingVideos: false, // 是否正在加载
    hasMoreVideos: true, // 是否还有更多
};

/* =========================
   3) Utils
========================= */
// 简化的 querySelector，返回单个元素
function $(selector, root = document){ return root.querySelector(selector); }

// 简化的 querySelectorAll，返回数组形式的所有匹配元素
function $all(selector, root = document){ return Array.from(root.querySelectorAll(selector)); }

// 格式化数字：将大数字转换为 K（千）或 M（百万）格式
function formatCount(n) {
    const num = Number(n) || 0;
    if (num >= 1_000_000) return (num / 1_000_000).toFixed(1).replace(/\.0$/, "") + "M";
    if (num >= 1_000) return (num / 1_000).toFixed(1).replace(/\.0$/, "") + "K";
    return String(num);
}

// 将数字补齐为两位数（例如：5 -> "05"）
function pad2(n){ return String(n).padStart(2, "0"); }

// 格式化时间：将秒数转换为 "分:秒" 格式（例如：125 -> "2:05"）
function formatTime(sec){
    if (!isFinite(sec)) return "0:00";
    sec = Math.max(0, Math.floor(sec));
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    return `${m}:${pad2(s)}`;
}

let toastTimer = null;
// 显示顶部提示消息（Toast 通知）
function toast(msg){
    let t = document.getElementById("toast");
    if (!t){
        t = document.createElement("div");
        t.id = "toast";
        t.style.position = "fixed";
        t.style.left = "50%";
        t.style.top = "72px";
        t.style.transform = "translateX(-50%)";
        t.style.padding = "10px 12px";
        t.style.borderRadius = "999px";
        t.style.border = "1px solid rgba(255,255,255,0.12)";
        t.style.background = "rgba(0,0,0,0.45)";
        t.style.backdropFilter = "blur(12px)";
        t.style.color = "rgba(255,255,255,0.92)";
        t.style.fontSize = "12px";
        t.style.zIndex = "9999";
        t.style.maxWidth = "80vw";
        t.style.textAlign = "center";
        document.body.appendChild(t);
    }
    t.textContent = msg;
    t.style.opacity = "1";
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (t.style.opacity = "0"), 1100);
}

/* =========================
   4) Icons
========================= */
// 生成各种图标的 SVG 代码（点赞、评论、分享、声音）
function iconSvg(type){
    const common = `fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"`;
    if (type === "like") {
        return `<svg width="22" height="22" viewBox="0 0 24 24">
      <path ${common} d="M20.84 4.61c-1.54-1.37-3.99-1.19-5.4.39L12 8.44 8.56 5c-1.41-1.58-3.86-1.76-5.4-.39-1.76 1.56-1.85 4.26-.2 5.93L12 21l9.04-10.46c1.65-1.67 1.56-4.37-.2-5.93Z"/>
    </svg>`;
    }
    if (type === "comment") {
        return `<svg width="22" height="22" viewBox="0 0 24 24">
      <path ${common} d="M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4v8Z"/>
    </svg>`;
    }
    if (type === "share") {
        return `<svg width="22" height="22" viewBox="0 0 24 24">
      <path ${common} d="M12 5v14"/>
      <path ${common} d="M19 12l-7-7-7 7"/>
    </svg>`;
    }
    if (type === "sound") {
        return `<svg width="18" height="18" viewBox="0 0 24 24">
      <path ${common} d="M11 5 6 9H2v6h4l5 4V5Z"/>
      <path ${common} d="M15.5 8.5a5 5 0 0 1 0 7"/>
      <path ${common} d="M18.5 5.5a9 9 0 0 1 0 13"/>
    </svg>`;
    }
    if (type === "muted") {
        return `<svg width="18" height="18" viewBox="0 0 24 24">
      <path ${common} d="M11 5 6 9H2v6h4l5 4V5Z"/>
      <path ${common} d="M15.5 8.5a5 5 0 0 1 0 7"/>
      <path ${common} d="M18.5 5.5a9 9 0 0 1 0 13"/>
      <line ${common} x1="3" y1="3" x2="21" y2="21" stroke-width="2.5" opacity="0.8"/>
    </svg>`;
    }
    return "";
}

// 生成点赞动画用的爱心 SVG
function heartSvg(){
    return `
  <svg viewBox="0 0 24 24" width="120" height="120" fill="none">
    <path d="M20.84 4.61c-1.54-1.37-3.99-1.19-5.4.39L12 8.44 8.56 5c-1.41-1.58-3.86-1.76-5.4-.39-1.76 1.56-1.85 4.26-.2 5.93L12 21l9.04-10.46c1.65-1.67 1.56-4.37-.2-5.93Z"
      fill="rgba(255,77,109,0.92)"/>
  </svg>`;
}

// 生成取消点赞动画用的破碎爱心 SVG
function brokenHeartSvg(){
    return `
  <svg viewBox="0 0 24 24" width="120" height="120" fill="none">
    <!-- 主体破碎的爱心 -->
    <path d="M20.84 4.61c-1.54-1.37-3.99-1.19-5.4.39L12 8.44 8.56 5c-1.41-1.58-3.86-1.76-5.4-.39-1.76 1.56-1.85 4.26-.2 5.93L12 21l9.04-10.46c1.65-1.67 1.56-4.37-.2-5.93Z"
      fill="rgba(255,77,109,0.92)" opacity="0.8"/>
    <!-- 裂痕 -->
    <path d="M12 8 L10 12 L12 16 L14 12 Z" fill="rgba(0,0,0,0.3)" stroke="rgba(0,0,0,0.5)" stroke-width="0.5"/>
    <line x1="12" y1="8" x2="12" y2="21" stroke="rgba(0,0,0,0.4)" stroke-width="1"/>
    <line x1="8" y1="10" x2="16" y2="14" stroke="rgba(0,0,0,0.3)" stroke-width="0.8"/>
  </svg>
  <!-- 碎片 -->
  <svg class="fragment" viewBox="0 0 24 24" width="30" height="30" style="left: 20px; top: 15px;">
    <path d="M12 5 L8 9 L12 13 Z" fill="rgba(255,77,109,0.7)"/>
  </svg>
  <svg class="fragment" viewBox="0 0 24 24" width="30" height="30" style="left: 70px; top: 15px;">
    <path d="M12 5 L16 9 L12 13 Z" fill="rgba(255,77,109,0.7)"/>
  </svg>
  <svg class="fragment" viewBox="0 0 24 24" width="25" height="25" style="left: 15px; top: 70px;">
    <path d="M8 12 L12 16 L8 20 Z" fill="rgba(255,77,109,0.6)"/>
  </svg>
  <svg class="fragment" viewBox="0 0 24 24" width="25" height="25" style="left: 75px; top: 70px;">
    <path d="M16 12 L12 16 L16 20 Z" fill="rgba(255,77,109,0.6)"/>
  </svg>
  `;
}

/* =========================
   5) Page Router (Preview <-> Player)
========================= */
// 切换页面显示（预览页 或 播放器页）
function showPage(page){
    const isPreview = page === "preview";
    el.previewPage.classList.toggle("page--active", isPreview);
    el.playerPage.classList.toggle("page--active", !isPreview);

    // 进入预览页：确保暂停所有视频
    if (isPreview) pauseAllVideos();
}

// 打开播放器页面并跳转到指定索引的视频
function openPlayerAt(index){
    showPage("player");
    // 确保 feed 已渲染
    if (!el.feed.dataset.rendered) {
        renderFeed(DATA.videos);
        initPlayerAutoPlayObserver();
        el.feed.dataset.rendered = "1";
    }

    // 滚动到指定视频
    const items = $all(".feed-item", el.feed);
    const target = items[index];
    if (target) target.scrollIntoView({ behavior: "auto", block: "start" });

    // 设置当前状态并自动播放
    setCurrentByIndex(index);
    
    // 延迟一下确保视频元素已经设置好，然后自动播放
    setTimeout(() => {
        if (state.currentVideoEl) {
            autoPlayCurrentVideo();
        }
    }, 100);
}

/* =========================
   6) Render: Preview Grid (首页缩略图)
========================= */

// 从服务器获取视频列表
async function fetchVideoList(page = 1, append = false) {
    if (state.isLoadingVideos) return;
    
    state.isLoadingVideos = true;
    
    try {
        // 构建请求URL，如果已登录则带上 token
        const token = localStorage.getItem("cwatchToken");
        const headers = {};
        if (token) {
            headers["Authorization"] = `Bearer ${token}`;
        }
        
        const res = await fetch(`http://localhost:5000/api/videos?page=${page}&page_size=${state.videoPageSize}`, {
            headers: headers
        });
        
        if (!res.ok) {
            throw new Error("获取视频列表失败");
        }
        
        const data = await res.json();
        
        // 转换服务器数据格式为前端格式
        const serverVideos = (data.videos || []).map(v => ({
            id: `server_${v.id}`,
            serverId: v.id,
            src: v.url,
            url_720p: v.url_720p,      // 720p视频URL
            url_1080p: v.url_1080p,    // 1080p视频URL
            title: v.title || "未命名视频",
            author: `@${v.username || "用户"}`,
            likes: v.likes || 0,
            comments: v.comments || 0,
            shares: 0,
            thumbText: "▶",
            coverUrl: v.cover_url,
            commentItems: [],
            isLiked: v.is_liked || false, // 使用服务器返回的点赞状态
        }));
        
        if (append) {
            // 分页加载：追加到现有列表
            state.videoList = [...state.videoList, ...serverVideos];
        } else {
            // 首次加载：重置列表
            state.videoList = serverVideos;
        }
        
        state.videoPage = page;
        state.videoTotal = data.total || 0;
        state.hasMoreVideos = state.videoList.length < state.videoTotal;
        
        // 更新全局数据源（只使用服务器数据）
        DATA.videos = state.videoList;
        
        // 渲染预览网格
        renderPreviewGrid(DATA.videos, append);
        
    } catch (err) {
        console.error("获取视频列表失败:", err);
        toast("获取视频列表失败，请检查网络连接");
        
        // 如果获取失败且是首次加载，显示空状态
        if (!append) {
            DATA.videos = [];
            renderPreviewGrid(DATA.videos, false);
        }
    } finally {
        state.isLoadingVideos = false;
    }
}

// 从 localStorage 获取已点赞的视频ID列表
function getLikedVideosFromStorage() {
    try {
        const username = state.currentUser?.username;
        if (!username) return [];
        
        const key = `likedVideos_${username}`;
        const stored = localStorage.getItem(key);
        return stored ? JSON.parse(stored) : [];
    } catch (err) {
        console.error("获取点赞状态失败:", err);
        return [];
    }
}

// 保存点赞状态到 localStorage
function saveLikedVideoToStorage(videoId, isLiked) {
    try {
        const username = state.currentUser?.username;
        if (!username) return;
        
        const key = `likedVideos_${username}`;
        let likedVideos = getLikedVideosFromStorage();
        
        if (isLiked) {
            // 添加到点赞列表
            if (!likedVideos.includes(videoId)) {
                likedVideos.push(videoId);
            }
        } else {
            // 从点赞列表移除
            likedVideos = likedVideos.filter(id => id !== videoId);
        }
        
        localStorage.setItem(key, JSON.stringify(likedVideos));
    } catch (err) {
        console.error("保存点赞状态失败:", err);
    }
}

// 加载更多视频
async function loadMoreVideos() {
    if (!state.hasMoreVideos || state.isLoadingVideos) return;
    await fetchVideoList(state.videoPage + 1, true);
}

// 渲染预览页的视频缩略图网格
function renderPreviewGrid(videos, append = false){
    if (!append) {
        el.previewGrid.innerHTML = "";
    }
    
    const frag = document.createDocumentFragment();
    
    if (append) {
        // 分页加载：只渲染新增的视频
        const existingCount = el.previewGrid.children.length;
        const newVideos = videos.slice(existingCount);
        
        newVideos.forEach((v, idx) => {
            const realIdx = existingCount + idx;
            const card = createPreviewCard(v, realIdx);
            frag.appendChild(card);
        });
    } else {
        // 首次加载：渲染所有视频
        videos.forEach((v, idx) => {
            const card = createPreviewCard(v, idx);
            frag.appendChild(card);
        });
    }

    el.previewGrid.appendChild(frag);
    
    // 如果没有视频，显示提示
    if (videos.length === 0) {
        el.previewGrid.innerHTML = `
            <div class="preview-empty">
                <p>暂无视频</p>
                <p>点击左侧上传按钮上传第一个视频吧！</p>
            </div>
        `;
    }
}

// 创建预览卡片的辅助函数
function createPreviewCard(videoData, index) {
    const card = document.createElement("article");
    card.className = "preview-card";
    card.dataset.index = String(index);

    // 如果有封面图则显示封面图
    const thumbContent = videoData.coverUrl 
        ? `<img src="${escapeHtml(videoData.coverUrl)}" alt="封面" class="preview-card__cover">`
        : `<div class="preview-card__play">${videoData.thumbText || "▶"}</div>`;

    card.innerHTML = `
      <div class="preview-card__thumb">
        ${thumbContent}
        <div class="preview-card__play-overlay">▶</div>
      </div>
      <div class="preview-card__meta">
        <div class="preview-card__title">${escapeHtml(videoData.title)}</div>
        <div class="preview-card__author">${escapeHtml(videoData.author)} · ${formatCount(videoData.likes)} 赞</div>
      </div>
    `;

    card.addEventListener("click", () => openPlayerAt(index));
    return card;
}

/* =========================
   7) Render: Feed (瀑布流播放列表)
========================= */
// 渲染播放器页面的视频列表（瀑布流）
function renderFeed(videos){
    el.feed.innerHTML = "";
    const frag = document.createDocumentFragment();

    videos.forEach((v, idx) => {
        frag.appendChild(createFeedItem(v, idx));
    });

    el.feed.appendChild(frag);
}

// 创建单个视频播放项（包含视频、信息、按钮、进度条等）
function createFeedItem(videoData, index){
    const item = document.createElement("article");
    item.className = "feed-item";
    item.dataset.index = String(index);
    item.dataset.videoId = videoData.id;

    item.innerHTML = `
    <div class="video-card">
      <video class="video-media" playsinline preload="metadata" loop>
        <source src="${videoData.src}">
        你的浏览器不支持 video 标签
      </video>

      <div class="video-overlay"></div>

      <div class="video-info">
        <div class="video-author">
          <span>${escapeHtml(videoData.author)}</span>
        </div>
        <div class="video-title">${escapeHtml(videoData.title)}</div>
      </div>

      <div class="video-actions">
        <button class="video-action" data-action="like" aria-label="Like">
          <span class="video-action__icon">${iconSvg("like")}</span>
          <span class="video-action__count" data-count="likes">${formatCount(videoData.likes)}</span>
        </button>

        <button class="video-action" data-action="comment" aria-label="Comment">
          <span class="video-action__icon">${iconSvg("comment")}</span>
          <span class="video-action__count" data-count="comments">${formatCount(videoData.comments)}</span>
        </button>

        <button class="video-action" data-action="share" aria-label="Share">
          <span class="video-action__icon">${iconSvg("share")}</span>
          <span class="video-action__count" data-count="shares">${formatCount(videoData.shares)}</span>
        </button>

        <button class="video-action" data-action="mute" aria-label="Mute/Unmute">
          <span class="video-action__icon">${iconSvg("sound")}</span>
          <span class="video-action__count">声音</span>
        </button>

        <button class="video-action" data-action="quality" aria-label="Quality">
          <span class="video-action__icon">HD</span>
          <span class="video-action__count" data-quality-label>自动</span>
        </button>
      </div>

      <!-- 清晰度选择菜单 -->
      <div class="video-quality-menu" data-quality-menu hidden>
        <div class="video-quality-menu__header">选择清晰度</div>
        <div class="video-quality-menu__list">
          <div class="video-quality-menu__item video-quality-menu__item--active" data-quality="auto">
            <span class="video-quality-menu__label">自动</span>
            <span class="video-quality-menu__check">✓</span>
          </div>
          <div class="video-quality-menu__item" data-quality="1080p">
            <span class="video-quality-menu__label">1080p 高清</span>
            <span class="video-quality-menu__check">✓</span>
          </div>
          <div class="video-quality-menu__item" data-quality="720p">
            <span class="video-quality-menu__label">720p 标清</span>
            <span class="video-quality-menu__check">✓</span>
          </div>
          <div class="video-quality-menu__item" data-quality="original">
            <span class="video-quality-menu__label">原画</span>
            <span class="video-quality-menu__check">✓</span>
          </div>
        </div>
      </div>

      <div class="video-progress" aria-label="Progress">
        <div class="video-progress__time" data-time="cur">0:00</div>
        <div class="video-progress__bar" data-progress-bar>
          <div class="video-progress__fill" data-progress-fill></div>
          <div class="video-progress__knob" data-progress-knob></div>
        </div>
        <div class="video-progress__time" data-time="dur">0:00</div>
      </div>

      <div class="video-like-burst" aria-hidden="true">${heartSvg()}</div>
      <div class="video-unlike-burst" aria-hidden="true">${brokenHeartSvg()}</div>
      <div class="video-pause-icon"></div>
    </div>
  `;

    const cardEl = $(".video-card", item);
    const videoEl = $(".video-media", item);

    // 点击视频区域：播放/暂停（不影响进度条/按钮）
    cardEl.addEventListener("click", (e) => {
        if (e.target.closest(".video-actions")) return;
        if (e.target.closest(".video-progress")) return;
        togglePlay(videoEl);
    });

    // 双击点赞（桌面）
    cardEl.addEventListener("dblclick", () => likeWithAnimation(item, videoData));

    // 右侧按钮事件
    $(".video-actions", item).addEventListener("click", (e) => {
        const btn = e.target.closest(".video-action");
        if (!btn) return;

        const action = btn.dataset.action;

        if (action === "mute") {
            videoEl.muted = !videoEl.muted;
            
            // 更新声音按钮图标
            const iconEl = btn.querySelector('.video-action__icon');
            if (iconEl) {
                iconEl.innerHTML = iconSvg(videoEl.muted ? "muted" : "sound");
            }
            
            toast(videoEl.muted ? "已静音" : "已开启声音");
            return;
        }

        if (action === "quality") {
            toggleQualityMenu(item);
            return;
        }

        if (action === "like") {
            likeWithAnimation(item, videoData);
            return;
        }

        if (action === "comment") {
            openCommentSheet(videoData);
            return;
        }

        if (action === "share") {
            openShareSheet(videoData);
            return;
        }
    });

    setupProgress(item, videoEl);
    setupGestures(item, cardEl, videoData);
    
    // 初始化声音按钮图标状态
    const muteBtn = item.querySelector('[data-action="mute"]');
    if (muteBtn) {
        const iconEl = muteBtn.querySelector('.video-action__icon');
        if (iconEl) {
            iconEl.innerHTML = iconSvg(videoEl.muted ? "muted" : "sound");
        }
    }
    
    // 初始化点赞按钮状态
    const likeBtn = item.querySelector('[data-action="like"]');
    if (likeBtn && videoData.isLiked) {
        likeBtn.classList.add('video-action--liked');
    }
    
    // 绑定清晰度菜单项点击事件
    const qualityMenu = $('[data-quality-menu]', item);
    if (qualityMenu) {
        $all('.video-quality-menu__item', qualityMenu).forEach(menuItem => {
            menuItem.addEventListener('click', (e) => {
                e.stopPropagation();
                const quality = menuItem.dataset.quality;
                switchVideoQuality(item, quality, videoData);
                qualityMenu.hidden = true;
            });
        });
    }
    
    // 根据视频数据设置可用的清晰度选项
    if (qualityMenu) {
        // 如果没有1080p，禁用该选项
        if (!videoData.url_1080p) {
            const item1080p = $('[data-quality="1080p"]', qualityMenu);
            if (item1080p) {
                item1080p.classList.add('video-quality-menu__item--disabled');
                item1080p.style.opacity = '0.4';
                item1080p.style.cursor = 'not-allowed';
            }
        }
        
        // 如果没有720p，禁用该选项
        if (!videoData.url_720p) {
            const item720p = $('[data-quality="720p"]', qualityMenu);
            if (item720p) {
                item720p.classList.add('video-quality-menu__item--disabled');
                item720p.style.opacity = '0.4';
                item720p.style.cursor = 'not-allowed';
            }
        }
    }

    return item;
}

/* =========================
   8) Player Behavior
========================= */
function initPlayerAutoPlayObserver(){
    // 只负责更新“当前项”，不负责自动播放（满足：进入首页不直接播放）
    state.io = new IntersectionObserver((entries) => {
        const visible = entries
            .filter(en => en.isIntersecting)
            .sort((a,b) => b.intersectionRatio - a.intersectionRatio)[0];

        if (!visible) return;

        const itemEl = visible.target;
        const idx = Number(itemEl.dataset.index);
        
        // 只有当视频真正切换时才处理
        if (idx !== state.currentIndex) {
            setCurrentByIndex(idx);
            
            // 暂停其他视频
            pauseAllExcept(state.currentVideoEl);
            
            // 自动播放当前视频
            setTimeout(() => {
                autoPlayCurrentVideo();
            }, 200);
        }
    }, { root: el.feed, threshold: [0.55, 0.75, 0.9] });

    $all(".feed-item", el.feed).forEach(node => state.io.observe(node));
}

function setCurrentByIndex(index){
    const itemEl = $(`.feed-item[data-index="${index}"]`, el.feed);
    if (!itemEl) return;

    state.currentIndex = index;
    state.currentFeedItemEl = itemEl;
    state.currentVideoEl = $(".video-media", itemEl);
    state.currentVideoData = DATA.videos[index] || null;
}

function pauseAllVideos(){
    $all("video.video-media", document).forEach(v => {
        if (!v.paused) v.pause();
    });
}
function pauseAllExcept(current){
    $all("video.video-media", el.feed).forEach(v => {
        if (v !== current && !v.paused) v.pause();
    });
}
function togglePlay(videoEl){
    if (!videoEl) return;
    const feedItem = videoEl.closest('.feed-item');
    const pauseIcon = feedItem?.querySelector('.video-pause-icon');
    
    if (videoEl.paused) {
        videoEl.play().catch(() => toast("播放被浏览器拦截：请尝试保持静音或手动允许播放"));
        if (pauseIcon) pauseIcon.classList.remove('video-pause-icon--visible');
    } else {
        videoEl.pause();
        if (pauseIcon) pauseIcon.classList.add('video-pause-icon--visible');
    }
}

// 自动播放当前视频
function autoPlayCurrentVideo(){
    if (!state.currentVideoEl) return;
    
    const feedItem = state.currentVideoEl.closest('.feed-item');
    const pauseIcon = feedItem?.querySelector('.video-pause-icon');
    
    // 默认不静音，直接播放
    state.currentVideoEl.muted = false;
    
    // 更新声音按钮图标为有声音状态
    const muteBtn = feedItem?.querySelector('[data-action="mute"]');
    if (muteBtn) {
        const iconEl = muteBtn.querySelector('.video-action__icon');
        if (iconEl) {
            iconEl.innerHTML = iconSvg("sound");
        }
    }
    
    // 尝试自动播放
    state.currentVideoEl.play().then(() => {
        // 播放成功，隐藏暂停图标
        if (pauseIcon) pauseIcon.classList.remove('video-pause-icon--visible');
    }).catch((error) => {
        console.log("自动播放被阻止:", error);
        // 如果浏览器阻止了有声音的自动播放，尝试静音播放
        state.currentVideoEl.muted = true;
        if (muteBtn) {
            const iconEl = muteBtn.querySelector('.video-action__icon');
            if (iconEl) {
                iconEl.innerHTML = iconSvg("muted");
            }
        }
        state.currentVideoEl.play().then(() => {
            if (pauseIcon) pauseIcon.classList.remove('video-pause-icon--visible');
            toast("浏览器阻止了自动播放声音，已静音播放");
        }).catch(() => {
            // 显示暂停图标，提示用户手动播放
            if (pauseIcon) pauseIcon.classList.add('video-pause-icon--visible');
            toast("点击视频开始播放");
        });
    });
}

/* =========================
   9) Like + Animation
========================= */

// 点赞视频（与后端交互）
async function likeVideo(videoData) {
    const token = localStorage.getItem("cwatchToken");
    if (!token) {
        toast("请先登录");
        showAuthPage();
        return;
    }

    // 获取视频ID（优先使用serverId，如果没有则从id中解析）
    let videoId;
    if (videoData.serverId) {
        videoId = videoData.serverId;
    } else if (videoData.id && videoData.id.startsWith('server_')) {
        videoId = parseInt(videoData.id.replace('server_', ''));
    } else {
        console.log("无法获取视频ID，videoData:", videoData);
        toast("无法获取视频ID");
        return;
    }

    console.log("准备点赞 - videoId:", videoId, "videoData:", videoData);

    try {
        const res = await fetch("http://localhost:5000/api/video/toggle-like", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${token}`
            },
            body: JSON.stringify({
                video_id: videoId
            })
        });

        console.log("点赞请求响应状态:", res.status);

        if (!res.ok) {
            const data = await res.json();
            console.error("点赞请求失败:", data);
            throw new Error(data.error || "点赞失败");
        }

        const result = await res.json();
        console.log("点赞请求成功:", result);
        
        // 更新本地数据
        videoData.likes = result.like_count;
        videoData.isLiked = result.is_liked; // 保存点赞状态
        
        // 更新所有显示该视频点赞数和状态的地方
        updateVideoLikeCount(videoData.id, result.like_count, result.is_liked);
        
        // 显示提示
        toast(result.message);
        
        return result;
    } catch (err) {
        console.error("点赞失败:", err);
        toast(err.message || "点赞失败，请检查网络连接");
        return null;
    }
}

// 更新视频点赞数显示
function updateVideoLikeCount(videoId, likeCount, isLiked) {
    // 更新播放页面的点赞数和按钮状态
    const feedItem = $(`.feed-item[data-video-id="${videoId}"]`);
    if (feedItem) {
        const countEl = $('[data-count="likes"]', feedItem);
        if (countEl) countEl.textContent = formatCount(likeCount);
        
        // 更新点赞按钮状态
        const likeBtn = $('[data-action="like"]', feedItem);
        if (likeBtn) {
            if (isLiked) {
                likeBtn.classList.add('video-action--liked');
            } else {
                likeBtn.classList.remove('video-action--liked');
            }
        }
    }
    
    // 更新预览页面的点赞数
    const previewCards = $all('.preview-card');
    previewCards.forEach(card => {
        const index = parseInt(card.dataset.index);
        if (DATA.videos[index] && DATA.videos[index].id === videoId) {
            const authorEl = $('.preview-card__author', card);
            if (authorEl) {
                const authorText = authorEl.textContent;
                const newText = authorText.replace(/\d+(\.\d+)?[KM]?\s*赞/, `${formatCount(likeCount)} 赞`);
                authorEl.textContent = newText;
            }
        }
    });
}

// 点赞动画效果
function likeWithAnimation(feedItemEl, videoData){
    console.log("点击点赞按钮 - videoData:", videoData);
    
    // 调用后端API进行点赞
    likeVideo(videoData).then(result => {
        if (result) {
            // 根据点赞状态播放不同的动画
            if (result.is_liked) {
                // 点赞：显示完整爱心弹出动画
                const likeBurst = $(".video-like-burst", feedItemEl);
                if (likeBurst){
                    likeBurst.classList.remove("is-show");
                    void likeBurst.offsetWidth; // 强制重绘
                    likeBurst.classList.add("is-show");
                }
            } else {
                // 取消点赞：显示破碎爱心动画
                const unlikeBurst = $(".video-unlike-burst", feedItemEl);
                if (unlikeBurst){
                    unlikeBurst.classList.remove("is-show");
                    void unlikeBurst.offsetWidth; // 强制重绘
                    unlikeBurst.classList.add("is-show");
                }
            }
        }
    });
}

/* =========================
   10) Progress + Seek
========================= */
function setupProgress(feedItemEl, videoEl){
    const bar = $("[data-progress-bar]", feedItemEl);
    const fill = $("[data-progress-fill]", feedItemEl);
    const knob = $("[data-progress-knob]", feedItemEl);
    const curEl = $('[data-time="cur"]', feedItemEl);
    const durEl = $('[data-time="dur"]', feedItemEl);
    const wrap = $(".video-progress", feedItemEl);

    let dragging = false;

    function setUI(){
        const dur = videoEl.duration || 0;
        const cur = videoEl.currentTime || 0;
        const p = dur ? (cur / dur) : 0;

        fill.style.width = `${(p * 100).toFixed(4)}%`;
        knob.style.left = `${(p * 100).toFixed(4)}%`;
        curEl.textContent = formatTime(cur);
        durEl.textContent = formatTime(dur);
    }

    videoEl.addEventListener("loadedmetadata", setUI);
    videoEl.addEventListener("timeupdate", () => { if (!dragging) setUI(); });

    function seekByClientX(clientX){
        const rect = bar.getBoundingClientRect();
        const x = Math.min(Math.max(clientX - rect.left, 0), rect.width);
        const p = rect.width ? (x / rect.width) : 0;
        videoEl.currentTime = (videoEl.duration || 0) * p;
        setUI();
    }

    function down(x){
        dragging = true;
        wrap.classList.add("video-progress--dragging");
        seekByClientX(x);
    }
    function move(x){
        if (!dragging) return;
        seekByClientX(x);
    }
    function up(){
        dragging = false;
        wrap.classList.remove("video-progress--dragging");
    }

    // mouse
    bar.addEventListener("mousedown", (e) => {
        e.preventDefault();
        down(e.clientX);
        const onMove = (ev) => move(ev.clientX);
        const onUp = () => {
            up();
            window.removeEventListener("mousemove", onMove);
            window.removeEventListener("mouseup", onUp);
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
    });

    // touch
    bar.addEventListener("touchstart", (e) => {
        const t = e.touches[0];
        if (t) down(t.clientX);
    }, { passive: true });

    bar.addEventListener("touchmove", (e) => {
        const t = e.touches[0];
        if (t) move(t.clientX);
    }, { passive: true });

    bar.addEventListener("touchend", up);
}

/* =========================
   10.5) Quality Selection
========================= */

// 切换清晰度菜单显示/隐藏
function toggleQualityMenu(feedItemEl) {
    const menu = $('[data-quality-menu]', feedItemEl);
    if (!menu) return;
    
    // 切换显示状态
    menu.hidden = !menu.hidden;
    
    // 如果显示菜单，点击其他地方关闭
    if (!menu.hidden) {
        const closeMenu = (e) => {
            if (!menu.contains(e.target) && !e.target.closest('[data-action="quality"]')) {
                menu.hidden = true;
                document.removeEventListener('click', closeMenu);
            }
        };
        // 延迟添加事件，避免立即触发
        setTimeout(() => {
            document.addEventListener('click', closeMenu);
        }, 100);
    }
}

// 切换视频清晰度
function switchVideoQuality(feedItemEl, quality, videoData) {
    const videoEl = $('.video-media', feedItemEl);
    if (!videoEl) return;
    
    // 保存当前播放状态
    const currentTime = videoEl.currentTime;
    const wasPaused = videoEl.paused;
    
    // 确定新的视频源
    let newSrc = '';
    let qualityLabel = '';
    
    switch (quality) {
        case '1080p':
            newSrc = videoData.url_1080p || videoData.src;
            qualityLabel = '1080p';
            break;
        case '720p':
            newSrc = videoData.url_720p || videoData.src;
            qualityLabel = '720p';
            break;
        case 'original':
            newSrc = videoData.src;
            qualityLabel = '原画';
            break;
        case 'auto':
        default:
            // 自动选择：优先1080p，其次720p，最后原画
            if (videoData.url_1080p) {
                newSrc = videoData.url_1080p;
                qualityLabel = '自动';
            } else if (videoData.url_720p) {
                newSrc = videoData.url_720p;
                qualityLabel = '自动';
            } else {
                newSrc = videoData.src;
                qualityLabel = '自动';
            }
            break;
    }
    
    // 如果没有对应清晰度的视频，提示用户
    if (!newSrc || newSrc === videoEl.src) {
        if (quality !== 'auto' && quality !== 'original') {
            toast(`${qualityLabel} 清晰度暂未生成`);
        }
        return;
    }
    
    // 切换视频源
    videoEl.src = newSrc;
    videoEl.currentTime = currentTime;
    
    // 恢复播放状态
    if (!wasPaused) {
        videoEl.play().catch(() => {
            console.log('自动播放被阻止');
        });
    }
    
    // 更新清晰度标签
    const qualityLabelEl = $('[data-quality-label]', feedItemEl);
    if (qualityLabelEl) {
        qualityLabelEl.textContent = qualityLabel;
    }
    
    // 更新菜单中的选中状态
    const menu = $('[data-quality-menu]', feedItemEl);
    if (menu) {
        $all('.video-quality-menu__item', menu).forEach(item => {
            item.classList.remove('video-quality-menu__item--active');
        });
        const selectedItem = $(`[data-quality="${quality}"]`, menu);
        if (selectedItem) {
            selectedItem.classList.add('video-quality-menu__item--active');
        }
    }
    
    toast(`已切换到 ${qualityLabel}`);
}

/* =========================
   11) Gestures (Swipe + Double Tap)
========================= */
function setupGestures(feedItemEl, cardEl, videoData){
    let touchStartY = 0;
    let touchStartX = 0;
    let touchStartTime = 0;

    // manual double tap
    let lastTapAt = 0;
    let lastTapX = 0;
    let lastTapY = 0;

    cardEl.addEventListener("touchstart", (e) => {
        const t = e.touches[0];
        if (!t) return;
        touchStartY = t.clientY;
        touchStartX = t.clientX;
        touchStartTime = Date.now();
    }, { passive: true });

    cardEl.addEventListener("touchend", (e) => {
        if (!el.sheetMask.hidden) return; // 弹层打开不处理

        const now = Date.now();
        const dt = now - touchStartTime;

        const t = e.changedTouches[0];
        if (!t) return;

        const endX = t.clientX;
        const endY = t.clientY;

        const dy = endY - touchStartY;
        const dx = endX - touchStartX;

        const isSwipe = Math.abs(dy) > 60 && Math.abs(dy) > Math.abs(dx) * 1.2 && dt < 380;
        if (isSwipe) {
            if (dy < 0) scrollToNext(feedItemEl);
            else scrollToPrev(feedItemEl);
            return;
        }

        const isTap = Math.abs(dy) < 10 && Math.abs(dx) < 10 && dt < 250;
        if (isTap) {
            const dist = Math.hypot(endX - lastTapX, endY - lastTapY);
            if (now - lastTapAt < 300 && dist < 24) {
                likeWithAnimation(feedItemEl, videoData);
                lastTapAt = 0;
            } else {
                lastTapAt = now;
                lastTapX = endX;
                lastTapY = endY;
            }
        }
    }, { passive: true });
}

function scrollToNext(curItem){
    const next = curItem.nextElementSibling;
    if (next) next.scrollIntoView({ behavior: "smooth", block: "start" });
}
function scrollToPrev(curItem){
    const prev = curItem.previousElementSibling;
    if (prev) prev.scrollIntoView({ behavior: "smooth", block: "start" });
}

/* =========================
   12) Sheets: Comment / Share
========================= */
function openSheet(type){
    el.sheetMask.hidden = false;
    if (type === "comment") {
        el.commentSheet.hidden = false;
        el.shareSheet.hidden = true;
    } else {
        el.shareSheet.hidden = false;
        el.commentSheet.hidden = true;
    }
    // 打开弹层时禁用 feed 滚动
    el.feed.style.overflowY = "hidden";
}
function closeSheets(){
    el.sheetMask.hidden = true;
    el.commentSheet.hidden = true;
    el.shareSheet.hidden = true;
    el.feed.style.overflowY = "auto";
}
el.sheetMask.addEventListener("click", closeSheets);
$all("[data-sheet-close]").forEach(btn => btn.addEventListener("click", closeSheets));

function openCommentSheet(videoData){
    state.currentVideoData = videoData;
    // 从后端获取评论列表
    fetchComments(videoData);
    openSheet("comment");
}

// 从后端获取评论列表
async function fetchComments(videoData) {
    // 获取视频ID
    let videoId;
    if (videoData.serverId) {
        videoId = videoData.serverId;
    } else if (videoData.id && videoData.id.startsWith('server_')) {
        videoId = parseInt(videoData.id.replace('server_', ''));
    } else {
        toast("无法获取视频ID");
        return;
    }
    
    try {
        const res = await fetch(`http://localhost:5000/api/video/${videoId}/comments`);
        
        if (!res.ok) {
            throw new Error("获取评论失败");
        }
        
        const data = await res.json();
        
        // 转换后端评论格式为前端格式
        videoData.commentItems = (data.comments || []).map(c => ({
            id: c.id,
            name: c.username,
            text: c.content,
            time: c.created_at,
            user_id: c.user_id,
            avatar_url: c.avatar_url
        }));
        
        // 渲染评论列表
        renderComments(videoData);
        
    } catch (err) {
        console.error("获取评论失败:", err);
        toast("获取评论失败");
        // 显示空评论列表
        videoData.commentItems = [];
        renderComments(videoData);
    }
}

function renderComments(videoData){
    el.commentList.innerHTML = "";
    const items = (videoData.commentItems || []).slice();

    if (items.length === 0) {
        el.commentList.innerHTML = `
            <div class="comment-empty">
                <p>暂无评论</p>
                <p>快来发表第一条评论吧！</p>
            </div>
        `;
        return;
    }

    items.forEach(c => {
        const node = document.createElement("div");
        node.className = "comment";
        node.dataset.commentId = c.id;
        
        // 头像显示：如果有头像URL则显示图片，否则显示首字母
        let avatarContent;
        if (c.avatar_url && c.avatar_url.trim() !== '') {
            avatarContent = `<img src="${escapeHtml(c.avatar_url)}" alt="头像" style="width: 100%; height: 100%; object-fit: cover; border-radius: 50%;">`;
        } else {
            avatarContent = escapeHtml((c.name || "U").slice(0,1).toUpperCase());
        }
        
        // 判断是否是当前用户的评论
        const isOwnComment = state.currentUser && c.user_id === state.currentUser.id;
        const deleteButton = isOwnComment ? `<span class="comment__delete" data-comment-id="${c.id}">删除</span>` : '';
        
        node.innerHTML = `
      <div class="comment__avatar">${avatarContent}</div>
      <div class="comment__content">
        <div class="comment__name">${escapeHtml(c.name)}</div>
        <div class="comment__text">${escapeHtml(c.text)}</div>
        <div class="comment__meta">
          <span>${escapeHtml(c.time)}</span>
          ${deleteButton}
        </div>
      </div>
    `;
        el.commentList.appendChild(node);
    });
    
    // 绑定删除按钮事件
    $all('.comment__delete', el.commentList).forEach(btn => {
        btn.addEventListener('click', async (e) => {
            const commentId = parseInt(e.target.dataset.commentId);
            await deleteComment(commentId, videoData);
        });
    });
}

// 发送评论
el.commentSend.addEventListener("click", async () => {
    const text = el.commentInput.value.trim();
    if (!text || !state.currentVideoData) return;
    
    const token = localStorage.getItem("cwatchToken");
    if (!token) {
        toast("请先登录");
        closeSheets();
        showAuthPage();
        return;
    }
    
    // 获取视频ID
    let videoId;
    if (state.currentVideoData.serverId) {
        videoId = state.currentVideoData.serverId;
    } else if (state.currentVideoData.id && state.currentVideoData.id.startsWith('server_')) {
        videoId = parseInt(state.currentVideoData.id.replace('server_', ''));
    } else {
        toast("无法获取视频ID");
        return;
    }
    
    try {
        const res = await fetch(`http://localhost:5000/api/video/comment/${videoId}`, {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${token}`
            },
            body: JSON.stringify({
                content: text
            })
        });
        
        if (!res.ok) {
            const data = await res.json();
            throw new Error(data.error || "评论失败");
        }
        
        const result = await res.json();
        
        // 清空输入框
        el.commentInput.value = "";
        
        // 重新获取评论列表
        await fetchComments(state.currentVideoData);
        
        // 更新评论数
        state.currentVideoData.comments = result.comment_count;
        updateVideoCommentCount(state.currentVideoData.id, result.comment_count);
        
        toast("评论成功");
        
    } catch (err) {
        console.error("评论失败:", err);
        toast(err.message || "评论失败");
    }
});

// 删除评论
async function deleteComment(commentId, videoData) {
    const token = localStorage.getItem("cwatchToken");
    if (!token) {
        toast("请先登录");
        closeSheets();
        showAuthPage();
        return;
    }
    
    if (!confirm("确定要删除这条评论吗？")) {
        return;
    }
    
    try {
        const res = await fetch(`http://localhost:5000/api/video/comment/${commentId}`, {
            method: "DELETE",
            headers: {
                "Authorization": `Bearer ${token}`
            }
        });
        
        if (!res.ok) {
            const data = await res.json();
            throw new Error(data.error || "删除失败");
        }
        
        toast("删除成功");
        
        // 重新获取评论列表
        await fetchComments(videoData);
        
        // 更新评论数（减1）
        videoData.comments = Math.max(0, (videoData.comments || 0) - 1);
        updateVideoCommentCount(videoData.id, videoData.comments);
        
    } catch (err) {
        console.error("删除评论失败:", err);
        toast(err.message || "删除失败");
    }
}

// 更新视频评论数显示
function updateVideoCommentCount(videoId, commentCount) {
    // 更新播放页面的评论数
    const feedItem = $(`.feed-item[data-video-id="${videoId}"]`);
    if (feedItem) {
        const countEl = $('[data-count="comments"]', feedItem);
        if (countEl) countEl.textContent = formatCount(commentCount);
    }
    
    // 更新预览页面的评论数（如果需要显示的话）
    // 目前预览页面只显示点赞数，不显示评论数
}

function openShareSheet(videoData){
    state.currentVideoData = videoData;
    renderShare(videoData);
    openSheet("share");
}
function renderShare(videoData){
    el.shareGrid.innerHTML = "";
    el.shareLink.value = `https://example.com/video/${videoData.id}`;

    DATA.shareApps.forEach(app => {
        const node = document.createElement("div");
        node.className = "share-item";
        node.innerHTML = `
      <div class="share-item__icon">${escapeHtml(app.icon)}</div>
      <div class="share-item__label">${escapeHtml(app.label)}</div>
    `;
        node.addEventListener("click", () => {
            if (app.label === "复制链接") {
                copyText(el.shareLink.value);
            }
        });
        el.shareGrid.appendChild(node);
    });
}
function copyText(text){
    if (navigator.clipboard?.writeText) {
        navigator.clipboard.writeText(text).then(() => toast("已复制链接")).catch(() => fallbackCopy(text));
    } else {
        fallbackCopy(text);
    }
}
function fallbackCopy(text){
    const ta = document.createElement("textarea");
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); toast("已复制链接"); }
    catch { toast("复制失败，请手动复制"); }
    ta.remove();
}
el.copyBtn.addEventListener("click", () => copyText(el.shareLink.value));

/* =========================
   13) Events / Navigation
========================= */

// 设置导航按钮激活状态
function setNavActive(activeEl) {
    // 移除所有按钮的激活状态
    [el.navExplore, el.navUpload, el.navFav, el.navSetting].forEach(btn => {
        if (btn) btn.classList.remove('sidebar__item--active');
    });
    // 添加当前按钮的激活状态
    if (activeEl) activeEl.classList.add('sidebar__item--active');
}

function bindGlobalEvents(){
    // 顶部 Home：回到预览页
    el.btnHome.addEventListener("click", () => {
        showPage("preview");
        setNavActive(el.navExplore);
    });

    // Sidebar 点击事件
    el.navExplore?.addEventListener("click", () => {
        showPage("preview");
        setNavActive(el.navExplore);
    });
    
    el.navFav.addEventListener("click", () => {
        toast("收藏功能入口（占位）");
        setNavActive(el.navFav);
    });
    
    el.navSetting.addEventListener("click", () => {
        toast("设置入口（占位）");
        setNavActive(el.navSetting);
    });

    // 键盘：Space 控制播放；Esc 关闭弹层
    window.addEventListener("keydown", (e) => {
        if (e.code === "Escape") closeSheets();

        if (e.code === "Space") {
            // 如果评论弹窗或分享弹窗打开，不响应空格键
            if (!el.sheetMask.hidden) return;
            
            // 只在 player 页面响应
            if (!el.playerPage.classList.contains("page--active")) return;
            
            e.preventDefault();
            togglePlay(state.currentVideoEl);
            if (el.hint) el.hint.style.opacity = "0.15";
        }
    });

    // 弹层打开时点遮罩已关闭，这里不重复
}

/* =========================
   14) Security: escapeHtml
========================= */
function escapeHtml(str){
    return String(str ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#039;");
}

/* =========================
   15) Init
========================= */
async function init(){
    console.log("应用初始化开始...");
    
    // 检查登录状态
    await checkLoginStatus();
    
    console.log("登录状态检查完成:", state.isLoggedIn ? "已登录" : "未登录");
    
    // 无论是否登录，都初始化应用（允许未登录用户浏览视频）
    console.log("初始化应用，显示首页");
    initApp();
}

init();

/* =========================
   16) 登录注册功能（新增）
========================= */

// 检查登录状态（从 localStorage 读取 token）
async function checkLoginStatus(){
    const token = localStorage.getItem("cwatchToken");
    
    console.log("检查登录状态 - Token存在:", !!token);
    
    // 如果没有 token，直接设置为未登录
    if (!token) {
        console.log("没有Token，设置为未登录");
        state.isLoggedIn = false;
        state.currentUser = null;
        return;
    }
    
    try {
        // 调用后端接口验证 token 是否有效（包括是否过期）
        const res = await fetch("http://localhost:5000/api/user/info", {
            method: "GET",
            headers: {
                "Authorization": `Bearer ${token}`
            }
        });
        
        // 如果返回 401，说明 token 无效或已过期
        if (!res.ok) {
            console.log("Token验证失败，状态码:", res.status);
            throw new Error("Token无效或已过期");
        }
        
        const data = await res.json();
        
        // 验证成功，设置登录状态
        if (data.user) {
            state.currentUser = data.user;
            state.isLoggedIn = true;
            // 更新本地存储的用户信息
            localStorage.setItem("cwatchUser", JSON.stringify(data.user));
            updateAvatarUI();
            console.log("登录状态验证成功:", data.user.username);
        } else {
            throw new Error("响应数据格式错误");
        }
    } catch (err) {
        // 验证失败，清除本地存储
        console.log("登录状态验证失败:", err.message);
        localStorage.removeItem("cwatchToken");
        localStorage.removeItem("cwatchUser");
        state.isLoggedIn = false;
        state.currentUser = null;
    }
}

// 显示登录注册页面（弹窗形式）
function showAuthPage(){
    el.authPage.hidden = false;
    el.authPage.classList.add("auth-page--active");
    el.loginForm.hidden = false;
    el.registerForm.hidden = true;
    // 暂停所有视频
    pauseAllVideos();
}

// 隐藏登录注册页面
function hideAuthPage(){
    el.authPage.classList.remove("auth-page--active");
    // 延迟隐藏，等待动画完成
    setTimeout(() => {
        el.authPage.hidden = true;
    }, 300);
}

// 初始化应用（登录后）
async function initApp(){
    // 默认显示预览页，不自动播放
    showPage("preview");
    
    // 只在第一次初始化时加载视频和绑定事件
    if (!state.previewRendered) {
        // 从服务器获取视频列表
        await fetchVideoList(1, false);
        state.previewRendered = true;
        
        // 绑定滚动加载更多
        setupScrollLoadMore();
    }
    
    // 只在第一次初始化时绑定全局事件
    if (!state.eventsInitialized) {
        bindGlobalEvents();
        state.eventsInitialized = true;
    }

    // hint 自动淡出（仅播放器页可见）
    setTimeout(() => { if (el.hint) el.hint.style.opacity = "0.35"; }, 2500);
}

// 设置滚动加载更多
function setupScrollLoadMore() {
    const previewPage = el.previewPage;
    
    previewPage.addEventListener("scroll", () => {
        // 检查是否滚动到底部附近
        const scrollTop = previewPage.scrollTop;
        const scrollHeight = previewPage.scrollHeight;
        const clientHeight = previewPage.clientHeight;
        
        // 距离底部 200px 时开始加载
        if (scrollHeight - scrollTop - clientHeight < 200) {
            loadMoreVideos();
        }
    });
}

// 更新头像 UI
function updateAvatarUI(){
    if (state.isLoggedIn && state.currentUser) {
        console.log("更新头像UI - 用户信息:", state.currentUser);
        console.log("avatar_url:", state.currentUser.avatar_url);
        
        // 清空之前的内容
        el.avatar.innerHTML = '';
        
        // 如果用户有头像URL且不为空，显示图片；否则显示用户名首字母
        if (state.currentUser.avatar_url && state.currentUser.avatar_url.trim() !== '') {
            console.log("开始加载头像图片:", state.currentUser.avatar_url);
            
            // 先显示加载中的占位符（用户名首字母）
            el.avatar.textContent = state.currentUser.username.slice(0, 1).toUpperCase();
            el.avatar.style.opacity = '0.5';
            
            // 创建图片元素
            const img = document.createElement('img');
            img.alt = '头像';
            img.style.width = '100%';
            img.style.height = '100%';
            img.style.objectFit = 'cover';
            img.style.borderRadius = '14px';
            img.style.display = 'none'; // 先隐藏，加载成功后再显示
            
            // 图片加载成功
            img.onload = () => {
                console.log("头像图片加载成功");
                el.avatar.innerHTML = '';
                el.avatar.style.opacity = '1';
                img.style.display = 'block';
                el.avatar.appendChild(img);
            };
            
            // 图片加载失败时保持显示用户名首字母
            img.onerror = () => {
                console.log("头像图片加载失败，保持显示用户名首字母");
                el.avatar.style.opacity = '1';
                // 不需要做任何事，因为已经显示了用户名首字母
            };
            
            // 开始加载图片
            img.src = state.currentUser.avatar_url;
            
        } else {
            console.log("没有头像URL，显示用户名首字母");
            el.avatar.textContent = state.currentUser.username.slice(0, 1).toUpperCase();
            el.avatar.style.opacity = '1';
        }
        
        el.avatar.classList.add("avatar--logged-in");
        el.avatar.title = `${state.currentUser.username} - 点击查看菜单`;
    } else {
        console.log("未登录，显示默认头像");
        el.avatar.innerHTML = '';
        el.avatar.textContent = "U";
        el.avatar.style.opacity = '1';
        el.avatar.classList.remove("avatar--logged-in");
        el.avatar.title = "用户头像（占位）";
    }
}

// 关闭登录注册弹窗
el.authClose.addEventListener("click", () => {
    hideAuthPage();
});

// 点击遮罩层关闭弹窗
el.authPage.addEventListener("click", (e) => {
    if (e.target === el.authPage) {
        hideAuthPage();
    }
});

// 切换到注册表单
el.showRegister.addEventListener("click", (e) => {
    e.preventDefault();
    el.loginForm.hidden = true;
    el.registerForm.hidden = false;
});

// 切换到登录表单
el.showLogin.addEventListener("click", (e) => {
    e.preventDefault();
    el.registerForm.hidden = true;
    el.loginForm.hidden = false;
});

// 处理登录
el.loginForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    
    const username = el.loginUsername.value.trim();
    const password = el.loginPassword.value;
    
    if (!username || !password) {
        toast("请输入用户名和密码");
        return;
    }
    
    try {
        // 调用后端登录接口
        const res = await fetch("http://localhost:5000/api/login", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({ username, password })
        });
        
        const data = await res.json();
        
        if (!res.ok) {
            throw new Error(data.error || "登录失败");
        }
        
        // 登录成功，保存 token
        localStorage.setItem("cwatchToken", data.token);
        
        // 调用获取用户信息接口，获取完整的用户信息（包括头像）
        try {
            const userInfoRes = await fetch("http://localhost:5000/api/user/info", {
                method: "GET",
                headers: {
                    "Authorization": `Bearer ${data.token}`
                }
            });
            
            if (userInfoRes.ok) {
                const userInfoData = await userInfoRes.json();
                state.currentUser = userInfoData.user;
                state.isLoggedIn = true;
                localStorage.setItem("cwatchUser", JSON.stringify(userInfoData.user));
                console.log("获取用户信息成功:", userInfoData.user);


            } else {
                // 如果获取用户信息失败，使用登录接口返回的用户信息
                state.currentUser = data.user;
                state.isLoggedIn = true;
                localStorage.setItem("cwatchUser", JSON.stringify(data.user));
                console.log("使用登录返回的用户信息:", data.user);
            }
        } catch (err) {
            // 如果获取用户信息失败，使用登录接口返回的用户信息
            state.currentUser = data.user;
            state.isLoggedIn = true;
            localStorage.setItem("cwatchUser", JSON.stringify(data.user));
            console.log("获取用户信息失败，使用登录返回的信息:", err);
        }
        
        // 先隐藏登录页面
        hideAuthPage();
        
        // 更新头像
        updateAvatarUI();
        
        // 初始化应用
        initApp();
        
        // 显示成功提示
        toast("登录成功！");
        
        // 清空表单
        el.loginUsername.value = "";
        el.loginPassword.value = "";
    } catch (err) {
        toast(err.message || "登录失败，请检查网络连接");
    }
});

// 处理注册
el.registerForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    
    const username = el.registerUsername.value.trim();
    const password = el.registerPassword.value;
    const passwordConfirm = el.registerPasswordConfirm.value;
    
    if (!username || !password || !passwordConfirm) {
        toast("请填写所有字段");
        return;
    }
    
    if (username.length < 3) {
        toast("用户名至少3个字符");
        return;
    }
    
    if (password.length < 6) {
        toast("密码至少6个字符");
        return;
    }
    
    if (password !== passwordConfirm) {
        toast("两次密码不一致");
        return;
    }
    
    try {
        // 调用后端注册接口
        const res = await fetch("http://localhost:5000/api/register", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({ username, password })
        });
        
        const data = await res.json();
        
        if (!res.ok) {
            throw new Error(data.error || "注册失败");
        }
        
        toast("注册成功！请登录");
        
        // 切换到登录表单
        el.registerForm.hidden = true;
        el.loginForm.hidden = false;
        
        // 清空表单
        el.registerUsername.value = "";
        el.registerPassword.value = "";
        el.registerPasswordConfirm.value = "";
        
        // 自动填充用户名到登录表单
        el.loginUsername.value = username;
        el.loginPassword.focus();
    } catch (err) {
        toast(err.message || "注册失败，请检查网络连接");
    }
});

// 点击头像显示/隐藏下拉菜单
el.avatar.addEventListener("click", (e) => {
    e.stopPropagation();
    if (!state.isLoggedIn) {
        showAuthPage();
        return;
    }
    
    // 更新下拉菜单中的用户名显示
    if (state.currentUser && state.currentUser.username) {
        const userInfoText = el.dropdownUserInfo.querySelector('.avatar-dropdown__text');
        if (userInfoText) {
            userInfoText.textContent = state.currentUser.username;
        }
    }
    
    // 切换下拉菜单显示状态
    el.avatarDropdown.hidden = !el.avatarDropdown.hidden;
});

// 点击页面其他地方关闭下拉菜单
document.addEventListener("click", (e) => {
    if (!el.avatarWrapper.contains(e.target)) {
        el.avatarDropdown.hidden = true;
    }
});

// 点击个人信息
el.dropdownUserInfo.addEventListener("click", () => {
    el.avatarDropdown.hidden = true;
    toast(`当前用户：${state.currentUser.username}`);
});

// 点击退出登录
el.dropdownLogout.addEventListener("click", async () => {
    el.avatarDropdown.hidden = true;
    
    const token = localStorage.getItem("cwatchToken");
    
    // 保存当前用户名，用于清除点赞状态
    const currentUsername = state.currentUser?.username;
    
    try {
        // 调用后端登出接口
        const res = await fetch("http://localhost:5000/api/logout", {
            method: "POST",
            headers: {
                "Authorization": `Bearer ${token}`
            }
        });
        
        if (!res.ok) {
            const data = await res.json();
            throw new Error(data.error || "登出失败");
        }
    } catch (err) {
        console.log("登出请求失败:", err.message);
        // 即使后端请求失败，也继续清理前端状态
    }
    
    // 清除当前用户的点赞状态（重要：防止下一个用户看到上一个用户的点赞状态）
    if (currentUsername) {
        const likedVideosKey = `likedVideos_${currentUsername}`;
        localStorage.removeItem(likedVideosKey);
    }
    
    // 清理前端状态
    state.isLoggedIn = false;
    state.currentUser = null;
    localStorage.removeItem("cwatchToken");
    localStorage.removeItem("cwatchUser");
    
    // 重置视频列表和状态
    state.videoList = [];
    state.videoPage = 1;
    state.hasMoreVideos = true;
    state.previewRendered = false;
    DATA.videos = [];
    
    // 清空预览网格
    el.previewGrid.innerHTML = "";
    
    // 清空播放器页面
    el.feed.innerHTML = "";
    el.feed.dataset.rendered = "";
    
    toast("已退出登录");
    updateAvatarUI();
    
    // 显示登录页面
    showAuthPage();
    
    // 暂停所有视频
    pauseAllVideos();
});

// 权限检查：拦截所有需要登录的操作
function requireLogin(callback) {
    return function(...args) {
        if (!state.isLoggedIn) {
            toast("请先登录");
            showAuthPage();
            return;
        }
        return callback.apply(this, args);
    };
}

// 不再拦截 openPlayerAt，允许未登录用户观看视频
// 只拦截需要登录的操作：点赞、评论、分享

// 包装需要登录才能执行的函数
const originalOpenCommentSheet = openCommentSheet;
openCommentSheet = requireLogin(originalOpenCommentSheet);

const originalOpenShareSheet = openShareSheet;
openShareSheet = requireLogin(originalOpenShareSheet);

const originalLikeWithAnimation = likeWithAnimation;
likeWithAnimation = requireLogin(originalLikeWithAnimation);


/* =========================
   17) 视频上传功能
========================= */

// 打开上传弹窗
function openUploadModal() {
    if (!state.isLoggedIn) {
        toast("请先登录");
        showAuthPage();
        return;
    }
    resetUploadState();
    el.uploadModal.hidden = false;
}

// 关闭上传弹窗
function closeUploadModal() {
    if (state.isUploading) {
        if (!confirm("正在上传中，确定要取消吗？")) {
            return;
        }
    }
    el.uploadModal.hidden = true;
    resetUploadState();
    // 恢复导航状态到首页
    setNavActive(el.navExplore);
}

// 重置上传状态
function resetUploadState() {
    state.uploadFile = null;
    state.isUploading = false;
    el.uploadDropzone.hidden = false;
    el.uploadFileInfo.hidden = true;
    el.uploadProgress.hidden = true;
    el.uploadTitle.value = "";
    el.uploadSubmit.disabled = true;
    el.uploadProgressFill.style.width = "0%";
    el.uploadVideoPreview.src = "";
}

// 格式化文件大小
function formatFileSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}

// 处理文件选择
function handleFileSelect(file) {
    // 验证文件类型
    const allowedTypes = [".mp4", ".webm", ".mov", ".avi", ".mkv"];
    const ext = "." + file.name.split(".").pop().toLowerCase();
    if (!allowedTypes.includes(ext)) {
        toast("不支持的视频格式");
        return;
    }
    
    // 验证文件大小（500MB）
    if (file.size > 500 * 1024 * 1024) {
        toast("文件大小超过限制（最大500MB）");
        return;
    }
    
    state.uploadFile = file;
    
    // 显示文件信息
    el.uploadDropzone.hidden = true;
    el.uploadFileInfo.hidden = false;
    el.uploadFileName.textContent = file.name;
    el.uploadFileSize.textContent = formatFileSize(file.size);
    
    // 视频预览
    const videoURL = URL.createObjectURL(file);
    el.uploadVideoPreview.src = videoURL;
    
    // 自动填充标题（去掉扩展名）
    if (!el.uploadTitle.value) {
        el.uploadTitle.value = file.name.replace(/\.[^/.]+$/, "");
    }
    
    // 启用上传按钮
    el.uploadSubmit.disabled = false;
}

// 上传视频
async function uploadVideo() {
    if (!state.uploadFile || state.isUploading) return;
    
    const token = localStorage.getItem("cwatchToken");
    if (!token) {
        toast("请先登录");
        closeUploadModal();
        showAuthPage();
        return;
    }
    
    state.isUploading = true;
    el.uploadSubmit.disabled = true;
    el.uploadProgress.hidden = false;
    el.uploadProgressText.textContent = "正在获取上传凭证...";
    
    try {
        // 1. 获取上传URL
        const urlRes = await fetch("http://localhost:5000/api/video/upload-url", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${token}`
            },
            body: JSON.stringify({
                filename: state.uploadFile.name,
                filesize: state.uploadFile.size,
                title: el.uploadTitle.value || state.uploadFile.name
            })
        });
        
        if (!urlRes.ok) {
            const data = await urlRes.json();
            throw new Error(data.error || "获取上传凭证失败");
        }
        
        const { upload_url, video_id } = await urlRes.json();
        
        // 2. 上传文件到 MinIO
        el.uploadProgressText.textContent = "正在上传视频...";
        
        const xhr = new XMLHttpRequest();
        
        // 监听上传进度
        xhr.upload.addEventListener("progress", (e) => {
            if (e.lengthComputable) {
                const percent = Math.round((e.loaded / e.total) * 100);
                el.uploadProgressFill.style.width = percent + "%";
                el.uploadProgressText.textContent = `正在上传... ${percent}%`;
            }
        });
        
        // 上传完成
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
            xhr.send(state.uploadFile);
        });
        
        // 3. 通知后端上传完成
        el.uploadProgressText.textContent = "正在确认上传...";
        
        const confirmRes = await fetch("http://localhost:5000/api/video/upload-complete", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${token}`
            },
            body: JSON.stringify({ video_id })
        });
        
        if (!confirmRes.ok) {
            const data = await confirmRes.json();
            throw new Error(data.error || "确认上传失败");
        }
        
        // 上传成功
        el.uploadProgressFill.style.width = "100%";
        el.uploadProgressText.textContent = "上传成功！";
        toast("视频上传成功！");
        
        // 重置上传状态（避免关闭时弹出确认对话框）
        state.isUploading = false;
        
        // 延迟关闭弹窗并刷新视频列表
        setTimeout(async () => {
            closeUploadModal();
            // 重新加载视频列表
            state.previewRendered = false;
            state.videoPage = 1;
            state.hasMoreVideos = true;
            await fetchVideoList(1, false);
            state.previewRendered = true;
        }, 1500);
        
    } catch (err) {
        toast(err.message || "上传失败");
        el.uploadProgressText.textContent = "上传失败：" + err.message;
        state.isUploading = false;
        el.uploadSubmit.disabled = false;
    }
}

// 绑定上传相关事件
el.navUpload.addEventListener("click", () => {
    setNavActive(el.navUpload);
    openUploadModal();
});
el.uploadModalClose.addEventListener("click", closeUploadModal);
el.uploadCancel.addEventListener("click", closeUploadModal);
el.uploadSubmit.addEventListener("click", uploadVideo);

// 点击遮罩关闭
el.uploadModal.querySelector(".upload-modal__backdrop").addEventListener("click", closeUploadModal);

// 点击选择文件区域
el.uploadDropzone.addEventListener("click", () => {
    el.uploadFileInput.click();
});

// 文件选择变化
el.uploadFileInput.addEventListener("change", (e) => {
    const file = e.target.files[0];
    if (file) handleFileSelect(file);
});

// 更换文件
el.uploadChangeFile.addEventListener("click", () => {
    el.uploadFileInput.click();
});

// 拖拽上传
el.uploadDropzone.addEventListener("dragover", (e) => {
    e.preventDefault();
    el.uploadDropzone.classList.add("upload-dropzone--dragover");
});

el.uploadDropzone.addEventListener("dragleave", () => {
    el.uploadDropzone.classList.remove("upload-dropzone--dragover");
});

el.uploadDropzone.addEventListener("drop", (e) => {
    e.preventDefault();
    el.uploadDropzone.classList.remove("upload-dropzone--dragover");
    const file = e.dataTransfer.files[0];
    if (file) handleFileSelect(file);
});
