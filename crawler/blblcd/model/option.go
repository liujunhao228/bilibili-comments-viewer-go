package model

type Option struct {
	Cookie        string
	Mid           int
	Pages         int
	Skip          int
	Vorder        string
	Bvid          string
	Corder        int
	Output        string
	ImgDownload   bool
	MaxTryCount   int
	CommentOutput string
	ImageOutput   string
	Workers       int
	FetchAll      bool // 新增：是否爬取所有视频
	DelayBaseMs   int  // 新增
	DelayJitterMs int  // 新增
}

// 新增构造函数确保默认值
func NewDefaultOption() *Option {
	return &Option{
		Pages:         10,        // 默认获取10页视频
		Vorder:        "pubdate", // 默认按最新发布排序
		Workers:       5,         // 默认5个协程
		MaxTryCount:   3,         // 默认重试3次
		FetchAll:      false,     // 默认不爬取所有视频
		DelayBaseMs:   3000,      // 新增默认值
		DelayJitterMs: 2000,      // 新增默认值
	}
}
