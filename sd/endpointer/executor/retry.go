package executor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/sd/interfaces"
	"github.com/dreamsxin/go-kit/utils"
)

// 用于保存多次重试错误
type RetryError struct {
	RawErrors []error
	Final     error // 最终结果，如果成功值为 nil
}

func (e RetryError) Error() string {
	var suffix string
	if len(e.RawErrors) > 1 {
		a := make([]string, len(e.RawErrors)-1)
		for i := 0; i < len(e.RawErrors)-1; i++ {
			a[i] = e.RawErrors[i].Error()
		}
		suffix = fmt.Sprintf(" (previously: %s)", strings.Join(a, "; "))
	}
	if e.Final == nil {
		return fmt.Sprintf("%v%s", e.RawErrors[len(e.RawErrors)-1], suffix)
	}
	return fmt.Sprintf("%v%s", e.Final, suffix)
}

// 重试执行器回调函数接口，返回是否重试
type RetryCallback func(n int, received error) (keepTrying bool, replacement error)

// 重试执行器，拥有最大重试次数
func Retry(max int, timeout time.Duration, b interfaces.Balancer) endpoint.Endpoint {
	return RetryWithCallback(timeout, b, maxRetries(max))
}

// 重试执行器，会一直重试直到成功
func RetryAlways(timeout time.Duration, b interfaces.Balancer) endpoint.Endpoint {
	return RetryWithCallback(timeout, b, alwaysRetry)
}

// 最大重试次数判断
func maxRetries(max int) RetryCallback {
	return func(n int, err error) (keepTrying bool, replacement error) {
		return n < max, nil
	}
}

func alwaysRetry(int, error) (keepTrying bool, replacement error) {
	return true, nil
}

func RetryWithCallback(timeout time.Duration, b interfaces.Balancer, cb RetryCallback) endpoint.Endpoint {
	if cb == nil {
		cb = alwaysRetry
	}
	if b == nil {
		panic("nil Balancer")
	}

	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		var (
			newctx, cancel = context.WithTimeout(ctx, timeout)
			responses      = make(chan interface{}, 1)
			errs           = make(chan error, 1)
			final          RetryError

			d time.Duration = 10 * time.Millisecond
		)
		defer cancel()

		for i := 1; ; i++ {
			go func() {
				e, err := b.Endpoint()
				if err != nil {
					errs <- err
					return
				}
				response, err := e(newctx, request)
				if err != nil {
					errs <- err
					return
				}
				responses <- response
			}()

			select {
			case <-newctx.Done():
				return nil, newctx.Err()

			case response := <-responses:
				return response, nil

			case err := <-errs:
				final.RawErrors = append(final.RawErrors, err)
				keepTrying, replacement := cb(i, err)
				if replacement != nil {
					err = replacement
				}
				if !keepTrying {
					final.Final = err
					return nil, final
				}
				time.Sleep(d)
				d = utils.Exponential(d)
				continue
			}
		}
	}
}
