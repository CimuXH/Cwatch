package services

import (
	"context"
	"math/rand/v2"

	"backend/utils"
)

type RandomFeedService struct{}

type RandomFeedNextResponse struct {
	Videos   []utils.VideoListItem `json:"videos"`
	NextPage int                   `json:"next_page"`
	HasMore  bool                  `json:"has_more"`
}

func (*RandomFeedService) NextRandomFeed(ctx context.Context, username string, page int, init bool, pageSize int) (*RandomFeedNextResponse, error) {
	if page < 1 || init {
		page = 1
	}
	items, total, err := utils.CachedVideoList(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}
	lastPage := int((total + int64(pageSize) - 1) / int64(pageSize))
	if init && lastPage > 1 {
		page = rand.IntN(lastPage) + 1
		if page > 1 {
			items, _, err = utils.CachedVideoList(ctx, page, pageSize)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := utils.FillLiked(ctx, username, items); err != nil {
		return nil, err
	}
	return &RandomFeedNextResponse{Videos: items, NextPage: page + 1, HasMore: page < lastPage && len(items) > 0}, nil
}
