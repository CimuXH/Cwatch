package services

import (
	"context"
	"backend/utils"
	"math/rand"
	"time"
	"errors"
)

// RandomFeedService 负责“随机起点 + 去重 feed”的服务端逻辑（MVP 版）
// 规则：
// - 每次返回 pageSize=3 条
// - 返回之前通过 Redis BloomFilter（用户维度 + 每日轮转）去重
// - 扫描候选页时按 created_at DESC 顺序取候选，然后用 Bloom 过滤
type RandomFeedService struct{}

type RandomFeedNextResponse struct {
	Videos   []utils.VideoListItem `json:"videos"`
	NextPage int                    `json:"next_page"`
	HasMore bool                   `json:"has_more"`
}

func (s *RandomFeedService) NextRandomFeed(username string, page int, init bool, pageSize int) (*RandomFeedNextResponse, error) {
	if username == "" {
		return nil, errors.New("未登录")
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 3
	}

	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	userID := user.ID

	dayKey := time.Now().Format("20060102")
	ctx := context.Background()

	// likedMap：给返回的视频补齐 is_liked（前端点赞按钮需要）
	likedVideoIDs, err := utils.GetUserLikedVideoIDs(userID)
	if err != nil {
		return nil, errors.New("获取点赞信息失败")
	}
	likedMap := make(map[uint]bool, len(likedVideoIDs))
	for _, vid := range likedVideoIDs {
		likedMap[vid] = true
	}

	// 计算最大页数：用于 has_more 的粗略判断
	totalCount, err := utils.GetUploadedVideoCount()
	if err != nil {
		return nil, errors.New("获取视频总数失败")
	}

	if totalCount == 0 {
		return &RandomFeedNextResponse{
			Videos:   []utils.VideoListItem{},
			NextPage: 1,
			HasMore: false,
		}, nil
	}

	maxPage := int((totalCount + int64(pageSize) - 1) / int64(pageSize))
	if maxPage <= 0 {
		maxPage = 1
	}

	// init：随机起点 page
	if init {
		if maxPage == 1 {
			page = 1
		} else {
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			page = rng.Intn(maxPage) + 1
		}
	} else {
		if page < 1 {
			page = 1
		}
	}

	// 最多扫描若干页，避免布隆误判或数据稀疏导致一直扫到底
	const maxScanPages = 10

	collected := make([]utils.VideoListItem, 0, pageSize)
	collectedIDs := make(map[uint]struct{}, pageSize)
	curPage := page
	nextPage := curPage + 1

	for scan := 0; scan < maxScanPages && curPage <= maxPage; scan++ {
		candidates, err := utils.GetVideoListPage(curPage, pageSize)
		if err != nil {
			return nil, errors.New("获取视频候选失败")
		}

		for _, cand := range candidates {
			might, err := utils.BloomMightContain(ctx, userID, dayKey, cand.ID)
			if err != nil {
				return nil, errors.New("布隆过滤器查询失败")
			}

			// Bloom 返回“可能存在” => 认为已看过，跳过
			if might {
				continue
			}

			// Bloom 返回“肯定不存在” => 接受并写入 Bloom
			if _, ok := collectedIDs[cand.ID]; ok {
				continue
			}

			if err := utils.BloomAdd(ctx, userID, dayKey, cand.ID); err != nil {
				return nil, errors.New("布隆过滤器写入失败")
			}

			// 补齐点赞状态，保证前端按钮正确
			cand.IsLiked = likedMap[cand.ID]

			collected = append(collected, cand)
			collectedIDs[cand.ID] = struct{}{}
			if len(collected) >= pageSize {
				nextPage = curPage + 1
				break
			}
		}

		if len(collected) >= pageSize {
			break
		}

		curPage++
		nextPage = curPage + 1
	}

	// 粗略 has_more：
	// 如果 nextPage 已经超过 maxPage，说明没候选页了。
	// 如果本次收集为空，MVP 直接认为“已无可用内容/已看完”，避免前端死循环请求空结果。
	// 如果 Bloom 去重后仍然为 0，为了避免前端“空白”，做一个 MVP 兜底：
	// 第二阶段：允许“可能存在”的视频补齐列表（同时仍写入 Bloom）。
	if len(collected) == 0 && maxPage > 0 {
		curPageFallback := page
		if curPageFallback < 1 {
			curPageFallback = 1
		}

		for fallbackScan := 0; fallbackScan < maxScanPages && curPageFallback <= maxPage && len(collected) < pageSize; fallbackScan++ {
			candidates, err := utils.GetVideoListPage(curPageFallback, pageSize)
			if err != nil {
				return nil, errors.New("获取视频候选失败（fallback）")
			}
			for _, cand := range candidates {
				if _, ok := collectedIDs[cand.ID]; ok {
					continue
				}

				// fallback 也写入 Bloom，保证后续不会重复太明显
				_ = utils.BloomAdd(ctx, userID, dayKey, cand.ID)

				cand.IsLiked = likedMap[cand.ID]
				collected = append(collected, cand)
				collectedIDs[cand.ID] = struct{}{}
				if len(collected) >= pageSize {
					nextPage = curPageFallback + 1
					break
				}
			}
			if len(collected) >= pageSize {
				break
			}
			curPageFallback++
			nextPage = curPageFallback + 1
		}
	}

	hasMore := nextPage <= maxPage && len(collected) > 0
	if len(collected) < pageSize {
		// 扫描页数不足/误判导致数量不满时，MVP 直接认为“可能还有”，但为避免死循环，
		// 仍按 nextPage<=maxPage 返回 has_more。
		hasMore = nextPage <= maxPage
	}

	if len(collected) == 0 {
		hasMore = false
	}

	return &RandomFeedNextResponse{
		Videos:   collected,
		NextPage: nextPage,
		HasMore: hasMore,
	}, nil
}

