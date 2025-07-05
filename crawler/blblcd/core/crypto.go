package core

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bilibili-comments-viewer-go/crawler/bili_info/config"
	"bilibili-comments-viewer-go/crawler/bili_info/util"
	"bilibili-comments-viewer-go/logger"

	"github.com/tidwall/gjson"
)

// core 包中 crypto.go 负责 B 站 API 请求的签名、加密、BVID/AVID 转换等工具方法

var (
	mixinKeyEncTab = []int{
		46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
		33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
		61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11,
		36, 20, 34, 44, 52,
	}
	cache          sync.Map // 缓存 imgKey 和 subKey，减少请求频率
	lastUpdateTime time.Time

	XOR_CODE = int64(23442827791579) // BVID/AVID 转换用常量
	MAX_CODE = int64(2251799813685247)
	CHARTS   = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
)

// SignAndGenerateURL 对 B 站 API 请求 URL 进行签名加密，防止被风控
// urlStr: 原始 URL
// cookie: 登录 cookie
// 返回值: 签名后的 URL 和错误
func SignAndGenerateURL(urlStr string, cookie string) (string, error) {
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	imgKey, subKey := getWbiKeysCached(cookie)
	query := urlObj.Query()
	params := map[string]string{}
	for k, v := range query {
		params[k] = v[0]
	}
	newParams := encWbi(params, imgKey, subKey)
	for k, v := range newParams {
		query.Set(k, v)
	}
	urlObj.RawQuery = query.Encode()
	newUrlStr := urlObj.String()
	return newUrlStr, nil
}

// encWbi 对参数进行加密签名，生成 w_rid
func encWbi(params map[string]string, imgKey, subKey string) map[string]string {
	mixinKey := getMixinKey(imgKey + subKey)
	currTime := strconv.FormatInt(time.Now().Unix(), 10)
	params["wts"] = currTime

	// Sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Remove unwanted characters
	for k, v := range params {
		v = sanitizeString(v)
		params[k] = v
	}

	// Build URL parameters
	query := url.Values{}
	for _, k := range keys {
		query.Set(k, params[k])
	}
	queryStr := query.Encode()

	// Calculate w_rid
	hash := md5.Sum([]byte(queryStr + mixinKey))
	params["w_rid"] = hex.EncodeToString(hash[:])
	return params
}

// getMixinKey 根据 imgKey+subKey 生成混淆 key
func getMixinKey(orig string) string {
	var str strings.Builder
	for _, v := range mixinKeyEncTab {
		if v < len(orig) {
			str.WriteByte(orig[v])
		}
	}
	return str.String()[:32]
}

// sanitizeString 移除参数中的特殊字符，保证签名一致性
func sanitizeString(s string) string {
	unwantedChars := []string{"!", "'", "(", ")", "*"}
	for _, char := range unwantedChars {
		s = strings.ReplaceAll(s, char, "")
	}
	return s
}

// updateCache 定时更新 imgKey 和 subKey 缓存，减少 API 请求
func updateCache(cookie string) {
	if time.Since(lastUpdateTime).Minutes() < 10 {
		return
	}
	imgKey, subKey := getWbiKeys(cookie)
	cache.Store("imgKey", imgKey)
	cache.Store("subKey", subKey)
	lastUpdateTime = time.Now()
}

// getWbiKeysCached 获取缓存的 imgKey 和 subKey
func getWbiKeysCached(cookie string) (string, string) {
	updateCache(cookie)
	imgKeyI, _ := cache.Load("imgKey")
	subKeyI, _ := cache.Load("subKey")
	return imgKeyI.(string), subKeyI.(string)
}

// getWbiKeys 实时请求 B 站接口获取 imgKey 和 subKey
func getWbiKeys(cookie string) (string, string) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.bilibili.com/x/web-interface/nav", nil)
	if err != nil {
		fmt.Printf("Error creating request: %s", err)
		return "", ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Cookie", string(cookie))

	var resp *http.Response
	err = util.Retry(config.MaxRetries, func() error {
		var reqErr error
		resp, reqErr = client.Do(req)
		if reqErr != nil {
			fmt.Printf("Error sending request: %s", reqErr)
			return reqErr
		}
		if resp.StatusCode >= 500 {
			return util.PermanentError{Err: fmt.Errorf("服务器错误: %d", resp.StatusCode)}
		}
		if resp.StatusCode >= 400 {
			return util.PermanentError{Err: fmt.Errorf("客户端错误: %d", resp.StatusCode)}
		}
		return nil
	}, func(err error) bool {
		if _, ok := err.(util.PermanentError); ok {
			return false
		}
		return true
	}, nil)

	if err != nil {
		fmt.Printf("Error sending request: %s", err)
		return "", ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %s", err)
		return "", ""
	}
	json := string(body)
	imgURL := gjson.Get(json, "data.wbi_img.img_url").String()
	subURL := gjson.Get(json, "data.wbi_img.sub_url").String()
	imgKey := strings.Split(strings.Split(imgURL, "/")[len(strings.Split(imgURL, "/"))-1], ".")[0]
	subKey := strings.Split(strings.Split(subURL, "/")[len(strings.Split(subURL, "/"))-1], ".")[0]
	return imgKey, subKey
}

// swapString 交换字符串中指定下标的字符
func swapString(s string, x, y int) string {
	chars := []rune(s)
	chars[x], chars[y] = chars[y], chars[x]
	return string(chars)
}

// Bvid2Avid 将 BVID 转换为 AVID
func Bvid2Avid(bvid string) (avid int64) {
	// 边界条件处理
	if len(bvid) < 4 {
		logger.GetLogger().Errorf("Invalid BVID: %s", bvid)
		return 0
	}

	s := swapString(swapString(bvid, 3, 9), 4, 7)
	bv1 := string([]rune(s)[3:])
	temp := int64(0)
	for _, c := range bv1 {
		idx := strings.IndexRune(CHARTS, c)
		temp = temp*int64(58) + int64(idx)
	}
	avid = (temp & MAX_CODE) ^ XOR_CODE
	return
}

// Avid2Bvid 将 AVID 转换为 BVID
func Avid2Bvid(avid int64) (bvid string) {
	arr := [12]string{"B", "V", "1"}
	bvIdx := len(arr) - 1
	temp := (avid | (MAX_CODE + 1)) ^ XOR_CODE
	for temp > 0 {
		idx := temp % 58
		arr[bvIdx] = string(CHARTS[idx])
		temp /= 58
		bvIdx--
	}
	raw := strings.Join(arr[:], "")
	bvid = swapString(swapString(raw, 3, 9), 4, 7)
	return
}
