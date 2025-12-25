package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPClient 结构体封装 HTTP 客户端和统计信息
type HTTPClient struct {
	client *http.Client
	stats  *RequestStats
}

// RequestStats 用于统计请求结果
type RequestStats struct {
	mu               sync.Mutex
	totalRequests    int
	successRequests  int
	failedRequests   int
	totalDuration    time.Duration
	minDuration      time.Duration
	maxDuration      time.Duration
	statusCodeCounts map[int]int
}

// Result 表示单个请求的结果
type Result struct {
	StatusCode  int
	Duration    time.Duration
	Err         error
	ResponseLen int
}

// NewHTTPClient 创建新的 HTTP 客户端
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: timeout,
			// 可选的 Transport 配置
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		stats: &RequestStats{
			statusCodeCounts: make(map[int]int),
			minDuration:      time.Hour, // 初始化为一个很大的值
		},
	}
}

// makeRequest 发送单个 HTTP 请求
func (h *HTTPClient) makeRequest(url string) Result {
	startTime := time.Now()

	resp, err := h.client.Get(url)
	duration := time.Since(startTime)

	if err != nil {
		return Result{
			Err:      err,
			Duration: duration,
		}
	}
	defer resp.Body.Close()

	// 读取响应体（可选，根据需求决定是否读取）
	body, err := ioutil.ReadAll(resp.Body)
	responseLen := 0
	if err == nil {
		responseLen = len(body)
	}

	return Result{
		StatusCode:  resp.StatusCode,
		Duration:    duration,
		ResponseLen: responseLen,
	}
}

// worker 工作协程，发送指定次数的请求
func (h *HTTPClient) worker(url string, requests int, wg *sync.WaitGroup, results chan<- Result) {
	defer wg.Done()

	for i := 0; i < requests; i++ {
		result := h.makeRequest(url)
		results <- result
	}
}

// StartConcurrentRequests 启动并发请求
func (h *HTTPClient) StartConcurrentRequests(url string, concurrency, totalRequests int) {
	var wg sync.WaitGroup
	results := make(chan Result, concurrency*10) // 缓冲通道

	startTime := time.Now()

	// 计算每个协程需要发送的请求数
	requestsPerWorker := totalRequests / concurrency
	remaining := totalRequests % concurrency

	// 启动工作协程
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		requests := requestsPerWorker
		if i < remaining {
			requests++ // 分配剩余的请求
		}
		go h.worker(url, requests, &wg, results)
	}

	// 启动结果收集器
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集并统计结果
	h.collectResults(results, startTime)
}

// collectResults 收集并统计结果
func (h *HTTPClient) collectResults(results <-chan Result, startTime time.Time) {
	for result := range results {
		h.stats.mu.Lock()
		h.stats.totalRequests++

		if result.Err != nil {
			h.stats.failedRequests++
		} else {
			h.stats.successRequests++
			h.stats.statusCodeCounts[result.StatusCode]++

			// 更新耗时统计
			h.stats.totalDuration += result.Duration
			if result.Duration < h.stats.minDuration {
				h.stats.minDuration = result.Duration
			}
			if result.Duration > h.stats.maxDuration {
				h.stats.maxDuration = result.Duration
			}
		}
		h.stats.mu.Unlock()
	}

	totalTime := time.Since(startTime)
	h.printStats(totalTime)
}

// printStats 打印统计信息
func (h *HTTPClient) printStats(totalTime time.Duration) {
	h.stats.mu.Lock()
	defer h.stats.mu.Unlock()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("HTTP 压力测试结果")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("目标 URL:             http://dev.web.test.com:30080/\n")
	fmt.Printf("总请求数:             %d\n", h.stats.totalRequests)
	fmt.Printf("成功请求:             %d\n", h.stats.successRequests)
	fmt.Printf("失败请求:             %d\n", h.stats.failedRequests)
	fmt.Printf("成功率:               %.2f%%\n",
		float64(h.stats.successRequests)/float64(h.stats.totalRequests)*100)
	fmt.Printf("总测试时间:           %v\n", totalTime)
	fmt.Printf("平均请求时间:         %v\n",
		h.stats.totalDuration/time.Duration(h.stats.successRequests))
	fmt.Printf("最快请求:             %v\n", h.stats.minDuration)
	fmt.Printf("最慢请求:             %v\n", h.stats.maxDuration)
	fmt.Printf("QPS (每秒查询率):     %.2f\n",
		float64(h.stats.totalRequests)/totalTime.Seconds())

	fmt.Println("\n状态码统计:")
	for code, count := range h.stats.statusCodeCounts {
		fmt.Printf("  %d: %d 次 (%.1f%%)\n",
			code, count, float64(count)/float64(h.stats.successRequests)*100)
	}
	fmt.Println(strings.Repeat("=", 60))
}

// 主函数
func main() {
	wg := sync.WaitGroup{}
	urlList := []string{
		"http://dev.web.test.com:30080/",
		//"http://dev.hello.test.com:30080/hello/user",
		//"http://dev.admin.test.com/:30080/admin/user",
		//"http://api.dev.ic2.com:30080/hello/user",
		//"http://api.dev.ic2.com:30080/admin/user",
		"http://dev.api.test.com:30080/admin/user",
		"http://dev.api.test.com:30080/hello/user",
	}
	for i := 0; i < len(urlList); i++ {
		wg.Add(1)
		go func() {
			url := urlList[i]
			defer wg.Done()
			// 这里可以修改并发数和总请求数
			concurrency := 10    // 并发数
			totalRequests := 100 // 总请求数

			fmt.Printf("开始压力测试:\n")
			fmt.Printf("URL: %s\n", url)
			fmt.Printf("并发数: %d\n", concurrency)
			fmt.Printf("总请求数: %d\n", totalRequests)

			// 创建 HTTP 客户端，设置超时时间
			httpClient := NewHTTPClient(10 * time.Second)

			// 启动并发请求
			httpClient.StartConcurrentRequests(url, concurrency, totalRequests)

		}()
	}
	wg.Wait()
}
