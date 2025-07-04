package model

import (
	"encoding/json"
)

type VideoInfo struct {
	BVID       string
	Title      string
	Cover      string
	LocalCover string // 本地图片路径
}

type VideoResponse struct {
	Code    int `json:"code"`
	Message string `json:"message"`
	Data    struct {
		BVID    string          `json:"bvid"`
		Aid     int64           `json:"aid"`
		Title   string          `json:"title"`
		Pic     string          `json:"pic"`
		Desc    string          `json:"desc"`
		DescV2  json.RawMessage `json:"desc_v2"`
		State   int             `json:"state"`
		Duration int            `json:"duration"`
		Owner   struct {
			Mid  int64  `json:"mid"`
			Name string `json:"name"`
			Face string `json:"face"`
		} `json:"owner"`
		Stat struct {
			View     int `json:"view"`
			Danmaku  int `json:"danmaku"`
			Reply    int `json:"reply"`
			Favorite int `json:"favorite"`
			Coin     int `json:"coin"`
			Share    int `json:"share"`
			Like     int `json:"like"`
		} `json:"stat"`
	} `json:"data"`
}