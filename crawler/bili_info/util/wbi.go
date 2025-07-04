package util

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func GenerateWBIParams(params url.Values) string {
	params.Set("wts", strconv.FormatInt(time.Now().Unix(), 10))
	
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	var query strings.Builder
	for _, k := range keys {
		query.WriteString(k + "=" + params.Get(k) + "&")
	}
	queryStr := strings.TrimSuffix(query.String(), "&")
	
	h := md5.New()
	h.Write([]byte(queryStr))
	sign := hex.EncodeToString(h.Sum(nil))
	
	return queryStr + "&w_rid=" + sign
}

func SetHeaders(req *http.Request, cookies string) {
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
		"Origin":     "https://www.bilibili.com",
		"Referer":    "https://www.bilibili.com/",
		"Cookie":     cookies,
	}
	
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}