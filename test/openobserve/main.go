package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	libErrors "github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

// 配置信息
const (
	ZOEndpoint = "http://openobserve.test:5080/api/default/go_app_logs/_json"
	ZOUser     = "i@.com"
	ZOPass     = ""
)

// OpenObserveWriter 自定义 Writer
type OpenObserveWriter struct {
	client *http.Client
}

func (w *OpenObserveWriter) Write(p []byte) (n int, err error) {
	fmt.Println("准备发送日志")
	// OpenObserve 接收 JSON 数组，zerolog 产生的是单行 JSON
	// 我们简单包装一下： [ {log} ]
	// 注意：实际生产中应该攒一批日志再发送 (Batch)，不要一条发一次 HTTP
	payload := []byte("[" + string(p) + "]")

	req, err := http.NewRequest("POST", ZOEndpoint, bytes.NewBuffer(payload))
	if err != nil {
		return 0, err
	}

	req.SetBasicAuth(ZOUser, ZOPass)
	req.Header.Set("Content-Type", "application/json")
	// 如果你没有配置 hosts 文件，必须手动指定 Host 头以匹配 Ingress
	req.Header.Set("Host", "openobserve.test")

	resp, err := w.client.Do(req)
	if err != nil {
		fmt.Printf("Error sending log to ZO: %v\n", err)
		return len(p), nil // 忽略错误，不要让日志阻塞主程序
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error sending log to ZO: %s\n", resp.Status)
		return 0, errors.New("error sending log to ZO")
	}

	return len(p), nil
}

func main() {
	// [修改点3] 全局设置：告诉 zerolog 使用 pkgerrors 来处理堆栈
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// 1. 设置 Console 输出
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}

	// 2. 设置 OpenObserve 输出
	zoWriter := &OpenObserveWriter{
		client: &http.Client{Timeout: 5 * time.Second},
	}

	// 3. 组合两者
	multi := zerolog.MultiLevelWriter(consoleWriter, zoWriter)

	// [修改点4] 初始化 Logger 时，必须加上 .Stack()
	// 这样 zerolog 才会去检查 error 里有没有堆栈信息
	log.Logger = zerolog.New(multi).With().Timestamp().Stack().Logger()

	// --- 开始测试 ---

	log.Info().Str("module", "payment").Msg("User initiated payment")

	// 模拟一个带堆栈的错误
	// 注意：这里必须用 pkg_errors.New 或者 pkg_errors.Wrap
	// 标准库 fmt.Errorf 是不会产生堆栈的
	err := libErrors.New("connection timeout to Redis")

	// 模拟多层调用包装错误 (这是生产环境常见的场景)
	err = libErrors.Wrap(err, "failed to update cache")

	// 打印错误
	// zerolog 会自动检测到 err 包含堆栈，并将其序列化到 'stack' 字段中
	log.Error().Err(err).Msg("Critical System Error")

	fmt.Println("日志已发送，请去 OpenObserve 查看，点击日志详情展开可以看到 'stack' 字段")
}
